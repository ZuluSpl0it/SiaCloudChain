package wallet

import (
	"fmt"

	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/persist"
	"gitlab.com/SiaPrime/SiaPrime/types"
)

const scanMultiplier = 4 // how many more keys to generate after each scan iteration

var errMaxKeys = fmt.Errorf("refused to generate more than %v keys from seed", maxScanKeys)

// maxScanKeys is the number of maximum number of keys the seedScanner will
// generate before giving up.
var maxScanKeys = func() uint64 {
	switch build.Release {
	case "dev":
		return 1e6
	case "standard":
		return 100e6
	case "testing":
		return 100e3
	default:
		panic("unrecognized build.Release")
	}
}()

// numInitialKeys is the number of keys generated by the seedScanner before
// scanning the blockchain for the first time.
var numInitialKeys = func() uint64 {
	switch build.Release {
	case "dev":
		return 10e3
	case "standard":
		return 1e6
	case "testing":
		return 1e3
	default:
		panic("unrecognized build.Release")
	}
}()

// A scannedOutput is an output found in the blockchain that was generated
// from a given seed.
type scannedOutput struct {
	id        types.OutputID
	value     types.Currency
	seedIndex uint64
}

// A seedScanner scans the blockchain for addresses that belong to a given
// seed.
type seedScanner struct {
	dustThreshold    types.Currency              // minimum value of outputs to be included
	keys             map[types.UnlockHash]uint64 // map address to seed index
	largestIndexSeen uint64                      // largest index that has appeared in the blockchain
	scannedHeight    types.BlockHeight
	seed             modules.Seed
	siacoinOutputs   map[types.SiacoinOutputID]scannedOutput
	siafundOutputs   map[types.SiafundOutputID]scannedOutput

	log *persist.Logger
}

func (s *seedScanner) numKeys() uint64 {
	return uint64(len(s.keys))
}

// generateKeys generates n additional keys from the seedScanner's seed.
func (s *seedScanner) generateKeys(n uint64) {
	initialProgress := s.numKeys()
	for i, k := range generateKeys(s.seed, initialProgress, n) {
		s.keys[k.UnlockConditions.UnlockHash()] = initialProgress + uint64(i)
	}
}

// ProcessConsensusChange scans the blockchain for information relevant to the
// seedScanner.
func (s *seedScanner) ProcessConsensusChange(cc modules.ConsensusChange) {
	// update outputs
	for _, diff := range cc.SiacoinOutputDiffs {
		if diff.Direction == modules.DiffApply {
			if index, exists := s.keys[diff.SiacoinOutput.UnlockHash]; exists && diff.SiacoinOutput.Value.Cmp(s.dustThreshold) > 0 {
				s.siacoinOutputs[diff.ID] = scannedOutput{
					id:        types.OutputID(diff.ID),
					value:     diff.SiacoinOutput.Value,
					seedIndex: index,
				}
			}
		} else if diff.Direction == modules.DiffRevert {
			// NOTE: DiffRevert means the output was either spent or was in a
			// block that was reverted.
			if _, exists := s.keys[diff.SiacoinOutput.UnlockHash]; exists {
				delete(s.siacoinOutputs, diff.ID)
			}
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		if diff.Direction == modules.DiffApply {
			// do not compare against dustThreshold here; we always want to
			// sweep every siafund found
			if index, exists := s.keys[diff.SiafundOutput.UnlockHash]; exists {
				s.siafundOutputs[diff.ID] = scannedOutput{
					id:        types.OutputID(diff.ID),
					value:     diff.SiafundOutput.Value,
					seedIndex: index,
				}
			}
		} else if diff.Direction == modules.DiffRevert {
			// NOTE: DiffRevert means the output was either spent or was in a
			// block that was reverted.
			if _, exists := s.keys[diff.SiafundOutput.UnlockHash]; exists {
				delete(s.siafundOutputs, diff.ID)
			}
		}
	}

	// update s.largestIndexSeen
	for _, diff := range cc.SiacoinOutputDiffs {
		index, exists := s.keys[diff.SiacoinOutput.UnlockHash]
		if exists {
			s.log.Debugln("Seed scanner found a key used at index", index)
			if index > s.largestIndexSeen {
				s.largestIndexSeen = index
			}
		}
	}
	for _, diff := range cc.SiafundOutputDiffs {
		index, exists := s.keys[diff.SiafundOutput.UnlockHash]
		if exists {
			s.log.Debugln("Seed scanner found a key used at index", index)
			if index > s.largestIndexSeen {
				s.largestIndexSeen = index
			}
		}
	}
	// Adjust the scanned height and print the scan progress.
	s.scannedHeight += types.BlockHeight(len(cc.AppliedBlocks) - len(cc.RevertedBlocks))
	if !cc.Synced {
		print("\rWallet: scanned to height ", s.scannedHeight, "...")
	} else {
		println("\nDone!")
	}
}

// scan subscribes s to cs and scans the blockchain for addresses that belong
// to s's seed. If scan returns errMaxKeys, additional keys may need to be
// generated to find all the addresses.
func (s *seedScanner) scan(cs modules.ConsensusSet, cancel <-chan struct{}) error {
	// generate a bunch of keys and scan the blockchain looking for them. If
	// none of the 'upper' half of the generated keys are found, we are done;
	// otherwise, generate more keys and try again (bounded by a sane
	// default).
	//
	// NOTE: since scanning is very slow, we aim to only scan once, which
	// means generating many keys.
	numKeys := numInitialKeys
	for s.numKeys() < maxScanKeys {
		s.generateKeys(numKeys)
		if err := cs.ConsensusSetSubscribe(s, modules.ConsensusChangeBeginning, cancel); err != nil {
			return err
		}
		cs.Unsubscribe(s)
		if s.largestIndexSeen < s.numKeys()/2 {
			return nil
		}
		// increase number of keys generated each iteration, capping so that
		// we do not exceed maxScanKeys
		numKeys *= scanMultiplier
		if numKeys > maxScanKeys-s.numKeys() {
			numKeys = maxScanKeys - s.numKeys()
		}
	}
	return errMaxKeys
}

// newSeedScanner returns a new seedScanner.
func newSeedScanner(seed modules.Seed, log *persist.Logger) *seedScanner {
	return &seedScanner{
		seed:           seed,
		keys:           make(map[types.UnlockHash]uint64, numInitialKeys),
		siacoinOutputs: make(map[types.SiacoinOutputID]scannedOutput),
		siafundOutputs: make(map[types.SiafundOutputID]scannedOutput),

		log: log,
	}
}
