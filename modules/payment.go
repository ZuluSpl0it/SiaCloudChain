package modules

import (
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/siamux"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// WithdrawalNonceSize is the size of the nonce in the WithdralMessage
	WithdrawalNonceSize = 8
)

var (
	// ErrUnknownPaymentMethod occurs when the payment method specified in the
	// PaymentRequest object is unknown. The possible options are outlined below
	// under "Payment identifiers".
	ErrUnknownPaymentMethod = errors.New("unknown payment method")

	// ErrInvalidPaymentMethod occurs when the payment method is not accepted
	// for a specific RPC.
	ErrInvalidPaymentMethod = errors.New("invalid payment method")

	// ErrInsufficientPaymentForRPC is returned when the provided payment was
	// lower than the cost of the RPC.
	ErrInsufficientPaymentForRPC = errors.New("Insufficient payment, the provided payment did not cover the cost of the RPC.")

	// ErrExpiredRPCPriceTable is returned when the renter performs an RPC call
	// and the current block height exceeds the expiry block height of the RPC
	// price table.
	ErrExpiredRPCPriceTable = errors.New("Expired RPC price table, ensure you have the latest prices by calling the updatePriceTable RPC.")

	// ErrWithdrawalsInactive occurs when the host is not synced yet. If that is
	// the case the account manager does not allow trading money from the
	// ephemeral accounts.
	ErrWithdrawalsInactive = errors.New("ephemeral account withdrawals are inactive because the host is not synced")

	// ErrWithdrawalExpired occurs when the withdrawal message's expiry block
	// height is in the past.
	ErrWithdrawalExpired = errors.New("ephemeral account withdrawal message expired")

	// ErrWithdrawalExtremeFuture occurs when the withdrawal message's expiry
	// block height is too far into the future.
	ErrWithdrawalExtremeFuture = errors.New("ephemeral account withdrawal message expires too far into the future")

	// ErrWithdrawalInvalidSignature occurs when the signature provided with the
	// withdrawal message was invalid.
	ErrWithdrawalInvalidSignature = errors.New("ephemeral account withdrawal message signature is invalid")
)

// PaymentProcessor is the interface implemented when receiving payment for an
// RPC.
type PaymentProcessor interface {
	// ProcessPayment takes a stream and handles the payment request objects
	// sent by the caller. Returns an object that implements the PaymentDetails
	// interface, or an error in case of failure.
	ProcessPayment(stream siamux.Stream) (PaymentDetails, error)
}

// PaymentDetails is an interface that defines method that give more information
// about the details of a processed payment.
type PaymentDetails interface {
	AccountID() AccountID
	Amount() types.Currency
	AddedCollateral() types.Currency
}

// Payment identifiers
var (
	PayByContract         = types.NewSpecifier("PayByContract")
	PayByEphemeralAccount = types.NewSpecifier("PayByEphemAcc")
)

type (
	// AccountID is the unique identifier of an ephemeral account on the host.
	// It is the string representation of a SiaPublicKey.
	AccountID string

	// PaymentRequest identifies the payment method. This can be either
	// PayByContract or PayByEphemeralAccount
	PaymentRequest struct {
		Type types.Specifier
	}

	// PayByEphemeralAccountRequest holds all payment details to pay from an
	// ephemeral account.
	PayByEphemeralAccountRequest struct {
		Message   WithdrawalMessage
		Signature crypto.Signature
		Priority  int64
	}

	// PayByEphemeralAccountResponse is the object sent in response to the
	// PayByEphemeralAccountRequest
	PayByEphemeralAccountResponse struct {
		Amount types.Currency
	}

	// PayByContractRequest holds all payment details to pay from a file
	// contract.
	PayByContractRequest struct {
		ContractID           types.FileContractID
		NewRevisionNumber    uint64
		NewValidProofValues  []types.Currency
		NewMissedProofValues []types.Currency
		Signature            []byte
	}

	// PayByContractResponse is the object sent in response to the
	// PayByContractRequest
	PayByContractResponse struct {
		Signature crypto.Signature
	}

	// WithdrawalMessage contains all details to spend from an ephemeral account
	WithdrawalMessage struct {
		Account AccountID
		Expiry  types.BlockHeight
		Amount  types.Currency
		Nonce   [WithdrawalNonceSize]byte
	}

	// Receipt is returned by the host after a successful deposit into an
	// ephemeral account and can be used as proof of payment.
	Receipt struct {
		Host      types.SiaPublicKey
		Account   AccountID
		Amount    types.Currency
		Timestamp int64
	}
)

// Validate checks the WithdrawalMessage's expiry and signature. If the
// signature is invalid, or if the WithdrawlMessage is already expired, or it
// expires too far into the future, an error is returned.
func (wm *WithdrawalMessage) Validate(blockHeight, expiry types.BlockHeight, hash crypto.Hash, sig crypto.Signature) error {
	return errors.Compose(
		wm.ValidateExpiry(blockHeight, expiry),
		wm.ValidateSignature(hash, sig),
	)
}

// ValidateExpiry returns an error if the withdrawal message is either already
// expired or if it expires too far into the future
func (wm *WithdrawalMessage) ValidateExpiry(blockHeight, expiry types.BlockHeight) error {
	// Verify the current blockheight does not exceed the expiry
	if blockHeight > wm.Expiry {
		return ErrWithdrawalExpired
	}
	// Verify the withdrawal is not too far into the future
	if wm.Expiry > expiry {
		return ErrWithdrawalExtremeFuture
	}
	return nil
}

// ValidateSignature returns an error if the provided signature is invalid
func (wm *WithdrawalMessage) ValidateSignature(hash crypto.Hash, sig crypto.Signature) error {
	var spk types.SiaPublicKey
	spk.LoadString(string(wm.Account))
	var pk crypto.PublicKey
	copy(pk[:], spk.Key)

	err := crypto.VerifyHash(hash, pk, sig)
	if err != nil {
		return errors.Extend(err, ErrWithdrawalInvalidSignature)
	}
	return nil
}
