package renter

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem/siafile"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
)

// TestBuildUnfinishedChunks probes buildUnfinishedChunks to make sure that the
// correct chunks are being added to the heap
func TestBuildUnfinishedChunks(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create file on disk
	path, err := rt.createZeroByteFileOnDisk()
	if err != nil {
		t.Fatal(err)
	}
	// Create file with more than 1 chunk and mark the first chunk at stuck
	rsc, _ := siafile.NewRSCode(1, 1)
	siaPath, err := modules.NewSiaPath("stuckFile")
	if err != nil {
		t.Fatal(err)
	}
	up := modules.FileUploadParams{
		Source:      path,
		SiaPath:     siaPath,
		ErasureCode: rsc,
	}
	err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), 10e3, persist.DefaultDiskPermissionsTest, false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := rt.renter.staticFileSystem.OpenSiaFile(up.SiaPath)
	if err != nil {
		t.Fatal(err)
	}
	if f.NumChunks() <= 1 {
		t.Fatalf("File created with not enough chunks for test, have %v need at least 2", f.NumChunks())
	}
	if err = f.SetStuck(uint64(0), true); err != nil {
		t.Fatal(err)
	}

	// Create maps to pass into methods
	hosts := make(map[string]struct{})
	offline := make(map[string]bool)
	goodForRenew := make(map[string]bool)

	// Manually add workers to worker pool
	rt.renter.staticWorkerPool.mu.Lock()
	for i := 0; i < int(f.NumChunks()); i++ {
		rt.renter.staticWorkerPool.workers[fmt.Sprint(i)] = &worker{
			killChan: make(chan struct{}),
		}
	}
	rt.renter.staticWorkerPool.mu.Unlock()

	// Call managedBuildUnfinishedChunks as not stuck loop, all un stuck chunks
	// should be returned
	uucs := rt.renter.managedBuildUnfinishedChunks(f, hosts, targetUnstuckChunks, offline, goodForRenew)
	if len(uucs) != int(f.NumChunks())-1 {
		t.Fatalf("Incorrect number of chunks returned, expected %v got %v", int(f.NumChunks())-1, len(uucs))
	}
	for _, c := range uucs {
		if c.stuck {
			t.Fatal("Found stuck chunk when expecting only unstuck chunks")
		}
	}

	// Call managedBuildUnfinishedChunks as stuck loop, all stuck chunks should
	// be returned
	uucs = rt.renter.managedBuildUnfinishedChunks(f, hosts, targetStuckChunks, offline, goodForRenew)
	if len(uucs) != 1 {
		t.Fatalf("Incorrect number of chunks returned, expected 1 got %v", len(uucs))
	}
	for _, c := range uucs {
		if !c.stuck {
			t.Fatal("Found unstuck chunk when expecting only stuck chunks")
		}
	}

	// Remove file on disk to make file not repairable
	err = os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}

	// Call managedBuildUnfinishedChunks as not stuck loop, since the file is
	// now not repairable it should return no chunks
	uucs = rt.renter.managedBuildUnfinishedChunks(f, hosts, targetUnstuckChunks, offline, goodForRenew)
	if len(uucs) != 0 {
		t.Fatalf("Incorrect number of chunks returned, expected 0 got %v", len(uucs))
	}

	// Call managedBuildUnfinishedChunks as stuck loop, all chunks should be
	// returned because they should have been marked as stuck by the previous
	// call and stuck chunks should still be returned if the file is not
	// repairable
	uucs = rt.renter.managedBuildUnfinishedChunks(f, hosts, targetStuckChunks, offline, goodForRenew)
	if len(uucs) != int(f.NumChunks()) {
		t.Fatalf("Incorrect number of chunks returned, expected %v got %v", f.NumChunks(), len(uucs))
	}
	for _, c := range uucs {
		if !c.stuck {
			t.Fatal("Found unstuck chunk when expecting only stuck chunks")
		}
	}
}

// TestBuildChunkHeap probes managedBuildChunkHeap to make sure that the correct
// chunks are being added to the heap
func TestBuildChunkHeap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create 2 files
	source, err := rt.createZeroByteFileOnDisk()
	if err != nil {
		t.Fatal(err)
	}
	rsc, _ := siafile.NewRSCode(1, 1)
	up := modules.FileUploadParams{
		Source:      source,
		SiaPath:     modules.RandomSiaPath(),
		ErasureCode: rsc,
	}
	err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), 10e3, persist.DefaultDiskPermissionsTest, false)
	if err != nil {
		t.Fatal(err)
	}
	f1, err := rt.renter.staticFileSystem.OpenSiaFile(up.SiaPath)
	if err != nil {
		t.Fatal(err)
	}

	// Manually add workers to worker pool and create host map
	hosts := make(map[string]struct{})
	rt.renter.staticWorkerPool.mu.Lock()
	for i := 0; i < int(f1.NumChunks()); i++ {
		rt.renter.staticWorkerPool.workers[fmt.Sprint(i)] = &worker{
			killChan: make(chan struct{}),
		}
	}
	rt.renter.staticWorkerPool.mu.Unlock()

	// Call managedBuildChunkHeap as repair loop, we should see all the chunks
	// from the file added
	offline, goodForRenew, _ := rt.renter.managedContractUtilityMaps()
	rt.renter.managedBuildChunkHeap(modules.RootSiaPath(), hosts, targetUnstuckChunks, offline, goodForRenew)
	if rt.renter.uploadHeap.managedLen() != int(f1.NumChunks()) {
		t.Fatalf("Expected heap length of %v but got %v", f1.NumChunks(), rt.renter.uploadHeap.managedLen())
	}
}

// addChunksOfDifferentHealth is a helper function for TestUploadHeap to add
// numChunks number of chunks that each have different healths to the uploadHeap
func addChunksOfDifferentHealth(r *Renter, numChunks int, priority, fileRecentlySuccessful, stuck, remote bool) error {
	var UID siafile.SiafileUID
	if priority {
		UID = "priority"
	} else if fileRecentlySuccessful {
		UID = "fileRecentlySuccessful"
	} else if stuck {
		UID = "stuck"
	} else if remote {
		UID = "remote"
	} else {
		UID = "unstuck"
	}

	// Add numChunks number of chunks to the upload heap. Set the id index and
	// health to the value of health. Since health of 0 is full health, start i
	// at 1
	for i := 1; i <= numChunks; i++ {
		chunk := &unfinishedUploadChunk{
			id: uploadChunkID{
				fileUID: UID,
				index:   uint64(i),
			},
			stuck:                  stuck,
			fileRecentlySuccessful: fileRecentlySuccessful,
			staticPriority:         priority,
			health:                 float64(i),
			onDisk:                 !remote,
			availableChan:          make(chan struct{}),
		}
		if !r.uploadHeap.managedPush(chunk) {
			return fmt.Errorf("unable to push chunk: %v", chunk)
		}
	}
	return nil
}

// TestUploadHeap probes the upload heap to make sure chunks are sorted
// correctly
func TestUploadHeap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Add chunks to heap. Chunks are prioritize by stuck status first and then
	// by piecesComplete/piecesNeeded
	//
	// Add 2 chunks of each type to confirm the type and the health is
	// prioritized properly
	err = addChunksOfDifferentHealth(rt.renter, 2, true, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	err = addChunksOfDifferentHealth(rt.renter, 2, false, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	err = addChunksOfDifferentHealth(rt.renter, 2, false, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	err = addChunksOfDifferentHealth(rt.renter, 2, false, false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	err = addChunksOfDifferentHealth(rt.renter, 2, false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}

	// There should be 8 chunks in the heap
	if rt.renter.uploadHeap.managedLen() != 10 {
		t.Fatalf("Expected %v chunks in heap found %v",
			8, rt.renter.uploadHeap.managedLen())
	}

	// Check order of chunks
	//  - First 2 chunks should be priority
	//  - Second 2 chunks should be fileRecentlyRepair
	//  - Third 2 chunks should be stuck
	//  - Fourth 2 chunks should be remote (ie !onDisk)
	//  - Last 2 chunks should be unstuck
	chunk1 := rt.renter.uploadHeap.managedPop()
	chunk2 := rt.renter.uploadHeap.managedPop()
	if !chunk1.staticPriority || !chunk2.staticPriority {
		t.Fatalf("Expected chunks to be priority, got priority %v and %v",
			chunk1.staticPriority, chunk2.staticPriority)
	}
	if chunk1.health < chunk2.health {
		t.Fatalf("expected top chunk to have worst health, chunk1: %v, chunk2: %v",
			chunk1.health, chunk2.health)
	}
	chunk1 = rt.renter.uploadHeap.managedPop()
	chunk2 = rt.renter.uploadHeap.managedPop()
	if !chunk1.fileRecentlySuccessful || !chunk2.fileRecentlySuccessful {
		t.Fatalf("Expected chunks to be fileRecentlySuccessful, got fileRecentlySuccessful %v and %v",
			chunk1.fileRecentlySuccessful, chunk2.fileRecentlySuccessful)
	}
	if chunk1.health < chunk2.health {
		t.Fatalf("expected top chunk to have worst health, chunk1: %v, chunk2: %v",
			chunk1.health, chunk2.health)
	}
	chunk1 = rt.renter.uploadHeap.managedPop()
	chunk2 = rt.renter.uploadHeap.managedPop()
	if !chunk1.stuck || !chunk2.stuck {
		t.Fatalf("Expected chunks to be stuck, got stuck %v and %v",
			chunk1.stuck, chunk2.stuck)
	}
	if chunk1.health < chunk2.health {
		t.Fatalf("expected top chunk to have worst health, chunk1: %v, chunk2: %v",
			chunk1.health, chunk2.health)
	}
	chunk1 = rt.renter.uploadHeap.managedPop()
	chunk2 = rt.renter.uploadHeap.managedPop()
	if chunk1.onDisk || chunk2.onDisk {
		t.Fatalf("Expected chunks to be remote and not onDisk, got onDisk %v and %v",
			chunk1.onDisk, chunk2.onDisk)
	}
	if chunk1.health < chunk2.health {
		t.Fatalf("expected top chunk to have worst health, chunk1: %v, chunk2: %v",
			chunk1.health, chunk2.health)
	}
	chunk1 = rt.renter.uploadHeap.managedPop()
	chunk2 = rt.renter.uploadHeap.managedPop()
	if chunk1.health < chunk2.health {
		t.Fatalf("expected top chunk to have worst health, chunk1: %v, chunk2: %v",
			chunk1.health, chunk2.health)
	}
}

// TestAddChunksToHeap probes the managedAddChunksToHeap method to ensure it is
// functioning as intended
func TestAddChunksToHeap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create File params
	_, rsc := testingFileParams()
	source, err := rt.createZeroByteFileOnDisk()
	if err != nil {
		t.Fatal(err)
	}
	up := modules.FileUploadParams{
		Source:      source,
		ErasureCode: rsc,
	}

	// Create files in multiple directories
	var numChunks uint64
	var dirSiaPaths []modules.SiaPath
	names := []string{"rootFile", "subdir/File", "subdir2/file"}
	for _, name := range names {
		siaPath, err := modules.NewSiaPath(name)
		if err != nil {
			t.Fatal(err)
		}
		up.SiaPath = siaPath
		err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), modules.SectorSize, persist.DefaultDiskPermissionsTest, false)
		if err != nil {
			t.Fatal(err)
		}
		f, err := rt.renter.staticFileSystem.OpenSiaFile(up.SiaPath)
		if err != nil {
			t.Fatal(err)
		}
		// Track number of chunks
		numChunks += f.NumChunks()
		dirSiaPath, err := siaPath.Dir()
		if err != nil {
			t.Fatal(err)
		}
		// Make sure directories are created
		err = rt.renter.CreateDir(dirSiaPath, modules.DefaultDirPerm)
		if err != nil && err != filesystem.ErrExists {
			t.Fatal(err)
		}
		dirSiaPaths = append(dirSiaPaths, dirSiaPath)
	}

	// Call bubbled to ensure directory metadata is updated
	for _, siaPath := range dirSiaPaths {
		err := rt.renter.managedBubbleMetadata(siaPath)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Manually add workers to worker pool and create host map
	hosts := make(map[string]struct{})
	rt.renter.staticWorkerPool.mu.Lock()
	for i := 0; i < rsc.MinPieces(); i++ {
		rt.renter.staticWorkerPool.workers[fmt.Sprint(i)] = &worker{
			killChan: make(chan struct{}),
		}
	}
	rt.renter.staticWorkerPool.mu.Unlock()

	// Make sure directory Heap is ready
	err = rt.renter.managedPushUnexploredDirectory(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}

	// call managedAddChunksTo Heap
	siaPaths, err := rt.renter.managedAddChunksToHeap(hosts)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm that all chunks from all the directories were added since there
	// are not enough chunks in only one directory to fill the heap
	totalDirs := len(siaPaths.childDirs) + len(siaPaths.parentDirs)
	if totalDirs != 3 {
		t.Fatal("Expected 3 siaPaths to be returned, got", totalDirs)
	}
	if rt.renter.uploadHeap.managedLen() != int(numChunks) {
		t.Fatalf("Expected uploadHeap to have %v chunks but it has %v chunks", numChunks, rt.renter.uploadHeap.managedLen())
	}
}

// TestAddRemoteChunksToHeap probes how the upload heap handles adding chunks
// when there are remote chunks present
func TestAddRemoteChunksToHeap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter with dependencies that prevent the background repair
	// loop from running as well as ensure that chunks viewed as not
	// repairable are added to the heap.
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyAddUnrepairableChunks{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create a local file for the local file uploads
	source, err := rt.createZeroByteFileOnDisk()
	if err != nil {
		t.Fatal(err)
	}

	// Create common File params
	_, rsc := testingFileParams()
	up := modules.FileUploadParams{
		ErasureCode: rsc,
	}

	// Create local and remote files in the root directory an a sub directory
	var numChunks int
	dirSiaPaths := rt.renter.newUniqueRefreshPaths()
	names := []string{"remoteFile", "localFile", "sub/remoteFile", "sub/localFile"}
	for _, name := range names {
		// Create the SiaPath for the file
		siaPath, err := modules.NewSiaPath(name)
		if err != nil {
			t.Fatal(err)
		}
		// Update the upload params
		up.SiaPath = siaPath
		if strings.Contains(name, "remoteFile") {
			up.Source = ""
		}
		if strings.Contains(name, "localFile") {
			up.Source = source
		}
		// Create the siafile and open it. Filesize is small as the files need
		// to each only have one chunk. This is because there are 4 files and
		// the uploadHeap size for testing is 5. If there are more than 5 chunks
		// total the test will fail and not hit the intended test case.
		err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), 100, persist.DefaultDiskPermissionsTest, false)
		if err != nil {
			t.Fatal(err)
		}
		f, err := rt.renter.staticFileSystem.OpenSiaFile(up.SiaPath)
		if err != nil {
			t.Fatal(err)
		}
		// Track number of chunks
		numChunks += int(f.NumChunks())
		// Make sure directories are created
		dirSiaPath, err := siaPath.Dir()
		if err != nil {
			t.Fatal(err)
		}
		err = rt.renter.CreateDir(dirSiaPath, modules.DefaultDirPerm)
		if err != nil && err != filesystem.ErrExists {
			t.Fatal(err)
		}
		dirSiaPaths.callAdd(dirSiaPath)
	}

	// Call bubbled to ensure directory metadata is updated
	dirSiaPaths.callRefreshAll()

	// Block until all bubbles are done
	rt.managedBlockUntilBubblesComplete()

	// Manually add workers to worker pool and create host map
	hosts := make(map[string]struct{})
	rt.renter.staticWorkerPool.mu.Lock()
	for i := 0; i < rsc.MinPieces(); i++ {
		rt.renter.staticWorkerPool.workers[fmt.Sprint(i)] = &worker{
			killChan: make(chan struct{}),
		}
	}
	rt.renter.staticWorkerPool.mu.Unlock()

	// Make sure directory Heap is ready
	err = rt.renter.managedPushUnexploredDirectory(modules.RootSiaPath())
	if err != nil {
		t.Fatal(err)
	}

	// call managedAddChunksToHeap
	_, err = rt.renter.managedAddChunksToHeap(hosts)
	if err != nil {
		t.Fatal(err)
	}

	// Since there are fewer chunks than the max size of the heap, the repair
	// code will add all the chunks to the heap.
	//
	// NOTE: through print statements it was validated that the test starts with
	// trying to add the chunks from the root directory. The local file is
	// skipped because the directory heap has a remote file in it. The remote
	// file in the root directory is added and the root directory is added back
	// to the directory heap, this time it is not seen as remote because the
	// remote file has already been added.
	//
	// The sub directory is then popped, since no more remote files are seen in
	// the directory heap, both the remote and local chunks are added.
	//
	// Then the root directory is popped again and now the local file is added.
	if rt.renter.uploadHeap.managedLen() != numChunks {
		t.Fatalf("Expected uploadHeap to have %v chunks but it has %v chunks", numChunks, rt.renter.uploadHeap.managedLen())
	}
}

// TestAddDirectoryBackToHeap ensures that when not all the chunks in a
// directory are added to the uploadHeap that the directory is added back to the
// directoryHeap with an updated Health
func TestAddDirectoryBackToHeap(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter with interrupt dependency
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create file
	rsc, _ := siafile.NewRSCode(1, 1)
	siaPath, err := modules.NewSiaPath("test")
	if err != nil {
		t.Fatal(err)
	}
	source, err := rt.createZeroByteFileOnDisk()
	if err != nil {
		t.Fatal(err)
	}
	up := modules.FileUploadParams{
		Source:      source,
		SiaPath:     siaPath,
		ErasureCode: rsc,
	}
	err = rt.renter.staticFileSystem.NewSiaFile(up.SiaPath, up.Source, up.ErasureCode, crypto.GenerateSiaKey(crypto.RandomCipherType()), modules.SectorSize, persist.DefaultDiskPermissionsTest, false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := rt.renter.staticFileSystem.OpenSiaFile(up.SiaPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create maps for method inputs
	hosts := make(map[string]struct{})
	offline := make(map[string]bool)
	goodForRenew := make(map[string]bool)

	// Manually add workers to worker pool
	rt.renter.staticWorkerPool.mu.Lock()
	for i := 0; i < int(f.NumChunks()); i++ {
		rt.renter.staticWorkerPool.workers[fmt.Sprint(i)] = &worker{
			killChan: make(chan struct{}),
		}
	}
	rt.renter.staticWorkerPool.mu.Unlock()

	// Confirm we are starting with an empty upload and directory heap
	if rt.renter.uploadHeap.managedLen() != 0 {
		t.Fatal("Expected upload heap to be empty but has length of", rt.renter.uploadHeap.managedLen())
	}
	// "Empty" -> gets initialized with the root dir, therefore should have one
	// directory in it.
	if rt.renter.directoryHeap.managedLen() != 1 {
		t.Fatal("Expected directory heap to be empty but has length of", rt.renter.directoryHeap.managedLen())
	}
	// Reset the dir heap to clear the root dir out, rest of test wants an empty
	// heap.
	rt.renter.directoryHeap.managedReset()

	// Add chunks from file to uploadHeap
	rt.renter.callBuildAndPushChunks([]*filesystem.FileNode{f}, hosts, targetUnstuckChunks, offline, goodForRenew)

	// Upload heap should now have NumChunks chunks and directory heap should still be empty
	if rt.renter.uploadHeap.managedLen() != int(f.NumChunks()) {
		t.Fatalf("Expected upload heap to be of size %v but was %v", f.NumChunks(), rt.renter.uploadHeap.managedLen())
	}
	if rt.renter.directoryHeap.managedLen() != 0 {
		t.Fatal("Expected directory heap to be empty but has length of", rt.renter.directoryHeap.managedLen())
	}

	// Empty uploadHeap
	rt.renter.uploadHeap.managedReset()

	// Fill upload heap with chunks that are a worse health than the chunks in
	// the file
	var i uint64
	for rt.renter.uploadHeap.managedLen() < maxUploadHeapChunks {
		chunk := &unfinishedUploadChunk{
			id: uploadChunkID{
				fileUID: "chunk",
				index:   i,
			},
			stuck:           false,
			piecesCompleted: -1,
			piecesNeeded:    1,
			availableChan:   make(chan struct{}),
		}
		if !rt.renter.uploadHeap.managedPush(chunk) {
			t.Fatal("Chunk should have been added to heap")
		}
		i++
	}

	// Record length of upload heap
	uploadHeapLen := rt.renter.uploadHeap.managedLen()

	// Try and add chunks to upload heap again
	rt.renter.callBuildAndPushChunks([]*filesystem.FileNode{f}, hosts, targetUnstuckChunks, offline, goodForRenew)

	// No chunks should have been added to the upload heap
	if rt.renter.uploadHeap.managedLen() != uploadHeapLen {
		t.Fatalf("Expected upload heap to be of size %v but was %v", uploadHeapLen, rt.renter.uploadHeap.managedLen())
	}
	// There should be one directory in the directory heap now
	if rt.renter.directoryHeap.managedLen() != 1 {
		t.Fatal("Expected directory heap to have 1 element but has length of", rt.renter.directoryHeap.managedLen())
	}
	// The directory should be marked as explored
	d := rt.renter.directoryHeap.managedPop()
	if !d.explored {
		t.Fatal("Directory should be explored")
	}
	// The directory should be the root directory as that is where we created
	// the test file
	if !d.staticSiaPath.Equals(modules.RootSiaPath()) {
		t.Fatal("Expected Directory siapath to be the root siaPath but was", d.staticSiaPath.String())
	}
	// The directory health should be that of the file since none of the chunks
	// were added
	health, _, _, _, _ := f.Health(offline, goodForRenew)
	if d.health != health {
		t.Fatalf("Expected directory health to be %v but was %v", health, d.health)
	}
}

// TestUploadHeapMaps tests that the uploadHeap's maps are properly updated
// through pushing, popping, and reseting the heap
func TestUploadHeapMaps(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Add stuck and unstuck chunks to heap to fill up the heap maps
	numHeapChunks := uint64(10)
	sf, err := rt.renter.newRenterTestFile()
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(0); i < numHeapChunks; i++ {
		// Create minimum chunk
		stuck := i%2 == 0
		chunk := &unfinishedUploadChunk{
			id: uploadChunkID{
				fileUID: siafile.SiafileUID(fmt.Sprintf("chunk - %v", i)),
				index:   i,
			},
			fileEntry:       sf.Copy(),
			stuck:           stuck,
			piecesCompleted: 1,
			piecesNeeded:    1,
			availableChan:   make(chan struct{}),
		}
		// push chunk to heap
		if !rt.renter.uploadHeap.managedPush(chunk) {
			t.Fatal("unable to push chunk", chunk)
		}
		// Confirm chunk is in the correct map
		if stuck {
			_, ok := rt.renter.uploadHeap.stuckHeapChunks[chunk.id]
			if !ok {
				t.Fatal("stuck chunk not in stuck chunk heap map")
			}
		} else {
			_, ok := rt.renter.uploadHeap.unstuckHeapChunks[chunk.id]
			if !ok {
				t.Fatal("unstuck chunk not in unstuck chunk heap map")
			}
		}
	}

	// Close original siafile entry
	sf.Close()

	// Confirm length of maps
	if len(rt.renter.uploadHeap.unstuckHeapChunks) != int(numHeapChunks/2) {
		t.Fatalf("Expected %v unstuck chunks in map but found %v", numHeapChunks/2, len(rt.renter.uploadHeap.unstuckHeapChunks))
	}
	if len(rt.renter.uploadHeap.stuckHeapChunks) != int(numHeapChunks/2) {
		t.Fatalf("Expected %v stuck chunks in map but found %v", numHeapChunks/2, len(rt.renter.uploadHeap.stuckHeapChunks))
	}
	if len(rt.renter.uploadHeap.repairingChunks) != 0 {
		t.Fatalf("Expected %v repairing chunks in map but found %v", 0, len(rt.renter.uploadHeap.repairingChunks))
	}

	// Pop off some chunks
	poppedChunks := 3
	for i := 0; i < poppedChunks; i++ {
		// Pop chunk
		chunk := rt.renter.uploadHeap.managedPop()
		// Confirm it is in the repairing map
		_, ok := rt.renter.uploadHeap.repairingChunks[chunk.id]
		if !ok {
			t.Fatal("popped chunk not found in repairing map")
		}
		// Confirm the chunk cannot be pushed back onto the heap
		if rt.renter.uploadHeap.managedPush(chunk) {
			t.Fatal("should not have been able to push chunk back onto heap")
		}
	}

	// Confirm length of maps
	if len(rt.renter.uploadHeap.repairingChunks) != poppedChunks {
		t.Fatalf("Expected %v repairing chunks in map but found %v", poppedChunks, len(rt.renter.uploadHeap.repairingChunks))
	}
	remainingChunks := len(rt.renter.uploadHeap.unstuckHeapChunks) + len(rt.renter.uploadHeap.stuckHeapChunks)
	if remainingChunks != int(numHeapChunks)-poppedChunks {
		t.Fatalf("Expected %v chunks to still be in the heap maps but found %v", int(numHeapChunks)-poppedChunks, remainingChunks)
	}

	// Reset the heap
	if err := rt.renter.uploadHeap.managedReset(); err != nil {
		t.Fatal(err)
	}

	// Confirm length of maps
	if len(rt.renter.uploadHeap.repairingChunks) != poppedChunks {
		t.Fatalf("Expected %v repairing chunks in map but found %v", poppedChunks, len(rt.renter.uploadHeap.repairingChunks))
	}
	remainingChunks = len(rt.renter.uploadHeap.unstuckHeapChunks) + len(rt.renter.uploadHeap.stuckHeapChunks)
	if remainingChunks != 0 {
		t.Fatalf("Expected %v chunks to still be in the heap maps but found %v", 0, remainingChunks)
	}
}

// TestUploadHeapPauseChan makes sure that sequential calls to pause and resume
// won't cause panics for closing a closed channel
func TestUploadHeapPauseChan(t *testing.T) {
	// Initial UploadHeap with the pauseChan initialized such that the uploads
	// and repairs are not paused
	uh := uploadHeap{
		pauseChan: make(chan struct{}),
	}
	close(uh.pauseChan)
	if uh.managedIsPaused() {
		t.Error("Repairs and Uploads should not be paused")
	}

	// Call resume on an initialized heap
	uh.managedResume()

	// Call Pause twice in a row
	uh.managedPause(DefaultPauseDuration)
	uh.managedPause(DefaultPauseDuration)
	// Call Resume twice in a row
	uh.managedResume()
	uh.managedResume()
}

// TestChunkSwitchStuckStatus is a regression test that confirms the upload heap
// won't panic due to a chunk's stuck status changing while it is in the heap
// and being added twice
func TestChunkSwitchStuckStatus(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create renter
	rt, err := newRenterTesterWithDependency(t.Name(), &dependencies.DependencyDisableRepairAndHealthLoops{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Create minimum chunk
	chunk := &unfinishedUploadChunk{
		id: uploadChunkID{
			fileUID: siafile.SiafileUID("chunk"),
			index:   0,
		},
	}
	// push chunk to heap
	if !rt.renter.uploadHeap.managedPush(chunk) {
		t.Fatal("unable to push chunk", chunk)
	}
	if rt.renter.uploadHeap.managedLen() != 1 {
		t.Error("Expected only 1 chunk in heap")
	}

	// Mark chunk as stuck and push again
	//
	// Regression check 1: previously this second push call would succeed and
	// the length of the heap would be 2
	chunk.stuck = true
	if rt.renter.uploadHeap.managedPush(chunk) {
		t.Error("should not be able to push chunk again")
	}
	if rt.renter.uploadHeap.managedLen() != 1 {
		t.Error("Expected only 1 chunk in heap")
	}

	// Pop the chunk
	chunk = rt.renter.uploadHeap.managedPop()
	if chunk == nil {
		t.Fatal("Nil chunk popped")
	}

	// A second pop call should not panic
	//
	// Regression check 2: previously this would trigger the build.Critical that
	// the popped chunk was already in the repair map
	chunk = rt.renter.uploadHeap.managedPop()
	if chunk != nil {
		t.Fatal("Expected nil chunk")
	}
}

// TestRenterAddChunksToHeapPanic tests that the log.Severe is triggered if
// there is an error getting a directory from the directory heap.
func TestRenterAddChunksToHeapPanic(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Renter
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Add maxConsecutiveDirHeapFailures non existent directories to the
	// directoryHeap
	for i := 0; i < maxConsecutiveDirHeapFailures; i++ {
		rt.renter.directoryHeap.managedPush(&directory{
			staticSiaPath: modules.RandomSiaPath(),
		})
	}

	// Recover panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()

	// Call managedAddChunksToHeap
	rt.renter.managedAddChunksToHeap(nil)
}

// managedBlockUntilBubblesComplete is a helper that blocks until all pending
// bubbles are complete
func (rt *renterTester) managedBlockUntilBubblesComplete() {
	for {
		select {
		case <-rt.renter.tg.StopChan():
			return
		default:
		}
		// Sleep to start the loop since most of the time bubble is called in a
		// go routine so we want to give it time to start.
		time.Sleep(100 * time.Millisecond)
		rt.renter.bubbleUpdatesMu.Lock()
		if len(rt.renter.bubbleUpdates) == 0 {
			rt.renter.bubbleUpdatesMu.Unlock()
			return
		}
		rt.renter.bubbleUpdatesMu.Unlock()
	}
}
