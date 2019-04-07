package siadir

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SiaPrime/SiaPrime/modules"
)

// checkMetadataInit is a helper that verifies that the metadata was initialized
// properly
func checkMetadataInit(md Metadata) error {
	if md.Health != DefaultDirHealth {
		return fmt.Errorf("SiaDir health not set properly: got %v expected %v", md.Health, DefaultDirHealth)
	}
	if md.ModTime.IsZero() {
		return errors.New("ModTime not initialized")
	}
	if md.NumStuckChunks != 0 {
		return fmt.Errorf("SiaDir NumStuckChunks not initialized properly, expected 0, got %v", md.NumStuckChunks)
	}
	if md.StuckHealth != DefaultDirHealth {
		return fmt.Errorf("SiaDir stuck health not set properly: got %v expected %v", md.StuckHealth, DefaultDirHealth)
	}
	return nil
}

// newRootDir creates a root directory for the test and removes old test files
func newRootDir(t *testing.T) (string, error) {
	dir := filepath.Join(os.TempDir(), "siadirs", t.Name())
	err := os.RemoveAll(dir)
	if err != nil {
		return "", err
	}
	return dir, nil
}

// TestNewSiaDir tests that siadirs are created on disk properly. It uses
// LoadSiaDir to read the metadata from disk
func TestNewSiaDir(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create New SiaDir that is two levels deep
	rootDir, err := newRootDir(t)
	if err != nil {
		t.Fatal(err)
	}
	siaPathDir, err := modules.NewSiaPath("TestDir")
	if err != nil {
		t.Fatal(err)
	}
	siaPathSubDir, err := modules.NewSiaPath("SubDir")
	if err != nil {
		t.Fatal(err)
	}
	siaPath, err := siaPathDir.Join(siaPathSubDir.String())
	if err != nil {
		t.Fatal(err)
	}
	wal, _ := newTestWAL()
	siaDir, err := New(siaPath, rootDir, wal)
	if err != nil {
		t.Fatal(err)
	}

	// Check Sub Dir
	//
	// Check that the metadta was initialized properly
	md := siaDir.metadata
	if err = checkMetadataInit(md); err != nil {
		t.Fatal(err)
	}
	// Check that the SiaPath was initialized properly
	if siaDir.SiaPath() != siaPath {
		t.Fatalf("SiaDir SiaPath not set properly: got %v expected %v", siaDir.SiaPath(), siaPath)
	}
	// Check that the directory and .siadir file were created on disk
	_, err = os.Stat(siaPath.SiaDirSysPath(rootDir))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(siaPath.SiaDirMetadataSysPath(rootDir))
	if err != nil {
		t.Fatal(err)
	}

	// Check Top Directory
	//
	// Check that the directory and .siadir file were created on disk
	_, err = os.Stat(siaPath.SiaDirSysPath(rootDir))
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(siaPath.SiaDirMetadataSysPath(rootDir))
	if err != nil {
		t.Fatal(err)
	}
	// Get SiaDir
	subDir, err := LoadSiaDir(rootDir, siaPathDir, modules.ProdDependencies, wal)
	// Check that the metadata was initialized properly
	md = subDir.metadata
	if err = checkMetadataInit(md); err != nil {
		t.Fatal(err)
	}
	// Check that the SiaPath was initialized properly
	if subDir.SiaPath() != siaPathDir {
		t.Fatalf("SiaDir SiaPath not set properly: got %v expected %v", subDir.SiaPath(), siaPathDir)
	}

	// Check Root Directory
	//
	// Get SiaDir
	rootSiaDir, err := LoadSiaDir(rootDir, modules.RootSiaPath(), modules.ProdDependencies, wal)
	// Check that the metadata was initialized properly
	md = rootSiaDir.metadata
	if err = checkMetadataInit(md); err != nil {
		t.Fatal(err)
	}
	// Check that the SiaPath was initialized properly
	if !rootSiaDir.SiaPath().IsRoot() {
		t.Fatalf("SiaDir SiaPath not set properly: got %v expected %v", rootSiaDir.SiaPath().String(), "")
	}
	// Check that the directory and .siadir file were created on disk
	_, err = os.Stat(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(modules.RootSiaPath().SiaDirMetadataSysPath(rootDir))
	if err != nil {
		t.Fatal(err)
	}
}

// Test UpdatedMetadata probes the UpdateMetadata method
func TestUpdateMetadata(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create new siaDir
	rootDir, err := newRootDir(t)
	if err != nil {
		t.Fatal(err)
	}
	siaPath, err := modules.NewSiaPath("TestDir")
	if err != nil {
		t.Fatal(err)
	}
	wal, _ := newTestWAL()
	siaDir, err := New(siaPath, rootDir, wal)
	if err != nil {
		t.Fatal(err)
	}

	// Check metadata was initialized properly in memory and on disk
	md := siaDir.metadata
	if err = checkMetadataInit(md); err != nil {
		t.Fatal(err)
	}
	siaDir, err = LoadSiaDir(rootDir, siaPath, modules.ProdDependencies, wal)
	if err != nil {
		t.Fatal(err)
	}
	md = siaDir.metadata
	if err = checkMetadataInit(md); err != nil {
		t.Fatal(err)
	}

	// Set the metadata
	checkTime := time.Now()
	metadataUpdate := md
	metadataUpdate.Health = 4
	metadataUpdate.StuckHealth = 2
	metadataUpdate.LastHealthCheckTime = checkTime
	metadataUpdate.NumStuckChunks = 5

	err = siaDir.UpdateMetadata(metadataUpdate)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the metadata was updated properly in memory and on disk
	md = siaDir.metadata
	err = equalMetadatas(md, metadataUpdate)
	if err != nil {
		t.Fatal(err)
	}
	siaDir, err = LoadSiaDir(rootDir, siaPath, modules.ProdDependencies, wal)
	if err != nil {
		t.Fatal(err)
	}
	md = siaDir.metadata
	// Check Time separately due to how the time is persisted
	if !md.LastHealthCheckTime.Equal(metadataUpdate.LastHealthCheckTime) {
		t.Fatalf("LastHealthCheckTimes not equal, got %v expected %v", md.LastHealthCheckTime, metadataUpdate.LastHealthCheckTime)
	}
	metadataUpdate.LastHealthCheckTime = md.LastHealthCheckTime
	if !md.ModTime.Equal(metadataUpdate.ModTime) {
		t.Fatalf("ModTimes not equal, got %v expected %v", md.ModTime, metadataUpdate.ModTime)
	}
	metadataUpdate.ModTime = md.ModTime
	// Check the rest of the metadata
	err = equalMetadatas(md, metadataUpdate)
	if err != nil {
		t.Fatal(err)
	}
}

// TestDelete tests if deleting a siadir removes the siadir from disk and sets
// the deleted flag correctly.
func TestDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create SiaFileSet with SiaDir
	entry, _, err := newTestSiaDirSetWithDir()
	if err != nil {
		t.Fatal(err)
	}
	// Delete siadir.
	if err := entry.Delete(); err != nil {
		t.Fatal("Failed to delete siadir", err)
	}
	// Check if siadir was deleted and if deleted flag was set.
	if !entry.Deleted() {
		t.Fatal("Deleted flag was not set correctly")
	}
	siaDirPath := entry.metadata.SiaPath.SiaDirSysPath(entry.metadata.RootDir)
	if _, err := os.Open(siaDirPath); !os.IsNotExist(err) {
		t.Fatal("Expected a siadir doesn't exist error but got", err)
	}
}
