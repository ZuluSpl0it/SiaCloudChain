package mdm

import (
	"testing"

	"gitlab.com/NebulousLabs/encoding"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestInstructionRevision tests executing a program with a single
// RevisionInstruction.
func TestInstructionRevision(t *testing.T) {
	host := newTestHost()
	mdm := New(host)
	defer mdm.Stop()

	so := host.newTestStorageObligation(true)
	so.sectorRoots = randomSectorRoots(1)

	// Add sector to the host.
	sectorRoot := so.sectorRoots[0]
	_, err := host.ReadSector(sectorRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Get revision, contract size and root.
	rev := so.RecentRevision()
	ics := rev.NewFileSize
	imr := rev.NewFileMerkleRoot
	var zmr crypto.Hash
	if ics == 0 || imr == zmr {
		t.Fatal("ics and/or were not initialized")
	}

	// Build the program.
	pt := newTestPriceTable()
	duration := types.BlockHeight(fastrand.Uint64n(5))
	tb := newTestProgramBuilder(pt, duration)
	tb.AddRevisionInstruction()

	// Execute it.
	outputs, err := mdm.ExecuteProgramWithBuilder(tb, so, duration, false)
	if err != nil {
		t.Fatal(err)
	}

	// Assert output.
	expectedOutput := encoding.Marshal(rev)
	err = outputs[0].assert(ics, imr, []crypto.Hash{}, expectedOutput)
	if err != nil {
		t.Fatal(err)
	}
}
