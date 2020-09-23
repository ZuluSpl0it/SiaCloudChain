package renter

// pubfile.go provides the tools for creating and uploading pubfiles, and then
// receiving the associated publinks to recover the files. The pubfile is the
// fundamental data structure underpinning Pubaccess.
//
// The primary trick of the pubfile is that the initial data is stored entirely
// in a single sector which is put on the ScPrime network using 1-of-N redundancy.
// Every replica has an identical Merkle root, meaning that someone attempting
// to fetch the file only needs the Merkle root and then some way to ask hosts
// on the network whether they have access to the Merkle root.
//
// That single sector then contains all of the other information that is
// necessary to recover the rest of the file. If the file is small enough, the
// entire file will be stored within the single sector. If the file is larger,
// the Merkle roots that are needed to download the remaining data get encoded
// into something called a 'fanout'. While the base chunk is required to use
// 1-of-N redundancy, the fanout chunks can use more sophisticated redundancy.
//
// The 1-of-N redundancy requirement really stems from the fact that Publinks
// are only 34 bytes of raw data, meaning that there's only enough room in a
// Publink to encode a single root. The fanout however has much more data to
// work with, meaning there is space to describe much fancier redundancy schemes
// and data fetching patterns.
//
// Pubfiles also contain some metadata which gets encoded as json. The
// intention is to allow uploaders to put any arbitrary metadata fields into
// their file and know that users will be able to see that metadata after
// downloading. A couple of fields such as the mode of the file are supported at
// the base level by ScPrime.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/fixtures"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem/siafile"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/NebulousLabs/errors"
)

const (
	// SkyfileLayoutSize describes the amount of space within the first sector
	// of a pubfile used to describe the rest of the pubfile.
	SkyfileLayoutSize = 99

	// SkyfileDefaultBaseChunkRedundancy establishes the default redundancy for
	// the base chunk of a pubfile.
	SkyfileDefaultBaseChunkRedundancy = 10

	// SkyfileVersion establishes the current version for creating pubfiles.
	// The pubfile versions are different from the siafile versions.
	SkyfileVersion = 1

	// layoutKeyDataSize is the size of the key-data field in a skyfileLayout.
	layoutKeyDataSize = 64
)

var (
	// ErrPublinkBlacklisted is the error returned when a publink is blacklisted
	ErrPublinkBlacklisted = errors.New("publink is blacklisted")

	// ErrMetadataTooBig is the error returned when the metadata exceeds a
	// sectorsize.
	ErrMetadataTooBig = errors.New("metadata exceeds sectorsize")

	// ErrRedundancyNotSupported is the error returned when trying to convert a
	// Siafile that was uploaded with redundancy that is not currently supported
	// by Pubaccess
	ErrRedundancyNotSupported = errors.New("publinks currently only support 1-of-N redundancy, other redundancies will be supported in a later version")

	// ExtendedSuffix is the suffix that is added to a pubfile siapath if it is
	// a large file upload
	ExtendedSuffix = "-extended"
)

// skyfileLayout explains the layout information that is used for storing data
// inside of the pubfile. The skyfileLayout always appears as the first bytes
// of the leading chunk.
type skyfileLayout struct {
	version            uint8
	filesize           uint64
	metadataSize       uint64
	fanoutSize         uint64
	fanoutDataPieces   uint8
	fanoutParityPieces uint8
	cipherType         crypto.CipherType
	keyData            [layoutKeyDataSize]byte // keyData is incompatible with ciphers that need keys larger than 64 bytes
}

// encode will return a []byte that has compactly encoded all of the layout
// data.
func (ll *skyfileLayout) encode() []byte {
	b := make([]byte, SkyfileLayoutSize)
	offset := 0
	b[offset] = ll.version
	offset += 1
	binary.LittleEndian.PutUint64(b[offset:], ll.filesize)
	offset += 8
	binary.LittleEndian.PutUint64(b[offset:], ll.metadataSize)
	offset += 8
	binary.LittleEndian.PutUint64(b[offset:], ll.fanoutSize)
	offset += 8
	b[offset] = ll.fanoutDataPieces
	offset += 1
	b[offset] = ll.fanoutParityPieces
	offset += 1
	copy(b[offset:], ll.cipherType[:])
	offset += len(ll.cipherType)
	copy(b[offset:], ll.keyData[:])
	offset += len(ll.keyData)

	// Sanity check. If this check fails, encode() does not match the
	// SkyfileLayoutSize.
	if offset != SkyfileLayoutSize {
		build.Critical("layout size does not match the amount of data encoded")
	}
	return b
}

// decode will take a []byte and load the layout from that []byte.
func (ll *skyfileLayout) decode(b []byte) {
	offset := 0
	ll.version = b[offset]
	offset += 1
	ll.filesize = binary.LittleEndian.Uint64(b[offset:])
	offset += 8
	ll.metadataSize = binary.LittleEndian.Uint64(b[offset:])
	offset += 8
	ll.fanoutSize = binary.LittleEndian.Uint64(b[offset:])
	offset += 8
	ll.fanoutDataPieces = b[offset]
	offset += 1
	ll.fanoutParityPieces = b[offset]
	offset += 1
	copy(ll.cipherType[:], b[offset:])
	offset += len(ll.cipherType)
	copy(ll.keyData[:], b[offset:])
	offset += len(ll.keyData)

	// Sanity check. If this check fails, decode() does not match the
	// SkyfileLayoutSize.
	if offset != SkyfileLayoutSize {
		build.Critical("layout size does not match the amount of data decoded")
	}
}

// skyfileBuildBaseSector will take all of the elements of the base sector and
// copy them into a freshly created base sector.
func skyfileBuildBaseSector(layoutBytes, fanoutBytes, metadataBytes, fileBytes []byte) ([]byte, uint64) {
	baseSector := make([]byte, modules.SectorSize)
	offset := 0
	copy(baseSector[offset:], layoutBytes)
	offset += len(layoutBytes)
	copy(baseSector[offset:], fanoutBytes)
	offset += len(fanoutBytes)
	copy(baseSector[offset:], metadataBytes)
	offset += len(metadataBytes)
	copy(baseSector[offset:], fileBytes)
	offset += len(fileBytes)
	return baseSector, uint64(offset)
}

// skyfileEstablishDefaults will set any zero values in the lup to be equal to
// the desired defaults.
func skyfileEstablishDefaults(lup *modules.PubfileUploadParameters) error {
	if lup.BaseChunkRedundancy == 0 {
		lup.BaseChunkRedundancy = SkyfileDefaultBaseChunkRedundancy
	}
	return nil
}

// skyfileMetadataBytes will return the marshalled/encoded bytes for the
// pubfile metadata.
func skyfileMetadataBytes(lm modules.PubfileMetadata) ([]byte, error) {
	// Compose the metadata into the leading chunk.
	metadataBytes, err := json.Marshal(lm)
	if err != nil {
		return nil, errors.AddContext(err, "unable to marshal the link file metadata")
	}
	return metadataBytes, nil
}

// fileUploadParamsFromLUP will derive the FileUploadParams to use when
// uploading the base chunk siafile of a pubfile using the pubfile's upload
// parameters.
func fileUploadParamsFromLUP(lup modules.PubfileUploadParameters) (modules.FileUploadParams, error) {
	// Create parameters to upload the file with 1-of-N erasure coding and no
	// encryption. This should cause all of the pieces to have the same Merkle
	// root, which is critical to making the file discoverable to viewnodes and
	// also resilient to host failures.
	ec, err := siafile.NewRSSubCode(1, int(lup.BaseChunkRedundancy)-1, crypto.SegmentSize)
	if err != nil {
		return modules.FileUploadParams{}, errors.AddContext(err, "unable to create erasure coder")
	}
	return modules.FileUploadParams{
		SiaPath:             lup.SiaPath,
		ErasureCode:         ec,
		Force:               lup.Force,
		DisablePartialChunk: true,  // must be set to true - partial chunks change, content addressed files must not change.
		Repair:              false, // indicates whether this is a repair operation
	}, nil
}

// streamerFromReader wraps a bytes.Reader to give it a Close() method, which
// allows it to satisfy the modules.Streamer interface.
type streamerFromReader struct {
	*bytes.Reader
}

// Close is a no-op because a bytes.Reader doesn't need to be closed.
func (sfr *streamerFromReader) Close() error {
	return nil
}

// streamerFromSlice returns a modules.Streamer given a slice. This is
// non-trivial because a bytes.Reader does not implement Close.
func streamerFromSlice(b []byte) modules.Streamer {
	reader := bytes.NewReader(b)
	return &streamerFromReader{
		Reader: reader,
	}
}

// CreatePublinkFromSiafile creates a pubfile from a siafile. This requires
// uploading a new pubfile which contains fanout information pointing to the
// siafile data. The SiaPath provided in 'lup' indicates where the new base
// sector pubfile will be placed, and the siaPath provided as its own input is
// the siaPath of the file that is being used to create the pubfile.
func (r *Renter) CreatePublinkFromSiafile(lup modules.PubfileUploadParameters, siaPath modules.SiaPath) (modules.Publink, error) {
	// Set reasonable default values for any lup fields that are blank.
	err := skyfileEstablishDefaults(&lup)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "pubfile upload parameters are incorrect")
	}

	// Grab the filenode for the provided siapath.
	fileNode, err := r.staticFileSystem.OpenSiaFile(siaPath)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to open siafile")
	}
	defer fileNode.Close()

	// Override the metadata with the info from the fileNode.
	lup.FileMetadata = modules.PubfileMetadata{
		Filename: siaPath.Name(),
		Mode:     fileNode.Mode(),
		Length:   fileNode.Size(),
	}
	return r.managedCreatePublinkFromFileNode(lup, fileNode)
}

// managedCreatePublinkFromFileNode creates a publink from a file node.
//
// The name needs to be passed in explicitly because a file node does not track
// its own name, which allows the file to be renamed concurrently without
// causing any race conditions.
func (r *Renter) managedCreatePublinkFromFileNode(lup modules.PubfileUploadParameters, fileNode *filesystem.FileNode) (modules.Publink, error) {
	// First check if any of the publinks associated with the siafile are
	// blacklisted
	skylinkstrs := fileNode.Metadata().Publinks
	for _, skylinkstr := range skylinkstrs {
		var publink modules.Publink
		err := publink.LoadString(skylinkstr)
		if err != nil {
			// If there is an error just continue as we shouldn't prevent the
			// conversion due to bad old publink
			//
			// Log the error for debugging purposes
			r.log.Printf("WARN: previous publink for siafile %v could not be loaded from string; potentially corrupt publink: %v", fileNode.SiaFilePath(), skylinkstr)
			continue
		}
		// Check if publink is blacklisted
		if r.staticSkynetBlacklist.IsBlacklisted(publink) {
			// Publink is blacklisted, return error and try and delete file
			return modules.Publink{}, errors.Compose(ErrPublinkBlacklisted, r.DeleteFile(lup.SiaPath))
		}
	}

	// Check that the encryption key and erasure code is compatible with the
	// pubfile format. This is intentionally done before any heavy computation
	// to catch early errors.
	var ll skyfileLayout
	masterKey := fileNode.MasterKey()
	if len(masterKey.Key()) > len(ll.keyData) {
		return modules.Publink{}, errors.New("cipher key is not supported by the pubfile format")
	}
	ec := fileNode.ErasureCode()
	if ec.Type() != siafile.ECReedSolomonSubShards64 {
		return modules.Publink{}, errors.New("siafile has unsupported erasure code type")
	}
	// Deny the conversion of siafiles that are not 1 data piece. Not because we
	// cannot download them, but because it is currently inefficient to download
	// them.
	if ec.MinPieces() != 1 {
		return modules.Publink{}, ErrRedundancyNotSupported
	}

	// Marshal the metadata.
	metadataBytes, err := skyfileMetadataBytes(lup.FileMetadata)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "error retrieving pubfile metadata bytes")
	}

	// Create the fanout for the siafile.
	fanoutBytes, err := skyfileEncodeFanout(fileNode)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to encode the fanout of the siafile")
	}
	headerSize := uint64(SkyfileLayoutSize + len(metadataBytes) + len(fanoutBytes))
	if headerSize > modules.SectorSize {
		return modules.Publink{}, fmt.Errorf("pubfile does not fit in leading chunk - metadata size plus fanout size must be less than %v bytes, metadata size is %v bytes and fanout size is %v bytes", modules.SectorSize-SkyfileLayoutSize, len(metadataBytes), len(fanoutBytes))
	}

	// Assemble the first chunk of the pubfile.
	ll = skyfileLayout{
		version:            SkyfileVersion,
		filesize:           fileNode.Size(),
		metadataSize:       uint64(len(metadataBytes)),
		fanoutSize:         uint64(len(fanoutBytes)),
		fanoutDataPieces:   uint8(ec.MinPieces()),
		fanoutParityPieces: uint8(ec.NumPieces() - ec.MinPieces()),
		cipherType:         masterKey.Type(),
	}
	// If we're uploading in plaintext, we put the key in the baseSector
	if !encryptionEnabled(lup) {
		copy(ll.keyData[:], masterKey.Key())
	}

	// Create the base sector.
	baseSector, fetchSize := skyfileBuildBaseSector(ll.encode(), fanoutBytes, metadataBytes, nil)

	// Encrypt the base sector if necessary.
	if encryptionEnabled(lup) {
		err = encryptBaseSectorWithSkykey(baseSector, ll, lup.FileSpecificSkykey)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "Failed to encrypt base sector for upload")
		}
	}

	// Create the publink.
	baseSectorRoot := crypto.MerkleRoot(baseSector)
	publink, err := modules.NewPublinkV1(baseSectorRoot, 0, fetchSize)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to build publink")
	}
	if lup.DryRun {
		return publink, nil
	}

	// Check if the new publink is blacklisted
	if r.staticSkynetBlacklist.IsBlacklisted(publink) {
		// publink is blacklisted, return error and try and delete file
		return modules.Publink{}, errors.Compose(ErrPublinkBlacklisted, r.DeleteFile(lup.SiaPath))
	}

	// Add the publink to the siafiles.
	err = fileNode.AddPublink(publink)
	if err != nil {
		return publink, errors.AddContext(err, "unable to add publink to the sianodes")
	}

	// Upload the base sector.
	err = r.managedUploadBaseSector(lup, baseSector, publink)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "Unable to upload base sector for file node. ")
	}

	return publink, errors.AddContext(err, "unable to add publink to the sianodes")
}

// managedCreateFileNodeFromReader takes the file upload parameters and a reader
// and returns a filenode. This method turns the reader into a FileNode without
// effectively uploading the data. It is used to perform a dry-run of a pubfile
// upload.
func (r *Renter) managedCreateFileNodeFromReader(up modules.FileUploadParams, reader io.Reader) (*filesystem.FileNode, error) {
	// Check the upload params first.
	fileNode, err := r.managedInitUploadStream(up)
	if err != nil {
		return nil, err
	}

	// Extract some helper variables
	hpk := types.SiaPublicKey{} // blank host key
	ec := fileNode.ErasureCode()
	psize := fileNode.PieceSize()
	csize := fileNode.ChunkSize()

	var peek []byte
	for chunkIndex := uint64(0); ; chunkIndex++ {
		// Grow the SiaFile to the right size.
		err := fileNode.SiaFile.GrowNumChunks(chunkIndex + 1)
		if err != nil {
			return nil, err
		}

		// Allocate data pieces and fill them with data from r.
		ss := NewStreamShard(reader, peek)
		err = func() error {
			defer ss.Close()

			dataPieces, total, errRead := readDataPieces(ss, ec, psize)
			if errRead != nil {
				return errRead
			}

			dataEncoded, _ := ec.EncodeShards(dataPieces)
			for pieceIndex, dataPieceEnc := range dataEncoded {
				if err := fileNode.SiaFile.AddPiece(hpk, chunkIndex, uint64(pieceIndex), crypto.MerkleRoot(dataPieceEnc)); err != nil {
					return err
				}
			}

			adjustedSize := fileNode.Size() - csize + total
			if err := fileNode.SetFileSize(adjustedSize); err != nil {
				return errors.AddContext(err, "failed to adjust FileSize")
			}
			return nil
		}()
		if err != nil {
			return nil, err
		}

		_, err = ss.Result()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return fileNode, nil
}

// Blacklist returns the merkleroots that are blacklisted
func (r *Renter) Blacklist() ([]crypto.Hash, error) {
	err := r.tg.Add()
	if err != nil {
		return []crypto.Hash{}, err
	}
	defer r.tg.Done()
	return r.staticSkynetBlacklist.Blacklist(), nil
}

// UpdateSkynetBlacklist updates the list of publinks that are blacklisted
func (r *Renter) UpdateSkynetBlacklist(additions, removals []modules.Publink) error {
	err := r.tg.Add()
	if err != nil {
		return err
	}
	defer r.tg.Done()
	return r.staticSkynetBlacklist.UpdateBlacklist(additions, removals)
}

// Portals returns the list of known pubaccess portals.
func (r *Renter) Portals() ([]modules.SkynetPortal, error) {
	err := r.tg.Add()
	if err != nil {
		return []modules.SkynetPortal{}, err
	}
	defer r.tg.Done()
	return r.staticSkynetPortals.Portals(), nil
}

// UpdateSkynetPortals updates the list of known pubaccess portals that are listed.
func (r *Renter) UpdateSkynetPortals(additions []modules.SkynetPortal, removals []modules.NetAddress) error {
	err := r.tg.Add()
	if err != nil {
		return err
	}
	defer r.tg.Done()
	return r.staticSkynetPortals.UpdatePortals(additions, removals)
}

// uploadSkyfileReadLeadingChunk will read the leading chunk of a pubfile. If
// entire file is small enough to fit inside of the leading chunk, the return
// value will be:
//
//   (fileBytes, nil, false, nil)
//
// And if the entire file is too large to fit inside of the leading chunk, the
// return value will be:
//
//   (nil, fileReader, true, nil)
//
// where the fileReader contains all of the data for the file, including the
// data that uploadSkyfileReadLeadingChunk had to read to figure out whether
// the file was too large to fit into the leading chunk.
func uploadSkyfileReadLeadingChunk(r io.Reader, headerSize uint64) ([]byte, io.Reader, bool, error) {
	// Check for underflow.
	if headerSize+1 > modules.SectorSize {
		return nil, nil, false, ErrMetadataTooBig
	}
	// Read data from the reader to fill out the remainder of the first sector.
	fileBytes := make([]byte, modules.SectorSize-headerSize, modules.SectorSize-headerSize+1) // +1 capacity for the peek byte
	size, err := io.ReadFull(r, fileBytes)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = nil
	}
	if err != nil {
		return nil, nil, false, errors.AddContext(err, "unable to read the file data")
	}
	// Set fileBytes to the right size.
	fileBytes = fileBytes[:size]

	// See whether there is more data in the reader. If there is no more data in
	// the reader, a small file will be signaled and the data that has been read
	// will be returned.
	peek := make([]byte, 1)
	n, peekErr := io.ReadFull(r, peek)
	if peekErr == io.EOF || peekErr == io.ErrUnexpectedEOF {
		peekErr = nil
	}
	if peekErr != nil {
		return nil, nil, false, errors.AddContext(err, "too much data provided, cannot create pubfile")
	}
	if n == 0 {
		// There is no more data, return the data that was read from the reader
		// and signal a small file.
		return fileBytes, nil, false, nil
	}

	// There is more data. Create a prepend reader using the data we've already
	// read plus the reader that we read from, effectively creating a new reader
	// that is identical to the one that was passed in if no data had been read.
	prependData := append(fileBytes, peek...)
	fullReader := io.MultiReader(bytes.NewReader(prependData), r)
	return nil, fullReader, true, nil
}

// managedUploadSkyfileLargeFile will accept a fileReader containing all of the
// data to a large siafile and upload it to the ScPrime network using
// 'callUploadStreamFromReader'. The final publink is created by calling
// 'CreatePublinkFromSiafile' on the resulting siafile.
func (r *Renter) managedUploadSkyfileLargeFile(lup modules.PubfileUploadParameters, fileReader io.Reader) (modules.Publink, error) {
	// Create the erasure coder to use when uploading the file. When going
	// through the 'managedUploadSkyfile' command, a 1-of-N scheme is always
	// used, where the redundancy of the data as a whole matches the proposed
	// redundancy for the base chunk.
	ec, err := siafile.NewRSSubCode(1, int(lup.BaseChunkRedundancy)-1, crypto.SegmentSize)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to create erasure coder for large file")
	}
	// Create the siapath for the pubfile extra data. This is going to be the
	// same as the pubfile upload siapath, except with a suffix.
	siaPath, err := modules.NewSiaPath(lup.SiaPath.String() + ExtendedSuffix)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to create SiaPath for large pubfile extended data")
	}
	fup := modules.FileUploadParams{
		SiaPath:             siaPath,
		ErasureCode:         ec,
		Force:               lup.Force,
		DisablePartialChunk: true,  // must be set to true - partial chunks change, content addressed files must not change.
		Repair:              false, // indicates whether this is a repair operation

		CipherType: crypto.TypePlain,
	}

	// Check if an encryption key was specified.
	if encryptionEnabled(lup) {
		fanoutSkykey, err := lup.FileSpecificSkykey.DeriveSubkey(fanoutNonceDerivation[:])
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "unable to derive fanout subkey")
		}
		fup.CipherKey, err = fanoutSkykey.CipherKey()
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "unable to get pubaccesskey cipherkey")
		}
		fup.CipherType = lup.FileSpecificSkykey.CipherType()
	}

	var fileNode *filesystem.FileNode
	if lup.DryRun {
		// In case of a dry-run we don't want to perform the actual upload,
		// instead we create a filenode that contains all of the data pieces and
		// their merkle roots.
		fileNode, err = r.managedCreateFileNodeFromReader(fup, fileReader)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "unable to upload large pubfile")
		}
	} else {
		// Upload the file using a streamer.
		fileNode, err = r.callUploadStreamFromReader(fup, fileReader)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "unable to upload large pubfile")
		}
	}

	// Defer closing and cleanup of the file in case this was a dry-run
	defer func() {
		err := fileNode.Close()
		if err != nil {
			r.log.Printf("Could not close node, err: %s\n", err.Error())
		}

		if lup.DryRun {
			if err := r.DeleteFile(siaPath); err != nil {
				r.log.Printf("unable to cleanup siafile after performing a dry run of the Pubfile upload, err: %s", err.Error())
			}
		}
	}()

	// Now that we know the filesize, extend the metadata.
	lup.FileMetadata.Length = fileNode.Size()

	// Convert the new siafile we just uploaded into a pubfile using the
	// convert function.
	return r.managedCreatePublinkFromFileNode(lup, fileNode)
}

// managedUploadBaseSector will take the raw baseSector bytes and upload them,
// returning the resulting merkle root, and the fileNode of the siafile that is
// tracking the base sector.
func (r *Renter) managedUploadBaseSector(lup modules.PubfileUploadParameters, baseSector []byte, publink modules.Publink) error {
	fileUploadParams, err := fileUploadParamsFromLUP(lup)
	if err != nil {
		return errors.AddContext(err, "failed to create siafile upload parameters")
	}
	fileUploadParams.CipherType = crypto.TypePlain // the baseSector should be encrypted by the caller.

	// Perform the actual upload. This will require turning the base sector into
	// a reader.
	baseSectorReader := bytes.NewReader(baseSector)
	fileNode, err := r.callUploadStreamFromReader(fileUploadParams, baseSectorReader)
	if err != nil {
		return errors.AddContext(err, "failed to stream upload small pubfile")
	}
	defer fileNode.Close()

	// Add the publink to the Siafile.
	err = fileNode.AddPublink(publink)
	return errors.AddContext(err, "unable to add publink to siafile")
}

// managedUploadSkyfileSmallFile uploads a file that fits entirely in the
// leading chunk of a pubfile to the ScPrime network and returns the publink that
// can be used to access the file.
func (r *Renter) managedUploadSkyfileSmallFile(lup modules.PubfileUploadParameters, fileBytes []byte) (modules.Publink, error) {
	// Complete the metadata by setting the length and marshal it.
	lup.FileMetadata.Length = uint64(len(fileBytes))
	metadataBytes, err := skyfileMetadataBytes(lup.FileMetadata)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to retrieve pubfile metadata bytes")
	}

	ll := skyfileLayout{
		version:      SkyfileVersion,
		filesize:     uint64(len(fileBytes)),
		metadataSize: uint64(len(metadataBytes)),
		// No fanout is set yet.
		// If encryption is set in the upload params, this will be overwritten.
		cipherType: crypto.TypePlain,
	}

	// Create the base sector. This is done as late as possible so that any
	// errors are caught before a large block of memory is allocated.
	baseSector, fetchSize := skyfileBuildBaseSector(ll.encode(), nil, metadataBytes, fileBytes) // 'nil' because there is no fanout

	if encryptionEnabled(lup) {
		err := encryptBaseSectorWithSkykey(baseSector, ll, lup.FileSpecificSkykey)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "Failed to encrypt base sector for upload")
		}
	}

	// Create the publink.
	baseSectorRoot := crypto.MerkleRoot(baseSector) // Should be identical to the sector roots for each sector in the siafile.
	publink, err := modules.NewPublinkV1(baseSectorRoot, 0, fetchSize)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "failed to build the publink")
	}

	// If this is a dry-run, we do not need to upload the base sector
	if lup.DryRun {
		return publink, nil
	}

	// Upload the base sector.
	err = r.managedUploadBaseSector(lup, baseSector, publink)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "failed to upload base sector")
	}
	return publink, nil
}

// parseSkyfileMetadata will pull the metadata (including layout and fanout) out
// of a pubfile.
func parseSkyfileMetadata(baseSector []byte) (sl skyfileLayout, fanoutBytes []byte, sm modules.PubfileMetadata, baseSectorPayload []byte, err error) {
	// Sanity check - baseSector should not be more than modules.SectorSize.
	// Note that the base sector may be smaller in the event of a packed
	// pubfile.
	if uint64(len(baseSector)) > modules.SectorSize {
		build.Critical("parseSkyfileMetadata given a baseSector that is too large")
	}

	// Parse the layout.
	var offset uint64
	sl.decode(baseSector)
	offset += SkyfileLayoutSize

	// Check the version.
	if sl.version != 1 {
		return skyfileLayout{}, nil, modules.PubfileMetadata{}, nil, errors.New("unsupported pubfile version")
	}

	// Currently there is no support for pubfiles with fanout + metadata that
	// exceeds the base sector.
	if offset+sl.fanoutSize+sl.metadataSize > uint64(len(baseSector)) || sl.fanoutSize > modules.SectorSize || sl.metadataSize > modules.SectorSize {
		return skyfileLayout{}, nil, modules.PubfileMetadata{}, nil, errors.New("this version of spd does not support pubfiles with large fanouts and metadata")
	}

	// Parse the fanout.
	// NOTE: we copy the fanoutBytes instead of returning a slice into baseSector
	// because in PinSkylink the baseSector may be re-encrypted.
	fanoutBytes = make([]byte, sl.fanoutSize)
	copy(fanoutBytes, baseSector[offset:offset+sl.fanoutSize])
	offset += sl.fanoutSize

	// Parse the metadata.
	metadataSize := sl.metadataSize
	err = json.Unmarshal(baseSector[offset:offset+metadataSize], &sm)
	if err != nil {
		return skyfileLayout{}, nil, modules.PubfileMetadata{}, nil, errors.AddContext(err, "unable to parse PubfileMetadata from pubfile base sector")
	}
	offset += metadataSize

	// In version 1, the base sector payload is nil unless there is no fanout.
	if sl.fanoutSize == 0 {
		baseSectorPayload = baseSector[offset : offset+sl.filesize]
	}

	return sl, fanoutBytes, sm, baseSectorPayload, nil
}

// DownloadPublink will take a link and turn it into the metadata and data of a
// download.
func (r *Renter) DownloadPublink(link modules.Publink, timeout time.Duration) (modules.PubfileMetadata, modules.Streamer, error) {
	if r.deps.Disrupt("resolveSkylinkToFixture") {
		sf, err := fixtures.LoadPublinkFixture(link)
		if err != nil {
			return modules.PubfileMetadata{}, nil, errors.AddContext(err, "failed to fetch fixture")
		}
		return sf.Metadata, streamerFromSlice(sf.Content), nil
	}

	// Check if link is blacklisted
	if r.staticSkynetBlacklist.IsBlacklisted(link) {
		return modules.PubfileMetadata{}, nil, ErrPublinkBlacklisted
	}

	// Check if this publink is already in the stream buffer set. If so, we can
	// skip the lookup procedure and use any data that other threads have
	// cached.
	id := link.DataSourceID()
	streamer, exists := r.staticStreamBufferSet.callNewStreamFromID(id, 0)
	if exists {
		return streamer.Metadata(), streamer, nil
	}

	// Pull the offset and fetchSize out of the publink.
	offset, fetchSize, err := link.OffsetAndFetchSize()
	if err != nil {
		return modules.PubfileMetadata{}, nil, errors.AddContext(err, "unable to parse publink")
	}

	// Fetch the leading chunk.
	baseSector, err := r.DownloadByRoot(link.MerkleRoot(), offset, fetchSize, timeout)
	if err != nil {
		return modules.PubfileMetadata{}, nil, errors.AddContext(err, "unable to fetch base sector of publink")
	}
	if len(baseSector) < SkyfileLayoutSize {
		return modules.PubfileMetadata{}, nil, errors.New("download did not fetch enough data, layout cannot be decoded")
	}

	// Check if the base sector is encrypted, and attempt to decrypt it.
	// This will fail if we don't have the decryption key.
	var fileSpecificSkykey pubaccesskey.Pubaccesskey
	if isEncryptedBaseSector(baseSector) {
		fileSpecificSkykey, err = r.decryptBaseSector(baseSector)
		if err != nil {
			return modules.PubfileMetadata{}, nil, errors.AddContext(err, "Unable to decrypt pubfile base sector")
		}
	}

	// Parse out the metadata of the pubfile.
	layout, fanoutBytes, metadata, baseSectorPayload, err := parseSkyfileMetadata(baseSector)
	if err != nil {
		return modules.PubfileMetadata{}, nil, errors.AddContext(err, "error parsing pubfile metadata")
	}

	// If there is no fanout, all of the data will be contained in the base
	// sector, return a streamer using the data from the base sector.
	if layout.fanoutSize == 0 {
		streamer := streamerFromSlice(baseSectorPayload)
		return metadata, streamer, nil
	}

	// There is a fanout, create a fanout streamer and return that.
	fs, err := r.newFanoutStreamer(link, layout, metadata, fanoutBytes, timeout, fileSpecificSkykey)
	if err != nil {
		return modules.PubfileMetadata{}, nil, errors.AddContext(err, "unable to create fanout fetcher")
	}
	return metadata, fs, nil
}

// PinPublink wil fetch the file associated with the Publink, and then pin all
// necessary content to maintain that Publink.
func (r *Renter) PinPublink(publink modules.Publink, lup modules.PubfileUploadParameters, timeout time.Duration) error {
	// Check if link is blacklisted
	if r.staticSkynetBlacklist.IsBlacklisted(publink) {
		return ErrPublinkBlacklisted
	}

	// Set sane defaults for unspecified values.
	skyfileEstablishDefaults(&lup)

	// Fetch the leading chunk.
	baseSector, err := r.DownloadByRoot(publink.MerkleRoot(), 0, modules.SectorSize, timeout)
	if err != nil {
		return errors.AddContext(err, "unable to fetch base sector of publink")
	}
	if uint64(len(baseSector)) != modules.SectorSize {
		return errors.New("download did not fetch enough data, file cannot be re-pinned")
	}

	// Check if the base sector is encrypted, and attempt to decrypt it.
	var fileSpecificSkykey pubaccesskey.Pubaccesskey
	encrypted := isEncryptedBaseSector(baseSector)
	if encrypted {
		fileSpecificSkykey, err = r.decryptBaseSector(baseSector)
		if err != nil {
			return errors.AddContext(err, "Unable to decrypt pubfile base sector")
		}
	}

	// Parse out the metadata of the pubfile.
	layout, fanoutBytes, metadata, _, err := parseSkyfileMetadata(baseSector)
	if err != nil {
		return errors.AddContext(err, "error parsing pubfile metadata")
	}

	// Start setting up the FUP.
	fup := modules.FileUploadParams{
		Force:               lup.Force,
		DisablePartialChunk: true,  // must be set to true - partial chunks change, content addressed files must not change.
		Repair:              false, // indicates whether this is a repair operation
		CipherType:          crypto.TypePlain,
	}

	// Re-encrypt the baseSector for upload and add the fanout key to the fup.
	if encrypted {
		err = encryptBaseSectorWithSkykey(baseSector, layout, fileSpecificSkykey)
		if err != nil {
			return errors.AddContext(err, "Error re-encrypting base sector")
		}

		// Derive the fanout key and add to the fup.
		fanoutSkykey, err := fileSpecificSkykey.DeriveSubkey(fanoutNonceDerivation[:])
		if err != nil {
			return errors.AddContext(err, "Error deriving fanout pubaccesskey")
		}
		fup.CipherKey, err = fanoutSkykey.CipherKey()
		if err != nil {
			return errors.AddContext(err, "Error getting fanout CipherKey")
		}
		fup.CipherType = fanoutSkykey.CipherType()

		// These fields aren't used yet, but we'll set them anyway to mimic behavior in
		// upload/download code for consistency.
		lup.SkykeyName = fileSpecificSkykey.Name
		lup.FileSpecificSkykey = fileSpecificSkykey
	}

	// Re-upload the baseSector.
	err = r.managedUploadBaseSector(lup, baseSector, publink)
	if err != nil {
		return errors.AddContext(err, "unable to upload base sector")
	}

	// If there is no fanout, nothing more to do, the pin is complete.
	if layout.fanoutSize == 0 {
		return nil
	}
	// Create the erasure coder to use when uploading the file bulk.
	fup.ErasureCode, err = siafile.NewRSSubCode(int(layout.fanoutDataPieces), int(layout.fanoutParityPieces), crypto.SegmentSize)
	if err != nil {
		return errors.AddContext(err, "unable to create erasure coder for large file")
	}
	// Create the siapath for the pubfile extra data. This is going to be the
	// same as the pubfile upload siapath, except with a suffix.
	fup.SiaPath, err = modules.NewSiaPath(lup.SiaPath.String() + "-extended")
	if err != nil {
		return errors.AddContext(err, "unable to create SiaPath for large pubfile extended data")
	}

	// Create the fanout streamer that will download the file.
	streamer, err := r.newFanoutStreamer(publink, layout, metadata, fanoutBytes, timeout, fileSpecificSkykey)
	if err != nil {
		return errors.AddContext(err, "Failed to create fanout streamer for large pubfile pin")
	}

	// Upload directly from the fanout download streamer.
	fileNode, err := r.callUploadStreamFromReader(fup, streamer)
	if err != nil {
		return errors.AddContext(err, "unable to upload large pubfile")
	}
	err = fileNode.AddPublink(publink)
	if err != nil {
		return errors.AddContext(err, "unable to upload pubfile fanout")
	}
	return nil
}

// UploadSkyfile will upload the provided data with the provided metadata,
// returning a publink which can be used by any viewnode to recover the full
// original file and metadata. The publink will be unique to the combination of
// both the file data and metadata.
func (r *Renter) UploadSkyfile(lup modules.PubfileUploadParameters) (modules.Publink, error) {
	// Set reasonable default values for any lup fields that are blank.
	err := skyfileEstablishDefaults(&lup)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "pubfile upload parameters are incorrect")
	}

	// If a pubaccesskey name or ID was specified, generate a file-specific key for this
	// upload.
	if encryptionEnabled(lup) && lup.SkykeyName != "" {
		key, err := r.SkykeyByName(lup.SkykeyName)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "UploadSkyfile unable to get pubaccesskey")
		}
		lup.FileSpecificSkykey, err = key.GenerateFileSpecificSubkey()
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "UploadSkyfile unable to generate subkey")
		}
	} else if encryptionEnabled(lup) {
		key, err := r.SkykeyByID(lup.PubaccesskeyID)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "UploadSkyfile unable to get pubaccesskey")
		}

		lup.FileSpecificSkykey, err = key.GenerateFileSpecificSubkey()
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "UploadSkyfile unable to generate subkey")
		}
	}

	// Additional input check - this check is unique to uploading a pubfile
	// from a streamer. The convert siafile function does not need to be passed
	// a reader.
	if lup.Reader == nil {
		return modules.Publink{}, errors.New("need to provide a stream of upload data")
	}

	// Create a dummy metadata and marshal it for computing the size of the
	// header. By using the maximum length, we make sure that we reserve enough
	// space.
	dummyMD := lup.FileMetadata
	dummyMD.Length = math.MaxUint64
	dummyMDBytes, err := skyfileMetadataBytes(dummyMD)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to retrieve pubfile metadata bytes")
	}

	// Read data from the lup reader. If the file data provided fits entirely
	// into the leading chunk, this method will use that data to upload a
	// pubfile directly. If the file data provided does not fit entirely into
	// the leading chunk, a separate method will be called to upload the file
	// separately using upload streaming, and then the siafile conversion
	// function will be used to generate the final publink.
	headerSize := uint64(SkyfileLayoutSize + len(dummyMDBytes))
	fileBytes, fileReader, largeFile, err := uploadSkyfileReadLeadingChunk(lup.Reader, headerSize)
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to retrieve leading chunk file bytes")
	}
	var publink modules.Publink
	if largeFile {
		publink, err = r.managedUploadSkyfileLargeFile(lup, fileReader)
	} else {
		publink, err = r.managedUploadSkyfileSmallFile(lup, fileBytes)
	}
	if err != nil {
		return modules.Publink{}, errors.AddContext(err, "unable to upload pubfile")
	}
	if lup.DryRun {
		return publink, nil
	}

	// Check if publink is blacklisted
	if !r.staticSkynetBlacklist.IsBlacklisted(publink) {
		return publink, nil
	}

	// Publink is blacklisted, try and delete the file and return an error
	deleteErr := r.DeleteFile(lup.SiaPath)
	if largeFile {
		extendedSiaPath, err := modules.NewSiaPath(lup.SiaPath.String() + ExtendedSuffix)
		if err != nil {
			return modules.Publink{}, errors.AddContext(err, "unable to create extended SiaPath for large pubfile deletion")
		}
		deleteErr = errors.Compose(deleteErr, r.DeleteFile(extendedSiaPath))
	}
	return modules.Publink{}, errors.Compose(ErrPublinkBlacklisted, deleteErr)
}
