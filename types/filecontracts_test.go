package types

import (
	"testing"

	"gitlab.com/NebulousLabs/errors"
)

// TestForkedTax probes the Tax function after SPF hardfork height.
func TestForkedTax(t *testing.T) {
	if Tax(SpfHardforkHeight+1, NewCurrency64(125e9)).Cmp(NewCurrency64(1875e7)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}
	if PostTax(SpfHardforkHeight+1, NewCurrency64(125e9)).Cmp(NewCurrency64(10625e7)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}
}

// TestFileContractTax probes the Tax function.
func TestTax(t *testing.T) {
	// Test explicit values for post-hardfork tax values.
	if Tax(TaxHardforkHeight+1, NewCurrency64(125e9)).Cmp(NewCurrency64(4875e6)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}
	if PostTax(TaxHardforkHeight+1, NewCurrency64(125e9)).Cmp(NewCurrency64(120125e6)) != 0 {
		t.Error("tax is being calculated incorrectly")
	}

	// Test equivalency for a series of values.
	if testing.Short() {
		t.SkipNow()
	}
	// COMPATv0.4.0 - check at height 0.
	for i := uint64(0); i < 10e3; i++ {
		val := NewCurrency64((1e3 * i) + i)
		tax := Tax(0, val)
		postTax := PostTax(0, val)
		if val.Cmp(tax.Add(postTax)) != 0 {
			t.Error("tax calculation inconsistent for", i)
		}
	}
	// Check at height 1e9
	for i := uint64(0); i < 10e3; i++ {
		val := NewCurrency64((1e3 * i) + i)
		tax := Tax(1e9, val)
		postTax := PostTax(1e9, val)
		if val.Cmp(tax.Add(postTax)) != 0 {
			t.Error("tax calculation inconsistent for", i)
		}
	}
}

// TestPaymentRevision probes the PaymentRevision function
func TestPaymentRevision(t *testing.T) {
	mock := func(renterFunds, hostCollateral uint64) FileContractRevision {
		return FileContractRevision{
			NewValidProofOutputs: []SiacoinOutput{
				{Value: NewCurrency64(renterFunds)},
				{Value: ZeroCurrency},
			},
			NewMissedProofOutputs: []SiacoinOutput{
				{Value: NewCurrency64(renterFunds)},
				{Value: NewCurrency64(hostCollateral)},
				{Value: ZeroCurrency},
			},
		}
	}

	// expect no error if amount is less than or equal to the renter funds
	rev := mock(100, 0)
	_, err := rev.PaymentRevision(NewCurrency64(99))
	if err != nil {
		t.Fatalf("Unexpected error '%v'", err)
	}
	_, err = rev.PaymentRevision(NewCurrency64(100))
	if err != nil {
		t.Fatalf("Unexpected error '%v'", err)
	}

	// expect ErrRevisionCostTooHigh
	rev = mock(100, 0)
	_, err = rev.PaymentRevision(NewCurrency64(100 + 1))
	if !errors.Contains(err, ErrRevisionCostTooHigh) {
		t.Fatalf("Expected error '%v' but received '%v'  ", ErrRevisionCostTooHigh, err)
	}

	// expect ErrRevisionCostTooHigh
	rev = mock(100, 0)
	rev.SetMissedRenterPayout(NewCurrency64(99))
	_, err = rev.PaymentRevision(NewCurrency64(100))
	if !errors.Contains(err, ErrRevisionCostTooHigh) {
		t.Fatalf("Expected error '%v' but received '%v'  ", ErrRevisionCostTooHigh, err)
	}

	// expect no error if amount is less than or equal to the host collateral
	rev = mock(100, 100)
	_, err = rev.PaymentRevision(NewCurrency64(99))
	if err != nil {
		t.Fatalf("Unexpected error '%v'", err)
	}
	_, err = rev.PaymentRevision(NewCurrency64(100))
	if err != nil {
		t.Fatalf("Unexpected error '%v'", err)
	}

	// verify funds moved to the appropriate outputs
	existing := mock(100, 0)
	amount := NewCurrency64(99)
	payment, err := existing.PaymentRevision(amount)
	if err != nil {
		t.Fatalf("Unexpected error '%v'", err)
	}
	if !existing.ValidRenterPayout().Sub(payment.ValidRenterPayout()).Equals(amount) {
		t.Fatal("Unexpected payout moved from renter to host")
	}
	if !payment.ValidHostPayout().Sub(existing.ValidHostPayout()).Equals(amount) {
		t.Fatal("Unexpected payout moved from renter to host")
	}
	if !payment.MissedHostPayout().Sub(existing.MissedHostPayout()).Equals(amount) {
		t.Fatal("Unexpected payout moved from renter to host")
	}
	if !payment.MissedRenterOutput().Value.Equals(existing.MissedRenterOutput().Value.Sub(amount)) {
		t.Fatal("Unexpected payout moved from renter to void")
	}
	pmvo, err1 := payment.MissedVoidOutput()
	emvo, err2 := existing.MissedVoidOutput()
	if err := errors.Compose(err1, err2); err != nil {
		t.Fatal(err)
	}
	if !pmvo.Value.Equals(emvo.Value) {
		t.Fatal("Unexpected payout moved from renter to void")
	}
}
