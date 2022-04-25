package types

// constants.go contains the ScPrime constants. Depending on which build tags are
// used, the constants will be initialized to different values.
//
// CONTRIBUTE: We don't have way to check that the non-test constants are all
// sane, plus we have no coverage for them.

import (
	"math"
	"math/big"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
)

var (
	// ASICHardforkHeight is the height at which the hardfork targeting
	// selected ASICs was activated.
	ASICHardforkHeight BlockHeight

	// ASICHardforkTotalTarget is the initial target after the ASIC hardfork.
	// The actual target at ASICHardforkHeight is replaced with this value in
	// order to prevent intolerably slow block times post-fork.
	ASICHardforkTotalTarget Target

	// ASICHardforkTotalTime is the initial total time after the ASIC
	// hardfork. The actual total time at ASICHardforkHeight is replaced with
	// this value in order to prevent intolerably slow block times post-fork.
	ASICHardforkTotalTime int64

	// ASICHardforkFactor is the factor by which the hashrate of targeted
	// ASICs will be reduced.
	ASICHardforkFactor = uint64(1)

	// ASICHardforkReplayProtectionPrefix is a byte that prefixes
	// SiacoinInputs and SiafundInputs when calculating SigHashes to protect
	// against replay attacks.
	ASICHardforkReplayProtectionPrefix = []byte(nil)

	// BlockFrequency is the desired number of seconds that
	// should elapse, on average, between successive Blocks.
	BlockFrequency BlockHeight

	// BlockSizeLimit is the maximum size of a binary-encoded Block
	// that is permitted by the consensus rules.
	BlockSizeLimit = uint64(2e6)

	// BlocksPerHour is the number of blocks expected to be mined per hour.
	BlocksPerHour = BlockHeight(5) //OSV was 6

	// BlocksPerDay is the number of blocks expected to be mined per day.
	BlocksPerDay = 24 * BlocksPerHour

	// BlocksPerWeek is the number of blocks expected to be mined per week.
	BlocksPerWeek = 7 * BlocksPerDay

	// BlocksPerMonth is the number of blocks expected to be mined per month.
	BlocksPerMonth = 30 * BlocksPerDay

	// BlocksPerYear is the number of blocks expected to be mined per year.
	BlocksPerYear = 365 * BlocksPerDay

	// BurnAddressBlockHeight is the height at which the dev fund will be burnt
	// instead of being claimed by the dev fund. Setting this value to 0 will
	// prevent the dev fund from being burnt at any height.
	BurnAddressBlockHeight = BlockHeight(0) //OSV was 105000

	// BurnAddressUnlockHash is the unlock hash for where to send coins to burn.
	BurnAddressUnlockHash = UnlockHashFromAddrStr("e3a19a1c70d75dc865f5ce539e0858e2450aa61b7136d8f25b0b800d52177919557208b73316")

	// DevFundEnabled is a boolean that when set to true will enable the ability to
	// configure a dev fund
	DevFundEnabled = true

	// DevFundInitialBlockHeight is the height at which the dev fund became mandatory
	DevFundInitialBlockHeight = BlockHeight(1)

	// DevFundDecayStartBlockHeight is the height at which the DevFundInitialPercentage
	// begins to linearly decay to the DevFundFinalPercentage
	DevFundDecayStartBlockHeight = BlockHeight(43800) //OSV was 30000, NSV = blks in 1 yr

	// DevFundDecayEndBlockHeight is the height at which the DevFundInitialPercentage
	// has fully decayed to the DevFundFinalPercentage
	DevFundDecayEndBlockHeight = BlockHeight(131400) //OSV was 105000, NSV = blks in 3 yrs

	// DevFundInitialPercentage is the initial percentage of the block reward that is
	// sent to the DevFundUnlockHash before any dev fund percentage decay happens
	DevFundInitialPercentage = uint64(10)

	// DevFundFinalPercentage is the final percentage of the block reward that is sent
	//  to the DevFundUnlockHash after the dev fund percentage is fully decayed
	DevFundFinalPercentage = uint64(5)

	// DevFundUnlockHash is the unlock hash for the dev fund subsidy
	// Do not set this to the Zero address as doing so will cause the test that
	// verifies that a dev fee is set to fail
	DevFundUnlockHash = UnlockHashFromAddrStr("cc05f0024d75d23002dfed47bcd6177c0e5dd652b146758951806bb8fb15fa158da76dcbaeb6")

	// EndOfTime is value to be used when a date in the future is needed for
	// validation
	EndOfTime = time.Unix(0, math.MaxInt64)

	// ExtremeFutureThreshold is a temporal limit beyond which Blocks are
	// discarded by the consensus rules. When incoming Blocks are processed, their
	// Timestamp is allowed to exceed the processor's current time by a small amount.
	// But if the Timestamp is further into the future than ExtremeFutureThreshold,
	// the Block is immediately discarded.
	ExtremeFutureThreshold Timestamp

	// FutureThreshold is a temporal limit beyond which Blocks are
	// discarded by the consensus rules. When incoming Blocks are processed, their
	// Timestamp is allowed to exceed the processor's current time by no more than
	// FutureThreshold. If the excess duration is larger than FutureThreshold, but
	// smaller than ExtremeFutureThreshold, the Block may be held in memory until
	// the Block's Timestamp exceeds the current time by less than FutureThreshold.
	FutureThreshold Timestamp

	// GenesisBlock is the first block of the block chain
	GenesisBlock Block

	// GenesisID is used in many places. Calculating it once saves lots of
	// redundant computation.
	GenesisID BlockID

	// GenesisAirdropAllocation is the output creating the initial coins allocated
	// for the airdrop at network launch
	GenesisAirdropAllocation []SiacoinOutput

	// GenesisSiacoinAllocation is the set of SiacoinOutputs created in the Genesis block
	GenesisSiacoinAllocation []SiacoinOutput

	// GenesisSiafundAllocation is the set of SiafundOutputs created in the Genesis
	// block.
	GenesisSiafundAllocation []SiafundOutput

	// ForkedGenesisSiafundAllocation is the set of SiafundOutputs created in the Genesis block.
	ForkedGenesisSiafundAllocation []SiafundOutput

	// SiafundHardforkAllocation is allocation of new Siafunds at various hardforks.
	SiafundHardforkAllocation map[BlockHeight][]SiafundOutput

	// GenesisTimestamp is the timestamp when genesis block was mined
	GenesisTimestamp Timestamp

	// InitialCoinbase is the coinbase reward of the Genesis block.
	InitialCoinbase = uint64(30) //OSV was 300e3; 30,000 SC = 30 SC

	//AirdropInvestorValue is the total amount of coins for for initial investors
	//Expecting three levels of investment: 2@10K, 10@5K, 20@1K
	AirdropInvestorValue = NewCurrency64(200000000).Mul(ScPrimecoinPrecision) // 200 x 10^27 = 200M SCC

	// AirdropCommunityValue is the total amount of coins the community members will split
	// from the genesis block airdrop.
	// OSV was AirdropCommunityValue = NewCurrency64(10000000000).Mul(ScPrimecoinPrecision)
	// NSV is for coins mined from blks 270K to 370K. into SiaClassic wallets.
	// Assigned at a rate of 30:1, beginning with blk 370K working backwards. This can
	// happen when a validated SiaClassic consensus file is recovered
	AirdropCommunityValue = NewCurrency64(100000000).Mul(ScPrimecoinPrecision) //100B SC = 100M SCC

	//AirdropDevCompValue is the total amount of coins for three years of developer compensation
	AirdropDevCompValue = NewCurrency64(10000000).Mul(ScPrimecoinPrecision) //10B SC = 10M SCC

	// AirdropSiaCloudValue is the total amount to help bootstrap expenses Equipment, Supplies and mining pool reserve.
	AirdropSiaCloudValue = NewCurrency64(9000000).Mul(ScPrimecoinPrecision) //9B SC = 9M SCC

	// AirdropSiaPrimeValue is a gift to the ScPrimeto acknowledge all their effort and hard work. THANK YOU!
	// OSV was AirdropSiaPrimeValue = NewCurrency64(300000000).Mul(ScPrimecoinPrecision)
	// NSV can be transfered to ScPrime team after blk 120K, provided there is enough liquidity.
	AirdropSiaPrimeValue = NewCurrency64(1000000).Mul(ScPrimecoinPrecision) // 1B SC = 1M SCC

	// AirdropPoolValue is the total amount of coins the pools get
	// airdrop so that they can pay out miners in the first 3600 blocks
	// OSV was AirdropPoolValue = NewCurrency64(51840000).Mul(ScPrimecoinPrecision)
	//Already included in AirdropSiaCloudValue
	AirdropPoolValue = NewCurrency64(126000).Mul(ScPrimecoinPrecision) //3600 SCC /day, 7 days, 5 pools

	// MaturityDelay specifies the number of blocks that a maturity-required output
	// is required to be on hold before it can be spent on the blockchain.
	// Outputs are maturity-required if they are highly likely to be altered or
	// invalidated in the event of a small reorg. One example is the block reward,
	// as a small reorg may invalidate the block reward. Another example is a siafund
	// payout, as a tiny reorg may change the value of the payout, and thus invalidate
	// any transactions spending the payout. File contract payouts also are subject to
	// a maturity delay.
	MaturityDelay BlockHeight

	// MaxTargetAdjustmentDown restrict how much the block difficulty is allowed to
	// change in a single step, which is important to limit the effect of difficulty
	// raising and lowering attacks.
	MaxTargetAdjustmentDown *big.Rat

	// MaxTargetAdjustmentUp restrict how much the block difficulty is allowed to
	// change in a single step, which is important to limit the effect of difficulty
	// raising and lowering attacks.
	MaxTargetAdjustmentUp *big.Rat

	// MedianTimestampWindow tells us how many blocks to look back when calculating
	// the median timestamp over the previous n blocks. The timestamp of a block is
	// not allowed to be less than or equal to the median timestamp of the previous n
	// blocks, where for ScPrime this number is typically 11.
	MedianTimestampWindow = uint64(11)

	// MinimumCoinbase is the minimum coinbase reward for a block.
	// The coinbase decreases in each block after the Genesis block,
	// but it will not decrease past MinimumCoinbase.
	MinimumCoinbase = uint64(30) //OSV = 30e3; 30,000 SC = 30 SC

	// Oak hardfork constants. Oak is the name of the difficulty algorithm for
	// ScPrime following a hardfork at block 135e3.

	// OakDecayDenom is the denominator for how much the total timestamp is decayed
	// each step.
	OakDecayDenom int64

	// OakDecayNum is the numerator for how much the total timestamp is decayed each
	// step.
	OakDecayNum int64

	// OakHardforkBlock is the height at which the hardfork to switch to the oak
	// difficulty adjustment algorithm is triggered.
	OakHardforkBlock BlockHeight

	// OakHardforkFixBlock is the height at which the hardfork to switch from the broken
	// oak difficulty adjustment algorithm to the fixed oak difficulty adjustment
	// algorithm is triggered.
	OakHardforkFixBlock BlockHeight

	// OakHardforkTxnSizeLimit is the maximum size allowed for a transaction, a change
	// which I believe was implemented simultaneously with the oak hardfork.
	OakHardforkTxnSizeLimit = uint64(64e3) // 64 KB

	// OakMaxBlockShift is the maximum number of seconds that the oak algorithm will shift
	// the difficulty.
	OakMaxBlockShift int64

	// OakMaxDrop is the drop is the maximum amount that the difficulty will drop each block.
	OakMaxDrop *big.Rat

	// OakMaxRise is the maximum amount that the difficulty will rise each block.
	OakMaxRise *big.Rat

	// RootDepth is the cumulative target of all blocks. The root depth is essentially
	// the maximum possible target, there have been no blocks yet, so there is no
	// cumulated difficulty yet.
	RootDepth = Target{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255}

	// RootTarget is the target for the genesis block - basically how much work needs
	// to be done in order to mine the first block. The difficulty adjustment algorithm
	// takes over from there.
	RootTarget Target

	// SiacoinPrecision is the number of base units in a siacoin. This constant is used
	// for mining rewards calculation and supported for compatibility with
	// existing 3rd party intergations.
	// DEPRECATED: Since February 2020 one scprimecoin equals 10^27 Hastings
	// Use the types.ScPrimecoinPrecision constant.
	//
	// The base unit for Bitcoin is called a satoshi. We call 10^8 satoshis a bitcoin,
	// even though the code itself only ever works with satoshis.
	SiacoinPrecision = NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil))

	// ScPrimecoinPrecision is the number of base units in a scprimecoin that is used
	// by clients (1 SCP = 10^27 H).
	ScPrimecoinPrecision = NewCurrency(new(big.Int).Exp(big.NewInt(10), big.NewInt(27), nil))

	// OldSiafundCount is the total number of Siafunds in existence before the SPF hardfork.
	OldSiafundCount = NewCurrency64(10000) // Not used

	// NewSiafundCount is the total number of Siafunds in existence after the SPF hardfork.
	NewSiafundCount = NewCurrency64(10000) //OSV = 30K ,NSV remains at 10K, Not used

	// NewerSiafundCount is the total number of Siafunds in existence after the second SPF hardfork.
	NewerSiafundCount = NewCurrency64(10000) //OSV was 200000000, NSV remains at 10K, Not used

	// FirstSiafundMul is multiplier for percentage of siacoins that is taxed from FileContracts
	// before the first SPF hardfork.
	FirstSiafundMul = int64(39)

	// FirstSiafundDiv is divider for percentage of siacoins that is taxed from FileContracts
	// before the first SPF hardfork.
	FirstSiafundDiv = int64(1000)

	// FirstSiafundPortion is the percentage of siacoins that is taxed from FileContracts before the first SPF hardfork.
	FirstSiafundPortion = big.NewRat(FirstSiafundMul, FirstSiafundDiv)

	// SecondSiafundMul is multiplier for percentage of siacoins that is taxed from FileContracts
	// between the first and the second SPF hardforks.
	SecondSiafundMul = int64(39) //OSV = 150, NSV is 39

	// SecondSiafundDiv is divider for percentage of siacoins that is taxed from FileContracts
	// between the first and the second SPF hardforks.
	SecondSiafundDiv = int64(1000)

	// SecondSiafundPortion is the percentage of siacoins that is taxed from FileContracts between the first and the second SPF hardforks.
	SecondSiafundPortion = big.NewRat(SecondSiafundMul, SecondSiafundDiv)

	// ThirdSiafundMul is multiplier for percentage of siacoins that is taxed from FileContracts
	// after the second SPF hardfork.
	ThirdSiafundMul = int64(39) //OSV = 150, NSV is 39

	// ThirdSiafundDiv is divider for percentage of siacoins that is taxed from FileContracts
	// after the second SPF hardfork.
	ThirdSiafundDiv = int64(1000)

	// ThirdSiafundPortion is the percentage of siacoins that is taxed from FileContracts after the second SPF hardfork.
	ThirdSiafundPortion = big.NewRat(ThirdSiafundMul, ThirdSiafundDiv)

	// TargetWindow is the number of blocks to look backwards when determining how much
	// time has passed vs. how many blocks have been created. It's only used in the old,
	// broken difficulty adjustment algorithm.
	TargetWindow BlockHeight
)

var (
	// TaxHardforkHeight is the height at which the tax hardfork occurred.
	TaxHardforkHeight = build.Select(build.Var{
		Dev:      BlockHeight(10),
		Standard: BlockHeight(math.MaxInt64), //max int64 so hardfork never happens
		Testing:  BlockHeight(10),
	}).(BlockHeight)

	// SpfAirdropHeight is the height of SPF airdrop.
	SpfAirdropHeight = build.Select(build.Var{
		Dev:      BlockHeight(20),
		Standard: BlockHeight(math.MaxInt64), //max int64 so hardfork never happens
		Testing:  BlockHeight(7200),
	}).(BlockHeight)

	// SpfHardforkHeight is the height of SPF hardfork.
	SpfHardforkHeight = build.Select(build.Var{
		Dev:      BlockHeight(100),
		Standard: BlockHeight(math.MaxInt64), //max int64 so hardfork never happens
		Testing:  BlockHeight(10000),
	}).(BlockHeight)

	// SpfSecondHardforkHeight is the height of second SPF hardfork.
	SpfSecondHardforkHeight = build.Select(build.Var{
		Dev:      BlockHeight(200),
		Standard: BlockHeight(math.MaxInt64), //max int64 so hardfork never happens
		Testing:  BlockHeight(20000),
	}).(BlockHeight)
)

// IsSpfHardfork returns true when one of Spf hardforks happens at given height.
func IsSpfHardfork(height BlockHeight) bool {
	if height == SpfHardforkHeight {
		return true
	}
	if height == SpfSecondHardforkHeight {
		return true
	}
	return false
}

// SiafundCount returns the total number of Siafunds by height.
func SiafundCount(height BlockHeight) Currency {
	if height > SpfSecondHardforkHeight {
		return NewerSiafundCount
	}
	if height > SpfHardforkHeight {
		return NewSiafundCount
	}
	return OldSiafundCount
}

// SiafundPortion returns SPF percentage by height.
func SiafundPortion(height BlockHeight) *big.Rat {
	if height > SpfSecondHardforkHeight {
		return ThirdSiafundPortion
	}
	if height > SpfHardforkHeight {
		return SecondSiafundPortion
	}
	return FirstSiafundPortion
}

// SiafundMul returns SPF percentage multiplier by height.
func SiafundMul(height BlockHeight) int64 {
	if height > SpfSecondHardforkHeight {
		return ThirdSiafundMul
	}
	if height > SpfHardforkHeight {
		return SecondSiafundMul
	}
	return FirstSiafundMul
}

// SiafundDiv returns SPF percentage divider by height.
func SiafundDiv(height BlockHeight) int64 {
	if height > SpfSecondHardforkHeight {
		return ThirdSiafundDiv
	}
	if height > SpfHardforkHeight {
		return SecondSiafundDiv
	}
	return FirstSiafundDiv
}

// scanAddress scans a types.UnlockHash from a string.
func scanAddress(addrStr string) (addr UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	if err != nil {
		return UnlockHash{}, err
	}
	return addr, nil
}

// UnlockHashFromAddrStr convert string address to UnlockHash
func UnlockHashFromAddrStr(addrStr string) (addr UnlockHash) {
	dest, err := scanAddress(addrStr)
	if err != nil {
		return UnlockHash{}
	}
	return dest
}

// init checks which build constant is in place and initializes the variables
// accordingly.
func init() {
	if build.Release == "dev" {
		// 'dev' settings are for small developer testnets, usually on the same
		// computer. Settings are slow enough that a small team of developers
		// can coordinate their actions over a the developer testnets, but fast
		// enough that there isn't much time wasted on waiting for things to
		// happen.
		ASICHardforkHeight = math.MaxInt64 //max int64 so hardfork never happens
		ASICHardforkTotalTarget = Target{0, 0, 0, 8}
		ASICHardforkTotalTime = 800

		BlockFrequency = 12                      // 12 seconds: slow enough for developers to see ~each block, fast enough that blocks don't waste time.
		MaturityDelay = 10                       // 60 seconds before a delayed output matures.
		GenesisTimestamp = Timestamp(1528293910) // Change as necessary.
		RootTarget = Target{0, 0, 2}             // Standard developer CPUs will be able to mine blocks with the race library activated.

		TargetWindow = 20                              // Difficulty is adjusted based on prior 20 blocks.
		MaxTargetAdjustmentUp = big.NewRat(120, 100)   // Difficulty adjusts quickly.
		MaxTargetAdjustmentDown = big.NewRat(100, 120) // Difficulty adjusts quickly.
		FutureThreshold = 2 * 60                       // 2 minutes.
		ExtremeFutureThreshold = 4 * 60                // 4 minutes.

		MinimumCoinbase = 30e3 //30e3 SC = 30 SCC

		OakHardforkBlock = 100
		OakHardforkFixBlock = 105
		OakDecayNum = 985
		OakDecayDenom = 1000
		OakMaxBlockShift = 3
		OakMaxRise = big.NewRat(102, 100)
		OakMaxDrop = big.NewRat(100, 102)

		GenesisAirdropAllocation = []SiacoinOutput{
			{
				Value:      AirdropCommunityValue,
				UnlockHash: UnlockHashFromAddrStr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			},
			{
				Value:      AirdropPoolValue,
				UnlockHash: UnlockHashFromAddrStr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			},
			{
				Value:      AirdropSiaPrimeValue,
				UnlockHash: UnlockHashFromAddrStr("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
			},
			{
				Value:      AirdropSiaCloudValue,
				UnlockHash: UnlockHashFromAddrStr("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
			},
		}

		GenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(10000),
				UnlockHash: UnlockHashFromAddrStr("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
			},
		}
		ForkedGenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(10000),
				UnlockHash: UnlockHashFromAddrStr("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			},
		}
		SiafundHardforkAllocation = map[BlockHeight][]SiafundOutput{
			SpfHardforkHeight: {
				{
					Value:      NewCurrency64(20000),
					UnlockHash: UnlockHashFromAddrStr("bbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaaaaaa"),
				},
			},
			SpfSecondHardforkHeight: {
				{
					Value:      NewCurrency64(50000 - 30000),
					UnlockHash: UnlockHashFromAddrStr("aaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaa"),
				},
			},
		}
	} else if build.Release == "testing" {
		// 'testing' settings are for automatic testing, and create much faster
		// environments than a human can interact with.
		ASICHardforkHeight = math.MaxInt64 //max int64 so hardfork never happens
		ASICHardforkTotalTarget = Target{255, 255}
		ASICHardforkTotalTime = 10e3

		BlockFrequency = 1 // As fast as possible
		MaturityDelay = 3
		GenesisTimestamp = CurrentTimestamp() - 1e6
		RootTarget = Target{128} // Takes an expected 2 hashes; very fast for testing but still probes 'bad hash' code.

		// A restrictive difficulty clamp prevents the difficulty from climbing
		// during testing, as the resolution on the difficulty adjustment is
		// only 1 second and testing mining should be happening substantially
		// faster than that.
		TargetWindow = 200
		MaxTargetAdjustmentUp = big.NewRat(10001, 10000)
		MaxTargetAdjustmentDown = big.NewRat(9999, 10000)
		FutureThreshold = 3        // 3 seconds
		ExtremeFutureThreshold = 6 // 6 seconds

		MinimumCoinbase = 299990 // Minimum coinbase is hit after 10 blocks to make testing minimum-coinbase code easier.

		// Do not let the difficulty change rapidly - blocks will be getting
		// mined far faster than the difficulty can adjust to.
		OakHardforkBlock = 20
		OakHardforkFixBlock = 23
		OakDecayNum = 9999
		OakDecayDenom = 10e3
		OakMaxBlockShift = 3
		OakMaxRise = big.NewRat(10001, 10e3)
		OakMaxDrop = big.NewRat(10e3, 10001)

		GenesisAirdropAllocation = []SiacoinOutput{
			{
				Value:      AirdropCommunityValue,
				UnlockHash: UnlockHashFromAddrStr("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			},
			{
				Value:      AirdropPoolValue,
				UnlockHash: UnlockHashFromAddrStr("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			},
			{
				Value:      AirdropSiaPrimeValue,
				UnlockHash: UnlockHashFromAddrStr("cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
			},
			{
				Value:      AirdropSiaCloudValue,
				UnlockHash: UnlockHashFromAddrStr("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
			},
		}

		GenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(10000),
				UnlockHash: UnlockHashFromAddrStr("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
			},
		}
		ForkedGenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(10000),
				UnlockHash: UnlockHashFromAddrStr("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			},
		}
		SiafundHardforkAllocation = map[BlockHeight][]SiafundOutput{
			SpfHardforkHeight: {
				{
					Value:      NewCurrency64(20000),
					UnlockHash: UnlockHashFromAddrStr("bbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaabbbbaaaaaaaaaaaa"),
				},
			},
			SpfSecondHardforkHeight: {
				{
					Value:      NewCurrency64(50000 - 30000),
					UnlockHash: UnlockHashFromAddrStr("aaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaaccccaaaa"),
				},
			},
		}

	} else if build.Release == "standard" {
		// 'standard' settings are for the full network. They are slow enough
		// that the network is secure in a real-world byzantine environment.

		// A hardfork height of max int64 was chosen to clarify that the we
		// expect the hardfork to never happen on the ScPrime blockchain.
		// A total time of 120,000 is chosen because that represents the total
		// time elapsed at a perfect equilibrium, indicating a visible average
		// block time that perfectly aligns with what is expected. A total
		// target of 67 leading zeroes is chosen because that aligns with the
		// amount of hashrate that we expect to be on the network after the
		// hardfork.
		ASICHardforkHeight = math.MaxInt64 //max int64 so hardfork never happens
		ASICHardforkTotalTarget = Target{0, 0, 0, 0, 0, 0, 0, 0, 32}
		ASICHardforkTotalTime = 120e3

		// A block time of 1 block per 10 minutes is chosen to follow Bitcoin's
		// example. The security lost by lowering the block time is not
		// insignificant, and the convenience gained by lowering the blocktime
		// even down to 90 seconds is not significant. I do feel that 10
		// minutes could even be too short, but it has worked well for Bitcoin.
		BlockFrequency = 720 //OSV was 600

		// Payouts take 1 day to mature. This is to prevent a class of double
		// spending attacks parties unintentionally spend coins that will stop
		// existing after a blockchain reorganization. There are multiple
		// classes of payouts in ScPrime that depend on a previous block - if that
		// block changes, then the output changes and the previously existing
		// output ceases to exist. This delay stops both unintentional double
		// spending and stops a small set of long-range mining attacks.
		MaturityDelay = 120 //OSV was 144

		// The genesis timestamp is set to June 6th, because that is when the
		// 100-block developer premine started. The trailing zeroes are a
		// bonus, and make the timestamp easier to memorize.
		GenesisTimestamp = Timestamp(1650179928) //OSV was 1540955779

		// The RootTarget was set such that the developers could reasonable
		// premine 100 blocks in a day. It was known to the developers at launch
		// this this was at least one and perhaps two orders of magnitude too
		// small.
		RootTarget = Target{0, 0, 0, 0, 0, 128} //OSV was 0, 0, 0, 0, 0, 0, 2

		// When the difficulty is adjusted, it is adjusted by looking at the
		// timestamp of the 1000th previous block. This minimizes the abilities
		// of miners to attack the network using rogue timestamps.
		TargetWindow = 500 //OSV was 1e3, new is 500

		// The difficulty adjustment is clamped to 2.5x every 500 blocks. This
		// corresponds to 6.25x every 2 weeks, which can be compared to
		// Bitcoin's clamp of 4x every 2 weeks. The difficulty clamp is
		// primarily to stop difficulty raising attacks. ScPrime's safety margin is
		// similar to Bitcoin's despite the looser clamp because ScPrime's
		// difficulty is adjusted four times as often. This does result in
		// greater difficulty oscillation, a tradeoff that was chosen to be
		// acceptable due to ScPrime's more vulnerable position as an altcoin.
		MaxTargetAdjustmentUp = big.NewRat(25, 10)
		MaxTargetAdjustmentDown = big.NewRat(10, 25)

		// Blocks will not be accepted if their timestamp is more than 3 hours
		// into the future, but will be accepted as soon as they are no longer
		// 3 hours into the future. Blocks that are greater than 5 hours into
		// the future are rejected outright, as it is assumed that by the time
		// 2 hours have passed, those blocks will no longer be on the longest
		// chain. Blocks cannot be kept forever because this opens a DoS
		// vector.
		FutureThreshold = 3 * 60 * 60        // 3 hours.
		ExtremeFutureThreshold = 5 * 60 * 60 // 5 hours.

		// The minimum coinbase is set to 10,000. Because the coinbase
		// decreases by 1 every time, it means that ScPrime's coinbase will have an
		// increasingly potent dropoff for about 5 years, until inflation more
		// or less permanently settles around 2%.
		MinimumCoinbase = 30 //OSV was 10e3; NSV for 12 min blks; 30e3 SC = 30 SCC

		// The oak difficulty adjustment hardfork is set to trigger at block
		// 135,000, which is just under 6 months after the hardfork was first
		// released as beta software to the network. This hopefully gives
		// everyone plenty of time to upgrade and adopt the hardfork, while also
		// being earlier than the most optimistic shipping dates for the miners
		// that would otherwise be very disruptive to the network.
		//
		// There was a bug in the original Oak hardfork that had to be quickly
		// followed up with another fix. The height of that fix is the
		// OakHardforkFixBlock.
		OakHardforkBlock = 1500
		OakHardforkFixBlock = 1500

		// The decay is kept at 995/1000, or a decay of about 0.5% each block.
		// This puts the halflife of a block's relevance at about 1 day. This
		// allows the difficulty to adjust rapidly if the hashrate is adjusting
		// rapidly, while still keeping a relatively strong insulation against
		// random variance.
		OakDecayNum = 995
		OakDecayDenom = 1e3

		// The block shift determines the most that the difficulty adjustment
		// algorithm is allowed to shift the target block time. With a block
		// frequency of 600 seconds, the min target block time is 200 seconds,
		// and the max target block time is 1800 seconds.
		OakMaxBlockShift = 3

		// The max rise and max drop for the difficulty is kept at 0.4% per
		// block, which means that in 1008 blocks the difficulty can move a
		// maximum of about 55x. This is significant, and means that dramatic
		// hashrate changes can be responded to quickly, while still forcing an
		// attacker to do a significant amount of work in order to execute a
		// difficulty raising attack, and minimizing the chance that an attacker
		// can get lucky and fake a ton of work.
		OakMaxRise = big.NewRat(1004, 1e3)
		OakMaxDrop = big.NewRat(1e3, 1004)

		GenesisAirdropAllocation = []SiacoinOutput{

			{
				Value:      AirdropInvestorValue,
				UnlockHash: UnlockHashFromAddrStr("303eae70f9e2b2b8bce1e66907551aeeae37c7ed105806b73c4d0301280c9ee811faa138d451"),
			},
			{
				Value:      AirdropCommunityValue,
				UnlockHash: UnlockHashFromAddrStr("ca1bf328d72169507dc3aee97e2a6c40b98ecc70b582f91fba96fbcf3c11d55c88d0a37d6449"),
			},
			{
				Value:      AirdropDevCompValue,
				UnlockHash: UnlockHashFromAddrStr("d7340084bbbf84137158df5e23423b57a504025c360b51777020921036e0cfca09f83da6b654"),
			},
			{
				Value:      AirdropSiaCloudValue,
				UnlockHash: UnlockHashFromAddrStr("21d94ad73315437219381b5dc23bbe6f5c1c8dd5c77e1fc6ee5d15f2b615649fe4e3aa19220a"),
			},
			{
				Value:      AirdropSiaPrimeValue,
				UnlockHash: UnlockHashFromAddrStr("e8ba42de3797e34001e953b6549e5706803c3584d558fed9920ca207bbbcd839d170f2c7f5b5"),
			},
			{
				Value:      AirdropPoolValue,
				UnlockHash: UnlockHashFromAddrStr("2ca34530025d8ce2b6f6dd3f30f44031cf8c411d884b54b06039b68e9660356b07ae09ebf58b"),
			},
			{
				Value:      NewCurrency64(1), // to DevFund address
				UnlockHash: UnlockHashFromAddrStr("cc05f0024d75d23002dfed47bcd6177c0e5dd652b146758951806bb8fb15fa158da76dcbaeb6"),
			},
			{
				Value:      NewCurrency64(1), // to Burn address
				UnlockHash: UnlockHashFromAddrStr("e3a19a1c70d75dc865f5ce539e0858e2450aa61b7136d8f25b0b800d52177919557208b73316"),
			},
		}

		ForkedGenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(0), //not used
				UnlockHash: UnlockHashFromAddrStr("d3c053d66f239564fa5d09d8619958c67e71a154ac24870e16d99524ef10119e67e7aee741d8"),
			},
		}
		SiafundHardforkAllocation = map[BlockHeight][]SiafundOutput{
			SpfHardforkHeight: {
				{
					Value:      NewCurrency64(0), //not used
					UnlockHash: UnlockHashFromAddrStr("77422e40c3c126120394f52edc3f414a3e80b1961abe20687fd4760c6989aefe13e93d163133"),
				},
			},
			SpfSecondHardforkHeight: {
				{
					Value:      NewCurrency64(0), //not used
					UnlockHash: UnlockHashFromAddrStr("61ac1f6e01315a2650203ba320bb935a3a427a3dfddc23cb129ab072f5d89a3091661e28b360"),
				},
			},
		}

		GenesisSiafundAllocation = []SiafundOutput{
			{
				Value:      NewCurrency64(10000), //NSV puts all 10K sf into one addr // Not used
				UnlockHash: UnlockHashFromAddrStr("e5b3a18d32c9bc89c6712d647ef85c2771fd1efe49b3e66280b3a724ec9d73537b1979e6b678"),
			},
		}
	}

	// Create the genesis block.
	GenesisBlock = Block{
		Timestamp: GenesisTimestamp,
		Transactions: []Transaction{
			{SiacoinOutputs: GenesisAirdropAllocation},
			{SiafundOutputs: GenesisSiafundAllocation},
		},
	}
	// Calculate the genesis ID.
	GenesisID = GenesisBlock.ID()
}
