package dependencies

import "gitlab.com/SiaPrime/SiaPrime/modules"

type (

	// DependencyUnsyncedConsensus makes the consensus set appear unsynced
	DependencyUnsyncedConsensus struct {
		modules.ProductionDependencies
	}
)

// Disrupt will prevent the consensus set from appearing synced
func (d *DependencyUnsyncedConsensus) Disrupt(s string) bool {
	return s == "UnsyncedConsensus"
}
