package mdm

import (
	"bytes"
	"context"
	"testing"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newHasSectorProgram is a convenience method which prepares the instructions
// and the program data for a program that executes a single
// HasSectorInstruction.
func newHasSectorProgram(merkleRoot crypto.Hash, pt modules.RPCPriceTable) ([]modules.Instruction, []byte, types.Currency, types.Currency, uint64) {
	i := NewHasSectorInstruction(0)
	instructions := []modules.Instruction{i}
	data := make([]byte, crypto.HashSize)
	copy(data[:crypto.HashSize], merkleRoot[:])

	// Compute cost and used memory.
	cost, refund := modules.MDMHasSectorCost(pt)
	usedMemory := modules.MDMHasSectorMemory()
	memoryCost := modules.MDMMemoryCost(pt, usedMemory, TimeHasSector+TimeCommit)
	initCost := modules.MDMInitCost(pt, uint64(len(data)))
	cost = cost.Add(memoryCost).Add(initCost)
	return instructions, data, cost, refund, usedMemory
}

// TestInstructionHasSector tests executing a program with a single
// HasSectorInstruction.
func TestInstructionHasSector(t *testing.T) {
	host := newTestHost()
	mdm := New(host)
	defer mdm.Stop()

	// Add a random sector to the host.
	sectorRoot := randomSector()
	_, err := host.ReadSector(sectorRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Create a program to check for a sector on the host.
	so := newTestStorageObligation(true)
	so.sectorRoots = randomSectorRoots(1)
	sectorRoot = so.sectorRoots[0]
	pt := newTestPriceTable()
	instructions, programData, cost, refund, usedMemory := newHasSectorProgram(sectorRoot, pt)
	dataLen := uint64(len(programData))
	// Execute it.
	finalize, outputs, err := mdm.ExecuteProgram(context.Background(), pt, instructions, modules.MDMInitCost(pt, dataLen).Add(cost), so, dataLen, bytes.NewReader(programData))
	if err != nil {
		t.Fatal(err)
	}
	// Check outputs.
	numOutputs := 0
	for output := range outputs {
		if err := output.Error; err != nil {
			t.Fatal(err)
		}
		if output.NewSize != so.ContractSize() {
			t.Fatalf("expected contract size to stay the same: %v != %v", so.ContractSize(), output.NewSize)
		}
		if output.NewMerkleRoot != so.MerkleRoot() {
			t.Fatalf("expected merkle root to stay the same: %v != %v", so.MerkleRoot(), output.NewMerkleRoot)
		}
		// Verify proof was created correctly.
		if len(output.Proof) != 0 {
			t.Fatalf("expected proof to have len %v but was %v", 0, len(output.Proof))
		}
		if !bytes.Equal(output.Output, []byte{1}) {
			t.Fatalf("expected returned value to be [1] for 'true' but was %v", output.Output)
		}
		if !output.ExecutionCost.Equals(cost.Sub(modules.MDMMemoryCost(pt, usedMemory, modules.MDMTimeCommit))) {
			t.Fatalf("execution cost doesn't match expected execution cost: %v != %v", output.ExecutionCost.HumanString(), cost.HumanString())
		}
		if !output.PotentialRefund.Equals(refund) {
			t.Fatalf("refund doesn't match expected refund: %v != %v", output.PotentialRefund.HumanString(), refund.HumanString())
		}
		numOutputs++
	}
	// There should be one output since there was one instruction.
	if numOutputs != 1 {
		t.Fatalf("numOutputs was %v but should be %v", numOutputs, 1)
	}
	// No need to finalize the program since this program is readonly.
	if finalize != nil {
		t.Fatal("finalize callback should be nil for readonly program")
	}
}
