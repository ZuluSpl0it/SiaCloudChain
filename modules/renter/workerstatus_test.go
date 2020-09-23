package renter

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestWorkerAccountStatus is a small unit test that verifies the output of the
// `managedStatus` method on the worker's account.
func TestWorkerAccountStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTesterCustomDependency(t.Name(), &dependencies.DependencyDisableCriticalOnMaxBalance{}, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := wt.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	w := wt.worker

	// allow the worker some time to fetch a PT and fund its EA
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		if w.staticAccount.managedMinExpectedBalance().IsZero() {
			return errors.New("account not funded yet")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// fetch the worker's account status and verify its output
	a := w.staticAccount
	status := a.managedStatus()
	if !(status.Funded == true &&
		status.AvailableBalance.Equals(w.staticBalanceTarget) &&
		status.RecentErr == "" &&
		status.RecentErrTime == time.Time{}) {
		t.Fatal("Unexpected account status", ToJSON(status))
	}

	// ensure the worker is not on maintenance cooldown
	if w.managedOnMaintenanceCooldown() {
		t.Fatal("Unexpected maintenance cooldown")
	}

	// nullify the account balance to ensure refilling triggers a max balance
	// exceeded on the host causing the worker's account to cool down
	a.mu.Lock()
	a.balance = types.ZeroCurrency
	a.mu.Unlock()
	w.managedRefillAccount()

	// fetch the worker's account status and verify the error is being set
	status = a.managedStatus()
	if !(status.Funded == false &&
		status.AvailableBalance.IsZero() &&
		status.RecentErr != "" &&
		status.RecentErrTime != time.Time{}) {
		t.Fatal("Unexpected account status", ToJSON(status))
	}

	// ensure the worker's RHP3 system is on cooldown
	if !w.managedOnMaintenanceCooldown() {
		t.Fatal("Expected RHP3 to have been put on cooldown")
	}
}

// TestWorkerPriceTableStatus is a small unit test that verifies the output of
// the worker's `staticPriceTableStatus` method.
func TestWorkerPriceTableStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	var hostClosed bool
	defer func() {
		if hostClosed {
			if err := wt.rt.Close(); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := wt.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	w := wt.worker

	// allow the worker some time to fetch a PT and fund its EA
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		if w.staticAccount.managedMinExpectedBalance().IsZero() {
			return errors.New("account not funded yet")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// fetch the worker's pricetable status and verify its output
	status := w.staticPriceTableStatus()
	if !(status.Active == true &&
		status.RecentErr == "" &&
		status.RecentErrTime == time.Time{}) {
		t.Fatal("Unexpected price table status", ToJSON(status))
	}

	// close the host to ensure the update PT call fails
	err = wt.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	hostClosed = true

	// trigger an update - to avoid sleeping until `UpdateTime` we manually
	// overwrite it on the PT and call wake
	wpt := w.staticPriceTable()
	wpt.staticUpdateTime = time.Now()
	w.staticSetPriceTable(wpt)
	w.staticWake()

	// fetch the worker's pricetable status
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		status = w.staticPriceTableStatus()
		if !(status.Active == true &&
			status.RecentErr != "" &&
			status.RecentErrTime != time.Time{}) {
			return fmt.Errorf("Unexpected pricetable status %v", ToJSON(status))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerReadJobStatus is a small unit test that verifies the output of the
// `callReadJobStatus` method on the worker.
func TestWorkerReadJobStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := wt.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	w := wt.worker

	// allow the worker some time to fetch a PT and fund its EA
	err = build.Retry(600, 100*time.Millisecond, func() error {
		if w.staticAccount.managedMinExpectedBalance().IsZero() {
			return errors.New("account not funded yet")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)
	err = wt.host.AddSector(sectorRoot, sectorData)
	if err != nil {
		t.Fatal(err)
	}

	// fetch the worker's read jobs status and verify its output
	status := w.callReadJobStatus()
	if !(status.AvgJobTime64k == 0 &&
		status.AvgJobTime1m == 0 &&
		status.AvgJobTime4m == 0 &&
		status.ConsecutiveFailures == 0 &&
		status.JobQueueSize == 0 &&
		status.RecentErr == "" &&
		status.RecentErrTime == time.Time{}) {
		t.Fatal("Unexpected read job status", ToJSON(status))
	}

	// prevent the worker from doing any work by manipulating its read limit
	current := atomic.LoadUint64(&w.staticLoopState.atomicReadDataOutstanding)
	limit := atomic.LoadUint64(&w.staticLoopState.atomicReadDataLimit)
	atomic.StoreUint64(&w.staticLoopState.atomicReadDataLimit, limit)

	// add the job to the worker
	cc := make(chan struct{})
	rc := make(chan *jobReadResponse)

	jhs := &jobReadSector{
		jobRead: jobRead{
			staticResponseChan: rc,
			staticLength:       modules.SectorSize,
			jobGeneric: &jobGeneric{
				staticCancelChan: cc,
				staticQueue:      w.staticJobReadQueue,
			},
		},
		staticOffset: 0,
		staticSector: sectorRoot,
	}
	if !w.staticJobReadQueue.callAdd(jhs) {
		t.Fatal("Could not add job to queue")
	}

	// fetch the worker's read job status again and verify its output
	status = w.callReadJobStatus()
	if status.JobQueueSize != 1 {
		t.Fatal("Unexpected read job status", ToJSON(status))
	}

	// restore the read limit
	atomic.StoreUint64(&w.staticLoopState.atomicReadDataLimit, current)

	// verify the status in a build.Retry to allow the worker some time to
	// process the job
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		status = w.callReadJobStatus()
		if status.AvgJobTime64k == 0 {
			return fmt.Errorf("Unexpected read job status %v", ToJSON(status))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// add another job to the worker
	jhs = &jobReadSector{
		jobRead: jobRead{
			staticResponseChan: rc,
			staticLength:       modules.SectorSize,
			jobGeneric: &jobGeneric{
				staticCancelChan: cc,
				staticQueue:      w.staticJobReadQueue,
			},
		},
		staticOffset: 0,
		staticSector: crypto.Hash{},
	}
	if !w.staticJobReadQueue.callAdd(jhs) {
		t.Fatal("Could not add job to queue")
	}

	// verify the status in a build.Retry to allow the worker some time to
	// process the job
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		status = w.callReadJobStatus()
		if !(status.ConsecutiveFailures == 1 &&
			status.RecentErr != "" &&
			status.RecentErrTime != time.Time{}) {
			return fmt.Errorf("Unexpected read job status %v", ToJSON(status))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerHasSectorJobStatus is a small unit test that verifies the output of
// the `callHasSectorJobStatus` method on the worker.
func TestWorkerHasSectorJobStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	var hostClosed bool
	defer func() {
		if hostClosed {
			if err := wt.rt.Close(); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := wt.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	w := wt.worker

	// allow the worker some time to fetch a PT and fund its EA
	if err := build.Retry(600, 100*time.Millisecond, func() error {
		if w.staticAccount.managedMinExpectedBalance().IsZero() {
			return errors.New("account not funded yet")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// fetch the worker's has sector jobs status and verify its output
	status := w.callHasSectorJobStatus()
	if !(status.AvgJobTime == 0 &&
		status.ConsecutiveFailures == 0 &&
		status.JobQueueSize == 0 &&
		status.RecentErr == "" &&
		status.RecentErrTime == time.Time{}) {
		t.Fatal("Unexpected has sector job status", ToJSON(status))
	}

	// prevent the worker from doing any work by manipulating its read limit
	current := atomic.LoadUint64(&w.staticLoopState.atomicReadDataOutstanding)
	limit := atomic.LoadUint64(&w.staticLoopState.atomicReadDataLimit)
	atomic.StoreUint64(&w.staticLoopState.atomicReadDataLimit, limit)

	// add the job to the worker
	cc := make(chan struct{})
	rc := make(chan *jobHasSectorResponse)
	jhs := w.newJobHasSector(cc, rc, crypto.Hash{})
	if !w.staticJobHasSectorQueue.callAdd(jhs) {
		t.Fatal("Could not add job to queue")
	}

	// fetch the worker's has sector job status again and verify its output
	status = w.callHasSectorJobStatus()
	if status.JobQueueSize != 1 {
		t.Fatal("Unexpected has sector job status", ToJSON(status))
	}

	// restore the read limit
	atomic.StoreUint64(&w.staticLoopState.atomicReadDataLimit, current)

	// verify the status in a build.Retry to allow the worker some time to
	// process the job
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		status = w.callHasSectorJobStatus()
		if status.AvgJobTime == 0 {
			return fmt.Errorf("Unexpected has sector job status %v", ToJSON(status))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// close the host to ensure the job will fail
	err = wt.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	hostClosed = true

	// add another job to the worker
	jhs = w.newJobHasSector(cc, rc, crypto.Hash{})
	if !w.staticJobHasSectorQueue.callAdd(jhs) {
		t.Fatal("Could not add job to queue")
	}

	// verify the status in a build.Retry to allow the worker some time to
	// process the job
	if err := build.Retry(100, 100*time.Millisecond, func() error {
		status = w.callHasSectorJobStatus()
		if !(status.ConsecutiveFailures == 1 &&
			status.RecentErr != "" &&
			status.RecentErrTime != time.Time{}) {
			return fmt.Errorf("Unexpected has sector job status %v", ToJSON(status))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// ToJSON is a helper function that wraps the jsonMarshalIndent function
func ToJSON(a interface{}) string {
	json, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(json)
}
