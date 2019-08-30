package siafile

import (
	"os"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"
)

type (
	// Snapshot is a snapshot of a SiaFile. A snapshot is a deep-copy and
	// can be accessed without locking at the cost of being a frozen readonly
	// representation of a siafile which only exists in memory.
	Snapshot struct {
		staticChunks      []Chunk
		staticFileSize    int64
		staticPieceSize   uint64
		staticErasureCode modules.ErasureCoder
		staticMasterKey   crypto.CipherKey
		staticMode        os.FileMode
		staticPubKeyTable []HostPublicKey
		staticSiaPath     modules.SiaPath
	}
)

// SnapshotReader is a helper type that allows reading a raw SiaFile from disk
// while keeping the file in memory locked.
type SnapshotReader struct {
	f  *os.File
	sf *SiaFile
}

// Close closes the underlying file.
func (sfr *SnapshotReader) Close() error {
	sfr.sf.mu.RUnlock()
	return sfr.f.Close()
}

// Read calls Read on the underlying file.
func (sfr *SnapshotReader) Read(b []byte) (int, error) {
	return sfr.f.Read(b)
}

// Stat returns the FileInfo of the underlying file.
func (sfr *SnapshotReader) Stat() (os.FileInfo, error) {
	return sfr.f.Stat()
}

// SnapshotReader creates a io.ReadCloser that can be used to read the raw
// Siafile from disk. Note that the underlying siafile holds a readlock until
// the SnapshotReader is closed, which means that no operations can be called to
// the underlying siafile which may cause it to grab a lock, because that will
// cause a deadlock.
//
// Operations which require grabbing a readlock on the underlying siafile are
// also not okay, because if some other thread has attempted to grab a writelock
// on the siafile, the readlock will block and then the Close() statement may
// never be reached for the SnapshotReader.
//
// TODO: Things upstream would be a lot easier if we could drop the requirement
// to hold a lock for the duration of the life of the snapshot reader.
func (sf *SiaFile) SnapshotReader() (*SnapshotReader, error) {
	// Lock the file.
	sf.mu.RLock()
	if sf.deleted {
		sf.mu.RUnlock()
		return nil, errors.New("can't copy deleted SiaFile")
	}
	// Open file.
	f, err := os.Open(sf.siaFilePath)
	if err != nil {
		sf.mu.RUnlock()
		return nil, err
	}
	return &SnapshotReader{
		sf: sf,
		f:  f,
	}, nil
}

// ChunkIndexByOffset will return the chunkIndex that contains the provided
// offset of a file and also the relative offset within the chunk. If the
// offset is out of bounds, chunkIndex will be equal to NumChunk().
func (s *Snapshot) ChunkIndexByOffset(offset uint64) (chunkIndex uint64, off uint64) {
	chunkIndex = offset / s.ChunkSize()
	off = offset % s.ChunkSize()
	return
}

// ChunkSize returns the size of a single chunk of the file.
func (s *Snapshot) ChunkSize() uint64 {
	return s.staticPieceSize * uint64(s.staticErasureCode.MinPieces())
}

// ErasureCode returns the erasure coder used by the file.
func (s *Snapshot) ErasureCode() modules.ErasureCoder {
	return s.staticErasureCode
}

// MasterKey returns the masterkey used to encrypt the file.
func (s *Snapshot) MasterKey() crypto.CipherKey {
	return s.staticMasterKey
}

// Mode returns the FileMode of the file.
func (s *Snapshot) Mode() os.FileMode {
	return s.staticMode
}

// NumChunks returns the number of chunks the file consists of. This will
// return the number of chunks the file consists of even if the file is not
// fully uploaded yet.
func (s *Snapshot) NumChunks() uint64 {
	return uint64(len(s.staticChunks))
}

// Pieces returns all the pieces for a chunk in a slice of slices that contains
// all the pieces for a certain index.
func (s *Snapshot) Pieces(chunkIndex uint64) ([][]Piece, error) {
	// Return the pieces. Since the snapshot is meant to be used read-only, we
	// don't have to return a deep-copy here.
	return s.staticChunks[chunkIndex].Pieces, nil
}

// PieceSize returns the size of a single piece of the file.
func (s *Snapshot) PieceSize() uint64 {
	return s.staticPieceSize
}

// SiaPath returns the SiaPath of the file.
func (s *Snapshot) SiaPath() modules.SiaPath {
	return s.staticSiaPath
}

// Size returns the size of the file.
func (s *Snapshot) Size() uint64 {
	return uint64(s.staticFileSize)
}

// Snapshot creates a snapshot of the SiaFile.
func (sf *siaFileSetEntry) Snapshot() (*Snapshot, error) {
	mk := sf.MasterKey()

	//////////////////////////////////////////////////////////////////////////////
	// RLock starts here. No way to exit the function until RUnlock is reached
	// below.
	//////////////////////////////////////////////////////////////////////////////
	sf.mu.RLock()

	// Copy PubKeyTable.
	pkt := make([]HostPublicKey, len(sf.pubKeyTable))
	copy(pkt, sf.pubKeyTable)

	chunks := make([]Chunk, 0, sf.numChunks)
	// Figure out how much memory we need to allocate for the piece sets and
	// pieces.
	var numPieceSets, numPieces int
	err := sf.iterateChunksReadonly(func(chunk chunk) error {
		numPieceSets += len(chunk.Pieces)
		for pieceIndex := range chunk.Pieces {
			numPieces += len(chunk.Pieces[pieceIndex])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Allocate all the piece sets and pieces at once.
	allPieceSets := make([][]Piece, numPieceSets)
	allPieces := make([]Piece, numPieces)

	// Copy chunks.
	err = sf.iterateChunksReadonly(func(chunk chunk) error {
		pieces := allPieceSets[:len(chunk.Pieces)]
		allPieceSets = allPieceSets[len(chunk.Pieces):]
		for pieceIndex := range pieces {
			pieces[pieceIndex] = allPieces[:len(chunk.Pieces[pieceIndex])]
			allPieces = allPieces[len(chunk.Pieces[pieceIndex]):]
			for i, piece := range chunk.Pieces[pieceIndex] {
				pieces[pieceIndex][i] = Piece{
					HostPubKey: sf.pubKeyTable[piece.HostTableOffset].PublicKey,
					MerkleRoot: piece.MerkleRoot,
				}
			}
		}
		chunks = append(chunks, Chunk{
			Pieces: pieces,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Get non-static metadata fields under lock.
	fileSize := sf.staticMetadata.FileSize
	mode := sf.staticMetadata.Mode

	sf.mu.RUnlock()
	//////////////////////////////////////////////////////////////////////////////
	// RLock ends here.
	//////////////////////////////////////////////////////////////////////////////

	sf.staticSiaFileSet.mu.Lock()
	sp := sf.staticSiaFileSet.siaPath(sf)
	sf.staticSiaFileSet.mu.Unlock()

	return &Snapshot{
		staticChunks:      chunks,
		staticFileSize:    fileSize,
		staticPieceSize:   sf.staticMetadata.StaticPieceSize,
		staticErasureCode: sf.staticMetadata.staticErasureCode,
		staticMasterKey:   mk,
		staticMode:        mode,
		staticPubKeyTable: pkt,
		staticSiaPath:     sp,
	}, nil
}
