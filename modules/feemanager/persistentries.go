package feemanager

import (
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/NebulousLabs/errors"
)

const (
	// persistEntrySize is the size of a persist entry in the persist file.
	persistEntrySize = 256

	// persistEntryPayloadSize is the size of a persist entry minus the size of
	// a types.Specifier.
	persistEntryPayloadSize = 240
)

var (
	// Persist entry types
	entryTypeAddFee    = types.NewSpecifier("add fee")
	entryTypeCancelFee = types.NewSpecifier("cancel fee")
	entryTypeUpdateFee = types.NewSpecifier("update fee")

	// errUnrecognizedEntryType is returned if the FeeManager tries to apply an
	// unrecognized entry type
	errUnrecognizedEntryType = errors.New("unrecognized entry type")
)

type (
	// entryAddFee is the persist entry that adds a new fee.
	entryAddFee struct {
		Fee modules.AppFee
	}

	// entryCancelFee is the persist entry that cancels a pending fee.
	entryCancelFee struct {
		FeeUID    modules.FeeUID
		Timestamp int64
	}

	// entryupdateFee is the persist entry that updates a pending fee's payout
	// height.
	entryupdateFee struct {
		FeeUID       modules.FeeUID
		PayoutHeight types.BlockHeight
	}

	// persistEntry is a generic entry in the persist database.
	persistEntry struct {
		EntryType types.Specifier
		Payload   [persistEntryPayloadSize]byte
	}
)

// createAddFeeEntry will create a persist entry for an add fee request.
func createAddFeeEntry(fee modules.AppFee) (ret [persistEntrySize]byte) {
	// Create the add fee entry and marshal it.
	eaf := entryAddFee{
		Fee: fee,
	}
	payload := encoding.Marshal(eaf)

	// Load the marshalled entry into the generic entry.
	entry := persistEntry{
		EntryType: entryTypeAddFee,
	}
	copy(entry.Payload[:], payload)

	// Encode the generic entry and check a size invariant.
	encodedEntry := encoding.Marshal(entry)
	if len(encodedEntry) != persistEntrySize {
		build.Critical("an encoded entry has the wrong size")
	}

	// Set the return value and return.
	copy(ret[:], encodedEntry)
	return
}

// createCancelFeeEntry will take a feeUID and create a persist entry for a
// cancel fee request.
func createCancelFeeEntry(feeUID modules.FeeUID) (ret [persistEntrySize]byte) {
	// Create the cancel fee entry and marshal it.
	ecf := entryCancelFee{
		FeeUID:    feeUID,
		Timestamp: time.Now().Unix(),
	}
	payload := encoding.Marshal(ecf)

	// Load the marshalled entry into the generic entry.
	entry := persistEntry{
		EntryType: entryTypeCancelFee,
	}
	copy(entry.Payload[:], payload)

	// Encode the generic entry and check a size invariant.
	encodedEntry := encoding.Marshal(entry)
	if len(encodedEntry) != persistEntrySize {
		build.Critical("an encoded entry has the wrong size")
	}

	// Set the return value and return.
	copy(ret[:], encodedEntry)
	return
}

// createUpdateFeeEntry will create a persist entry for an update fee request.
func createUpdateFeeEntry(feeUID modules.FeeUID, payoutHeight types.BlockHeight) (ret [persistEntrySize]byte) {
	// Create the cancel fee entry and marshal it.
	euf := entryupdateFee{
		FeeUID:       feeUID,
		PayoutHeight: payoutHeight,
	}
	payload := encoding.Marshal(euf)

	// Load the marshalled entry into the generic entry.
	entry := persistEntry{
		EntryType: entryTypeUpdateFee,
	}
	copy(entry.Payload[:], payload)

	// Encode the generic entry and check a size invariant.
	encodedEntry := encoding.Marshal(entry)
	if len(encodedEntry) != persistEntrySize {
		build.Critical("an encoded entry has the wrong size")
	}

	// Set the return value and return.
	copy(ret[:], encodedEntry)
	return
}

// applyEntry will apply the provided entry to the fee manager.
func (fm *FeeManager) applyEntry(entry []byte) error {
	var pe persistEntry
	err := encoding.Unmarshal(entry, &pe)
	if err != nil {
		return errors.AddContext(err, "could not unmarshal generic entry")
	}
	switch pe.EntryType {
	case entryTypeAddFee:
		return fm.applyEntryAddFee(pe.Payload)
	case entryTypeCancelFee:
		return fm.applyEntryCancelFee(pe.Payload)
	case entryTypeUpdateFee:
		return fm.applyEntryUpdateFee(pe.Payload)
	}
	return errUnrecognizedEntryType
}

// applyEntryAddFee will apply an add fee entry to the fee manager.
func (fm *FeeManager) applyEntryAddFee(payload [persistEntryPayloadSize]byte) error {
	var eaf entryAddFee
	err := encoding.Unmarshal(payload[:], &eaf)
	if err != nil {
		return errors.AddContext(err, "could not unmarshal add fee entry payload")
	}
	fm.fees[eaf.Fee.FeeUID] = &eaf.Fee
	return nil
}

// applyEntryCancelFee will apply a cancel fee entry to the fee manager.
func (fm *FeeManager) applyEntryCancelFee(payload [persistEntryPayloadSize]byte) error {
	var ecf entryCancelFee
	err := encoding.Unmarshal(payload[:], &ecf)
	if err != nil {
		return errors.AddContext(err, "could not unmarshal cancel fee entry payload")
	}
	delete(fm.fees, ecf.FeeUID)
	return nil
}

// applyEntryUpdateFee will apply an update fee entry to the fee manager.
func (fm *FeeManager) applyEntryUpdateFee(payload [persistEntryPayloadSize]byte) error {
	var euf entryupdateFee
	err := encoding.Unmarshal(payload[:], &euf)
	if err != nil {
		return errors.AddContext(err, "could not unmarshal update fee entry payload")
	}
	fee, ok := fm.fees[euf.FeeUID]
	if !ok {
		return errors.New("Fee Update found for non existent or cancelled fee")
	}
	fee.PayoutHeight = euf.PayoutHeight
	return nil
}
