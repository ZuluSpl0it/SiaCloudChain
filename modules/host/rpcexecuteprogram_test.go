package host

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/siamux/mux"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host/mdm"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
	"gitlab.com/scpcorp/ScPrime/types"
)

// updateRunningCosts is a testing helper function for updating the running
// costs of a program after adding an instruction.
func updateRunningCosts(pt *modules.RPCPriceTable, runningCost, runningRefund, runningCollateral types.Currency, runningMemory uint64, cost, refund, collateral types.Currency, memory, time uint64) (types.Currency, types.Currency, types.Currency, uint64) {
	runningMemory = runningMemory + memory
	memoryCost := modules.MDMMemoryCost(pt, runningMemory, time)
	runningCost = runningCost.Add(memoryCost).Add(cost)
	runningRefund = runningRefund.Add(refund)
	runningCollateral = runningCollateral.Add(collateral)

	return runningCost, runningRefund, runningCollateral, runningMemory
}

// newHasSectorInstruction is a convenience method for creating a single
// 'HasSector' instruction.
func newHasSectorInstruction(dataOffset uint64, pt *modules.RPCPriceTable) (modules.Instruction, types.Currency, types.Currency, types.Currency, uint64, uint64) {
	i := mdm.NewHasSectorInstruction(dataOffset)
	cost, refund := modules.MDMHasSectorCost(pt)
	collateral := modules.MDMHasSectorCollateral()
	return i, cost, refund, collateral, modules.MDMHasSectorMemory(), modules.MDMTimeHasSector
}

// newHasSectorProgram is a convenience method which prepares the instructions
// and the program data for a program that executes a single
// HasSectorInstruction.
func newHasSectorProgram(merkleRoot crypto.Hash, pt *modules.RPCPriceTable) ([]modules.Instruction, []byte, types.Currency, types.Currency, types.Currency, uint64) {
	data := make([]byte, crypto.HashSize)
	copy(data[:crypto.HashSize], merkleRoot[:])
	initCost := modules.MDMInitCost(pt, uint64(len(data)), 1)
	i, cost, refund, collateral, memory, time := newHasSectorInstruction(0, pt)
	cost, refund, collateral, memory = updateRunningCosts(pt, initCost, types.ZeroCurrency, types.ZeroCurrency, modules.MDMInitMemory(), cost, refund, collateral, memory, time)
	instructions := []modules.Instruction{i}
	return instructions, data, cost, refund, collateral, memory
}

// newReadSectorInstruction is a convenience method for creating a single
// 'ReadSector' instruction.
func newReadSectorInstruction(length uint64, merkleProof bool, dataOffset uint64, pt *modules.RPCPriceTable) (modules.Instruction, types.Currency, types.Currency, types.Currency, uint64, uint64) {
	i := mdm.NewReadSectorInstruction(dataOffset, dataOffset+8, dataOffset+16, merkleProof)
	cost, refund := modules.MDMReadCost(pt, length)
	collateral := modules.MDMReadCollateral()
	return i, cost, refund, collateral, modules.MDMReadMemory(), modules.MDMTimeReadSector
}

// newReadSectorProgram is a convenience method which prepares the instructions
// and the program data for a program that executes a single
// ReadSectorInstruction.
func newReadSectorProgram(length, offset uint64, merkleRoot crypto.Hash, pt *modules.RPCPriceTable) ([]modules.Instruction, []byte, types.Currency, types.Currency, types.Currency, uint64) {
	data := make([]byte, 8+8+crypto.HashSize)
	binary.LittleEndian.PutUint64(data[:8], length)
	binary.LittleEndian.PutUint64(data[8:16], offset)
	copy(data[16:], merkleRoot[:])
	initCost := modules.MDMInitCost(pt, uint64(len(data)), 1)
	i, cost, refund, collateral, memory, time := newReadSectorInstruction(length, true, 0, pt)
	cost, refund, collateral, memory = updateRunningCosts(pt, initCost, types.ZeroCurrency, types.ZeroCurrency, modules.MDMInitMemory(), cost, refund, collateral, memory, time)
	instructions := []modules.Instruction{i}
	return instructions, data, cost, refund, collateral, memory
}

// executeProgramResponse is a helper struct that wraps the
// RPCExecuteProgramResponse together with the output data
type executeProgramResponse struct {
	modules.RPCExecuteProgramResponse
	Output []byte
}

// executeProgram executes an MDM program on the host using an EA payment and
// returns the responses received by the host. A failure to execute an
// instruction won't result in an error. Instead the returned responses need to
// be inspected for that depending on the testcase.
func (rhp *renterHostPair) executeProgram(epr modules.RPCExecuteProgramRequest, programData []byte, budget types.Currency) ([]executeProgramResponse, mux.BandwidthLimit, error) {
	// create stream
	stream := rhp.newStream()
	defer stream.Close()

	// Get the limit to track bandwidth.
	limit := stream.Limit()

	// Write the specifier.
	err := modules.RPCWrite(stream, modules.RPCExecuteProgram)
	if err != nil {
		return nil, limit, err
	}

	// Write the pricetable uid.
	err = modules.RPCWrite(stream, rhp.latestPT.UID)
	if err != nil {
		return nil, limit, err
	}

	// Send the payment request.
	err = modules.RPCWrite(stream, modules.PaymentRequest{Type: modules.PayByEphemeralAccount})
	if err != nil {
		return nil, limit, err
	}

	// Send the payment details.
	pbear := newPayByEphemeralAccountRequest(rhp.accountID, rhp.ht.host.BlockHeight()+6, budget, rhp.accountKey)
	err = modules.RPCWrite(stream, pbear)
	if err != nil {
		return nil, limit, err
	}

	// Receive payment confirmation.
	var pc modules.PayByEphemeralAccountResponse
	err = modules.RPCRead(stream, &pc)
	if err != nil {
		return nil, limit, err
	}

	// Send the execute program request.
	err = modules.RPCWrite(stream, epr)
	if err != nil {
		return nil, limit, err
	}

	// Send the programData.
	_, err = stream.Write(programData)
	if err != nil {
		return nil, limit, err
	}

	// Read the responses.
	responses := make([]executeProgramResponse, len(epr.Program))
	for i := range epr.Program {
		// Read the response.
		err = modules.RPCRead(stream, &responses[i])
		if err != nil {
			return nil, limit, err
		}

		// Read the output data.
		outputLen := responses[i].OutputLength
		responses[i].Output = make([]byte, outputLen, outputLen)
		_, err = io.ReadFull(stream, responses[i].Output)
		if err != nil {
			return nil, limit, err
		}

		// If the response contains an error we are done.
		if responses[i].Error != nil {
			return responses, limit, nil
		}
	}

	// The next read should return io.EOF since the host closes the connection
	// after the RPC is done.
	err = modules.RPCRead(stream, struct{}{})
	if !errors.Contains(err, io.ErrClosedPipe) {
		return nil, limit, err
	}
	return responses, limit, nil
}

// TestExecuteProgramWriteDeadline verifies the ExecuteProgramRPC sets a write
// deadline
func TestExecuteProgramWriteDeadline(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	delay := modules.MDMProgramWriteResponseTime * 2
	deps := dependencies.NewHostMDMProgramWriteDelay(delay)
	rhp, err := newCustomRenterHostPair(t.Name(), deps)
	if err != nil {
		t.Fatal(err)
	}
	defer rhp.Close()

	// create stream
	stream := rhp.newStream()
	defer stream.Close()

	// create a random sector
	sectorRoot, _, err := rhp.addRandomSector()
	if err != nil {
		t.Fatal(err)
	}

	// create the 'ReadSector' program.
	program, programData, _, _, _, _ := newReadSectorProgram(modules.SectorSize, 0, sectorRoot, rhp.latestPT)

	// prepare the request.
	epr := modules.RPCExecuteProgramRequest{
		FileContractID:    rhp.fcid,
		Program:           program,
		ProgramDataLength: uint64(len(programData)),
	}

	// prefund the EA
	rhp.prefundAccount()

	// execute program.
	budget := types.NewCurrency64(math.MaxUint64)
	_, _, err = rhp.executeProgram(epr, programData, budget)
	if err == nil || !errors.Contains(err, io.ErrClosedPipe) {
		t.Fatal("Expected executeProgram to fail with an ErrClosedPipe, instead err was", err)
	}
}

// TestExecuteReadSectorProgram tests the managedRPCExecuteProgram with a valid
// 'ReadSector' program.
func TestExecuteReadSectorProgram(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rhp.Close()
	ht := rhp.ht

	// create a random sector
	sectorRoot, sectorData, err := rhp.addRandomSector()
	if err != nil {
		t.Fatal(err)
	}

	// get a snapshot of the SO before running the program.
	sos, err := ht.host.managedGetStorageObligationSnapshot(rhp.fcid)
	if err != nil {
		t.Fatal(err)
	}
	// verify the root is not the zero root to begin with
	if sos.staticMerkleRoot == crypto.MerkleRoot([]byte{}) {
		t.Fatalf("expected merkle root to be non zero: %v", sos.staticMerkleRoot)
	}

	// create the 'ReadSector' program.
	program, data, programCost, refund, collateral, _ := newReadSectorProgram(modules.SectorSize, 0, sectorRoot, rhp.latestPT)

	// prepare the request.
	epr := modules.RPCExecuteProgramRequest{
		FileContractID:    rhp.fcid, // TODO: leave this empty since it's not required for a readonly program.
		Program:           program,
		ProgramDataLength: uint64(len(data)),
	}

	// fund an account.
	his := rhp.ht.host.managedInternalSettings()
	maxBalance := his.MaxEphemeralAccountBalance
	fundingAmt := maxBalance.Add(rhp.latestPT.FundAccountCost)
	_, err = rhp.fundEphemeralAccount(fundingAmt)
	if err != nil {
		t.Fatal(err)
	}

	// Compute expected bandwidth cost. These hardcoded values were chosen after
	// running this test with a high budget and measuring the used bandwidth for
	// this particular program on the "renter" side. This way we can test that
	// the bandwidth measured by the renter is large enough to be accepted by
	// the host.
	expectedDownload := uint64(10220) // download
	expectedUpload := uint64(18980)   // upload
	downloadCost := rhp.latestPT.DownloadBandwidthCost.Mul64(expectedDownload)
	uploadCost := rhp.latestPT.UploadBandwidthCost.Mul64(expectedUpload)
	bandwidthCost := downloadCost.Add(uploadCost)
	cost := programCost.Add(bandwidthCost)

	// execute program.
	resps, limit, err := rhp.executeProgram(epr, data, cost)
	if err != nil {
		t.Log("cost", cost.HumanString())
		t.Log("expected ea balance", rhp.ht.host.managedInternalSettings().MaxEphemeralAccountBalance.HumanString())
		t.Fatal(err)
	}

	// Log the bandwidth used by this RPC.
	t.Logf("Used bandwidth (read full sector program): %v down, %v up", limit.Downloaded(), limit.Uploaded())

	// there should only be a single response.
	if len(resps) != 1 {
		t.Fatalf("expected 1 response but got %v", len(resps))
	}
	resp := resps[0]

	// check response.
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if resp.NewSize != sos.staticContractSize {
		t.Fatalf("expected contract size to stay the same: %v != %v", sos.staticContractSize, resp.NewSize)
	}
	if resp.NewMerkleRoot != sos.staticMerkleRoot {
		t.Fatalf("expected merkle root to stay the same: %v != %v", sos.staticMerkleRoot, resp.NewMerkleRoot)
	}
	if len(resp.Proof) != 0 {
		t.Fatalf("expected proof length to be %v but was %v", 0, len(resp.Proof))
	}

	if !resp.AdditionalCollateral.Equals(collateral) {
		t.Fatalf("collateral doesnt't match expected collateral: %v != %v", resp.AdditionalCollateral.HumanString(), collateral.HumanString())
	}
	if !resp.PotentialRefund.Equals(refund) {
		t.Fatalf("refund doesn't match expected refund: %v != %v", resp.PotentialRefund.HumanString(), refund.HumanString())
	}
	if uint64(len(resp.Output)) != modules.SectorSize {
		t.Fatalf("expected returned data to have length %v but was %v", modules.SectorSize, len(resp.Output))
	}
	if !bytes.Equal(sectorData, resp.Output) {
		t.Fatal("Unexpected data")
	}

	// verify the cost
	if !resp.TotalCost.Equals(programCost) {
		t.Fatalf("wrong TotalCost %v != %v", resp.TotalCost.HumanString(), cost.HumanString())
	}

	// verify the EA balance
	am := rhp.ht.host.staticAccountManager
	expectedBalance := maxBalance.Sub(cost)
	err = verifyBalance(am, rhp.accountID, expectedBalance)
	if err != nil {
		t.Fatal(err)
	}

	// rerun the program but now make sure the given budget does not cover the
	// cost, we expect this to return ErrInsufficientBandwidthBudget
	cost = cost.Sub64(1)
	_, limit, err = rhp.executeProgram(epr, data, cost)
	if err == nil || !strings.Contains(err.Error(), modules.ErrInsufficientBandwidthBudget.Error()) {
		t.Fatalf("expected executeProgram to fail due to insufficient bandwidth budget: %v", err)
	}

	// verify the host charged us by checking the EA balance and Check that the
	// remaining balance is correct again. We expect the host to charge us for
	// the program since the bandwidth limit was reached when sending the
	// response, after executing the program.
	downloadCost = rhp.latestPT.DownloadBandwidthCost.Mul64(limit.Downloaded())
	uploadCost = rhp.latestPT.UploadBandwidthCost.Mul64(limit.Uploaded())

	expectedBalance = expectedBalance.Sub(downloadCost).Sub(uploadCost).Sub(programCost)
	err = verifyBalance(am, rhp.accountID, expectedBalance)
	if err != nil {
		t.Fatal(err)
	}
}

// TestExecuteReadPartialSectorProgram tests the managedRPCExecuteProgram with a
// valid 'ReadSector' program that only reads half a sector.
func TestExecuteReadPartialSectorProgram(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rhp.Close()
	ht := rhp.ht

	// get a snapshot of the SO before running the program.
	sos, err := ht.host.managedGetStorageObligationSnapshot(rhp.fcid)
	if err != nil {
		t.Fatal(err)
	}

	// create a random sector
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)
	// modify the host's storage obligation to add the sector
	so, err := ht.host.managedGetStorageObligation(rhp.fcid)
	if err != nil {
		t.Fatal(err)
	}
	so.SectorRoots = append(so.SectorRoots, sectorRoot)
	ht.host.managedLockStorageObligation(rhp.fcid)
	err = ht.host.managedModifyStorageObligation(so, []crypto.Hash{}, map[crypto.Hash][]byte{sectorRoot: sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(rhp.fcid)

	offset := uint64(fastrand.Uint64n((modules.SectorSize/crypto.SegmentSize)-1) * crypto.SegmentSize)
	length := uint64(crypto.SegmentSize)

	// create the 'ReadSector' program.
	program, data, programCost, refund, collateral, _ := newReadSectorProgram(length, offset, sectorRoot, rhp.latestPT)

	// prepare the request.
	epr := modules.RPCExecuteProgramRequest{
		FileContractID:    rhp.fcid, // TODO: leave this empty since it's not required for a readonly program.
		Program:           program,
		ProgramDataLength: uint64(len(data)),
	}

	// fund an account.
	fundingAmt := rhp.ht.host.managedInternalSettings().MaxEphemeralAccountBalance.Add(rhp.latestPT.FundAccountCost)
	_, err = rhp.fundEphemeralAccount(fundingAmt)
	if err != nil {
		t.Fatal(err)
	}

	// Compute expected bandwidth cost. These hardcoded values were chosen after
	// running this test with a high budget and measuring the used bandwidth for
	// this particular program on the "renter" side. This way we can test that
	// the bandwidth measured by the renter is large enough to be accepted by
	// the host.
	expectedDownload := uint64(10220)
	expectedUpload := uint64(18980)
	downloadCost := rhp.latestPT.DownloadBandwidthCost.Mul64(expectedDownload)
	uploadCost := rhp.latestPT.UploadBandwidthCost.Mul64(expectedUpload)
	bandwidthCost := downloadCost.Add(uploadCost)
	cost := programCost.Add(bandwidthCost)

	// execute program.
	resps, bandwidth, err := rhp.executeProgram(epr, data, cost)
	if err != nil {
		t.Log("cost", cost.HumanString())
		t.Log("expected ea balance", rhp.ht.host.managedInternalSettings().MaxEphemeralAccountBalance.HumanString())
		t.Fatal(err)
	}
	// there should only be a single response.
	if len(resps) != 1 {
		t.Fatalf("expected 1 response but got %v", len(resps))
	}
	resp := resps[0]

	// check response.
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if resp.NewSize != sos.staticContractSize {
		t.Fatalf("expected contract size to stay the same: %v != %v", sos.staticContractSize, resp.NewSize)
	}
	if resp.NewMerkleRoot != sos.staticMerkleRoot {
		t.Fatalf("expected merkle root to stay the same: %v != %v", sos.staticMerkleRoot, resp.NewMerkleRoot)
	}
	if !resp.AdditionalCollateral.Equals(collateral) {
		t.Fatalf("collateral doesnt't match expected collateral: %v != %v", resp.AdditionalCollateral.HumanString(), collateral.HumanString())
	}
	if !resp.PotentialRefund.Equals(refund) {
		t.Fatalf("refund doesn't match expected refund: %v != %v", resp.PotentialRefund.HumanString(), refund.HumanString())
	}
	if uint64(len(resp.Output)) != length {
		t.Fatalf("expected returned data to have length %v but was %v", length, len(resp.Output))
	}

	if !bytes.Equal(sectorData[offset:offset+length], resp.Output) {
		t.Fatal("Unexpected data")
	}

	// verify the proof
	proofStart := int(offset) / crypto.SegmentSize
	proofEnd := int(offset+length) / crypto.SegmentSize
	proof := crypto.MerkleRangeProof(sectorData, proofStart, proofEnd)
	if !reflect.DeepEqual(proof, resp.Proof) {
		t.Fatal("proof doesn't match expected proof")
	}

	// verify the cost
	if !resp.TotalCost.Equals(programCost) {
		t.Fatalf("wrong TotalCost %v != %v", resp.TotalCost.HumanString(), cost.HumanString())
	}

	t.Logf("Used bandwidth (read partial sector program): %v down, %v up", bandwidth.Downloaded(), bandwidth.Uploaded())
}

// TestExecuteHasSectorProgram tests the managedRPCExecuteProgram with a valid
// 'HasSector' program.
func TestExecuteHasSectorProgram(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rhp.Close()
	ht := rhp.ht

	// get a snapshot of the SO before running the program.
	sos, err := rhp.ht.host.managedGetStorageObligationSnapshot(rhp.fcid)
	if err != nil {
		t.Fatal(err)
	}

	// Add a sector to the host but not the storage obligation or contract. This
	// instruction should also work for foreign sectors.
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)
	err = ht.host.AddSector(sectorRoot, sectorData)
	if err != nil {
		t.Fatal(err)
	}

	// Create the 'HasSector' program.
	program, data, programCost, refund, collateral, _ := newHasSectorProgram(sectorRoot, rhp.latestPT)

	// Prepare the request.
	epr := modules.RPCExecuteProgramRequest{
		FileContractID:    rhp.fcid, // TODO: leave this empty since it's not required for a readonly program.
		Program:           program,
		ProgramDataLength: uint64(len(data)),
	}

	// Fund an account with the max balance.
	maxBalance := rhp.ht.host.managedInternalSettings().MaxEphemeralAccountBalance
	fundingAmt := maxBalance.Add(rhp.latestPT.FundAccountCost)
	_, err = rhp.fundEphemeralAccount(fundingAmt)
	if err != nil {
		t.Fatal(err)
	}

	// Compute expected bandwidth cost. These hardcoded values were chosen after
	// running this test with a high budget and measuring the used bandwidth for
	// this particular program on the "renter" side. This way we can test that
	// the bandwidth measured by the renter is large enough to be accepted by
	// the host.
	expectedDownload := uint64(7300) // download
	expectedUpload := uint64(18980)  // upload
	downloadCost := rhp.latestPT.DownloadBandwidthCost.Mul64(expectedDownload)
	uploadCost := rhp.latestPT.UploadBandwidthCost.Mul64(expectedUpload)
	bandwidthCost := downloadCost.Add(uploadCost)

	// Execute program.
	cost := programCost.Add(bandwidthCost)
	resps, limit, err := rhp.executeProgram(epr, data, cost)
	if err != nil {
		t.Fatal(err)
	}
	// Log the bandwidth used by this RPC.
	t.Logf("Used bandwidth (valid program): %v down, %v up", limit.Downloaded(), limit.Uploaded())
	// There should only be a single response.
	if len(resps) != 1 {
		t.Fatalf("expected 1 response but got %v", len(resps))
	}
	resp := resps[0]

	// Check response.
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	if !resp.AdditionalCollateral.Equals(collateral) {
		t.Fatalf("wrong AdditionalCollateral %v != %v", resp.AdditionalCollateral.HumanString(), collateral.HumanString())
	}
	if resp.NewMerkleRoot != sos.MerkleRoot() {
		t.Fatalf("wrong NewMerkleRoot %v != %v", resp.NewMerkleRoot, sos.MerkleRoot())
	}
	if resp.NewSize != 0 {
		t.Fatalf("wrong NewSize %v != %v", resp.NewSize, 0)
		t.Fatal("wrong NewSize")
	}
	if len(resp.Proof) != 0 {
		t.Fatalf("wrong Proof %v != %v", resp.Proof, []crypto.Hash{})
	}
	if resp.Output[0] != 1 {
		t.Fatalf("wrong Output %v != %v", resp.Output[0], []byte{1})
	}
	if !resp.TotalCost.Equals(programCost) {
		t.Fatalf("wrong TotalCost %v != %v", resp.TotalCost.HumanString(), cost.HumanString())
	}
	if !resp.PotentialRefund.Equals(refund) {
		t.Fatalf("wrong PotentialRefund %v != %v", resp.PotentialRefund.HumanString(), refund.HumanString())
	}
	// Make sure the right amount of money remains on the EA.
	am := rhp.ht.host.staticAccountManager
	expectedBalance := maxBalance.Sub(cost)
	err = verifyBalance(am, rhp.accountID, expectedBalance)
	if err != nil {
		t.Fatal(err)
	}

	// Execute program again. This time pay for 1 less byte of bandwidth. This should fail.
	cost = programCost.Add(bandwidthCost.Sub64(1))
	_, limit, err = rhp.executeProgram(epr, data, cost)
	if err == nil || !strings.Contains(err.Error(), modules.ErrInsufficientBandwidthBudget.Error()) {
		t.Fatalf("expected executeProgram to fail due to insufficient bandwidth budget: %v", err)
	}
	// Log the bandwidth used by this RPC.
	t.Logf("Used bandwidth (invalid program): %v down, %v up", limit.Downloaded(), limit.Uploaded())
	// Check that the remaining balance is correct again. We expect the host to
	// charge us for the program since the bandwidth limit was reached when
	// sending the response, after executing the program.
	downloadCost = rhp.latestPT.DownloadBandwidthCost.Mul64(limit.Downloaded())
	uploadCost = rhp.latestPT.UploadBandwidthCost.Mul64(limit.Uploaded())

	expectedBalance = expectedBalance.Sub(downloadCost).Sub(uploadCost).Sub(programCost)
	err = verifyBalance(am, rhp.accountID, expectedBalance)
	if err != nil {
		t.Fatal(err)
	}
}

// verifyBalance is a helper function that will verify if the ephemeral account
// with given id has a certain balance. It does this in a (short) build.Retry
// loop to avoid race conditions.
func verifyBalance(am *accountManager, id modules.AccountID, expected types.Currency) error {
	return build.Retry(100, 100*time.Millisecond, func() error {
		actual := am.callAccountBalance(id)
		if !actual.Equals(expected) {
			return fmt.Errorf("expected %v remaining balance but got %v", expected, actual)
		}
		return nil
	})
}
