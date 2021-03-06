package node

import "time"

// templates.go contains a bunch of sane default templates that you can use to
// create ScPrime nodes.

var (
	// AllModulesTemplate is a template for a ScPrime node that has all modules
	// enabled.
	AllModulesTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false, // TODO: Implement explorer.
		CreateGateway:                 true,
		CreateHost:                    true,
		CreateMiner:                   true,
		CreateRenter:                  true,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// FeeManagerTemplate is a template for a ScPrime node that has a functioning
	// FeeManager. The node has a FeeManager and all dependencies, but no other
	// modules.
	FeeManagerTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    false,
		CreateMiner:                   false,
		CreateRenter:                  false,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// GatewayTemplate is a template for a ScPrime node that has a functioning
	// Gateway. The node has a Gateway but no other modules.
	GatewayTemplate = NodeParams{
		CreateConsensusSet:            false,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    false,
		CreateMiner:                   false,
		CreateRenter:                  false,
		CreateTransactionPool:         false,
		CreateWallet:                  false,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// HostTemplate is a template for a ScPrime node that has a functioning host.
	// The node has a host and all dependencies, but no other modules.
	HostTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    true,
		CreateMiner:                   false,
		CreateRenter:                  false,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// MinerTemplate is a template for a ScPrime node that has a functioning miner.
	// The node has a miner and all dependencies, but no other modules.
	MinerTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    false,
		CreateMiner:                   true,
		CreateRenter:                  false,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// RenterTemplate is a template for a ScPrime node that has a functioning
	// renter. The node has a renter and all dependencies, but no other
	// modules.
	RenterTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    false,
		CreateMiner:                   false,
		CreateRenter:                  true,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
	// WalletTemplate is a template for a ScPrime node that has a functioning
	// wallet. The node has a wallet and all dependencies, but no other
	// modules.
	WalletTemplate = NodeParams{
		CreateConsensusSet:            true,
		CreateExplorer:                false,
		CreateGateway:                 true,
		CreateHost:                    false,
		CreateMiner:                   false,
		CreateRenter:                  false,
		CreateTransactionPool:         true,
		CreateWallet:                  true,
		CheckTokenExpirationFrequency: 5 * time.Second,
	}
)

// AllModules returns an AllModulesTemplate filled out with the provided dir.
func AllModules(dir string) NodeParams {
	template := AllModulesTemplate
	template.Dir = dir
	return template
}

// Gateway returns a GatewayTemplate filled out with the provided dir.
func Gateway(dir string) NodeParams {
	template := GatewayTemplate
	template.Dir = dir
	return template
}

// Host returns a HostTemplate filled out with the provided dir.
func Host(dir string) NodeParams {
	template := HostTemplate
	template.Dir = dir
	template.HostAPIAddr = ":0"
	return template
}

// Miner returns a MinerTemplate filled out with the provided dir.
func Miner(dir string) NodeParams {
	template := MinerTemplate
	template.Dir = dir
	return template
}

// Renter returns a RenterTemplate filled out with the provided dir.
func Renter(dir string) NodeParams {
	template := RenterTemplate
	template.Dir = dir
	return template
}

// Wallet returns a WalletTemplate filled out with the provided dir.
func Wallet(dir string) NodeParams {
	template := WalletTemplate
	template.Dir = dir
	return template
}
