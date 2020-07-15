package renter

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
)

type (
	// jobReadOffset contains information about a ReadOffset job.
	jobReadOffset struct {
		jobRead

		staticOffset uint64
	}
)

// callExecute executes the jobReadOffset.
func (j *jobReadOffset) callExecute() {
	// Track how long the job takes.
	start := time.Now()
	data, err := j.managedReadOffset()
	jobTime := time.Since(start)

	// Finish the execution.
	j.jobRead.managedFinishExecute(data, err, jobTime)
}

// managedReadOffset returns the sector data for given root.
func (j *jobReadOffset) managedReadOffset() ([]byte, error) {
	// create the program
	w := j.staticQueue.staticWorker()
	pt := w.staticPriceTable().staticPriceTable
	pb := modules.NewProgramBuilder(&pt, 0) // 0 duration since Read doesn't depend on it.
	pb.AddReadOffsetInstruction(j.staticLength, j.staticOffset, true)
	program, programData := pb.Program()
	cost, _, _ := pb.Cost(true)

	// take into account bandwidth costs
	ulBandwidth, dlBandwidth := j.callExpectedBandwidth()
	bandwidthCost := modules.MDMBandwidthCost(pt, ulBandwidth, dlBandwidth)
	cost = cost.Add(bandwidthCost)

	// Read response.
	out, err := j.jobRead.managedRead(w, program, programData, cost)
	if err != nil {
		return nil, errors.AddContext(err, "jobReadOffset: failed to execute managedRead")
	}

	// Verify proof.
	proofStart := int(j.staticOffset) / crypto.SegmentSize
	proofEnd := int(j.staticOffset+j.staticLength) / crypto.SegmentSize
	ok := crypto.VerifyMixedRangeProof(out.Output, out.Proof, out.NewMerkleRoot, proofStart, proofEnd)
	if !ok {
		return nil, errors.New("verifying proof failed")
	}
	return out.Output, nil
}

// ReadOffset is a helper method to run a ReadOffset job on a worker.
func (w *worker) ReadOffset(ctx context.Context, offset, length uint64) ([]byte, error) {
	readOffsetRespChan := make(chan *jobReadResponse)
	jro := &jobReadOffset{
		jobRead: jobRead{
			staticResponseChan: readOffsetRespChan,
			staticLength:       length,
			jobGeneric:         newJobGeneric(w.staticJobReadQueue, ctx.Done()),
		},
		staticOffset: offset,
	}

	// Add the job to the queue.
	if !w.staticJobReadQueue.callAdd(jro) {
		return nil, errors.New("worker unavailable")
	}

	// Wait for the response.
	var resp *jobReadResponse
	select {
	case <-ctx.Done():
		return nil, errors.New("Read interrupted")
	case resp = <-readOffsetRespChan:
	}
	return resp.staticData, resp.staticErr
}
