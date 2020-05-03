package proto

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/NebulousLabs/fastrand"
)

// dependencyInterruptContractInsertion will interrupt inserting a contract
// after writing the header but before writing the roots.
type dependencyInterruptContractInsertion struct {
	modules.ProductionDependencies
}

// Disrupt returns true if the correct string is provided.
func (d *dependencyInterruptContractInsertion) Disrupt(s string) bool {
	return s == "InterruptContractInsertion"
}

// TestContractUncommittedTxn tests that if a contract revision is left in an
// uncommitted state, either version of the contract can be recovered.
func TestContractUncommittedTxn(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create contract set with one contract
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	initialHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    1,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	c, err := cs.managedInsertContract(initialHeader, initialRoots)
	if err != nil {
		t.Fatal(err)
	}

	// apply an update to the contract, but don't commit it
	sc := cs.mustAcquire(t, c.ID)
	revisedHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    2,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
		StorageSpending: types.NewCurrency64(7),
		UploadSpending:  types.NewCurrency64(17),
	}
	revisedRoots := []crypto.Hash{{1}, {2}}
	fcr := revisedHeader.Transaction.FileContractRevisions[0]
	newRoot := revisedRoots[1]
	storageCost := revisedHeader.StorageSpending.Sub(initialHeader.StorageSpending)
	bandwidthCost := revisedHeader.UploadSpending.Sub(initialHeader.UploadSpending)
	walTxn, err := sc.managedRecordUploadIntent(fcr, newRoot, storageCost, bandwidthCost)
	if err != nil {
		t.Fatal(err)
	}

	// the state of the contract should match the initial state
	// NOTE: can't use reflect.DeepEqual for the header because it contains
	// types.Currency fields
	merkleRoots, err := sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(initialHeader)) {
		t.Fatal("contractHeader should match initial contractHeader")
	} else if !reflect.DeepEqual(merkleRoots, initialRoots) {
		t.Fatal("Merkle roots should match initial Merkle roots")
	}

	// close and reopen the contract set.
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs, err = NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	// the uncommitted transaction should be stored in the contract
	sc = cs.mustAcquire(t, c.ID)
	if len(sc.unappliedTxns) != 1 {
		t.Fatal("expected 1 unappliedTxn, got", len(sc.unappliedTxns))
	} else if !bytes.Equal(sc.unappliedTxns[0].Updates[0].Instructions, walTxn.Updates[0].Instructions) {
		t.Fatal("WAL transaction changed")
	}
	// the state of the contract should match the initial state
	merkleRoots, err = sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots:", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(initialHeader)) {
		t.Fatal("contractHeader should match initial contractHeader", sc.header, initialHeader)
	} else if !reflect.DeepEqual(merkleRoots, initialRoots) {
		t.Fatal("Merkle roots should match initial Merkle roots")
	}

	// apply the uncommitted transaction
	err = sc.managedCommitTxns()
	if err != nil {
		t.Fatal(err)
	}
	// the uncommitted transaction should be gone now
	if len(sc.unappliedTxns) != 0 {
		t.Fatal("expected 0 unappliedTxns, got", len(sc.unappliedTxns))
	}
	// the state of the contract should now match the revised state
	merkleRoots, err = sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots:", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(revisedHeader)) {
		t.Fatal("contractHeader should match revised contractHeader", sc.header, revisedHeader)
	} else if !reflect.DeepEqual(merkleRoots, revisedRoots) {
		t.Fatal("Merkle roots should match revised Merkle roots")
	}
}

// TestContractIncompleteWrite tests that if the merkle root section has the wrong
// length due to an incomplete write, it is truncated and the wal transactions
// are applied.
func TestContractIncompleteWrite(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create contract set with one contract
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	initialHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    1,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	c, err := cs.managedInsertContract(initialHeader, initialRoots)
	if err != nil {
		t.Fatal(err)
	}

	// apply an update to the contract, but don't commit it
	sc := cs.mustAcquire(t, c.ID)
	revisedHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    2,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
		StorageSpending: types.NewCurrency64(7),
		UploadSpending:  types.NewCurrency64(17),
	}
	revisedRoots := []crypto.Hash{{1}, {2}}
	fcr := revisedHeader.Transaction.FileContractRevisions[0]
	newRoot := revisedRoots[1]
	storageCost := revisedHeader.StorageSpending.Sub(initialHeader.StorageSpending)
	bandwidthCost := revisedHeader.UploadSpending.Sub(initialHeader.UploadSpending)
	_, err = sc.managedRecordUploadIntent(fcr, newRoot, storageCost, bandwidthCost)
	if err != nil {
		t.Fatal(err)
	}

	// the state of the contract should match the initial state
	// NOTE: can't use reflect.DeepEqual for the header because it contains
	// types.Currency fields
	merkleRoots, err := sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get merkle roots", err)
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(initialHeader)) {
		t.Fatal("contractHeader should match initial contractHeader")
	} else if !reflect.DeepEqual(merkleRoots, initialRoots) {
		t.Fatal("Merkle roots should match initial Merkle roots")
	}

	// get the size of the merkle roots file.
	size, err := sc.merkleRoots.rootsFile.Size()
	if err != nil {
		t.Fatal(err)
	}
	// the size should be crypto.HashSize since we have exactly one root.
	if size != crypto.HashSize {
		t.Fatal("unexpected merkle root file size", size)
	}
	// truncate the rootsFile to simulate a corruption while writing the second
	// root.
	err = sc.merkleRoots.rootsFile.Truncate(size + crypto.HashSize/2)
	if err != nil {
		t.Fatal(err)
	}

	// close and reopen the contract set.
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs, err = NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	// the uncommitted txn should be gone.
	sc = cs.mustAcquire(t, c.ID)
	if len(sc.unappliedTxns) != 0 {
		t.Fatal("expected 0 unappliedTxn, got", len(sc.unappliedTxns))
	}
	if sc.merkleRoots.len() != 2 {
		t.Fatal("expected 2 roots, got", sc.merkleRoots.len())
	}
	cs.Return(sc)
	cs.Close()
}

// TestContractLargeHeader tests if adding or modifying a contract with a large
// header works as expected.
func TestContractLargeHeader(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create contract set with one contract
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	largeHeader := contractHeader{
		Transaction: types.Transaction{
			ArbitraryData: [][]byte{fastrand.Bytes(1 << 20 * 5)}, // excessive 5 MiB Transaction
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    1,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	// Inserting a contract with a large header should work.
	c, err := cs.managedInsertContract(largeHeader, initialRoots)
	if err != nil {
		t.Fatal(err)
	}

	sc, ok := cs.Acquire(c.ID)
	if !ok {
		t.Fatal("failed to acquire contract")
	}
	// Applying a large header update should also work.
	if err := sc.applySetHeader(largeHeader); err != nil {
		t.Fatal(err)
	}
}

// TestContractSetInsert checks if inserting contracts into the set is ACID.
func TestContractSetInsertInterrupted(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// create contract set with a custom dependency.
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, &dependencyInterruptContractInsertion{})
	if err != nil {
		t.Fatal(err)
	}
	contractHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber:    1,
				NewValidProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	// Inserting the contract should fail due to the dependency.
	c, err := cs.managedInsertContract(contractHeader, initialRoots)
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatal("insertion should have been interrupted")
	}

	// Reload the contract set. The contract should be there.
	cs, err = NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	sc, ok := cs.Acquire(c.ID)
	if !ok {
		t.Fatal("faild to acquire contract")
	}
	if !bytes.Equal(encoding.Marshal(sc.header), encoding.Marshal(contractHeader)) {
		t.Log(sc.header)
		t.Log(contractHeader)
		t.Error("header doesn't match")
	}
	mr, err := sc.merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mr, initialRoots) {
		t.Error("roots don't match")
	}
}

// TestContractCommitAndRecordPaymentIntent verifies the functionality of the
// RecordPaymentIntent and CommitPaymentIntent methods on the SafeContract
func TestContractRecordAndCommitPaymentIntent(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	blockHeight := types.BlockHeight(fastrand.Intn(100))

	// create contract set
	dir := build.TempDir(filepath.Join("proto", t.Name()))
	cs, err := NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}

	// add a contract
	initialHeader := contractHeader{
		Transaction: types.Transaction{
			FileContractRevisions: []types.FileContractRevision{{
				NewRevisionNumber: 1,
				NewValidProofOutputs: []types.SiacoinOutput{
					{Value: types.SiacoinPrecision},
					{Value: types.ZeroCurrency},
				},
				NewMissedProofOutputs: []types.SiacoinOutput{
					{Value: types.SiacoinPrecision},
					{Value: types.ZeroCurrency},
					{Value: types.ZeroCurrency},
				},
				UnlockConditions: types.UnlockConditions{
					PublicKeys: []types.SiaPublicKey{{}, {}},
				},
			}},
		},
	}
	initialRoots := []crypto.Hash{{1}}
	contract, err := cs.managedInsertContract(initialHeader, initialRoots)
	if err != nil {
		t.Fatal(err)
	}
	sc := cs.mustAcquire(t, contract.ID)

	// create a payment revision
	curr := sc.LastRevision()
	amount := types.NewCurrency64(fastrand.Uint64n(100))
	rev, err := curr.PaymentRevision(amount)
	if err != nil {
		t.Fatal(err)
	}

	// record the payment intent
	rpc := modules.RPCExecuteProgram
	walTxn, err := sc.RecordPaymentIntent(rev, amount, rpc)
	if err != nil {
		t.Fatal("Failed to record payment intent")
	}

	// create transaction containing the revision
	signedTxn := rev.ToTransaction()
	sig := sc.Sign(signedTxn.SigHash(0, blockHeight))
	signedTxn.TransactionSignatures[0].Signature = sig[:]

	err = sc.CommitPaymentIntent(walTxn, signedTxn, amount, rpc)
	if err != nil {
		t.Fatal("Failed to commit payment intent")
	}

	// reload the contract set
	cs, err = NewContractSet(dir, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	sc = cs.mustAcquire(t, contract.ID)

	if sc.LastRevision().NewRevisionNumber != rev.NewRevisionNumber {
		t.Fatal("Unexpected revision number after reloading the contract set")
	}

	// TODO: extend this test when we add the spending metrics to the header
}
