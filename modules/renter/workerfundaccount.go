package renter

import (
	"sync"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/types"
)

// fundAccountJobQueue is the primary structure for managing fund ephemeral
// account jobs from the worker.
type fundAccountJobQueue struct {
	queue []*fundAccountJob
	mu    sync.Mutex
}

// fundAccountJob contains the details of which ephemeral account to fund and
// with how much.
type fundAccountJob struct {
	amount     types.Currency
	resultChan chan fundAccountJobResult
}

// fundAccountJobResult contains the result from funding an ephemeral account
// on the host.
type fundAccountJobResult struct {
	funded types.Currency
	err    error
}

// managedLen returns the length of the fundAccountJobQueue queue
func (queue *fundAccountJobQueue) managedLen() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.queue)
}

// sendResult is a helper function that sends the rsult to the resultChan and
// closes the result channel
func (job *fundAccountJob) sendResult(funded types.Currency, err error) {
	result := fundAccountJobResult{
		funded: funded,
		err:    err,
	}

	select {
	case job.resultChan <- result:
		close(job.resultChan)
	default:
	}
}

// callQueueFundAccountJob will add a fund account job to the worker's
// queue. A channel will be returned, this channel will have the result of the
// job returned down it when the job is completed.
func (w *worker) callQueueFundAccount(amount types.Currency) chan fundAccountJobResult {
	resultChan := make(chan fundAccountJobResult)
	w.staticFundAccountJobQueue.mu.Lock()
	w.staticFundAccountJobQueue.queue = append(w.staticFundAccountJobQueue.queue, &fundAccountJob{
		amount:     amount,
		resultChan: resultChan,
	})
	w.staticFundAccountJobQueue.mu.Unlock()
	w.staticWake()

	return resultChan
}

// managedKillFundAccountJobs will throw an error for all queued fund account
// jobs, as they will not complete due to the worker being shut down.
func (w *worker) managedKillFundAccountJobs() {
	// clear the queue
	w.staticFundAccountJobQueue.mu.Lock()
	queue := w.staticFundAccountJobQueue.queue
	w.staticFundAccountJobQueue.queue = nil
	w.staticFundAccountJobQueue.mu.Unlock()

	// send an error result to all result chans that were enqueued
	for _, job := range queue {
		job.sendResult(types.ZeroCurrency, errors.New("worker killed before account could be funded"))
	}
}

// managedPerformFundAcountJob will try and execute a fund account job if there
// is one in the queue.
func (w *worker) managedPerformFundAcountJob() bool {
	// Try to dequeue a job, return if there's no work to be performed
	w.staticFundAccountJobQueue.mu.Lock()
	if len(w.staticFundAccountJobQueue.queue) == 0 {
		w.staticFundAccountJobQueue.mu.Unlock()
		return false
	}

	job := w.staticFundAccountJobQueue.queue[0]
	w.staticFundAccountJobQueue.queue = w.staticFundAccountJobQueue.queue[1:]
	w.staticFundAccountJobQueue.mu.Unlock()

	client, err := w.renter.managedRPCClient(w.staticHostPubKey)
	if err != nil {
		job.sendResult(types.ZeroCurrency, err)
		return true
	}

	err = client.FundEphemeralAccount(w.staticAccount.staticID, job.amount)
	if err != nil {
		job.sendResult(types.ZeroCurrency, err)
		return true
	}

	job.sendResult(job.amount, nil)
	return true
}
