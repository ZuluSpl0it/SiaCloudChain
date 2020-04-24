package mdm

import (
	"bytes"
	"context"
	"testing"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newHasSectorInstruction is a convenience method for creating a single
// 'HasSector' instruction.
func newHasSectorInstruction(dataOffset uint64, pt *modules.RPCPriceTable) (modules.Instruction, types.Currency, types.Currency, types.Currency, uint64, uint64) {
	i := NewHasSectorInstruction(dataOffset)
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

// TestInstructionHasSector tests executing a program with a single
// HasSectorInstruction.
func TestInstructionHasSector(t *testing.T) {
	host := newTestHost()
	mdm := New(host)
	defer mdm.Stop()

	// Create a program to check for a sector on the host.
	so := newTestStorageObligation(true)
	so.sectorRoots = randomSectorRoots(1)

	// Add sector to the host.
	sectorRoot := so.sectorRoots[0]
	_, err := host.ReadSector(sectorRoot)
	if err != nil {
		t.Fatal(err)
	}

	pt := newTestPriceTable()
	instructions, programData, cost, refund, collateral, _ := newHasSectorProgram(sectorRoot, pt)
	dataLen := uint64(len(programData))
	// Execute it.
	budget := modules.NewBudget(cost)
	finalize, outputs, err := mdm.ExecuteProgram(context.Background(), pt, instructions, budget, collateral, so, dataLen, bytes.NewReader(programData))
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
		if !output.ExecutionCost.Equals(cost) {
			t.Fatalf("execution cost doesn't match expected execution cost: %v != %v", output.ExecutionCost.HumanString(), cost.HumanString())
		}
		if !budget.Remaining().Equals(cost.Sub(output.ExecutionCost)) {
			t.Fatalf("budget should be equal to the initial budget minus the execution cost: %v != %v",
				budget.Remaining().HumanString(), cost.Sub(output.ExecutionCost).HumanString())
		}
		if !output.AdditionalCollateral.Equals(collateral) {
			t.Fatalf("collateral doesnt't match expected collateral: %v != %v", output.AdditionalCollateral.HumanString(), collateral.HumanString())
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
