package renter

import (
	"io"
	"os"
	"path/filepath"

	"gitlab.com/SiaPrime/SiaPrime/build"
	"gitlab.com/SiaPrime/SiaPrime/encoding"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/siadir"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/siafile"
	"gitlab.com/SiaPrime/SiaPrime/persist"
	"gitlab.com/SiaPrime/SiaPrime/types"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/writeaheadlog"
)

const (
	logFile = modules.RenterDir + ".log"
	// PersistFilename is the filename to be used when persisting renter
	// information to a JSON file
	PersistFilename = "renter.json"
	// SiaDirMetadata is the name of the metadata file for the sia directory
	SiaDirMetadata = ".siadir"
	// walFile is the filename of the renter's writeaheadlog's file.
	walFile = modules.RenterDir + ".wal"
	// repairLoopFilename is the filename to be used when persisting bubble
	// updates that are called from the repair loop
	repairLoopFilename = "repairloop.json"
)

var (
	//ErrBadFile is an error when a file does not qualify as .sia file
	ErrBadFile = errors.New("not a .sia file")
	// ErrIncompatible is an error when file is not compatible with current
	// version
	ErrIncompatible = errors.New("file is not compatible with current version")
	// ErrNoNicknames is an error when no nickname is given
	ErrNoNicknames = errors.New("at least one nickname must be supplied")
	// ErrNonShareSuffix is an error when the suffix of a file does not match
	// the defined share extension
	ErrNonShareSuffix = errors.New("suffix of file must be " + modules.SiaFileExtension)

	settingsMetadata = persist.Metadata{
		Header:  "Renter Persistence",
		Version: persistVersion,
	}

	shareHeader  = [15]byte{'S', 'i', 'a', ' ', 'S', 'h', 'a', 'r', 'e', 'd', ' ', 'F', 'i', 'l', 'e'}
	shareVersion = "0.4"

	// Persist Version Numbers
	persistVersion040 = "0.4"
	persistVersion133 = "1.3.3"
	persistVersion140 = "1.4.0"
)

type (
	// persist contains all of the persistent renter data.
	persistence struct {
		MaxDownloadSpeed int64
		MaxUploadSpeed   int64
		StreamCacheSize  uint64
	}
)

// MarshalSia implements the encoding.SiaMarshaller interface, writing the
// file data to w.
func (f *file) MarshalSia(w io.Writer) error {
	enc := encoding.NewEncoder(w)

	// encode easy fields
	err := enc.EncodeAll(
		f.name,
		f.size,
		f.masterKey,
		f.pieceSize,
		f.mode,
	)
	if err != nil {
		return err
	}
	// COMPATv0.4.3 - encode the bytesUploaded and chunksUploaded fields
	// TODO: the resulting .sia file may confuse old clients.
	err = enc.EncodeAll(f.pieceSize*f.numChunks()*uint64(f.erasureCode.NumPieces()), f.numChunks())
	if err != nil {
		return err
	}

	// encode erasureCode
	switch code := f.erasureCode.(type) {
	case *siafile.RSCode:
		err = enc.EncodeAll(
			"Reed-Solomon",
			uint64(code.MinPieces()),
			uint64(code.NumPieces()-code.MinPieces()),
		)
		if err != nil {
			return err
		}
	default:
		if build.DEBUG {
			panic("unknown erasure code")
		}
		return errors.New("unknown erasure code")
	}
	// encode contracts
	if err := enc.Encode(uint64(len(f.contracts))); err != nil {
		return err
	}
	for _, c := range f.contracts {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalSia implements the encoding.SiaUnmarshaler interface,
// reconstructing a file from the encoded bytes read from r.
func (f *file) UnmarshalSia(r io.Reader) error {
	dec := encoding.NewDecoder(r, 100e6)

	// COMPATv0.4.3 - decode bytesUploaded and chunksUploaded into dummy vars.
	var bytesUploaded, chunksUploaded uint64

	// Decode easy fields.
	err := dec.DecodeAll(
		&f.name,
		&f.size,
		&f.masterKey,
		&f.pieceSize,
		&f.mode,
		&bytesUploaded,
		&chunksUploaded,
	)
	if err != nil {
		return err
	}
	f.staticUID = persist.RandomSuffix()

	// Decode erasure coder.
	var codeType string
	if err := dec.Decode(&codeType); err != nil {
		return err
	}
	switch codeType {
	case "Reed-Solomon":
		var nData, nParity uint64
		err = dec.DecodeAll(
			&nData,
			&nParity,
		)
		if err != nil {
			return err
		}
		rsc, err := siafile.NewRSCode(int(nData), int(nParity))
		if err != nil {
			return err
		}
		f.erasureCode = rsc
	default:
		return errors.New("unrecognized erasure code type: " + codeType)
	}

	// Decode contracts.
	var nContracts uint64
	if err := dec.Decode(&nContracts); err != nil {
		return err
	}
	f.contracts = make(map[types.FileContractID]fileContract)
	var contract fileContract
	for i := uint64(0); i < nContracts; i++ {
		if err := dec.Decode(&contract); err != nil {
			return err
		}
		f.contracts[contract.ID] = contract
	}
	return nil
}

// saveBubbleUpdates stores the current bubble updates to disk and then syncs to disk.
func (r *Renter) saveBubbleUpdates() error {
	return persist.SaveJSON(persist.Metadata{}, r.bubbleUpdates, filepath.Join(r.persistDir, repairLoopFilename))
}

// saveSync stores the current renter data to disk and then syncs to disk.
func (r *Renter) saveSync() error {
	return persist.SaveJSON(settingsMetadata, r.persist, filepath.Join(r.persistDir, PersistFilename))
}

// loadAndExecuteBubbleUpdates loads any bubble updates from disk and calls each
// update in its own thread
func (r *Renter) loadAndExecuteBubbleUpdates() error {
	err := persist.LoadJSON(persist.Metadata{}, r.bubbleUpdates, filepath.Join(r.persistDir, repairLoopFilename))
	if os.IsNotExist(err) {
		err = r.saveBubbleUpdates()
	}
	if err != nil {
		return err
	}
	var siaPath modules.SiaPath
	for dir := range r.bubbleUpdates {
		siaPath.LoadString(dir)
		go r.threadedBubbleMetadata(siaPath)
	}
	return nil
}

// managedLoadSettings fetches the saved renter data from disk.
func (r *Renter) managedLoadSettings() error {
	r.persist = persistence{}
	err := persist.LoadJSON(settingsMetadata, &r.persist, filepath.Join(r.persistDir, PersistFilename))
	if os.IsNotExist(err) {
		// No persistence yet, set the defaults and continue.
		r.persist.MaxDownloadSpeed = DefaultMaxDownloadSpeed
		r.persist.MaxUploadSpeed = DefaultMaxUploadSpeed
		r.persist.StreamCacheSize = DefaultStreamCacheSize
		err = r.saveSync()
		if err != nil {
			return err
		}
	} else if err == persist.ErrBadVersion {
		// Outdated version, try the 040 to 133 upgrade.
		err = convertPersistVersionFrom040To133(filepath.Join(r.persistDir, PersistFilename))
		if err != nil {
			r.log.Println("WARNING: 040 to 133 renter upgrade failed, trying 133 to 140 next", err)
		}
		// Then upgrade from 133 to 140.
		oldContracts := r.hostContractor.OldContracts()
		err = r.convertPersistVersionFrom133To140(filepath.Join(r.persistDir, PersistFilename), oldContracts)
		if err != nil {
			r.log.Println("WARNING: 133 to 140 renter upgrade failed", err)
			// Nothing left to try.
			return err
		}
		r.log.Println("Renter upgrade successful")
		// Re-load the settings now that the file has been upgraded.
		return r.managedLoadSettings()
	} else if err != nil {
		return err
	}

	// Set the bandwidth limits on the contractor, which was already initialized
	// without bandwidth limits.
	return r.setBandwidthLimits(r.persist.MaxDownloadSpeed, r.persist.MaxUploadSpeed)
}

// managedInitPersist handles all of the persistence initialization, such as creating
// the persistence directory and starting the logger.
func (r *Renter) managedInitPersist() error {
	// Create the persist and files directories if they do not yet exist.
	//
	// Note: the os package needs to be used here instead of the renter's
	// CreateDir method because the staticDirSet has not been initialized yet.
	// The directory is needed before the staticDirSet can be initialized
	// because the wal needs the directory to be created and the staticDirSet
	// needs the wal.
	err := os.MkdirAll(r.staticFilesDir, 0700)
	if err != nil {
		return err
	}

	// Initialize the logger.
	r.log, err = persist.NewFileLogger(filepath.Join(r.persistDir, logFile))
	if err != nil {
		return err
	}

	// Initialize the writeaheadlog.
	txns, wal, err := writeaheadlog.New(filepath.Join(r.persistDir, walFile))
	if err != nil {
		return err
	}

	// Apply unapplied wal txns before loading the persistence structure to
	// avoid loading potentially corrupted files.
	for _, txn := range txns {
		applyTxn := true
		for _, update := range txn.Updates {
			if siafile.IsSiaFileUpdate(update) {
				if err := siafile.ApplyUpdates(update); err != nil {
					return errors.AddContext(err, "failed to apply SiaFile update")
				}
			} else if siadir.IsSiaDirUpdate(update) {
				if err := siadir.ApplyUpdates(update); err != nil {
					return errors.AddContext(err, "failed to apply SiaDir update")
				}
			} else {
				applyTxn = false
			}
		}
		if applyTxn {
			if err := txn.SignalUpdatesApplied(); err != nil {
				return err
			}
		}
	}

	// Initialize the wal, staticFileSet and the staticDirSet. With the
	// staticDirSet finish the initialization of the files directory
	r.wal = wal
	r.staticFileSet = siafile.NewSiaFileSet(r.staticFilesDir, wal)
	r.staticDirSet = siadir.NewSiaDirSet(r.staticFilesDir, wal)
	if err := r.staticDirSet.InitRootDir(); err != nil {
		return err
	}
	// Load the prior persistence structures.
	return r.managedLoadSettings()
}
