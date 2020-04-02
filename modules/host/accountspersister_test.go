package host

import (
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestAccountsReload verifies that an account is properly saved to disk and
// gets reinstated properly when the host is reloaded
func TestAccountsReload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := blankHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	am := ht.host.staticAccountManager

	// Generate couple of accounts and deposit some coins into them
	accounts := make(map[modules.AccountID]types.Currency)
	var i uint64
	for i = 1; i <= 10; i++ {
		_, accountID := prepareAccount()
		expected := types.NewCurrency64(i)
		err = callDeposit(am, accountID, expected)
		if err != nil {
			t.Fatal(err)
		}
		actual := getAccountBalance(am, accountID)
		if !expected.Equals(actual) {
			t.Log("Expected:", expected.String())
			t.Log("Actual:", actual.String())
			t.Fatal("Deposit was unsuccessful")
		}
		accounts[accountID] = expected
	}

	// Reload the host
	err = reloadHost(ht)
	if err != nil {
		t.Fatal(err)
	}

	// Important, reload the accountmanager to avoid looking at old data
	am = ht.host.staticAccountManager

	// Verify the account balances were reloaded properly
	for id, expected := range accounts {
		reloaded := getAccountBalance(am, id)
		if !reloaded.Equals(expected) {
			t.Log("Expected:", expected.String())
			t.Log("Reloaded:", reloaded.String())
			t.Fatal("Balance after host reload did not equal the expected balance")
		}
	}
}

// TestFingerprintsReload verifies fingerprints are properly reloaded
func TestFingerprintsReload(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := blankHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	am := ht.host.staticAccountManager

	// Prepare an account
	sk, accountID := prepareAccount()
	err = callDeposit(am, accountID, types.NewCurrency64(10))
	if err != nil {
		t.Fatal(err)
	}

	// Prepare a withdrawal message
	amount := types.NewCurrency64(1)
	msg1, sig1 := prepareWithdrawal(accountID, amount, am.h.blockHeight+10, sk)
	err = callWithdraw(am, msg1, sig1)
	if err != nil {
		t.Fatal(err)
	}

	msg2, sig2 := prepareWithdrawal(accountID, amount, am.h.blockHeight+10, sk)
	err = callWithdraw(am, msg2, sig2)
	if err != nil {
		t.Fatal(err)
	}

	// Because fingerprints are enqueued to get persisted to disk, the
	// threadgroup wouldn't await them if we called close or flush. Sleep here
	// to allow some time for the fp to get persisted to disk.
	time.Sleep(time.Second)

	// Reload the host
	err = reloadHost(ht)
	if err != nil {
		t.Fatal(err)
	}

	// Important, reload the accountmanager to avoid looking at old data
	am = ht.host.staticAccountManager

	// Verify fingerprints got reloaded
	fp1 := crypto.HashObject(*msg1)
	exists := am.fingerprints.has(fp1)
	if !exists {
		t.Log(fp1)
		t.Error("Fingerprint 1 hash not found after reload")
	}
	fp2 := crypto.HashObject(*msg2)
	exists = am.fingerprints.has(fp2)
	if !exists {
		t.Log(fp2)
		t.Error("Fingerprint 2 hash not found after reload")
	}
}

// TestFingerprintsRotate will verify if mining blocks properly rotates the
// fingerprints, both in-memory and on-disk.
func TestFingerprintsRotate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := blankHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	am := ht.host.staticAccountManager

	// Unlock the wallet
	err = ht.wallet.Reset()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.initWallet()
	if err != nil {
		t.Fatal(err)
	}

	// Prepare account
	sk, accountID := prepareAccount()
	err = callDeposit(am, accountID, types.NewCurrency64(2))
	if err != nil {
		t.Fatal(err)
	}

	// Prepare 2 withdrawal messages, one that will end up in the current
	// bucket, and one that'll end up in the next fingerprints bucket
	cbh := ht.host.BlockHeight()
	msg1, sig1 := prepareWithdrawal(accountID, types.NewCurrency64(1), cbh+1, sk)
	msg2, sig2 := prepareWithdrawal(accountID, types.NewCurrency64(1), cbh+bucketBlockRange, sk)
	if err = errors.Compose(
		callWithdraw(am, msg1, sig1),
		callWithdraw(am, msg2, sig2),
	); err != nil {
		t.Fatal(err)
	}

	// Vefify we have the fingerprints in memory by perform the same withdrawal
	// and asserting ErrWithdrawalSpent
	if err = callWithdraw(am, msg1, sig1); !errors.Contains(err, ErrWithdrawalSpent) {
		t.Fatal("Unexpected error, expected ErrWithdrawalSpent but got:", err)
	}
	if err = callWithdraw(am, msg2, sig2); !errors.Contains(err, ErrWithdrawalSpent) {
		t.Fatal("Unexpected error, expected ErrWithdrawalSpent but got:", err)
	}

	// Because fingerprints are enqueued to get persisted to disk, the
	// threadgroup wouldn't await them if we called close or flush. Sleep here
	// to allow some time for the fp to get persisted to disk.
	time.Sleep(time.Second)

	// Verify we have the fingerprints on disk by using the persister
	data, err := am.staticAccountsPersister.callLoadData()
	if err != nil {
		t.Fatal(err)
	}
	fp1 := crypto.HashObject(*msg1)
	_, ok := data.fingerprints[fp1]
	if !ok {
		t.Fatal("Fingerprint of withdrawal msg 1 not found on disk")
	}
	fp2 := crypto.HashObject(*msg2)
	_, ok = data.fingerprints[fp2]
	if !ok {
		t.Fatal("Fingerprint of withdrawal msg 2 not found on disk")
	}

	// Mine blocks until we've reached the block height threshold at which the
	// fingerprints are expected to rotate
	numBlocks := bucketBlockRange
	for numBlocks > 0 {
		_, err = ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		numBlocks--
	}

	// Verify the fingerprint for withdrawal 1 is gone from memory. Verify the
	// fingerprint for withdrawal 2 moved to the current bucket
	am.mu.Lock()
	_, found1 := am.fingerprints.current[fp1]
	_, found2 := am.fingerprints.current[fp2]
	has1 := am.fingerprints.has(fp1)
	has2 := am.fingerprints.has(fp2)
	am.mu.Unlock()

	if found1 || has1 {
		t.Fatal("Fingerprint should have been removed from memory")
	}
	if !found2 {
		if has2 {
			t.Fatal("Fingerprint not in the correct bucket")
		} else {
			t.Fatal("Fingerprint not found in memory")
		}
	}

	// Verify the fingerprints got reloaded on disk as well
	data, err = am.staticAccountsPersister.callLoadData()
	if err != nil {
		t.Fatal(err)
	}
	_, has1 = data.fingerprints[fp1]
	_, has2 = data.fingerprints[fp2]
	if !(has1 == false && has2 == true) {
		t.Log("Found fp1", fp1, has1)
		t.Log("Found fp2", fp2, has2)
		t.Log(data)
		t.Fatal("Unexpected contents of fingerprint buckets on disk")
	}
}

// reloadHost will close the given host and reload it on the given host tester
func reloadHost(ht *hostTester) error {
	err := ht.host.Close()
	if err != nil {
		return err
	}

	host, err := New(ht.cs, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		return err
	}
	ht.host = host

	return nil
}

// getAccountBalance will return the balance for given account
func getAccountBalance(am *accountManager, id modules.AccountID) types.Currency {
	am.mu.Lock()
	defer am.mu.Unlock()

	acc, exists := am.accounts[id]
	if !exists {
		return types.ZeroCurrency
	}

	return acc.balance
}
