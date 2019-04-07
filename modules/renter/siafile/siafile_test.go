package siafile

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/types"
)

// randomChunk is a helper method for testing that creates a random chunk.
func randomChunk() chunk {
	numPieces := 30
	chunk := chunk{}
	chunk.Pieces = make([][]piece, numPieces)
	fastrand.Read(chunk.ExtensionInfo[:])

	// Add 0-3 pieces for each pieceIndex within the file.
	for pieceIndex := range chunk.Pieces {
		n := fastrand.Intn(4) // [0;3]
		// Create and add n pieces at pieceIndex i.
		for i := 0; i < n; i++ {
			var piece piece
			piece.HostTableOffset = uint32(fastrand.Intn(100))
			fastrand.Read(piece.MerkleRoot[:])
			chunk.Pieces[pieceIndex] = append(chunk.Pieces[pieceIndex], piece)
		}
	}
	return chunk
}

// randomPiece is a helper method for testing that creates a random piece.
func randomPiece() piece {
	var piece piece
	piece.HostTableOffset = uint32(fastrand.Intn(100))
	fastrand.Read(piece.MerkleRoot[:])
	return piece
}

func TestPruneHosts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	sf := newBlankTestFile()

	// Add 3 random hostkeys to the file.
	sf.addRandomHostKeys(3)

	// Save changes to disk.
	if err := sf.saveFile(); err != nil {
		t.Fatal(err)
	}

	// Add one piece for every host to every pieceSet of the SiaFile.
	for _, hk := range sf.HostPublicKeys() {
		for chunkIndex, chunk := range sf.staticChunks {
			for pieceIndex := range chunk.Pieces {
				if err := sf.AddPiece(hk, uint64(chunkIndex), uint64(pieceIndex), crypto.Hash{}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	// Mark hostkeys 0 and 2 as unused.
	sf.pubKeyTable[0].Used = false
	sf.pubKeyTable[2].Used = false
	remainingKey := sf.pubKeyTable[1]

	// Prune the file.
	sf.pruneHosts()

	// Check that there is only a single key left.
	if len(sf.pubKeyTable) != 1 {
		t.Fatalf("There should only be 1 key left but was %v", len(sf.pubKeyTable))
	}
	// The last key should be the correct one.
	if !reflect.DeepEqual(remainingKey, sf.pubKeyTable[0]) {
		t.Fatal("Remaining key doesn't match")
	}
	// Loop over all the pieces and make sure that the pieces with missing
	// hosts were pruned and that the remaining pieces have the correct offset
	// now.
	for chunkIndex := range sf.staticChunks {
		for _, pieceSet := range sf.staticChunks[chunkIndex].Pieces {
			if len(pieceSet) != 1 {
				t.Fatalf("Expected 1 piece in the set but was %v", len(pieceSet))
			}
			// The HostTableOffset should always be 0 since the keys at index 0
			// and 2 were pruned which means that index 1 is now index 0.
			for _, piece := range pieceSet {
				if piece.HostTableOffset != 0 {
					t.Fatalf("HostTableOffset should be 0 but was %v", piece.HostTableOffset)
				}
			}
		}
	}
}

// TestNumPieces tests the chunk's numPieces method.
func TestNumPieces(t *testing.T) {
	// create a random chunk.
	chunk := randomChunk()

	// get the number of pieces of the chunk.
	totalPieces := 0
	for _, pieceSet := range chunk.Pieces {
		totalPieces += len(pieceSet)
	}

	// compare it to the one reported by numPieces.
	if totalPieces != chunk.numPieces() {
		t.Fatalf("Expected %v pieces but was %v", totalPieces, chunk.numPieces())
	}
}

// TestDefragChunk tests if the defragChunk methods correctly prunes pieces
// from a chunk.
func TestDefragChunk(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Get a blank siafile.
	sf := newBlankTestFile()

	// Use the first chunk of the file for testing.
	chunk := &sf.staticChunks[0]

	// Add 100 pieces to each set of pieces, all belonging to the same unused
	// host.
	sf.pubKeyTable = append(sf.pubKeyTable, HostPublicKey{Used: false})
	for i := range chunk.Pieces {
		for j := 0; j < 100; j++ {
			chunk.Pieces[i] = append(chunk.Pieces[i], piece{HostTableOffset: 0})
		}
	}

	// Defrag the chunk. This should remove all the pieces since the host is
	// unused.
	sf.defragChunk(chunk)
	if chunk.numPieces() != 0 {
		t.Fatalf("chunk should have 0 pieces after defrag but was %v", chunk.numPieces())
	}

	// Do the same thing again, but this time the host is marked as used.
	sf.pubKeyTable[0].Used = true
	for i := range chunk.Pieces {
		for j := 0; j < 100; j++ {
			chunk.Pieces[i] = append(chunk.Pieces[i], piece{HostTableOffset: 0})
		}
	}

	// Defrag the chunk.
	maxChunkSize := int64(sf.staticMetadata.StaticPagesPerChunk) * pageSize
	maxPieces := (maxChunkSize - marshaledChunkOverhead) / marshaledPieceSize
	maxPiecesPerSet := maxPieces / int64(len(chunk.Pieces))
	sf.defragChunk(chunk)

	// The chunk should be smaller than maxChunkSize.
	if chunkSize := marshaledChunkSize(chunk.numPieces()); chunkSize > maxChunkSize {
		t.Errorf("chunkSize is too big %v > %v", chunkSize, maxChunkSize)
	}
	// The chunk should have less than maxPieces pieces.
	if int64(chunk.numPieces()) > maxPieces {
		t.Errorf("chunk should have <= %v pieces after defrag but was %v",
			maxPieces, chunk.numPieces())
	}
	// The chunk should have numPieces * maxPiecesPerSet pieces.
	if expectedPieces := int64(sf.ErasureCode().NumPieces()) * maxPiecesPerSet; expectedPieces != int64(chunk.numPieces()) {
		t.Errorf("chunk should have %v pieces but was %v", expectedPieces, chunk.numPieces())
	}
	// Every set of pieces should have maxPiecesPerSet pieces.
	for i, pieceSet := range chunk.Pieces {
		if int64(len(pieceSet)) != maxPiecesPerSet {
			t.Errorf("pieceSet%v length is %v which is greater than %v",
				i, len(pieceSet), maxPiecesPerSet)
		}
	}

	// Create a new file with 2 used hosts and 1 unused one. This file should
	// use 2 pages per chunk.
	sf = newBlankTestFile()
	sf.staticMetadata.StaticPagesPerChunk = 2
	sf.pubKeyTable = append(sf.pubKeyTable, HostPublicKey{Used: true})
	sf.pubKeyTable = append(sf.pubKeyTable, HostPublicKey{Used: true})
	sf.pubKeyTable = append(sf.pubKeyTable, HostPublicKey{Used: false})
	sf.pubKeyTable[0].PublicKey.Key = fastrand.Bytes(crypto.EntropySize)
	sf.pubKeyTable[1].PublicKey.Key = fastrand.Bytes(crypto.EntropySize)
	sf.pubKeyTable[2].PublicKey.Key = fastrand.Bytes(crypto.EntropySize)

	// Save the above changes to disk to avoid failing sanity checks when
	// calling AddPiece.
	err := sf.saveFile()
	if err != nil {
		t.Fatal(err)
	}

	// Add 500 pieces to the first chunk of the file, randomly belonging to
	// any of the 3 hosts. This should never produce an error.
	var duration time.Duration
	for i := 0; i < 50; i++ {
		pk := sf.pubKeyTable[fastrand.Intn(len(sf.pubKeyTable))].PublicKey
		pieceIndex := fastrand.Intn(len(sf.staticChunks[0].Pieces))
		before := time.Now()
		if err := sf.AddPiece(pk, 0, uint64(pieceIndex), crypto.Hash{}); err != nil {
			t.Fatal(err)
		}
		duration += time.Since(before)
	}

	// Finally load the file from disk again and compare it to the original.
	sf2, err := LoadSiaFile(sf.siaFilePath, sf.wal)
	if err != nil {
		t.Fatal(err)
	}
	// Compare the files.
	if err := equalFiles(sf, sf2); err != nil {
		t.Fatal(err)
	}
}

// TestChunkHealth probes the chunkHealth method
func TestChunkHealth(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Get a blank siafile.
	// Get new file params, ensure at least 2 chunks
	siaFilePath, siaPath, source, rc, sk, _, numChunks, fileMode := newTestFileParams()
	numChunks++
	pieceSize := modules.SectorSize - sk.Type().Overhead()
	fileSize := pieceSize * uint64(rc.MinPieces()) * uint64(numChunks)
	// Create the path to the file.
	dir, _ := filepath.Split(siaFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Create the file.
	wal, _ := newTestWAL()
	sf, err := New(siaPath, siaFilePath, source, wal, rc, sk, fileSize, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the number of chunks in the file is correct.
	if len(sf.staticChunks) != numChunks {
		t.Fatal("newTestFile didn't create the expected number of chunks")
	}

	// Create offline map
	offlineMap := make(map[string]bool)
	goodForRenewMap := make(map[string]bool)

	// Check and Record file health of initialized file
	fileHealth, _, _ := sf.Health(offlineMap, goodForRenewMap)
	initHealth := float64(1) - (float64(0-rc.MinPieces()) / float64(rc.NumPieces()-rc.MinPieces()))
	if fileHealth != initHealth {
		t.Fatalf("Expected file to be %v, got %v", initHealth, fileHealth)
	}

	// Since we are using a pre set offlineMap, all the chunks should have the
	// same health as the file
	for i := range sf.staticChunks {
		chunkHealth := sf.chunkHealth(i, offlineMap, goodForRenewMap)
		if chunkHealth != fileHealth {
			t.Log("ChunkHealth:", chunkHealth)
			t.Log("FileHealth:", fileHealth)
			t.Fatal("Expected file and chunk to have same health")
		}
	}

	// Add good piece to first chunk
	host := fmt.Sprintln("host_0")
	spk := types.SiaPublicKey{}
	spk.LoadString(host)
	offlineMap[spk.String()] = false
	goodForRenewMap[spk.String()] = true
	if err := sf.AddPiece(spk, 0, 0, crypto.Hash{}); err != nil {
		t.Fatal(err)
	}

	// Chunk at index 0 should now have a health of 1 higher than before
	newHealth := float64(1) - (float64(1-rc.MinPieces()) / float64(rc.NumPieces()-rc.MinPieces()))
	if sf.chunkHealth(0, offlineMap, goodForRenewMap) != newHealth {
		t.Fatalf("Expected chunk health to be %v, got %v", newHealth, sf.chunkHealth(0, offlineMap, goodForRenewMap))
	}

	// Chunk at index 1 should still have lower health
	if sf.chunkHealth(1, offlineMap, goodForRenewMap) != fileHealth {
		t.Fatalf("Expected chunk health to be %v, got %v", fileHealth, sf.chunkHealth(1, offlineMap, goodForRenewMap))
	}

	// Add good piece to second chunk
	host = fmt.Sprintln("host_1")
	spk = types.SiaPublicKey{}
	spk.LoadString(host)
	offlineMap[spk.String()] = false
	goodForRenewMap[spk.String()] = true
	if err := sf.AddPiece(spk, 1, 0, crypto.Hash{}); err != nil {
		t.Fatal(err)
	}

	// Chunk at index 1 should now have a health of 1 higher than before
	if sf.chunkHealth(1, offlineMap, goodForRenewMap) != newHealth {
		t.Fatalf("Expected chunk health to be %v, got %v", newHealth, sf.chunkHealth(1, offlineMap, goodForRenewMap))
	}

	// Mark Chunk at index 1 as stuck and confirm that doesn't impact the result
	// of chunkHealth
	sf.staticChunks[1].Stuck = true
	if sf.chunkHealth(1, offlineMap, goodForRenewMap) != newHealth {
		t.Fatalf("Expected file to be %v, got %v", newHealth, sf.chunkHealth(1, offlineMap, goodForRenewMap))
	}
}

// TestMarkHealthyChunksAsUnstuck probes the MarkAllHealthyChunksAsUnstuck
// method
func TestMarkHealthyChunksAsUnstuck(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Get a blank siafile.
	// Get new file params, ensure at least 2 chunks
	siaFilePath, siaPath, source, rc, sk, _, numChunks, fileMode := newTestFileParams()
	numChunks++
	pieceSize := modules.SectorSize - sk.Type().Overhead()
	fileSize := pieceSize * uint64(rc.MinPieces()) * uint64(numChunks)
	// Create the path to the file.
	dir, _ := filepath.Split(siaFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Create the file.
	wal, _ := newTestWAL()
	sf, err := New(siaPath, siaFilePath, source, wal, rc, sk, fileSize, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the number of chunks in the file is correct.
	if len(sf.staticChunks) != numChunks {
		t.Fatal("newTestFile didn't create the expected number of chunks")
	}

	// Create offline and goodForRenew maps
	offlineMap := make(map[string]bool)
	goodForRenewMap := make(map[string]bool)

	// Mark all chunks as stuck
	err = sf.MarkAllUnhealthyChunksAsStuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	if sf.NumStuckChunks() != sf.NumChunks() {
		t.Fatalf("numStuckChunks should be %v but was %v", sf.NumChunks(), sf.NumStuckChunks())
	}

	// Try and mark chunks as unstuck
	err = sf.MarkAllHealthyChunksAsUnstuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that all chunks are still stuck
	if sf.NumStuckChunks() != sf.NumChunks() {
		t.Fatalf("numStuckChunks should be %v but was %v", sf.NumChunks(), sf.NumStuckChunks())
	}

	// Add good pieces to first chunk
	for pieceIndex := range sf.staticChunks[0].Pieces {
		host := fmt.Sprintln("host", 0, pieceIndex)
		spk := types.SiaPublicKey{}
		spk.LoadString(host)
		offlineMap[spk.String()] = false
		goodForRenewMap[spk.String()] = true
		if err := sf.AddPiece(spk, 0, uint64(pieceIndex), crypto.Hash{}); err != nil {
			t.Fatal(err)
		}
	}

	// Try and mark first chunk as unstuck
	err = sf.MarkAllHealthyChunksAsUnstuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that all chunks are still stuck
	if sf.NumStuckChunks() != sf.NumChunks()-1 {
		t.Fatalf("numStuckChunks should be %v but was %v", sf.NumChunks()-1, sf.NumStuckChunks())
	}

	// Make file fully healthy
	// Reset maps
	offlineMap = make(map[string]bool)
	goodForRenewMap = make(map[string]bool)

	// Add good pieces to all chunks
	for chunkIndex, chunk := range sf.staticChunks {
		for pieceIndex := range chunk.Pieces {
			host := fmt.Sprintln("host", chunkIndex, pieceIndex)
			spk := types.SiaPublicKey{}
			spk.LoadString(host)
			offlineMap[spk.String()] = false
			goodForRenewMap[spk.String()] = true
			if err := sf.AddPiece(spk, uint64(chunkIndex), uint64(pieceIndex), crypto.Hash{}); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Mark health chunks as unstuck
	err = sf.MarkAllHealthyChunksAsUnstuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm that all chunks were marked as unstuck
	if sf.NumStuckChunks() != 0 {
		t.Fatalf("numStuckChunks should be %v but was %v", 0, sf.NumStuckChunks())
	}
}

// TestMarkUnhealthyChunksAsStuck probes the MarkAllUnhealthyChunksAsStuck method
func TestMarkUnhealthyChunksAsStuck(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	// Get a blank siafile.
	// Get new file params, ensure at least 2 chunks
	siaFilePath, siaPath, source, rc, sk, _, numChunks, fileMode := newTestFileParams()
	numChunks++
	pieceSize := modules.SectorSize - sk.Type().Overhead()
	fileSize := pieceSize * uint64(rc.MinPieces()) * uint64(numChunks)
	// Create the path to the file.
	dir, _ := filepath.Split(siaFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Create the file.
	wal, _ := newTestWAL()
	sf, err := New(siaPath, siaFilePath, source, wal, rc, sk, fileSize, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	// Check that the number of chunks in the file is correct.
	if len(sf.staticChunks) != numChunks {
		t.Fatal("newTestFile didn't create the expected number of chunks")
	}

	// Create offline and goodForRenew maps
	offlineMap := make(map[string]bool)
	goodForRenewMap := make(map[string]bool)

	// Mark chunks as stuck
	err = sf.MarkAllUnhealthyChunksAsStuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that all chunks were marked as stuck
	_, _, numStuckChunks := sf.Health(offlineMap, goodForRenewMap)
	if numStuckChunks != sf.NumChunks() {
		t.Fatalf("numStuckChunks should be %v but was %v", sf.NumChunks(), numStuckChunks)
	}

	// Reset chunk stuck status
	for chunkIndex := range sf.staticChunks {
		if err := sf.SetStuck(uint64(chunkIndex), false); err != nil {
			t.Fatal(err)
		}
	}

	// Add good pieces to first chunk
	for pieceIndex := range sf.staticChunks[0].Pieces {
		host := fmt.Sprintln("host", 0, pieceIndex)
		spk := types.SiaPublicKey{}
		spk.LoadString(host)
		offlineMap[spk.String()] = false
		goodForRenewMap[spk.String()] = true
		if err := sf.AddPiece(spk, 0, uint64(pieceIndex), crypto.Hash{}); err != nil {
			t.Fatal(err)
		}
	}

	// Try and mark chunks as stuck
	err = sf.MarkAllUnhealthyChunksAsStuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that all but 1 chunk was marked as stuck
	_, _, numStuckChunks = sf.Health(offlineMap, goodForRenewMap)
	if numStuckChunks != sf.NumChunks()-1 {
		t.Fatalf("numStuckChunks should be %v but was %v", sf.NumChunks()-1, numStuckChunks)
	}

	// Reset chunk stuck status
	for chunkIndex := range sf.staticChunks {
		if err := sf.SetStuck(uint64(chunkIndex), false); err != nil {
			t.Fatal(err)
		}
	}

	// Reset maps
	offlineMap = make(map[string]bool)
	goodForRenewMap = make(map[string]bool)

	// Add good pieces to all chunks
	for chunkIndex, chunk := range sf.staticChunks {
		for pieceIndex := range chunk.Pieces {
			host := fmt.Sprintln("host", chunkIndex, pieceIndex)
			spk := types.SiaPublicKey{}
			spk.LoadString(host)
			offlineMap[spk.String()] = false
			goodForRenewMap[spk.String()] = true
			if err := sf.AddPiece(spk, uint64(chunkIndex), uint64(pieceIndex), crypto.Hash{}); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Try and mark chunks as stuck
	err = sf.MarkAllUnhealthyChunksAsStuck(offlineMap, goodForRenewMap)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm that no chunks were marked as stuck
	_, _, numStuckChunks = sf.Health(offlineMap, goodForRenewMap)
	if numStuckChunks != 0 {
		t.Fatal("numStuckChunks should be 0 but was", numStuckChunks)
	}
}

// TestStuckChunks checks to make sure the NumStuckChunks return the expected
// values and that the stuck chunks are persisted properly
func TestStuckChunks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create siafile
	sf, sfs, err := newTestSiaFileSetWithFile()
	if err != nil {
		t.Fatal(err)
	}

	// Mark every other chunk as stuck
	expectedStuckChunks := 0
	for i := range sf.staticChunks {
		if (i % 2) != 0 {
			continue
		}
		if err := sf.SetStuck(uint64(i), true); err != nil {
			t.Fatal(err)
		}
		expectedStuckChunks++
	}

	// Sanity Check
	if expectedStuckChunks == 0 {
		t.Fatal("No chunks were set to stuck")
	}

	// Check that the total number of stuck chunks is consistent
	numStuckChunks := sf.NumStuckChunks()
	if numStuckChunks != uint64(expectedStuckChunks) {
		t.Fatalf("Wrong number of stuck chunks, got %v expected %v", numStuckChunks, expectedStuckChunks)
	}

	// Close file and confirm it is out of memory
	siaPath := sf.SiaPath()
	if err = sf.Close(); err != nil {
		t.Fatal(err)
	}
	if len(sfs.siaFileMap) != 0 {
		t.Fatal("File not removed from memory")
	}

	// Load siafile from disk
	sf, err = sfs.Open(siaPath)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the total number of stuck chunks is consistent
	if numStuckChunks != sf.NumStuckChunks() {
		t.Fatalf("Wrong number of stuck chunks, got %v expected %v", numStuckChunks, sf.NumStuckChunks())
	}

	// Check chunks and Stuck Chunk Table
	for i, chunk := range sf.staticChunks {
		if i%2 != 0 {
			if chunk.Stuck {
				t.Fatal("Found stuck chunk when un-stuck chunk was expected")
			}
			continue
		}
		if !chunk.Stuck {
			t.Fatal("Found un-stuck chunk when stuck chunk was expected")
		}
	}
}
