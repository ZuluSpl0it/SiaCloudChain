package transactionpool

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/persist"
	"gitlab.com/SiaPrime/SiaPrime/types"

	bolt "github.com/coreos/bbolt"
)

// TestRescan triggers a rescan in the transaction pool, verifying that the
// rescan code does not cause deadlocks or crashes.
func TestRescan(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	tpt, err := createTpoolTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tpt.Close()

	// Create a valid transaction set using the wallet.
	txns, err := tpt.wallet.SendSiacoins(types.NewCurrency64(100), types.UnlockHash{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tpt.tpool.transactionSets) != 1 {
		t.Error("sending coins did not increase the transaction sets by 1")
	}
	// Mine the transaction into a block, so that it's in the consensus set.
	_, err = tpt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Close the tpool, delete the persistence, then restart the tpool. The
	// tpool should still recognize the transaction set as a duplicate.
	persistDir := tpt.tpool.persistDir
	err = tpt.tpool.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll(persistDir)
	if err != nil {
		t.Fatal(err)
	}
	tpt.tpool, err = New(tpt.cs, tpt.gateway, persistDir)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, 100*time.Millisecond, func() error {
		err := tpt.tpool.AcceptTransactionSet(txns)
		if err != modules.ErrDuplicateTransactionSet {
			return fmt.Errorf("expecting modules.ErrDuplicateTransactionSet, got: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Close the tpool, corrupt the database, then restart the tpool. The tpool
	// should still recognize the transaction set as a duplicate.
	err = tpt.tpool.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err := persist.OpenDatabase(dbMetadata, filepath.Join(persistDir, dbFilename))
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		ccBytes := tx.Bucket(bucketRecentConsensusChange).Get(fieldRecentConsensusChange)
		// copy the bytes due to bolt's mmap.
		newCCBytes := make([]byte, len(ccBytes))
		copy(newCCBytes, ccBytes)
		newCCBytes[0]++
		return tx.Bucket(bucketRecentConsensusChange).Put(fieldRecentConsensusChange, newCCBytes)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	tpt.tpool, err = New(tpt.cs, tpt.gateway, persistDir)
	if err != nil {
		t.Fatal(err)
	}
	err = tpt.tpool.AcceptTransactionSet(txns)
	if err != modules.ErrDuplicateTransactionSet {
		t.Fatal("expecting modules.ErrDuplicateTransactionSet, got:", err)
	}
}
