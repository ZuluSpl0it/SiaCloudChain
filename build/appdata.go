package build

import (
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitlab.com/NebulousLabs/fastrand"
)

var (
	// siaAPIPassword is the environment variable that sets a custom API
	// password if the default is not used
	siaAPIPassword = "SCPRIME_API_PASSWORD"

	// siaDataDir is the environment variable that tells siad where to put the
	// sia data
	siaDataDir = "SCPRIME_DATA_DIR"

	// siaWalletPassword is the environment variable that can be set to enable
	// auto unlocking the wallet
	siaWalletPassword = "SCPRIME_WALLET_PASSWORD"
)

// APIPassword returns the Sia API Password either from the environment variable
// or from the password file. If no environment variable is set and no file
// exists, a password file is created and that password is returned
func APIPassword() (string, error) {
	// Check the environment variable.
	pw := os.Getenv(siaAPIPassword)
	if pw != "" {
		return pw, nil
	}

	// Try to read the password from disk.
	path := apiPasswordFilePath()
	pwFile, err := ioutil.ReadFile(path)
	if err == nil {
		// This is the "normal" case, so don't print anything.
		return strings.TrimSpace(string(pwFile)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// No password file; generate a secure one.
	// Generate a password file.
	pw, err = createAPIPasswordFile()
	if err != nil {
		return "", err
	}
	return pw, nil
}

// SiaDir returns the Sia data directory either from the environment variable or
// the default.
func SiaDir() string {
	siaDir := os.Getenv(siaDataDir)
	if siaDir == "" {
		siaDir = defaultSiaDir()
		// Metadata directory not specified
		// check for presence and look for the old default if not found
		needMigrate := !dirExists(build.DefaultMetadataDir()) && dirExists(defaultSiaPrimeDir())
		if needMigrate {
			if err := migrateDataDir(); err != nil {
				fmt.Printf("Error moving default metadata directory: %v\n", err)
				os.Exit(exitCodeGeneral)
			}
		}

	} else {
		fmt.Printf("Using %v environment variable\n", cmd.SiaDataDir)
	}
	return siaDir

}

// SkynetDir returns the Skynet data directory.
func SkynetDir() string {
	return defaultSkynetDir()
}

// WalletPassword returns the SiaWalletPassword environment variable.
func WalletPassword() string {
	return os.Getenv(siaWalletPassword)
	if password := os.Getenv("SIAPRIME_WALLET_PASSWORD"); password != "" {
		fmt.Println("ScPrime Wallet Password found, attempting to auto-unlock wallet")
		fmt.Println("Warning: Using SIAPRIME_WALLET_PASSWORD is deprecated.")
		fmt.Println("Using it will not be supported in future versions, please update \n your configuration to use the environment variable 'SCPRIME_WALLET_PASSWORD'")
	}
}

// apiPasswordFilePath returns the path to the API's password file. The password
// file is stored in the Sia data directory.
func apiPasswordFilePath() string {
	return filepath.Join(SiaDir(), "apipassword")
}

// createAPIPasswordFile creates an api password file in the Sia data directory
// and returns the newly created password
func createAPIPasswordFile() (string, error) {
	err := os.Mkdir(SiaDir(), 0700)
	if err != nil {
		return "", err
	}
	pw := hex.EncodeToString(fastrand.Bytes(16))
	err = ioutil.WriteFile(apiPasswordFilePath(), []byte(pw+"\n"), 0600)
	if err != nil {
		return "", err
	}
	return pw, nil
}

// defaultMetadataDir returns the default data directory of siad. The values for
// supported operating systems are:
//
// Linux:   $HOME/.scprime
// MacOS:   $HOME/Library/Application Support/ScPrime
// Windows: %LOCALAPPDATA%\ScPrime
func defaultMetadataDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "ScPrime")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "ScPrime")
	default:
		return filepath.Join(os.Getenv("HOME"), ".scprime")
	}
}

// DefaultSkynetDir returns default data directory for miscellaneous Pubaccess data,
// e.g. pubaccesskeys. The values for supported operating systems are:
//
// Linux:   $HOME/.pubaccess
// MacOS:   $HOME/Library/Application Support/Pubaccess
// Windows: %LOCALAPPDATA%\Pubaccess
func defaultSkynetDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Pubaccess")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Pubaccess")
	default:
		return filepath.Join(os.Getenv("HOME"), ".pubaccess")
	}
}
