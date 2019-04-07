package siafile

import (
	"os"
	"strings"
	"time"

	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"

	"gitlab.com/NebulousLabs/errors"
)

type (
	// FileData is a helper struct that contains all the relevant information
	// of a file. It simplifies passing the necessary data between modules and
	// keeps the interface clean.
	FileData struct {
		Name        string
		FileSize    uint64
		MasterKey   [crypto.EntropySize]byte
		ErasureCode modules.ErasureCoder
		RepairPath  string
		PieceSize   uint64
		Mode        os.FileMode
		Deleted     bool
		UID         string
		Chunks      []FileChunk
	}
	// FileChunk is a helper struct that contains data about a chunk.
	FileChunk struct {
		Pieces [][]Piece
	}
)

// NewFromLegacyData creates a new SiaFile from data that was previously loaded
// from a legacy file.
func (sfs *SiaFileSet) NewFromLegacyData(fd FileData) (*SiaFileSetEntry, error) {
	sfs.mu.Lock()
	defer sfs.mu.Unlock()

	// Trim any leading slashes in the legacy name.
	fd.Name = strings.TrimPrefix(fd.Name, "/")

	// Legacy master keys are always twofish keys.
	mk, err := crypto.NewSiaKey(crypto.TypeTwofish, fd.MasterKey[:])
	if err != nil {
		return nil, errors.AddContext(err, "failed to restore master key")
	}
	currentTime := time.Now()
	ecType, ecParams := marshalErasureCoder(fd.ErasureCode)
	siaPath, err := modules.NewSiaPath(fd.Name)
	if err != nil {
		return &SiaFileSetEntry{}, err
	}
	file := &SiaFile{
		staticMetadata: metadata{
			AccessTime:              currentTime,
			ChunkOffset:             defaultReservedMDPages * pageSize,
			ChangeTime:              currentTime,
			CreateTime:              currentTime,
			StaticFileSize:          int64(fd.FileSize),
			LocalPath:               fd.RepairPath,
			StaticMasterKey:         mk.Key(),
			StaticMasterKeyType:     mk.Type(),
			Mode:                    fd.Mode,
			ModTime:                 currentTime,
			staticErasureCode:       fd.ErasureCode,
			StaticErasureCodeType:   ecType,
			StaticErasureCodeParams: ecParams,
			StaticPagesPerChunk:     numChunkPagesRequired(fd.ErasureCode.NumPieces()),
			StaticPieceSize:         fd.PieceSize,
			SiaPath:                 siaPath,
		},
		deleted:        fd.Deleted,
		deps:           modules.ProdDependencies,
		siaFilePath:    siaPath.SiaFileSysPath(sfs.siaFileDir),
		staticUniqueID: fd.UID,
		wal:            sfs.wal,
	}
	file.staticChunks = make([]chunk, len(fd.Chunks))
	for i := range file.staticChunks {
		file.staticChunks[i].Pieces = make([][]piece, file.staticMetadata.staticErasureCode.NumPieces())
	}

	// Populate the pubKeyTable of the file and add the pieces.
	pubKeyMap := make(map[string]uint32)
	for chunkIndex, chunk := range fd.Chunks {
		for pieceIndex, pieceSet := range chunk.Pieces {
			for _, p := range pieceSet {
				// Check if we already added that public key.
				tableOffset, exists := pubKeyMap[string(p.HostPubKey.Key)]
				if !exists {
					tableOffset = uint32(len(file.pubKeyTable))
					pubKeyMap[string(p.HostPubKey.Key)] = tableOffset
					file.pubKeyTable = append(file.pubKeyTable, HostPublicKey{
						PublicKey: p.HostPubKey,
						Used:      true,
					})
				}
				// Add the piece to the SiaFile.
				file.staticChunks[chunkIndex].Pieces[pieceIndex] = append(file.staticChunks[chunkIndex].Pieces[pieceIndex], piece{
					HostTableOffset: tableOffset,
					MerkleRoot:      p.MerkleRoot,
				})
			}
		}
	}
	entry := sfs.newSiaFileSetEntry(file)
	threadUID := randomThreadUID()
	entry.threadMap[threadUID] = newThreadInfo()
	sfs.siaFileMap[siaPath] = entry
	sfse := &SiaFileSetEntry{
		siaFileSetEntry: entry,
		threadUID:       threadUID,
	}
	return sfse, errors.AddContext(file.saveFile(), "unable to save file")
}
