package host

import (
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"time"
)

// Constants related to the host's alerts.
const (
	// AlertMSGHostInsufficientCollateral indicates that a host has insufficient
	// collateral budget remaining
	AlertMSGHostInsufficientCollateral = "host has insufficient collateral budget"
)

const (
	// defaultMaxDuration defines the maximum number of blocks into the future
	// that the host will accept for the duration of an incoming file contract
	// obligation. 4 months are chosen as the network is in buildout.
	defaultMaxDuration = 144 * 30 * 4 // 4 months.

	// iteratedConnectionTime is the amount of time that is allowed to pass
	// before the host will stop accepting new iterations on an iterated
	// connection.
	iteratedConnectionTime = 1200 * time.Second

	// resubmissionTimeout defines the number of blocks that a host will wait
	// before attempting to resubmit a transaction to the blockchain.
	// Typically, this transaction will contain either a file contract, a file
	// contract revision, or a storage proof.
	resubmissionTimeout = 3

	// rpcRequestInterval is the amount of time that the renter has to send
	// the next RPC ID in the new RPC loop. (More time is alloted for sending
	// the actual RPC request object.)
	rpcRequestInterval = 2 * time.Minute

	// maxObligationLockTimeout is the maximum amount of time the host will wait
	// to lock a storage obligation.
	maxObligationLockTimeout = 10 * time.Minute
)

var (
	// connectablityCheckFirstWait defines how often the host's connectability
	// check is run.
	connectabilityCheckFirstWait = build.Select(build.Var{
		Standard: time.Minute * 2,
		Dev:      time.Minute * 1,
		Testing:  time.Second * 3,
	}).(time.Duration)

	// connectablityCheckFrequency defines how often the host's connectability
	// check is run.
	connectabilityCheckFrequency = build.Select(build.Var{
		Standard: time.Minute * 10,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 10,
	}).(time.Duration)

	// connectabilityCheckTimeout defines how long a connectability check's dial
	// will be allowed to block before it times out.
	connectabilityCheckTimeout = build.Select(build.Var{
		Standard: time.Minute * 2,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 90,
	}).(time.Duration)

	// defaultBaseRPCPrice is the default price of talking to the host. It is
	// roughly equal to the default bandwidth cost of exchanging a pair of
	// 4096-byte messages.
	// defaultBaseRPCPrice = types.ScPrimecoinPrecision.Mul64(10).Div64(1e9) // 10 nS
	defaultBaseRPCPrice = defaultDownloadBandwidthPrice.Mul64(modules.MaxBaseRPCPriceVsBandwidth).Div64(5)

	// defaultCollateral defines the amount of money that the host puts up as
	// collateral per-byte by default. The collateral should be considered as
	// an absolute instead of as a percentage, because low prices result in
	// collaterals which may be significant by percentage, but insignificant
	// overall. A default of 25 KS / TB / Month has been chosen, which is 2.5x
	// the default price for storage. The host is expected to put up a
	// significant amount of collateral as a commitment to faithfulness,
	// because this guarantees that the incentives are aligned for the host to
	// keep the data even if the price of SCP fluctuates, the price of raw
	// storage fluctuates, or the host realizes that there is unexpected
	// opportunity cost in being a host.
	defaultCollateral = types.ScPrimecoinPrecision.Mul64(35).Div(modules.BlockBytesPerMonthTerabyte) // 35 SCP / TB / Month

	// defaultCollateralBudget defines the maximum number of SCP that the
	// host is going to allocate towards collateral. The number has been chosen
	// as a number that is large, but not so large that someone would be
	// furious for losing access to it for a few weeks.
	defaultCollateralBudget = types.ScPrimecoinPrecision.Mul64(4e3) //4 KS

	// defaultContractPrice defines the default price of creating a contract
	// with the host. The current default is 0.1. This was chosen since it is
	// the minimum fee estimation of the transactionpool for a filecontract
	// transaction..
	defaultContractPrice = types.ScPrimecoinPrecision.Div64(1e6).Mul64(modules.EstimatedFileContractRevisionAndProofTransactionSetSize)

	// defaultDownloadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 500 SCP per gigabyte, because
	// download bandwidth is expected to be plentiful but also in-demand.
	defaultDownloadBandwidthPrice = types.ScPrimecoinPrecision.Mul64(2).Div(modules.BytesPerTerabyte) // 2 SCP / TB

	// defaultMaxCollateral defines the maximum amount of collateral that the
	// host is comfortable putting into a single file contract. 10e3 is a
	// relatively small file contract, but millions of SCP could be locked
	// away by only a few hundred file contracts. As the ecosystem matures, it
	// is expected that the safe default for this value will increase quite a
	// bit.
	defaultMaxCollateral = types.ScPrimecoinPrecision.Mul64(400) // 400 SCP

	// defaultMaxDownloadBatchSize defines the maximum number of bytes that the
	// host will allow to be requested by a single download request. 33 MiB has
	// been chosen because it's 8 full sectors plus some wiggle room. 33 MiB is
	// a conservative default, most hosts will be fine with a number like 65
	// MiB.
	defaultMaxDownloadBatchSize = 33 * (1 << 20)

	// defaultMaxReviseBatchSize defines the maximum number of bytes that the
	// host will allow to be sent during a single batch update in a revision
	// RPC. 33 MiB has been chosen because it's eight full sectors, plus some
	// wiggle room for the extra data or a few delete operations. The whole
	// batch will be held in memory, so the batch size should only be increased
	// substantially if the host has a lot of memory. Additionally, the whole
	// batch is sent in one network connection. Additionally, the renter can
	// steal funds for upload bandwidth all the way out to the size of a batch.
	// 33 MiB is a conservative default, most hosts are likely to be just fine
	// with a number like 65 MiB.
	defaultMaxReviseBatchSize = 33 * (1 << 20)

	// defaultSectorAccessPrice defines the default price of a sector access. It
	// is roughly equal to the cost of downloading 64 KiB.
	// defaultSectorAccessPrice = defaultDownloadBandwidthPrice.Mul64(modules.MaxSectorAccessPriceVsBandwidth).Div64(2)
	defaultSectorAccessPrice = defaultDownloadBandwidthPrice.Mul64(modules.MaxSectorAccessPriceVsBandwidth).Div64(2)

	// defaultStoragePrice defines the starting price for hosts selling
	// storage. We try to match a number that is both reasonably profitable and
	// reasonably competitive.
	defaultStoragePrice = types.ScPrimecoinPrecision.Mul64(20).Div(modules.BlockBytesPerMonthTerabyte) // 20 SCP / TB / Month

	// defaultUploadBandwidthPrice defines the default price of upload
	// bandwidth. The default is set to 250 SCP per GB, because the host is
	// presumed to have a large amount of downstream bandwidth. Furthermore,
	// the host is typically only downloading data if it is planning to store
	// the data, meaning that the host serves to profit from accepting the
	// data.
	defaultUploadBandwidthPrice = types.ScPrimecoinPrecision.Mul64(2).Div(modules.BytesPerTerabyte) // 2 SCP / TB

	// defaultEphemeralAccountExpiry defines the default maximum amount of
	// time an ephemeral account can be inactive before it expires and gets
	// deleted.
	defaultEphemeralAccountExpiry = uint64(604800) // 1 week

	// defaultMaxEphemeralAccountBalance defines the default maximum amount of
	// money that the host will allow to deposit into a single ephemeral account
	defaultMaxEphemeralAccountBalance = types.ScPrimecoinPrecision.Div64(100)

	// defaultMaxEphemeralAccountRisk is the maximum amount of money that the
	// host is willing to risk to a power loss. If a user's withdrawal would put
	// the host over the maxunsaveddelat, the host will wait to complete the
	// user's transaction until the host has persisted the widthdrawal, to
	// prevent the host from having too much money at risk.
	defaultMaxEphemeralAccountRisk = types.ScPrimecoinPrecision.Div64(20)

	// defaultWindowSize is the size of the proof of storage window requested
	// by the host. The host will not delete any obligations until the window
	// has closed and buried under several confirmations. For release builds,
	// the default is set to 144 blocks, or about 1 day. This gives the host
	// flexibility to experience downtime without losing file contracts. The
	// optimal default, especially as the network matures, is probably closer
	// to 36 blocks. An experienced or high powered host should not be
	// frustrated by lost coins due to long periods of downtime.
	defaultWindowSize = build.Select(build.Var{
		Dev:      types.BlockHeight(36),  // 3.6 minutes.
		Standard: types.BlockHeight(144), // 1 day.
		Testing:  types.BlockHeight(5),   // 5 seconds.
	}).(types.BlockHeight)

	// logAllLimit is the number of errors of each type that the host will log
	// before switching to probabilistic logging. If there are not many errors,
	// it is reasonable that all errors get logged. If there are lots of
	// errors, to cut down on the noise only some of the errors get logged.
	logAllLimit = build.Select(build.Var{
		Dev:      uint64(50),
		Standard: uint64(250),
		Testing:  uint64(100),
	}).(uint64)

	// logFewLimit is the number of errors of each type that the host will log
	// before substantially constricting the amount of logging that it is
	// doing.
	logFewLimit = build.Select(build.Var{
		Dev:      uint64(500),
		Standard: uint64(2500),
		Testing:  uint64(500),
	}).(uint64)

	// obligationLockTimeout defines how long a thread will wait to get a lock
	// on a storage obligation before timing out and reporting an error to the
	// renter.
	obligationLockTimeout = build.Select(build.Var{
		Dev:      time.Second * 20,
		Standard: time.Second * 60,
		Testing:  time.Second * 3,
	}).(time.Duration)

	// revisionSubmissionBuffer describes the number of blocks ahead of time
	// that the host will submit a file contract revision. The host will not
	// accept any more revisions once inside the submission buffer.
	revisionSubmissionBuffer = build.Select(build.Var{
		Dev:      types.BlockHeight(20),  // About 4 minutes
		Standard: types.BlockHeight(144), // 1 day.
		Testing:  types.BlockHeight(4),
	}).(types.BlockHeight)

	// rpcRatelimit prevents someone from spamming the host with connections,
	// causing it to spin up enough goroutines to crash.
	rpcRatelimit = build.Select(build.Var{
		Dev:      time.Millisecond * 10,
		Standard: time.Millisecond * 50,
		Testing:  time.Millisecond,
	}).(time.Duration)

	// workingStatusFirstCheck defines how frequently the Host's working status
	// check runs
	workingStatusFirstCheck = build.Select(build.Var{
		Standard: time.Minute * 3,
		Dev:      time.Minute * 1,
		Testing:  time.Second * 3,
	}).(time.Duration)

	// workingStatusFrequency defines how frequently the Host's working status
	// check runs
	workingStatusFrequency = build.Select(build.Var{
		Standard: time.Minute * 10,
		Dev:      time.Minute * 5,
		Testing:  time.Second * 10,
	}).(time.Duration)

	// workingStatusThreshold defines how many settings calls must occur over the
	// workingStatusFrequency for the host to be considered working.
	workingStatusThreshold = build.Select(build.Var{
		Standard: uint64(3),
		Dev:      uint64(1),
		Testing:  uint64(1),
	}).(uint64)
)

// All of the following variables define the names of buckets used by the host
// in the database.
var (
	// bucketActionItems maps a blockchain height to a list of storage
	// obligations that need to be managed in some way at that height. The
	// height is stored as a big endian uint64, which means that bolt will
	// store the heights sorted in numerical order. The action item itself is
	// an array of file contract ids. The host is able to contextually figure
	// out what the necessary actions for that item are based on the file
	// contract id and the associated storage obligation that can be retrieved
	// using the id.
	bucketActionItems = []byte("BucketActionItems")

	// bucketStorageObligations contains a set of serialized
	// 'storageObligations' sorted by their file contract id.
	bucketStorageObligations = []byte("BucketStorageObligations")
)

// init runs a series of sanity checks to verify that the constants have sane
// values.
func init() {
	// The revision submission buffer should be greater than the resubmission
	// timeout, because there should be time to perform resubmission if the
	// first attempt to submit the revision fails.
	if revisionSubmissionBuffer < resubmissionTimeout {
		build.Critical("revision submission buffer needs to be larger than or equal to the resubmission timeout")
	}
}
