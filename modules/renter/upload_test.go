package renter

import (
	"io/ioutil"
	"os"
	"testing"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem/siafile"
)

// TestRenterUploadDirectory verifies that the renter returns an error if a
// directory is provided as the source of an upload.
func TestRenterUploadDirectory(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	testUploadPath, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testUploadPath)

	ec, err := siafile.NewRSCode(DefaultDataPieces, DefaultParityPieces)
	if err != nil {
		t.Fatal(err)
	}
	params := modules.FileUploadParams{
		Source:      testUploadPath,
		SiaPath:     modules.RandomSiaPath(),
		ErasureCode: ec,
	}
	err = rt.renter.Upload(params)
	if err == nil {
		t.Fatal("expected Upload to fail with empty directory as source")
	}
	if err != ErrUploadDirectory {
		t.Fatal("expected ErrUploadDirectory, got", err)
	}
}
