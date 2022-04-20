package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/config"
)

var (
	// globalConfig is used by the cobra package to fill out the configuration
	// variables.
	globalConfig Config
)

// exit codes
// inspired by sysexits.h
const (
	exitCodeGeneral = 1  // Not in sysexits.h, but is standard practice.
	exitCodeUsage   = 64 // EX_USAGE in sysexits.h
)

// The Config struct contains all configurable variables for spd. It is
// compatible with gcfg.
type Config struct {
	// The APIPassword is input by the user after the daemon starts up, if the
	// --authenticate-api flag is set.
	APIPassword string

	// The Spd variables are referenced directly by cobra, and are set
	// according to the flags.
	Spd struct {
		APIaddr       string
		RPCaddr       string
		HostAddr      string
		SiaMuxTCPAddr string
		SiaMuxWSAddr  string
		HostApiAddr   string
		AllowAPIBind  bool

		Modules           string
		NoBootstrap       bool
		RequiredUserAgent string
		AuthenticateAPI   bool
		TempPassword      bool

		Profile    string
		ProfileDir string
		DataDir    string
	}

	MiningPoolConfig config.MiningPoolConfig
	IndexConfig      config.IndexConfig
}

// die prints its arguments to stderr, then exits the program with the default
// error code.
func die(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(exitCodeGeneral)
}

// versionCmd is a cobra command that prints the version of spd.
func versionCmd(*cobra.Command, []string) {
	switch build.Release {
	case "dev":
		fmt.Println("ScPrime Daemon v" + build.Version + "-dev")
	case "standard":
		fmt.Println("ScPrime Daemon v" + build.Version)
	case "testing":
		fmt.Println("ScPrime Daemon v" + build.Version + "-testing")
	default:
		fmt.Println("ScPrime Daemon v" + build.Version + "-???")
	}
}

// modulesCmd is a cobra command that prints help info about modules.
func modulesCmd(*cobra.Command, []string) {
	fmt.Println(`Use the -M or --modules flag to only run specific modules. Modules are
independent components of ScPrime. This flag should only be used by developers or
people who want to reduce overhead from unused modules. Modules are specified by
their first letter. If the -M or --modules flag is not specified the default
modules are run. The default modules are:
	gateway, consensus set, host, miner, renter, transaction pool, wallet
This is equivalent to:
	spd -M cghmrtw
Below is a list of all the modules available.

Gateway (g):
	The gateway maintains a peer to peer connection to the network and
	enables other modules to perform RPC calls on peers.
	The gateway is required by all other modules.
	Example:
		spd -M g
Consensus Set (c):
	The consensus set manages everything related to consensus and keeps the
	blockchain in sync with the rest of the network.
	The consensus set requires the gateway.
	Example:
		spd -M gc
Transaction Pool (t):
	The transaction pool manages unconfirmed transactions.
	The transaction pool requires the consensus set.
	Example:
		spd -M gct
Wallet (w):
	The wallet stores and manages scprimecoins and scprimefunds.
	The wallet requires the consensus set and transaction pool.
	Example:
		spd -M gctw
Renter (r):
	The renter manages the user's files on the network.
	The renter requires the consensus set, transaction pool, and wallet.
	Example:
		spd -M gctwr
Host (h):
	The host provides storage from local disks to the network. The host
	negotiates file contracts with remote renters to earn money for storing
	other users' files.
	The host requires the consensus set, transaction pool, and wallet.
	Example:
		spd -M gctwh
Miner (m):
	The miner provides a basic CPU mining implementation as well as an API
	for external miners to use.
	The miner requires the consensus set, transaction pool, and wallet.
	Example:
		spd -M gctwm
Mining Pool (p):
	The pool provides a decentralized pool as well as an API for external
	clients (web pages) to access for user stats.
	The pool requires the gateway,consensus set, transactions pool and wallet.
	Example:
		spd -M gctwp
Explorer (e):
	The explorer provides statistics about the blockchain and can be
	queried for information about specific transactions or other objects on
	the blockchain.
	The explorer requires the consenus set.
	Example:
		spd -M gce
Stratum Miner (s):
	The stratum miner provides a CPU stratum mining client as well an as API
	for monitoring statistics and controlling the miner.
	The stratum miner requires no other modules to run.
	Example:
		spd -M s`)
}

// main establishes a set of commands and flags using the cobra package.
func main() {
	if build.DEBUG {
		fmt.Println("Running with debugging enabled")
	}
	root := &cobra.Command{
		Use:   os.Args[0],
		Short: "ScPrime Daemon v" + build.Version,
		Long:  "ScPrime Daemon v" + build.Version,
		Run:   startDaemonCmd,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information about the ScPrime Daemon",
		Run:   versionCmd,
	})

	root.AddCommand(&cobra.Command{
		Use:   "modules",
		Short: "List available modules for use with -M, --modules flag",
		Long:  "List available modules for use with -M, --modules flag and their uses",
		Run:   modulesCmd,
	})

	// Set default values, which have the lowest priority.
	root.Flags().StringVarP(&globalConfig.Spd.RequiredUserAgent, "agent", "", "ScPrime-Agent", "required substring for the user agent")
	root.Flags().StringVarP(&globalConfig.Spd.HostAddr, "host-addr", "", ":4282", "which port the host listens on")
	root.Flags().StringVarP(&globalConfig.Spd.ProfileDir, "profile-directory", "", "profiles", "location of the profiling directory")
	root.Flags().StringVarP(&globalConfig.Spd.APIaddr, "api-addr", "", "localhost:4280", "which host:port the API server listens on")
	root.Flags().StringVarP(&globalConfig.Spd.DataDir, "scprime-directory", "d", "", "location of the scprime daemon metadata directory")
	root.Flags().BoolVarP(&globalConfig.Spd.NoBootstrap, "no-bootstrap", "", false, "disable bootstrapping on this run")
	root.Flags().StringVarP(&globalConfig.Spd.Profile, "profile", "", "", "enable profiling with flags 'cmt' for CPU, memory, trace")
	root.Flags().StringVarP(&globalConfig.Spd.RPCaddr, "rpc-addr", "", ":4281", "which port the gateway listens on")
	root.Flags().StringVarP(&globalConfig.Spd.SiaMuxTCPAddr, "siamux-addr", "", ":4283", "which port the SiaMux listens on")
	root.Flags().StringVarP(&globalConfig.Spd.SiaMuxWSAddr, "siamux-addr-ws", "", ":4284", "which port the SiaMux websocket listens on")
	root.Flags().StringVarP(&globalConfig.Spd.HostApiAddr, "host-api-addr", "", ":4285", "which port the Host API listens on")
	root.Flags().StringVarP(&globalConfig.Spd.Modules, "modules", "M", "gctwrh", "enabled modules, see 'spd modules' for more info")
	root.Flags().BoolVarP(&globalConfig.Spd.AuthenticateAPI, "authenticate-api", "", true, "enable API password protection")
	root.Flags().BoolVarP(&globalConfig.Spd.TempPassword, "temp-password", "", false, "enter a temporary API password during startup")
	root.Flags().BoolVarP(&globalConfig.Spd.AllowAPIBind, "disable-api-security", "", false, "allow spd to listen on a non-localhost address (DANGEROUS)")

	// If globalConfig.Spd.DataDir is not set, use the environment variable provided.
	if globalConfig.Spd.DataDir == "" {
		globalConfig.Spd.DataDir = build.SiaDir()
	}

	// Check for existence of datadir and move it if not found
	// The only possible scenario when this move is expected is when the target
	// is the default datadir (not specified) && it does not exist yet && there
	// exists a directory at the old default path.
	targetpath := globalConfig.Spd.DataDir
	if targetpath == build.DefaultMetadataDir() {
		_, err := os.Stat(targetpath)
		if os.IsNotExist(err) { //Metadata directory does not exist
			//Check for pre v1.4.3 default data directory
			oldpath := build.DefaultSiaPrimeDir()
			info, err := os.Stat(oldpath)
			if err == nil && info.IsDir() {
				fmt.Printf("Migrating metadata from %v to %v\n", oldpath, targetpath)
				err = os.Rename(oldpath, targetpath)
				if err != nil {
					//Panic, there is old metadata at the old default path but it can not be moved.
					die("Can not move existing old SiaPrime metadata directory to new default path", err)
				} else {
					fmt.Printf("Done moving data from %v to %v\n", oldpath, targetpath)
				}
			}
		}
	}

	// Parse cmdline flags, overwriting both the default values and the config
	// file values.
	if err := root.Execute(); err != nil {
		// Since no commands return errors (all commands set Command.Run instead of
		// Command.RunE), Command.Execute() should only return an error on an
		// invalid command or flag. Therefore Command.Usage() was called (assuming
		// Command.SilenceUsage is false) and we should exit with exitCodeUsage.
		os.Exit(exitCodeUsage)
	}
}
