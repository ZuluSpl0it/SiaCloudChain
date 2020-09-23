package renter

// projectdownloadbyroot.go creates a worker project to fetch the data of an
// underlying sector root.

import (
	"fmt"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/threadgroup"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
)

var (
	// ErrRootNotFound is returned if all workers were unable to recover the
	// root
	ErrRootNotFound = errors.New("workers were unable to recover the data by sector root - all workers failed")

	// ErrProjectTimedOut is returned when the project timed out
	ErrProjectTimedOut = errors.New("project timed out")
)

// jobDownloadByRoot contains all of the information necessary to execute a
// perform download job.
type jobDownloadByRoot struct {
	// jobDownloadByRoot exists as two phases. The first is a startup phase,
	// which determines whether or not the host is capable of executing the job.
	// The second is the fetching phase, where the worker actually fetches data
	// from the remote host.
	//
	// If staticStartupCompleted is set to false, it means the job is in phase
	// one, and if startupCompleted is set to true, it means the job is in phase
	// two.
	staticProject          *projectDownloadByRoot
	staticStartupCompleted bool
}

// projectDownloadByRoot is a project to download a piece of data knowing
// nothing more than the sector root. If the root's location on the network
// cannot easily be found by looking up a cache, the project will have every
// single worker check its respective host for the root, and then will
// coordinate fetching the root among the workers that have the root.
type projectDownloadByRoot struct {
	// Information about the data that is being retrieved.
	staticRoot   crypto.Hash
	staticLength uint64
	staticOffset uint64

	// rootFound is a bool indicating that data has been discovered on the
	// network and is actively being fetched. If rootFound is set to true, any
	// new workers that discover they have the root in question will go on
	// standby.
	//
	// workersRegistered is the list of workers that are actively working on
	// either figuring out whether their host has the data, or are working on
	// actually fetching the data. This map should be empty only if there are no
	// workers at the moment that are actively tasked with work. A worker that
	// is woken from standby needs to be placed back into the list of registered
	// workers when it is removed from standby.
	//
	// workersStandby is the list of workers that have completed checking
	// whether or not the root is available on their host and have discovered
	// that the root is available, but then the worker did not start fetching
	// the data. Workers will typically end up on standby if they see that other
	// workers are actively fetching data. The standby workers get activated if
	// the other workers fetching data experience errors.
	rootFound         bool
	workersRegistered map[string]struct{}
	workersStandby    []*worker

	// Project output. Once the project has been completed, completeChan will be
	// closed. The data and error fields contain the final output for the
	// project.
	data         []byte
	err          error
	completeChan chan struct{}

	staticDeps modules.Dependencies
	tg         *threadgroup.ThreadGroup
	mu         sync.Mutex
}

// callPerformJobDownloadByRoot will perform a download by root job.
func (jdbr *jobDownloadByRoot) callPerformJobDownloadByRoot(w *worker) {
	// The job is broken into two phases, startup and resume. A bool in the
	// worker indicates which phase needs to be run.
	if jdbr.staticStartupCompleted {
		jdbr.staticProject.managedResumeJobDownloadByRoot(w)
	} else {
		jdbr.staticProject.managedStartJobDownloadByRoot(w)
	}
}

// managedRemoveWorker will remove a worker from the project. This is typically
// called after a worker has finished its job, successfully or unsuccessfully.
func (pdbr *projectDownloadByRoot) managedRemoveWorker(w *worker) {
	pdbr.mu.Lock()
	defer pdbr.mu.Unlock()

	// Delete the worker from the list of registered workers.
	delete(pdbr.workersRegistered, w.staticHostPubKeyStr)

	// Delete every instance of the worker in the list of standby workers. The
	// worker should only be in the list once, but we check the whole list
	// anyway.
	totalRemoved := 0
	for i := 0; i < len(pdbr.workersStandby); i++ {
		if pdbr.workersStandby[i] == w {
			pdbr.workersStandby = append(pdbr.workersStandby[:i], pdbr.workersStandby[i+1:]...)
			i-- // Since the array has been shifted, adjust the iterator to ensure every item is visited.
			totalRemoved++
		}
	}
	if totalRemoved > 1 {
		w.renter.log.Critical("one worker appeared in the standby list multiple times")
	}

	// Check whether the pdbr is already completed. If so, nothing else needs to
	// be done.
	if pdbr.staticComplete() {
		return
	}

	// Sector download is not yet complete. Check whether this is the last
	// worker in the pdbr, requiring a shutdown / failure to be sent.
	if len(pdbr.workersRegistered) == 0 {
		// Sanity check - a worker should only go on standby if there are
		// registered workers actively trying to download the data. If those
		// workers fail and remove themselves, they should wake a standby worker
		// before removing themselves from the project, meaning that there
		// should never be a case where the list of registered workers is empty
		// but the list of standby workers is not empty.
		if len(pdbr.workersStandby) != 0 {
			w.renter.log.Critical("pdbr has standby workers but no registered workers:", len(pdbr.workersStandby))
		}
		pdbr.markComplete(ErrRootNotFound)
	}
}

// managedResumeJobDownloadByRoot is called after a worker has confirmed that a
// root exists on a host, and after the worker has gained the imperative to
// fetch the data from the host.
func (pdbr *projectDownloadByRoot) managedResumeJobDownloadByRoot(w *worker) {
	data, err := w.Download(pdbr.staticRoot, pdbr.staticOffset, pdbr.staticLength)
	if err != nil {
		w.renter.log.Debugln("worker failed a projectDownloadByRoot job:", err)
		pdbr.managedWakeStandbyWorker()
		pdbr.managedRemoveWorker(w)
		return
	}
	// Block if necessary.
	pdbr.staticDeps.Disrupt("BlockUntilTimeout")

	// Set the data and perform cleanup.
	pdbr.mu.Lock()
	pdbr.data = data
	pdbr.markComplete(nil)
	pdbr.mu.Unlock()
}

// managedStartJobDownloadByRoot will execute the first stage of downloading
// data by merkle root for a worker. The first stage consists of determining
// whether or not the worker's host has the merkle root in question.
func (pdbr *projectDownloadByRoot) managedStartJobDownloadByRoot(w *worker) {
	// Check if the project is already completed, do no more work if so.
	if pdbr.staticComplete() {
		pdbr.managedRemoveWorker(w)
		return
	}

	// Download a single byte to see if the root is available.
	_, err := w.Download(pdbr.staticRoot, 0, 1)
	if err != nil {
		w.renter.log.Debugln("worker failed a download by root job:", err)
		pdbr.managedRemoveWorker(w)
		return
	}

	// The host has the root. Check in with the project and see if the root
	// needs to be fetched. If 'rootFound' is set to false, it means that
	// nobody is actively fetching the root.
	pdbr.mu.Lock()
	if pdbr.rootFound {
		pdbr.workersStandby = append(pdbr.workersStandby, w)
		pdbr.mu.Unlock()
		return
	}
	pdbr.rootFound = true
	pdbr.mu.Unlock()

	// Have the worker attempt the full download.
	pdbr.managedResumeJobDownloadByRoot(w)
	return
}

// managedWakeStandbyWorker is called when a worker that was performing actual
// download work has failed and needs to be replaced. If there are any standby
// workers, one of the standby workers will assume its place.
//
// managedWakeStandbyWorker assumes that rootFound is currently set to true,
// because it will be called by a worker that failed and had set rootFound to
// true.
func (pdbr *projectDownloadByRoot) managedWakeStandbyWorker() {
	// If there are no workers on standby, set rootFound to false, indicating
	// that no workers are actively fetching the piece; any worker that finds
	// the piece should immediately start fetching it.
	pdbr.mu.Lock()
	if len(pdbr.workersStandby) == 0 {
		pdbr.rootFound = false
		pdbr.mu.Unlock()
		return
	}
	// There is a standby worker that has found the piece previously, pop it off
	// of the array.
	newWorker := pdbr.workersStandby[0]
	pdbr.workersStandby = pdbr.workersStandby[1:]
	pdbr.mu.Unlock()

	// Schedule a job with the worker to resume the download. Can't download
	// directly because any work that is being performed needs to go through the
	// worker so that the worker can actively control how the connection is
	// used.
	jdbr := jobDownloadByRoot{
		staticProject:          pdbr,
		staticStartupCompleted: true,
	}
	newWorker.callQueueJobDownloadByRoot(jdbr)
}

// markComplete marks the project as done and assigns the provided error to
// pdbr.err.
func (pdbr *projectDownloadByRoot) markComplete(err error) {
	if pdbr.staticComplete() {
		return
	}
	pdbr.err = err
	close(pdbr.completeChan)
}

// threadedHandleTimeout sets a timeout on the project. If the root is not found
// before the timeout expires, the project is finished. A zero timeout is
// ignored.
func (pdbr *projectDownloadByRoot) threadedHandleTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	if err := pdbr.tg.Add(); err != nil {
		return
	}
	defer pdbr.tg.Done()

	// Block until the timeout has expired or the project has completed,
	// whichever comes first
	select {
	case <-pdbr.tg.StopChan():
		return
	case <-pdbr.completeChan:
		return
	case <-time.After(timeout):
	}
	// Project timed out. Trigger waiting depenencies.
	pdbr.staticDeps.Disrupt("ResumeOnTimeout")

	pdbr.managedTriggerTimeout(timeout)
}

// managedTriggerTimeout handles a timeout. It will close out the completeChan
// and set the appropriate error.
func (pdbr *projectDownloadByRoot) managedTriggerTimeout(t time.Duration) {
	pdbr.mu.Lock()
	defer pdbr.mu.Unlock()
	err := errors.Compose(ErrRootNotFound, errors.AddContext(ErrProjectTimedOut, fmt.Sprintf("timed out after %vs", t.Seconds())))
	pdbr.markComplete(err)
}

// staticComplete is a helper function to check if the project has already
// completed. Workers use this method to determine whether to abort early.
func (pdbr *projectDownloadByRoot) staticComplete() bool {
	select {
	case <-pdbr.completeChan:
		return true
	default:
		return false
	}
}

// DownloadByRootLegacy will spin up a project to locate a root and then
// download that root.
//
// This is the legacy version of the function that uses the synchronous renter
// protocol. We are only keeping the code becauase v1.4.9 needs to support this
// version. Starting with v1.5.0, this code will be removed.
func (r *Renter) DownloadByRootLegacy(root crypto.Hash, offset, length uint64, timeout time.Duration) ([]byte, error) {
	// Create the download by root project.
	pdbr := &projectDownloadByRoot{
		staticRoot:   root,
		staticLength: length,
		staticOffset: offset,

		workersRegistered: make(map[string]struct{}),

		completeChan: make(chan struct{}),

		staticDeps: r.deps,
		tg:         &r.tg,
	}

	// Apply the timeout to the project. A timeout of 0 will be ignored.
	if r.deps.Disrupt("timeoutProjectDownloadByRoot") {
		pdbr.managedTriggerTimeout(timeout)
		return nil, pdbr.err
	}
	go pdbr.threadedHandleTimeout(timeout)

	// Give the project to every worker. The list of workers needs to be fetched
	// first, and then the job can be queued because cleanup of the project
	// assumes that no more workers will be added to the project once the first
	// worker has begun work.
	workers := r.staticWorkerPool.callWorkers()
	if len(workers) == 0 {
		return nil, errors.New("cannot perform DownloadByRoot, no workers in worker pool")
	}
	for _, w := range workers {
		pdbr.workersRegistered[w.staticHostPubKeyStr] = struct{}{}
	}
	// Queue the jobs in the workers.
	jdbr := jobDownloadByRoot{
		staticProject:          pdbr,
		staticStartupCompleted: false,
	}
	for _, w := range workers {
		w.callQueueJobDownloadByRoot(jdbr)
	}

	// Block until the project has completed.
	select {
	case <-pdbr.tg.StopChan():
	case <-pdbr.completeChan:
	}

	// Fetch the error and the data. Then nil out the data in the pdbr so that
	// other workers who haven't finished and are holding a reference to the
	// pdbr aren't keeping a reference to this heavy object.
	pdbr.mu.Lock()
	err := pdbr.err
	data := pdbr.data
	pdbr.data = nil
	pdbr.mu.Unlock()
	if err != nil {
		return nil, errors.AddContext(err, "unable to fetch sector root from the network")
	}
	return data, nil
}

// checkDownloadPriceProtections looks at the current renter allowance and the
// active settings for a host and determines whether a download by root job
// should be halted due to price gouging.
func checkDownloadPriceProtections(allowance modules.Allowance, hostSettings modules.HostExternalSettings) error {
	// Check whether the base RPC price is too high.
	if !allowance.MaxRPCPrice.IsZero() && allowance.MaxRPCPrice.Cmp(hostSettings.BaseRPCPrice) < 0 {
		errStr := fmt.Sprintf("rpc price of host is above the allowance max: %v vs %v", hostSettings.BaseRPCPrice, allowance.MaxRPCPrice)
		return errors.New(errStr)
	}
	// Check whether the download bandwidth price is too high.
	if !allowance.MaxDownloadBandwidthPrice.IsZero() && allowance.MaxDownloadBandwidthPrice.Cmp(hostSettings.DownloadBandwidthPrice) < 0 {
		dbp := hostSettings.DownloadBandwidthPrice
		max := allowance.MaxDownloadBandwidthPrice
		errStr := fmt.Sprintf("download bandwidth price of host is above the allowance max: %v vs %v", dbp, max)
		return errors.New(errStr)
	}
	// Check whether the sector access price is too high.
	if !allowance.MaxSectorAccessPrice.IsZero() && allowance.MaxSectorAccessPrice.Cmp(hostSettings.SectorAccessPrice) < 0 {
		sap := hostSettings.SectorAccessPrice
		max := allowance.MaxSectorAccessPrice
		errStr := fmt.Sprintf("sector access price of host is above the allowance max: %v vs %v", sap, max)
		return errors.New(errStr)
	}

	return nil
}

// Download will fetch data from a host, first checking any price protections
// that are in place.
func (w *worker) Download(root crypto.Hash, offset, length uint64) ([]byte, error) {
	// Fetch a session to use in retrieving the sector.
	downloader, err := w.renter.hostContractor.Downloader(w.staticHostPubKey, w.renter.tg.StopChan())
	if err != nil {
		return nil, errors.AddContext(err, "unable to open downloader for download")
	}
	defer downloader.Close()

	// Check for price gouging before completing the job.
	allowance := w.renter.hostContractor.Allowance()
	hostSettings := downloader.HostSettings()
	err = checkDownloadPriceProtections(allowance, hostSettings)
	if err != nil {
		return nil, errors.AddContext(err, "price protections are blocking download")
	}

	// Fetch the data. Need to ensure that the length is a factor of 64, need to
	// add and remove padding.
	//
	// NOTE: This padding is a requirement of the current Downloader, when the
	// MDM gets deployed and used, this download operation shouldn't need any
	// padding anymore.
	padding := crypto.SegmentSize - length%crypto.SegmentSize
	if padding == crypto.SegmentSize {
		padding = 0
	}
	sectorData, err := downloader.Download(root, uint32(offset), uint32(length+padding))
	if err != nil {
		return nil, errors.AddContext(err, "download failed")
	}
	return sectorData[:length], nil
}
