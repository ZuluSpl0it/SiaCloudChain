package mdm

import (
	"math"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newTestWriteStorePriceTable returns a custom price table for the cost tests.
func newTestWriteStorePriceTable() *modules.RPCPriceTable {
	pt := &modules.RPCPriceTable{}
	pt.Expiry = time.Now().Add(time.Minute).Unix()
	pt.WriteBaseCost = types.ZeroCurrency
	pt.WriteLengthCost = types.ZeroCurrency
	pt.WriteStoreCost = modules.DefaultStoragePrice
	return pt
}

// TestCostForAppendProgram calculates the cost for a program which appends 1
// TiB of data.
//
// NOTE: We use a modified cost function for Append which returns the cost for
// production-sized sectors, as sectors in testing are only 4 KiB.
func TestCostForAppendProgram(t *testing.T) {
	pt := newTestWriteStorePriceTable()

	// Define helper variables.
	tib := uint64(1e12)

	// Initialize starting values.
	numInstructions := uint64(math.Ceil(float64(tib) / float64(modules.SectorSizeStandard)))
	runningCost := modules.MDMInitCost(pt, tib, numInstructions)
	runningRefund := types.ZeroCurrency
	runningCollateral := types.ZeroCurrency
	runningMemory := modules.MDMInitMemory()
	runningSize := uint64(0)

	// Simulate running a program to append 1 TiB of data.
	for runningSize < tib {
		cost, refund := appendTrueCost(pt)
		collateral := modules.MDMAppendCollateral(pt)
		memory := modules.SectorSizeStandard // override MDMAppendMemory()
		time := uint64(modules.MDMTimeAppend)
		runningCost, runningRefund, runningCollateral, runningMemory = updateRunningCosts(pt, runningCost, runningRefund, runningCollateral, runningMemory, cost, refund, collateral, memory, time)
		runningSize += modules.SectorSizeStandard
	}
	runningCost = runningCost.Add(modules.MDMMemoryCost(pt, runningMemory, modules.MDMTimeCommit))

	expectedCost := modules.DefaultStoragePrice.Mul(modules.BytesPerTerabyte)
	if !aboutEquals(expectedCost, runningCost) {
		t.Errorf("expected cost for appending 1 TiB to be %v, got cost %v", expectedCost.HumanString(), runningCost.HumanString())
	}

	// The expected refund is equal to the expected cost because we are testing
	// the cost of storage. When we refund an append, we give back only the full
	// cost of storage.
	expectedRefund := expectedCost
	if !aboutEquals(expectedRefund, runningRefund) {
		t.Errorf("expected refund for appending 1 TiB to be %v, got refund %v", expectedRefund.HumanString(), runningRefund.HumanString())
	}

	expectedMemory := runningSize + modules.MDMInitMemory() // 1 TiB + 1 MiB
	if expectedMemory != runningMemory {
		t.Errorf("expected memory for appending 1 TiB to be %v, got %v", expectedMemory, runningMemory)
	}
}

// appendTrueCost returns the true, production cost of an append. This is
// necessary because in tests the sector size is only 4 KiB and the append cost
// is misleading.
func appendTrueCost(pt *modules.RPCPriceTable) (types.Currency, types.Currency) {
	writeCost := pt.WriteLengthCost.Mul64(modules.SectorSizeStandard).Add(pt.WriteBaseCost)
	storeCost := pt.WriteStoreCost.Mul64(modules.SectorSizeStandard) // potential refund
	return writeCost.Add(storeCost), storeCost
}

// TestCosts tests the costs for individual instructions so that we have a sense
// of their relative costs and to make sure they are sensible values.
func TestCosts(t *testing.T) {
	pt := newTestWriteStorePriceTable()

	// Define helper variables.
	sectorsPerTB := modules.BytesPerTerabyte.Div64(modules.SectorSizeStandard)

	// Append
	cost, refund := appendTrueCost(pt)
	// Scale the cost from a single, production-sized sector up to a TB of data.
	costPerTB := cost.Mul(sectorsPerTB)
	expectedCostPerTB := modules.DefaultStoragePrice.Mul(modules.BytesPerTerabyte)
	if !aboutEquals(costPerTB, expectedCostPerTB) {
		t.Errorf("expected append cost %v, got %v", expectedCostPerTB.HumanString(), costPerTB.HumanString())
	}
	// cost == refund because we are testing the storage costs, and the refund
	// comprises only the storage cost.
	expectedRefundPerTB := expectedCostPerTB
	refundPerTB := refund.Mul(sectorsPerTB)
	if !aboutEquals(refundPerTB, expectedRefundPerTB) {
		t.Errorf("expected append refund %v, got %v", expectedRefundPerTB.HumanString(), refundPerTB.HumanString())
	}
}

// TestAboutEquals verifies the correctness of the aboutEquals helper.
func TestAboutEquals(t *testing.T) {
	c := types.NewCurrency64
	tests := []struct {
		cExpected, cActual types.Currency
		out                bool
	}{
		{c(100), c(90), true},
		{c(100), c(110), true},
		{c(100), c(89), false},
		{c(100), c(111), false},
	}
	for _, test := range tests {
		out := aboutEquals(test.cExpected, test.cActual)
		if out != test.out {
			t.Errorf("aboutEquals(%v, %v): expected '%v', got '%v'", test.cExpected, test.cActual, test.out, out)
		}
	}
}

// aboutEquals checks that two currencies are approximately equal.
func aboutEquals(cExpected, cActual types.Currency) bool {
	// The precision with which we check results is 10% of the expected value. We
	// don't need to know that the exact cost of an append is
	// '25425636574074000000000000', we just need a rough value.
	errorWindow := cExpected.Div64(10)
	return cExpected.Add(errorWindow).Cmp(cActual) >= 0 && cExpected.Sub(errorWindow).Cmp(cActual) <= 0
}
