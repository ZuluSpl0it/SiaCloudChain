package renter

import (
	"bytes"
	"io"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/scpcorp/siamux"
	"gitlab.com/scpcorp/siamux/mux"
)

// defaultNewStreamTimeout is a default timeout for creating a new stream.
var defaultNewStreamTimeout = build.Select(build.Var{
	Standard: 5 * time.Minute,
	Testing:  10 * time.Second,
	Dev:      time.Minute,
}).(time.Duration)

// defaultRPCDeadline is a default timeout for executing an RPC.
var defaultRPCDeadline = build.Select(build.Var{
	Standard: 5 * time.Minute,
	Testing:  10 * time.Second,
	Dev:      time.Minute,
}).(time.Duration)

// programResponse is a helper struct that wraps the RPCExecuteProgramResponse
// alongside the data output
type programResponse struct {
	modules.RPCExecuteProgramResponse
	Output []byte
}

// managedExecuteProgram performs the ExecuteProgramRPC on the host
func (w *worker) managedExecuteProgram(p modules.Program, data []byte, fcid types.FileContractID, cost types.Currency) (responses []programResponse, limit mux.BandwidthLimit, err error) {
	// check host version
	cache := w.staticCache()
	if build.VersionCmp(cache.staticHostVersion, minAsyncVersion) < 0 {
		build.Critical("Executing new RHP RPC on host with version", cache.staticHostVersion)
	}

	// track the withdrawal
	// TODO: this is very naive and does not consider refunds at all
	w.staticAccount.managedTrackWithdrawal(cost)
	defer func() {
		w.staticAccount.managedCommitWithdrawal(cost, err == nil)
	}()

	// create a new stream
	var stream siamux.Stream
	stream, err = w.staticNewStream()
	if err != nil {
		err = errors.AddContext(err, "Unable to create a new stream")
		return
	}
	defer func() {
		if err := stream.Close(); err != nil {
			w.renter.log.Println("ERROR: failed to close stream", err)
		}
	}()

	// set the stream's limit
	limit = stream.Limit()

	// prepare a buffer so we can optimize our writes
	buffer := bytes.NewBuffer(nil)

	// write the specifier
	err = modules.RPCWrite(buffer, modules.RPCExecuteProgram)
	if err != nil {
		return
	}

	// send price table uid
	pt := w.staticPriceTable().staticPriceTable
	err = modules.RPCWrite(buffer, pt.UID)
	if err != nil {
		return
	}

	// provide payment
	err = w.staticAccount.ProvidePayment(buffer, w.staticHostPubKey, modules.RPCUpdatePriceTable, cost, w.staticAccount.staticID, cache.staticBlockHeight)
	if err != nil {
		return
	}

	// prepare the request.
	epr := modules.RPCExecuteProgramRequest{
		FileContractID:    fcid,
		Program:           p,
		ProgramDataLength: uint64(len(data)),
	}

	// send the execute program request.
	err = modules.RPCWrite(buffer, epr)
	if err != nil {
		return
	}

	// send the programData.
	_, err = buffer.Write(data)
	if err != nil {
		return
	}

	// write contents of the buffer to the stream
	_, err = stream.Write(buffer.Bytes())
	if err != nil {
		return
	}

	// read the cancellation token.
	var ct modules.MDMCancellationToken
	err = modules.RPCRead(stream, &ct)
	if err != nil {
		return
	}

	// read the responses.
	responses = make([]programResponse, len(epr.Program))
	for i := range responses {
		err = modules.RPCRead(stream, &responses[i])
		if err != nil {
			return
		}

		// Read the output data.
		outputLen := responses[i].OutputLength
		responses[i].Output = make([]byte, outputLen)
		_, err = io.ReadFull(stream, responses[i].Output)
		if err != nil {
			return
		}

		// If the response contains an error we are done.
		if responses[i].Error != nil {
			break
		}
	}
	return
}

// staticNewStream returns a new stream to the worker's host
func (w *worker) staticNewStream() (siamux.Stream, error) {
	if build.VersionCmp(w.staticCache().staticHostVersion, minAsyncVersion) < 0 {
		w.renter.log.Critical("calling staticNewStream on a host that doesn't support the new protocol")
		return nil, errors.New("host doesn't support this")
	}

	// Create a stream with a reasonable dial up timeout.
	stream, err := w.renter.staticMux.NewStreamTimeout(modules.HostSiaMuxSubscriberName, w.staticHostMuxAddress, defaultNewStreamTimeout, modules.SiaPKToMuxPK(w.staticHostPubKey))
	if err != nil {
		return nil, err
	}

	// Set the deadline.
	return stream, stream.SetDeadline(time.Now().Add(defaultRPCDeadline))
}
