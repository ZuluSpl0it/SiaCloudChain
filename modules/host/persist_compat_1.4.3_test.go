package host

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/siamux"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/persist"
)

const (
	// v120Host is the name of the file that contains the legacy host
	// persistence directory testdata.
	v120Host = "v120Host.tar.gz"
)

// TestV120HostUpgrade creates a host with a legacy persistence file,
// and then attempts to upgrade.
func TestV120HostUpgrade(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// ensure the host directory is empty
	persistDir := build.TempDir(modules.HostDir, t.Name())
	hostPersistDir := build.TempDir(modules.HostDir, t.Name(), modules.HostDir)
	err := os.RemoveAll(hostPersistDir)
	if err != nil {
		t.Fatal(err)
	}

	// copy the testdir legacy persistence data to the temp directory
	source := filepath.Join("testdata", v120Host)
	err = build.ExtractTarGz(source, persistDir)
	if err != nil {
		t.Fatal(err)
	}

	// simulate an existing siamux in the persist dir.
	logFile, err := os.Create(filepath.Join(persistDir, "siamux.log"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := persist.NewLogger(logFile)
	if err != nil {
		t.Fatal(err)
	}
	smux, err := siamux.New("localhost:0", "localhost:0", logger.Logger, persistDir)
	if err != nil {
		t.Fatal(err)
	}
	err = smux.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = logFile.Close()
	if err != nil {
		t.Fatal(err)
	}

	// load a new host, the siamux should be created in the sia root.
	siaMuxDir := filepath.Join(persistDir, modules.SiaMuxDir)
	closefn, host, err := loadExistingHostWithNewDeps(persistDir, siaMuxDir, hostPersistDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := closefn()
		if err != nil {
			t.Error(err)
		}
	}()

	// the old siamux files should be gone.
	_, err1 := os.Stat(filepath.Join(persistDir, "siamux.json"))
	_, err2 := os.Stat(filepath.Join(persistDir, "siamux.json_temp"))
	_, err3 := os.Stat(filepath.Join(persistDir, "siamux.log"))
	if !os.IsNotExist(err1) || !os.IsNotExist(err2) || !os.IsNotExist(err3) {
		t.Fatal("files still exist", err1, err2, err3)
	}

	// the new siamux files should be in the right spot.
	_, err1 = os.Stat(filepath.Join(siaMuxDir, "siamux.json"))
	_, err2 = os.Stat(filepath.Join(siaMuxDir, "siamux.json_temp"))
	_, err3 = os.Stat(filepath.Join(siaMuxDir, "siamux.log"))
	if err := errors.Compose(err1, err2, err3); err != nil {
		t.Fatal("files should exist", err1, err2, err3)
	}

	// verify the upgrade properly decorated the ephemeral account related
	// settings onto the persistence object
	his := host.InternalSettings()
	if his.EphemeralAccountExpiry != modules.DefaultEphemeralAccountExpiry {
		t.Fatal("EphemeralAccountExpiry not properly decorated on the persistence object after upgrade")
	}

	if !his.MaxEphemeralAccountBalance.Equals(modules.DefaultMaxEphemeralAccountBalance) {
		t.Fatal("MaxEphemeralAccountBalance not properly decorated on the persistence object after upgrade")
	}

	if !his.MaxEphemeralAccountRisk.Equals(defaultMaxEphemeralAccountRisk) {
		t.Fatal("MaxEphemeralAccountRisk not properly decorated on the persistence object after upgrade")
	}

	// sanity check the metadata version
	hostSettingsFilename := filepath.Join(hostPersistDir, modules.HostSettingsFile)
	fi, err := os.Stat(hostSettingsFilename)
	err = persist.LoadJSON(modules.Hostv151PersistMetadata, struct{}{}, hostSettingsFilename)
	if err != nil {
		t.Fatalf("Error %v reading file:%+v", err, fi)
	}
}

// loadExistingHostWithNewDeps will create all of the dependencies for a host,
// then load the host on top of the given directory.
func loadExistingHostWithNewDeps(modulesDir, siaMuxDir, hostDir string) (closeFn, modules.Host, error) {
	// Create the siamux
	mux, err := modules.NewSiaMux(siaMuxDir, modulesDir, "localhost:0", "localhost:0")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating SiaMux: %w", err)
	}

	// Create the host dependencies.
	g, err := gateway.New("localhost:0", false, filepath.Join(modulesDir, modules.GatewayDir))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating gateway: %w", err)
	}
	cs, errChan := consensus.New(g, false, filepath.Join(modulesDir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, nil, fmt.Errorf("error creating consensus: %w", err)
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(modulesDir, modules.TransactionPoolDir))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating transactionpool: %w", err)
	}
	w, err := wallet.New(cs, tp, filepath.Join(modulesDir, modules.WalletDir))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating wallet: %w", err)
	}

	// Create the host.
	h, err := NewCustomHost(modules.ProdDependencies, cs, g, tp, w, mux, "localhost:0", hostDir, nil, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating host: %w", err)
	}

	pubKey := mux.PublicKey()
	if !bytes.Equal(h.publicKey.Key, pubKey[:]) {
		return nil, nil, fmt.Errorf("host and siamux pubkeys don't match")
	}
	privKey := mux.PrivateKey()
	if !bytes.Equal(h.secretKey[:], privKey[:]) {
		return nil, nil, fmt.Errorf("host and siamux privkeys don't match")
	}

	closefn := func() error {
		return errors.Compose(h.Close(), w.Close(), tp.Close(), cs.Close(), g.Close(), mux.Close())
	}
	return closefn, h, nil
}
