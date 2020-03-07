package mdm

import (
	"sync"
	"testing"

	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/types"

	"gitlab.com/NebulousLabs/fastrand"
)

type (
	// TestHost is a dummy host for testing which satisfies the Host interface.
	TestHost struct {
		blockHeight types.BlockHeight
		sectors     map[crypto.Hash][]byte
		mu          sync.Mutex
	}
	// TestStorageObligation is a dummy storage obligation for testing which
	// satisfies the StorageObligation interface.
	TestStorageObligation struct {
		locked bool
	}
)

func newTestHost() Host {
	return &TestHost{
		sectors: make(map[crypto.Hash][]byte),
	}
}

func newTestStorageObligation(locked bool) StorageObligation {
	return &TestStorageObligation{
		locked: locked,
	}
}

// BlockHeight returns an incremented blockheight every time it's called.
func (h *TestHost) BlockHeight() types.BlockHeight {
	h.blockHeight++
	return h.blockHeight
}

// ReadSector implements the Host interface by returning a random sector for
// each root. Calling ReadSector multiple times on the same root will result in
// the same data.
func (h *TestHost) ReadSector(sectorRoot crypto.Hash) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, exists := h.sectors[sectorRoot]
	if !exists {
		data = fastrand.Bytes(int(modules.SectorSize))
		h.sectors[sectorRoot] = data
	}
	return data, nil
}

// Locked implements the StorageObligation interface.
func (so *TestStorageObligation) Locked() bool {
	return so.locked
}

// Update implements the StorageObligation interface
func (so *TestStorageObligation) Update(sectorsRemoved, sectorsGained []crypto.Hash, gainedSectorData [][]byte) error {
	return nil
}

// TestNew tests the New method to create a new MDM.
func TestNew(t *testing.T) {
	host := newTestHost()
	mdm := New(host)
	defer mdm.Stop()

	// Check fields.
	if mdm.host != host {
		t.Fatal("host wasn't set correctly")
	}
}
