package renter

import (
	"sync"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/scpcorp/siamux"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

// withdrawalValidityPeriod defines the period (in blocks) a withdrawal message
// remains spendable after it has been created. Together with the current block
// height at time of creation, this period makes up the WithdrawalMessage's
// expiry height.
const withdrawalValidityPeriod = 6

type (
	// account represents a renter's ephemeral account on a host.
	account struct {
		// Information related to host communications.
		staticID        modules.AccountID
		staticHostKey   types.SiaPublicKey
		staticSecretKey crypto.SecretKey

		// Money has multiple states in an account, this is all the information
		// we need to understand the current state of the account's balance and
		// pending updates.
		balance            types.Currency
		negativeBalance    types.Currency
		pendingWithdrawals types.Currency
		pendingDeposits    types.Currency

		// Error handling and cooldown tracking.
		consecutiveFailures uint64
		cooldownUntil       time.Time
		recentErr           error

		// Variables to manage a race condition around account creation, where
		// the account must be available in the data structure before it has
		// been synced to disk successfully (to avoid holding a lock on the
		// account manager during a disk fsync). Anyone trying to use the
		// account will need to block on 'staticReady', and then after that is
		// closed needs to check the status of 'externActive', 'false'
		// indicating that account creation failed and the account was deleted.
		//
		// 'externActive' can be accessed freely once 'staticReady' has been
		// closed.
		staticReady  chan struct{}
		externActive bool

		// Utils. The offset refers to the offset within the file that the
		// account uses.
		mu           sync.Mutex
		staticFile   modules.File
		staticOffset int64
		staticRenter *Renter
	}
)

// ProvidePayment takes a stream and various payment details and handles the
// payment by sending and processing payment request and response objects.
// Returns an error in case of failure.
func (a *account) ProvidePayment(stream siamux.Stream, host types.SiaPublicKey, rpc types.Specifier, amount types.Currency, refundAccount modules.AccountID, blockHeight types.BlockHeight) error {
	if rpc == modules.RPCFundAccount && !refundAccount.IsZeroAccount() {
		return errors.New("Refund account is expected to be the zero account when funding an ephemeral account")
	}
	// NOTE: we purposefully do not verify if the account has sufficient funds.
	// Seeing as withdrawals are a blocking action on the host, it is perfectly
	// ok to trigger them from an account with insufficient balance.

	// create a withdrawal message
	msg := newWithdrawalMessage(a.staticID, amount, blockHeight)
	sig := crypto.SignHash(crypto.HashObject(msg), a.staticSecretKey)

	// send PaymentRequest
	err := modules.RPCWrite(stream, modules.PaymentRequest{Type: modules.PayByEphemeralAccount})
	if err != nil {
		return err
	}

	// send PayByEphemeralAccountRequest
	err = modules.RPCWrite(stream, modules.PayByEphemeralAccountRequest{
		Message:   msg,
		Signature: sig,
	})
	if err != nil {
		return err
	}
	return nil
}

// availableBalance returns the amount of money that is available to
// spend. It is calculated by taking into account pending spends and pending
// funds.
func (a *account) availableBalance() types.Currency {
	total := a.balance.Add(a.pendingDeposits)
	if total.Cmp(a.negativeBalance) <= 0 {
		return types.ZeroCurrency
	}
	total = total.Sub(a.negativeBalance)
	if a.pendingWithdrawals.Cmp(total) < 0 {
		return total.Sub(a.pendingWithdrawals)
	}
	return types.ZeroCurrency
}

// managedAvailableBalance returns the amount of money that is available to
// spend. It is calculated by taking into account pending spends and pending
// funds.
func (a *account) managedAvailableBalance() types.Currency {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.availableBalance()
}

// managedCommitDeposit commits a pending deposit, either after success or
// failure. Depending on the outcome the given amount will be added to the
// balance or not. If the pending delta is zero, and we altered the account
// balance, we update the account.
func (a *account) managedCommitDeposit(amount types.Currency, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// (no need to sanity check - the implementation of 'Sub' does this for us)
	a.pendingDeposits = a.pendingDeposits.Sub(amount)

	// reflect the successful deposit in the balance field
	if success {
		if amount.Cmp(a.negativeBalance) <= 0 {
			a.negativeBalance = a.negativeBalance.Sub(amount)
		} else {
			amount = amount.Sub(a.negativeBalance)
			a.negativeBalance = types.ZeroCurrency
			a.balance = a.balance.Add(amount)
		}
	}
}

// managedCommitWithdrawal commits a pending withdrawal, either after success or
// failure. Depending on the outcome the given amount will be deducted from the
// balance or not. If the pending delta is zero, and we altered the account
// balance, we update the account.
func (a *account) managedCommitWithdrawal(amount types.Currency, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// (no need to sanity check - the implementation of 'Sub' does this for us)
	a.pendingWithdrawals = a.pendingWithdrawals.Sub(amount)

	// reflect the successful withdrawal in the balance field
	if success {
		if a.balance.Cmp(amount) >= 0 {
			a.balance = a.balance.Sub(amount)
		} else {
			amount = amount.Sub(a.balance)
			a.balance = types.ZeroCurrency
			a.negativeBalance = a.negativeBalance.Add(amount)
		}
	}
}

// managedTrackDeposit keeps track of pending deposits by adding the given
// amount to the 'pendingDeposits' field.
func (a *account) managedTrackDeposit(amount types.Currency) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingDeposits = a.pendingDeposits.Add(amount)
}

// managedTrackWithdrawal keeps track of pending withdrawals by adding the given
// amount to the 'pendingWithdrawals' field.
func (a *account) managedTrackWithdrawal(amount types.Currency) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pendingWithdrawals = a.pendingWithdrawals.Add(amount)
}

// newWithdrawalMessage is a helper function that takes a set of parameters and
// a returns a new WithdrawalMessage.
func newWithdrawalMessage(id modules.AccountID, amount types.Currency, blockHeight types.BlockHeight) modules.WithdrawalMessage {
	expiry := blockHeight + withdrawalValidityPeriod
	var nonce [modules.WithdrawalNonceSize]byte
	fastrand.Read(nonce[:])
	return modules.WithdrawalMessage{
		Account: id,
		Expiry:  expiry,
		Amount:  amount,
		Nonce:   nonce,
	}
}

// managedAccountNeedsRefill will check whether the worker's account needs to be
// refilled. This function will return false if any conditions are met which
// are likely to prevent the refill from being successful.
func (w *worker) managedAccountNeedsRefill() bool {
	// Check if the host version is compatible with accounts.
	cache := w.staticCache()
	if build.VersionCmp(cache.staticHostVersion, minAsyncVersion) < 0 {
		return false
	}
	/* - will be enabled when more of the worker code is in place.
	// Check if the price table is valid.
	if !w.staticPriceTable().staticValid() {
		return false
	}
	*/

	// Check if there is a cooldown in place, and check if the balance is low
	// enough to justify a refill.
	w.staticAccount.mu.Lock()
	cooldownUntil := w.staticAccount.cooldownUntil
	balance := w.staticAccount.availableBalance()
	w.staticAccount.mu.Unlock()
	if time.Now().Before(cooldownUntil) {
		return false
	}
	refillAt := w.staticBalanceTarget.Div64(2)
	if balance.Cmp(refillAt) >= 0 {
		return false
	}

	// A refill is needed.
	return true
}

/* - will be enabled once more of the worker code is in place.
// managedTryRefillAccount will check if the account needs to be refilled
func (w *worker) managedRefillAccount() {
	// the account balance dropped to below half the balance target, refill
	balance := w.staticAccount.managedAvailableBalance()
	amount := w.staticBalanceTarget.Sub(balance)

	// We track that there is a deposit in progress. Because filling an account
	// is an interactive protocol with another machine, we are never sure of the
	// exact moment that the deposit has reached our account. Instead, we track
	// the deposit as a "maybe" until we know for sure that the deposit has
	// either reached the remove machine or failed.
	//
	// At the same time that we track the deposit, we defer a function to check
	// the error on the deposit
	w.staticAccount.managedTrackDeposit(amount)
	var err error
	defer func() {
		// If there was no error, the account should now be full, and will not
		// need to be refilled until the worker has spent up the funds in the
		// account.
		w.staticAccount.managedCommitDeposit(amount, err == nil)
		if err == nil {
			return
		}

		// If the error is not nil, increment the cooldown.
		w.staticAccount.mu.Lock()
		cd := cooldownUntil(w.staticAccount.consecutiveFailures)
		w.staticAccount.cooldownUntil = cd
		w.staticAccount.consecutiveFailures++
		w.staticAccount.recentErr = err
		w.staticAccount.mu.Unlock()

		// Have the threadgroup wake the worker when the account comes off of
		// cooldown.
		w.renter.tg.AfterFunc(cd.Sub(time.Now()), func() {
			w.staticWake()
		})
	}()

	// create a new stream
	var stream siamux.Stream
	stream, err = w.staticNewStream()
	if err != nil {
		err = errors.AddContext(err, "Unable to create a new stream")
		return
	}
	defer func() {
		closeErr := stream.Close()
		if closeErr != nil {
			w.renter.log.Println("ERROR: failed to close stream", closeErr)
		}
	}()

	// write the specifier
	err = modules.RPCWrite(stream, modules.RPCFundAccount)
	if err != nil {
		err = errors.AddContext(err, "could not write fund account specifier")
		return
	}

	// send price table uid
	pt := w.staticPriceTable().staticPriceTable
	err = modules.RPCWrite(stream, pt.UID)
	if err != nil {
		err = errors.AddContext(err, "could not write price table uid")
		return
	}

	// send fund account request
	err = modules.RPCWrite(stream, modules.FundAccountRequest{Account: w.staticAccount.staticID})
	if err != nil {
		err = errors.AddContext(err, "could not write the fund account request")
		return
	}

	// provide payment
	err = w.renter.hostContractor.ProvidePayment(stream, w.staticHostPubKey, modules.RPCFundAccount, amount.Add(pt.FundAccountCost), modules.ZeroAccountID, w.staticCache().staticBlockHeight)
	if err != nil {
		err = errors.AddContext(err, "could not provide payment for the account")
		return
	}

	// receive FundAccountResponse. The response contains a receipt and a
	// signature, which is useful for places where accountability is required,
	// but no accountability is required in this case, so we ignore the
	// response.
	var resp modules.FundAccountResponse
	err = modules.RPCRead(stream, &resp)
	err = errors.AddContext(err, "could not read the account response")
	return
}
*/
