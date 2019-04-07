package renter

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/siadir"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/siafile"
	"gitlab.com/SiaPrime/SiaPrime/persist"
	"gitlab.com/SiaPrime/SiaPrime/siatest"
)

// TestStresstestSiaFileSet is a vlong test that performs multiple operations
// which modify the siafileset in parallel for a period of time.
func TestStresstestSiaFileSet(t *testing.T) {
	if !build.VLONG {
		t.SkipNow()
	}
	// Create a group for the test.
	groupParams := siatest.GroupParams{
		Hosts:   5,
		Renters: 1,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(renterTestDir(t.Name()), groupParams)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	// Run the test for a set amount of time.
	timer := time.NewTimer(2 * time.Minute)
	stop := make(chan struct{})
	go func() {
		<-timer.C
		close(stop)
	}()
	wg := new(sync.WaitGroup)
	r := tg.Renters()[0]
	// Upload params.
	dataPieces := uint64(1)
	parityPieces := uint64(1)
	// One thread uploads new files.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get a random directory to upload the file to.
			dirs, err := r.Dirs()
			if err != nil && strings.Contains(err.Error(), siadir.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil && strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := dirs[fastrand.Intn(len(dirs))]
			sp := filepath.Join(dir, persist.RandomSuffix())
			// 30% chance for the file to be a 0-byte file.
			size := int(modules.SectorSize) + siatest.Fuzz()
			if fastrand.Intn(3) == 0 {
				size = 0
			}
			// Upload the file
			lf, err := r.FilesDir().NewFile(size)
			if err != nil {
				t.Fatal(err)
			}
			rf, err := r.Upload(lf, sp, dataPieces, parityPieces, false)
			if err != nil && !strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) && !errors.Contains(err, siatest.ErrFileNotTracked) {
				t.Fatal(err)
			}
			if err := r.WaitForUploadRedundancy(rf, 1.0); err != nil && !errors.Contains(err, siatest.ErrFileNotTracked) {
				t.Fatal(err)
			}
			time.Sleep(time.Duration(fastrand.Intn(1000))*time.Millisecond + time.Second) // between 1s and 2s
		}
	}()
	// One thread force uploads new files to an existing siapath.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get existing files and choose one randomly.
			files, err := r.Files()
			if err != nil {
				t.Fatal(err)
			}
			// If there are no files we try again later.
			if len(files) == 0 {
				time.Sleep(time.Second)
				continue
			}
			// 30% chance for the file to be a 0-byte file.
			size := int(modules.SectorSize) + siatest.Fuzz()
			if fastrand.Intn(3) == 0 {
				size = 0
			}
			// Upload the file.
			sp := files[fastrand.Intn(len(files))].SiaPath
			lf, err := r.FilesDir().NewFile(size)
			if err != nil {
				t.Fatal(err)
			}
			rf, err := r.Upload(lf, sp, dataPieces, parityPieces, true)
			if err != nil && !strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) && !errors.Contains(err, siatest.ErrFileNotTracked) {
				t.Fatal(err)
			}
			if err := r.WaitForUploadRedundancy(rf, 1.0); err != nil && !errors.Contains(err, siatest.ErrFileNotTracked) {
				t.Fatal(err)
			}
			time.Sleep(time.Duration(fastrand.Intn(4000))*time.Millisecond + time.Second) // between 4s and 5s
		}
	}()
	// One thread renames files and sometimes uploads a new file directly
	// afterwards.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get existing files and choose one randomly.
			files, err := r.Files()
			if err != nil {
				t.Fatal(err)
			}
			// If there are no files we try again later.
			if len(files) == 0 {
				time.Sleep(time.Second)
				continue
			}
			sp := files[fastrand.Intn(len(files))].SiaPath
			err = r.RenterRenamePost(sp, persist.RandomSuffix())
			if err != nil && !strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) {
				t.Fatal(err)
			}
			// 50% chance to replace renamed file with new one.
			if fastrand.Intn(2) == 0 {
				lf, err := r.FilesDir().NewFile(int(modules.SectorSize) + siatest.Fuzz())
				if err != nil {
					t.Fatal(err)
				}
				err = r.RenterUploadForcePost(lf.Path(), sp, dataPieces, parityPieces, false)
				if err != nil && !strings.Contains(err.Error(), siafile.ErrPathOverload.Error()) {
					t.Fatal(err)
				}
			}
			time.Sleep(time.Duration(fastrand.Intn(4000))*time.Millisecond + time.Second) // between 4s and 5s
		}
	}()
	// One thread deletes files.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get existing files and choose one randomly.
			files, err := r.Files()
			if err != nil {
				t.Fatal(err)
			}
			// If there are no files we try again later.
			if len(files) == 0 {
				time.Sleep(time.Second)
				continue
			}
			sp := files[fastrand.Intn(len(files))].SiaPath
			err = r.RenterDeletePost(sp)
			if err != nil && !strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) {
				t.Fatal(err)
			}
			time.Sleep(time.Duration(fastrand.Intn(5000))*time.Millisecond + time.Second) // between 5s and 6s
		}
	}()
	// One thread creates empty dirs.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get a random directory to create a dir in.
			dirs, err := r.Dirs()
			if err != nil && strings.Contains(err.Error(), siadir.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil && strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := dirs[fastrand.Intn(len(dirs))]
			sp := filepath.Join(dir, persist.RandomSuffix())
			if err := r.RenterDirCreatePost(sp); err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Duration(fastrand.Intn(500))*time.Millisecond + 500*time.Millisecond) // between 0.5s and 1s
		}
	}()
	// One thread deletes a random directory and sometimes creates an empty one
	// in its place or simply renames it to be a sub dir of a random directory.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Get a random directory to delete.
			dirs, err := r.Dirs()
			if err != nil && strings.Contains(err.Error(), siadir.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil && strings.Contains(err.Error(), siafile.ErrUnknownPath.Error()) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := dirs[fastrand.Intn(len(dirs))]
			if fastrand.Intn(2) == 0 {
				// 50% chance to delete and recreate the directory.
				if err := r.RenterDirDeletePost(dir); err != nil {
					t.Fatal(err)
				}
				err := r.RenterDirCreatePost(dir)
				// NOTE we could probably avoid ignoring ErrPathOverload if we
				// decided that `siadir.New` returns a potentially existing
				// directory instead.
				if err != nil && !strings.Contains(err.Error(), siadir.ErrPathOverload.Error()) {
					t.Fatal(err)
				}
			} else {
				// TODO uncomment this one rename is implemented
				// 50% chance to rename the directory to be the child of a
				// random existing directory.
				//newParent := dirs[fastrand.Intn(len(dirs))]
				//newDir := filepath.Join(newParent, persist.RandomSuffix())
				//if err := r.RenterDirRenamePost(dir, newDir); err != nil {
				//	t.Fatal(err)
				//}
			}
			time.Sleep(time.Duration(fastrand.Intn(500))*time.Millisecond + 500*time.Millisecond) // between 0.5s and 1s
		}
	}()
	// One thread kills hosts to trigger repairs.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Break out if we only have dataPieces hosts left.
			hosts := tg.Hosts()
			if uint64(len(hosts)) == dataPieces {
				break
			}
			time.Sleep(10 * time.Second)
			// Choose random host.
			host := hosts[fastrand.Intn(len(hosts))]
			if err := tg.RemoveNode(host); err != nil {
				t.Fatal(err)
			}
		}
	}()
	// Wait until threads are done.
	wg.Wait()
}
