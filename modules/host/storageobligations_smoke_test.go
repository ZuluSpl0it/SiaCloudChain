package host

// storageobligations_smoke_test.go performs smoke testing on the the storage
// obligation management. This includes adding valid storage obligations, and
// waiting until they expire, to see if the failure modes are all handled
// correctly.

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	bolt "go.etcd.io/bbolt"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newTestTPool returns a tpool with custom dependencies for testing
func newTestTPool(name string, deps modules.Dependencies) (func() error, *transactionpool.TransactionPool, error) {
	testdir := build.TempDir(modules.HostDir, name)
	// Create the modules needed.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, nil, err
	}
	// Create the tpool.
	tp, err := transactionpool.NewCustomTPool(cs, g, filepath.Join(testdir, modules.TransactionPoolDir), deps)
	if err != nil {
		return nil, nil, err
	}
	closefn := func() error {
		return errors.Compose(tp.Close(), cs.Close(), g.Close())
	}
	return closefn, tp, nil
}

// randSector creates a random sector, returning the sector along with the
// Merkle root of the sector.
func randSector() (crypto.Hash, []byte) {
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)
	return sectorRoot, sectorData
}

// newTesterStorageObligation uses the wallet to create and fund a file
// contract that will form the foundation of a storage obligation.
func (ht *hostTester) newTesterStorageObligation() (storageObligation, error) {
	// Create the file contract that will be used in the obligation.
	builder, err := ht.wallet.StartTransaction()
	if err != nil {
		return storageObligation{}, err
	}
	// Fund the file contract with a payout. The payout needs to be big enough
	// that the expected revenue is larger than the fee that the host may end
	// up paying.
	uc, err := ht.wallet.UnlockConditions(ht.host.unlockHash)
	if err != nil {
		builder.Drop()
		return storageObligation{}, errors.AddContext(err, "Can not get host unlockhash")
	}
	payout := types.ScPrimecoinPrecision.Mul64(10e3)
	collateral := types.ScPrimecoinPrecision.Mul64(10)
	err = builder.FundSiacoinsFixedAddress(payout, uc, uc)
	if err != nil {
		builder.Drop()
		return storageObligation{}, errors.AddContext(err, "Unable to fund storage obligation")
	}
	// Add the file contract that consumes the funds.
	_ = builder.AddFileContract(types.FileContract{
		// Because this file contract needs to be able to accept file contract
		// revisions, the expiration is put more than
		// 'revisionSubmissionBuffer' blocks into the future.
		WindowStart: ht.host.blockHeight + revisionSubmissionBuffer + 2,
		WindowEnd:   ht.host.blockHeight + revisionSubmissionBuffer + modules.DefaultWindowSize + 2,

		Payout: payout,
		ValidProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout).Sub(collateral),
			},
			{
				Value: collateral,
			},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			{
				Value: types.PostTax(ht.host.blockHeight, payout).Sub(collateral),
			},
			{
				Value: collateral,
			},
			{
				Value: types.ZeroCurrency,
			},
		},
		UnlockHash:     (types.UnlockConditions{}).UnlockHash(),
		RevisionNumber: 0,
	})
	// Sign the transaction.
	tSet, err := builder.Sign(true)
	if err != nil {
		return storageObligation{}, err
	}

	// Assemble and return the storage obligation.
	so := storageObligation{
		OriginTransactionSet: tSet,

		h: ht.host,
	}
	return so, nil
}

// TestBlankStorageObligation checks that the host correctly manages a blank
// storage obligation.
func TestBlankStorageObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestBlankStorageObligation")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// The number of contracts reported by the host should be zero.
	fm := ht.host.FinancialMetrics()
	if fm.ContractCount != 0 {
		t.Error("host does not start with 0 contracts:", fm.ContractCount)
	}

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}
	fm = ht.host.FinancialMetrics()
	if fm.ContractCount != 1 {
		t.Error("host should have 1 contract:", fm.ContractCount)
	}
	// Mine a block to confirm the transaction containing the storage
	// obligation.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}

	// Mine until the host would be submitting a storage proof. Check that the
	// host has cleared out the storage proof - the consensus code makes it
	// impossible to submit a storage proof for an empty file contract, so the
	// host should fail and give up by deleting the storage obligation.
	for i := types.BlockHeight(0); i <= revisionSubmissionBuffer*2+1; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = ht.host.tg.Flush()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(100, 100*time.Millisecond, func() error {
		fm = ht.host.FinancialMetrics()
		if fm.ContractCount != 0 {
			return fmt.Errorf("host should have 0 contracts, the contracts were all completed: %v", fm.ContractCount)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestSingleSectorObligationStack checks that the host correctly manages a
// storage obligation with a single sector, the revision is created the same
// block as the file contract.
func TestSingleSectorStorageObligationStack(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestSingleSectorStorageObligationStack")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.ScPrimecoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.mu.Lock()
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
	ht.host.mu.Unlock()
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 2,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	so.RevisionTransactionSet = revisionSet
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedModifyStorageObligation(so, nil, map[crypto.Hash][]byte{sectorRoot: sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to confirm the transactions containing the file contract
	// and the file contract revision.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = build.Retry(100, 100*time.Millisecond, func() error {
		ht.host.mu.Lock()
		err := ht.host.db.View(func(tx *bolt.Tx) error {
			so, err = ht.host.getStorageObligation(tx, so.id())
			if err != nil {
				return err
			}
			return nil
		})
		ht.host.mu.Unlock()
		if err != nil {
			return err
		}
		if !so.OriginConfirmed {
			return errors.New("origin transaction for storage obligation was not confirmed after a block was mined")
		}
		if !so.RevisionConfirmed {
			return errors.New("revision transaction for storage obligation was not confirmed after a block was mined")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine until the host submits a storage proof.
	ht.host.mu.Lock()
	bh := ht.host.blockHeight
	ht.host.mu.Unlock()
	for i := bh; i < so.expiration()+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Need Sleep for online CI, otherwise threadedHandleActionItem thread group
	// is not added in time and Flush() does not block
	time.Sleep(time.Second)

	// Flush the host - flush will block until the host has submitted the
	// storage proof to the transaction pool.
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Mine another block, to get the storage proof from the transaction pool
	// into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Grab the storage proof and inspect the contents.
	ht.host.mu.Lock()
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.ProofConfirmed {
		t.Fatal("storage obligation is not saying that the storage proof was confirmed on the blockchain")
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will finalize the obligation.
	for i := 0; i <= int(modules.DefaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	ht.host.mu.Lock()
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared when the host finalized the obligation")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("obligation is not being reported as successful:", so.ObligationStatus)
		}
		return nil
	})
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	storageRevenue := ht.host.financialMetrics.StorageRevenue
	ht.host.mu.Unlock()
	if !storageRevenue.Equals(sectorCost) {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestMultiSectorObligationStack checks that the host correctly manages a
// storage obligation with a single sector, the revision is created the same
// block as the file contract.
//
// Unlike the SingleSector test, the multi sector test attempts to spread file
// contract revisions over multiple blocks.
func TestMultiSectorStorageObligationStack(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestMultiSectorStorageObligationStack")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}
	// Deviation from SingleSector test - mine a block here to confirm the
	// storage obligation before a file contract revision is created.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	ht.host.mu.Lock()
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.ScPrimecoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.mu.Lock()
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
	ht.host.mu.Unlock()
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 2,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	so.RevisionTransactionSet = revisionSet
	ht.host.managedLockStorageObligation(so.id())

	err = ht.host.managedModifyStorageObligation(so, nil, map[crypto.Hash][]byte{sectorRoot: sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet)
	if err != nil {
		t.Fatal(err)
	}

	// Create a second file contract revision, which is going to be submitted
	// to the transaction pool after the first revision. Though, in practice
	// this should never happen, we want to check that the transaction pool is
	// correctly handling multiple file contract revisions being submitted in
	// the same block cycle. This test will additionally tell us whether or not
	// the host can correctly handle building storage proofs for files with
	// multiple sectors.
	sectorRoot2, sectorData2 := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot, sectorRoot2}
	sectorCost2 := types.ScPrimecoinPrecision.Mul64(650)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost2)
	ht.host.mu.Lock()
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost2)
	ht.host.mu.Unlock()
	validPayouts, missedPayouts = so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost2)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost2)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost2)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost2)
	combinedSectors := append(sectorData, sectorData2...)
	combinedRoot := crypto.MerkleRoot(combinedSectors)
	revisionSet2 := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 3,

			NewFileSize:           uint64(len(sectorData) + len(sectorData2)),
			NewFileMerkleRoot:     combinedRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedModifyStorageObligation(so, nil, map[crypto.Hash][]byte{sectorRoot2: sectorData2})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Submit the revision set to the transaction pool.
	err = ht.tpool.AcceptTransactionSet(revisionSet2)
	if err != nil {
		t.Fatal(err)
	}

	// Mine a block to confirm the transactions containing the file contract
	// and the file contract revision.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	// Load the storage obligation from the database, see if it updated
	// correctly.
	err = build.Retry(100, 100*time.Millisecond, func() error {
		ht.host.mu.Lock()
		err := ht.host.db.View(func(tx *bolt.Tx) error {
			so, err = ht.host.getStorageObligation(tx, so.id())
			if err != nil {
				return err
			}
			return nil
		})
		ht.host.mu.Unlock()
		if err != nil {
			return err
		}
		if !so.OriginConfirmed {
			return errors.New("origin transaction for storage obligation was not confirmed after a block was mined")
		}
		if !so.RevisionConfirmed {
			return errors.New("revision transaction for storage obligation was not confirmed after a block was mined")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine until the host submits a storage proof.
	ht.host.mu.Lock()
	bh := ht.host.blockHeight
	ht.host.mu.Unlock()
	for i := bh; i < so.expiration()+resubmissionTimeout; i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Need Sleep for online CI, otherwise threadedHandleActionItem thread group
	// is not added in time and Flush() does not block
	time.Sleep(time.Second)

	// Flush the host - flush will block until the host has submitted the
	// storage proof to the transaction pool.
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}

	// Mine another block, to get the storage proof from the transaction pool
	// into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.mu.Lock()
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		return nil
	})
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !so.OriginConfirmed {
		t.Fatal("origin transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.RevisionConfirmed {
		t.Fatal("revision transaction for storage obligation was not confirmed after a block was mined")
	}
	if !so.ProofConfirmed {
		t.Fatal("storage obligation is not saying that the storage proof was confirmed on the blockchain")
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= int(modules.DefaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	ht.host.mu.Lock()
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared out when the storage proof was finalized")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("storage obligation was not reported as a success")
		}
		return nil
	})
	ht.host.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !ht.host.financialMetrics.StorageRevenue.Equals(sectorCost.Add(sectorCost2)) {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestAutoRevisionSubmission checks that the host correctly submits a file
// contract revision to the consensus set.
func TestAutoRevisionSubmission(t *testing.T) {
	if testing.Short() || !build.VLONG {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestAutoRevisionSubmission")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Start by adding a storage obligation to the host. To emulate conditions
	// of a renter creating the first contract, the storage obligation has no
	// data, but does have money.
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	// Storage obligation should not be marked as having the transaction
	// confirmed on the blockchain.
	if so.OriginConfirmed {
		t.Fatal("storage obligation should not yet be marked as confirmed, confirmation is on the way")
	}

	// Add a file contract revision, moving over a small amount of money to pay
	// for the file contract.
	sectorRoot, sectorData := randSector()
	so.SectorRoots = []crypto.Hash{sectorRoot}
	sectorCost := types.ScPrimecoinPrecision.Mul64(550)
	so.PotentialStorageRevenue = so.PotentialStorageRevenue.Add(sectorCost)
	ht.host.financialMetrics.PotentialStorageRevenue = ht.host.financialMetrics.PotentialStorageRevenue.Add(sectorCost)
	validPayouts, missedPayouts := so.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(sectorCost)
	validPayouts[1].Value = validPayouts[1].Value.Add(sectorCost)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(sectorCost)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(sectorCost)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 1,

			NewFileSize:           uint64(len(sectorData)),
			NewFileMerkleRoot:     sectorRoot,
			NewWindowStart:        so.expiration(),
			NewWindowEnd:          so.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	so.RevisionTransactionSet = revisionSet
	ht.host.managedLockStorageObligation(so.id())

	err = ht.host.managedModifyStorageObligation(so, nil, map[crypto.Hash][]byte{sectorRoot: sectorData})
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so.id())
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}
	// Unlike the other tests, this test does not submit the file contract
	// revision to the transaction pool for the host, the host is expected to
	// do it automatically.
	count := 0
	err = build.Retry(500, 100*time.Millisecond, func() error {
		// Mine another block every 10 iterations, to get the storage proof from
		// the transaction pool into the blockchain.
		if count%10 == 0 {
			_, err = ht.miner.AddBlock()
			if err != nil {
				t.Fatal(err)
			}
			err = ht.host.tg.Flush()
			if err != nil {
				t.Fatal(err)
			}
		}
		count++
		err = ht.host.db.View(func(tx *bolt.Tx) error {
			so, err = ht.host.getStorageObligation(tx, so.id())
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return (err)
		}
		if !so.OriginConfirmed {
			return errors.New("origin transaction for storage obligation was not confirmed after blocks were mined")
		}
		if !so.RevisionConfirmed {
			return errors.New("revision transaction for storage obligation was not confirmed after blocks were mined")
		}
		if !so.ProofConfirmed {
			return errors.New("storage obligation is not saying that the storage proof was confirmed on the blockchain")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks until the storage proof has enough confirmations that the
	// host will delete the file entirely.
	for i := 0; i <= int(modules.DefaultWindowSize); i++ {
		_, err := ht.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
		err = ht.host.tg.Flush()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = ht.host.db.View(func(tx *bolt.Tx) error {
		so, err = ht.host.getStorageObligation(tx, so.id())
		if err != nil {
			return err
		}
		if so.SectorRoots != nil {
			t.Error("sector roots were not cleared out when the storage proof was finalized")
		}
		if so.ObligationStatus != obligationSucceeded {
			t.Error("storage obligation was not reported as a success")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ht.host.financialMetrics.StorageRevenue.Equals(sectorCost) {
		t.Fatal("the host should be reporting revenue after a successful storage proof")
	}
}

// TestLargeContractBlock tests that a storage obligation can still be rapidly
// updated while another storage obligation modification is blocked by the
// largeContractDelay.
func TestLargeContractBlock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestLargeContractBlock")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Create 2 storage obligations for the test and add them to the host.
	so1, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so1.id())
	err = ht.host.managedAddStorageObligation(so1, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so1.id())
	so2, err := ht.newTesterStorageObligation()
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedLockStorageObligation(so2.id())
	err = ht.host.managedAddStorageObligation(so2, false)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so2.id())

	// Add a file contract revision, increasing the filesize of the obligation
	// beyong the largeContractSize.
	validPayouts, missedPayouts := so1.payouts()
	validPayouts[0].Value = validPayouts[0].Value.Sub(types.ZeroCurrency)
	validPayouts[1].Value = validPayouts[1].Value.Add(types.ZeroCurrency)
	missedPayouts[0].Value = missedPayouts[0].Value.Sub(types.ZeroCurrency)
	missedPayouts[1].Value = missedPayouts[1].Value.Add(types.ZeroCurrency)
	revisionSet := []types.Transaction{{
		FileContractRevisions: []types.FileContractRevision{{
			ParentID:          so1.id(),
			UnlockConditions:  types.UnlockConditions{},
			NewRevisionNumber: 1,

			NewFileSize:           uint64(largeContractSize),
			NewFileMerkleRoot:     crypto.Hash{},
			NewWindowStart:        so1.expiration(),
			NewWindowEnd:          so1.proofDeadline(),
			NewValidProofOutputs:  validPayouts,
			NewMissedProofOutputs: missedPayouts,
			NewUnlockHash:         types.UnlockConditions{}.UnlockHash(),
		}},
	}}
	so1.RevisionTransactionSet = revisionSet
	ht.host.managedLockStorageObligation(so1.id())
	err = ht.host.managedModifyStorageObligation(so1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ht.host.managedUnlockStorageObligation(so1.id())
	err = ht.host.tg.Flush()
	if err != nil {
		t.Fatal(err)
	}

	// Lock so1 for the remaining test. This shouldn't block operations on so2.
	ht.host.managedLockStorageObligation(so1.id())
	defer ht.host.managedUnlockStorageObligation(so1.id())

	done := make(chan struct{})
	go func() {
		// Modify so1. This should at least take
		// largeContractUpdateDelay seconds.
		defer close(done)
		start := time.Now()
		err := ht.host.managedModifyStorageObligation(so1, nil, nil)
		delay := time.Since(start)
		if err != nil {
			t.Error(err)
		}
		if delay < largeContractUpdateDelay {
			t.Errorf("delay should be at least %v but was %v", largeContractUpdateDelay, delay)
		}
	}()
	// Lock so2 and modify it repeatedly. This simulates uploads to a different
	// contract. No modification sho
	numMods := 0
LOOP:
	for {
		select {
		case <-done:
			break LOOP
		default:
		}
		numMods++
		ht.host.managedLockStorageObligation(so2.id())
		start := time.Now()
		err := ht.host.managedModifyStorageObligation(so2, nil, nil)
		delay := time.Since(start)
		ht.host.managedUnlockStorageObligation(so2.id())
		if err != nil {
			t.Fatal(err)
		}
		if delay >= largeContractUpdateDelay {
			t.Fatal("delay was longer than largeContractDelay which means so2 got blocked by so1", delay, largeContractUpdateDelay)
		}
	}
	if numMods == 0 {
		t.Fatal("expected at least one modification to happen to so2")
	}
	t.Logf("updated so2 %v times", numMods)
}
