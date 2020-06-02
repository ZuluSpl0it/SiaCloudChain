package pubaccesskey

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// headerLen is the length of the pubaccesskey file header.
	// It is the length of the magic, the version, and and the file length.
	headerLen = types.SpecifierLen + types.SpecifierLen + 8

	// Permissions match those in modules/renter.go
	// Redefined here to avoid an import cycle.
	defaultFilePerm = 0600
	defaultDirPerm  = 0700
)

var (
	skykeyVersionString = "1.4.3.1"
	skykeyVersion       = types.NewSpecifier(skykeyVersionString)

	// oldFormatSkykeyVersionString is the version number which used a different
	// marshaling/unmarshaling scheme for skykeys.
	oldFormatSkykeyVersionString = "1.4.3"

	// SkykeyFileMagic is the first piece of data found in a Pubaccesskey file.
	SkykeyFileMagic = types.NewSpecifier("PubaccesskeyFile")

	// ErrSkykeyWithNameAlreadyExists indicates that a key cannot be created or added
	// because a key with the same name is already being stored.
	ErrSkykeyWithNameAlreadyExists = errors.New("Pubaccesskey name already used by another key.")

	// ErrSkykeyWithIDAlreadyExists indicates that a key cannot be created or
	// added because a key with the same ID (and therefore same key entropy) is
	// already being stored.
	ErrSkykeyWithIDAlreadyExists = errors.New("Pubaccesskey ID already exists.")

	errNoSkykeysWithThatName = errors.New("No Pubaccesskey with that name")
	errNoSkykeysWithThatID   = errors.New("No Pubaccesskey is assocated with that ID")
	errSkykeyNameToolong     = errors.New("Pubaccesskey name exceeds max length")

	// SkykeyPersistFilename is the name of the pubaccesskey persistence file.
	SkykeyPersistFilename = "skykeys.dat"
)

// SkykeyManager manages the creation and handling of new skykeys which can be
// referenced by their unique name or identifier.
type SkykeyManager struct {
	idsByName map[string]SkykeyID
	keysByID  map[SkykeyID]Pubaccesskey

	staticVersion types.Specifier
	fileLen       uint64 // Invariant: fileLen is at least headerLen

	staticPersistFile string
	mu                sync.Mutex
}

// countingWriter is a wrapper of an io.Writer that keeps track of the total
// amount of bytes written.
type countingWriter struct {
	writer io.Writer
	count  int
}

// newCountingWriter returns a countingWriter.
func newCountingWriter(w io.Writer) *countingWriter {
	return &countingWriter{w, 0}
}

// Write implements the io.Writer interface.
func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.writer.Write(p)
	cw.count += n
	return
}

// BytesWritten returns the total number of returns the bytes written through
// this writer.
func (cw countingWriter) BytesWritten() uint64 {
	return uint64(cw.count)
}

// SupportsSkykeyType returns true if and only if the SkykeyManager supports
// skykeys with the given type.
func (sm *SkykeyManager) SupportsSkykeyType(skykeyType SkykeyType) bool {
	switch skykeyType {
	case TypePublicID:
		return true
	default:
		return false
	}
}

// CreateKey creates a new Pubaccesskey under the given name and SkykeyType.
func (sm *SkykeyManager) CreateKey(name string, skykeyType SkykeyType) (Pubaccesskey, error) {
	if len(name) > MaxKeyNameLen {
		return Pubaccesskey{}, errSkykeyNameToolong
	}
	if !sm.SupportsSkykeyType(skykeyType) {
		return Pubaccesskey{}, errUnsupportedSkykeyType
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.idsByName[name]
	if ok {
		return Pubaccesskey{}, ErrSkykeyWithNameAlreadyExists
	}

	// Generate the new key.
	cipherKey := crypto.GenerateSiaKey(skykeyType.CipherType())
	pubaccesskey := Pubaccesskey{name, skykeyType, cipherKey.Key()}

	err := sm.saveKey(pubaccesskey)
	if err != nil {
		return Pubaccesskey{}, err
	}
	return pubaccesskey, nil
}

// AddKey adds the given Pubaccesskey to the pubaccesskey manager.
func (sm *SkykeyManager) AddKey(sk Pubaccesskey) error {
	if err := sk.IsValid(); err != nil {
		return errors.AddContext(err, "Invalid pubaccesskey cannot be added")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.keysByID[sk.ID()]
	if ok {
		return ErrSkykeyWithIDAlreadyExists
	}

	_, ok = sm.idsByName[sk.Name]
	if ok {
		return ErrSkykeyWithNameAlreadyExists
	}

	return sm.saveKey(sk)
}

// IDByName returns the ID associated with the given key name.
func (sm *SkykeyManager) IDByName(name string) (SkykeyID, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id, ok := sm.idsByName[name]
	if !ok {
		return SkykeyID{}, errNoSkykeysWithThatName
	}
	return id, nil
}

// KeyByName returns the Pubaccesskey associated with that key name.
func (sm *SkykeyManager) KeyByName(name string) (Pubaccesskey, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id, ok := sm.idsByName[name]
	if !ok {
		return Pubaccesskey{}, errNoSkykeysWithThatName
	}

	key, ok := sm.keysByID[id]
	if !ok {
		return Pubaccesskey{}, errNoSkykeysWithThatID
	}

	return key, nil
}

// KeyByID returns the Pubaccesskey associated with that ID.
func (sm *SkykeyManager) KeyByID(id SkykeyID) (Pubaccesskey, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key, ok := sm.keysByID[id]
	if !ok {
		return Pubaccesskey{}, errNoSkykeysWithThatID
	}
	return key, nil
}

// Skykeys returns a slice containing each Pubaccesskey being stored.
func (sm *SkykeyManager) Skykeys() []Pubaccesskey {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	keys := make([]Pubaccesskey, 0, len(sm.keysByID))
	for _, sk := range sm.keysByID {
		keys = append(keys, sk)
	}
	return keys
}

// NewSkykeyManager creates a SkykeyManager for managing skykeys.
func NewSkykeyManager(persistDir string) (*SkykeyManager, error) {
	sm := &SkykeyManager{
		idsByName:         make(map[string]SkykeyID),
		keysByID:          make(map[SkykeyID]Pubaccesskey),
		fileLen:           0,
		staticPersistFile: filepath.Join(persistDir, SkykeyPersistFilename),
	}

	// create the persist dir if it doesn't already exist.
	err := os.MkdirAll(persistDir, defaultDirPerm)
	if err != nil {
		return nil, err
	}

	// Load the persist. If it's empty, it will be initialized.
	err = sm.load()
	if err != nil {
		return nil, err
	}
	return sm, nil
}

// loadHeader loads the header from the pubaccesskey file.
func (sm *SkykeyManager) loadHeader(file *os.File) error {
	headerBytes := make([]byte, headerLen)
	_, err := file.Read(headerBytes)
	if err != nil {
		return errors.AddContext(err, "Error reading Pubaccesskey file metadata")
	}

	dec := encoding.NewDecoder(bytes.NewReader(headerBytes), encoding.DefaultAllocLimit)
	var magic types.Specifier
	dec.Decode(&magic)
	if magic != SkykeyFileMagic {
		return errors.New("Expected pubaccesskey file magic")
	}

	dec.Decode(&sm.staticVersion)
	if dec.Err() != nil {
		return errors.AddContext(dec.Err(), "Error decoding pubaccesskey file version")
	}

	versionBytes, err := sm.staticVersion.MarshalText()
	if err != nil {
		return err
	}
	version := strings.ReplaceAll(string(versionBytes), string(0x0), "")

	if !build.IsVersion(version) {
		return errors.New("pubaccesskey file header missing version")
	}

	// Check if the version is the version using the old pubaccesskey format (v1.4.4), or the
	// updated format (v1.4.9).
	if build.VersionCmp(skykeyVersionString, version) != 0 && build.VersionCmp(oldFormatSkykeyVersionString, version) != 0 {
		return errors.AddContext(errors.New("Unknown pubaccesskey version"), version)
	}

	// Read the length of the file into the key manager.
	dec.Decode(&sm.fileLen)
	if err = dec.Err(); err != nil {
		return err
	}
	return nil
}

// saveHeader saves the header data of the pubaccesskey file to disk and syncs the
// file.
func (sm *SkykeyManager) saveHeader(file *os.File) error {
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return errors.AddContext(err, "Unable to save pubaccesskey header")
	}

	e := encoding.NewEncoder(file)
	e.Encode(SkykeyFileMagic)
	e.Encode(sm.staticVersion)
	e.Encode(sm.fileLen)
	if e.Err() != nil {
		return errors.AddContext(e.Err(), "Error encoding pubaccesskey file header")
	}
	return file.Sync()
}

// load initializes the SkykeyManager with the data stored in the pubaccesskey file if
// it exists. If it does not exist, it initializes that file with the default
// header values.
func (sm *SkykeyManager) load() error {
	file, err := os.OpenFile(sm.staticPersistFile, os.O_RDWR|os.O_CREATE, defaultFilePerm)
	if err != nil {
		return errors.AddContext(err, "Unable to open SkykeyManager persist file")
	}
	defer file.Close()

	// Check if the file has a header. If there is not, then set the default
	// values and save it.
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if fileInfo.Size() < int64(headerLen) {
		sm.staticVersion = skykeyVersion
		sm.fileLen = uint64(headerLen)
		return sm.saveHeader(file)
	}

	// Otherwise load the existing header and all the skykeys in the file.
	err = sm.loadHeader(file)
	if err != nil {
		return errors.AddContext(err, "Error loading header")
	}

	_, err = file.Seek(int64(headerLen), io.SeekStart)
	if err != nil {
		return err
	}

	// Read all the skykeys up to the length set in the header.
	n := headerLen
	for n < int(sm.fileLen) {
		var sk Pubaccesskey
		err = sk.unmarshalSia(file)

		// Try unmarshaling with the old format and converting if the error could be
		// a data-related error.
		if err != nil {
			// Seek back to the beginning of this key.
			_, seekErr := file.Seek(int64(n), io.SeekStart)
			if seekErr != nil {
				return errors.Compose(err, seekErr)
			}

			oldFormatUnmarshalErr := sk.unmarshalAndConvertFromOldFormat(file)
			if oldFormatUnmarshalErr != nil {
				err = errors.Compose(err, oldFormatUnmarshalErr)
				return errors.AddContext(err, "Error unmarshaling Pubaccesskey")
			}
		}

		// Store the pubaccesskey.
		sm.idsByName[sk.Name] = sk.ID()
		sm.keysByID[sk.ID()] = sk

		// Set n to current offset in file.
		currOffset, err := file.Seek(0, io.SeekCurrent)
		n = int(currOffset)
		if err != nil {
			return errors.AddContext(err, "Error getting pubaccesskey file offset")
		}
	}

	if n != int(sm.fileLen) {
		return errors.New("Expected to read entire specified pubaccesskey file length")
	}

	// Update the stored version if necessary.
	if sm.staticVersion != skykeyVersion {
		sm.staticVersion = skykeyVersion
		return sm.saveHeader(file)
	}
	return nil
}

// saveKey saves the key and appends it to the pubaccesskey file and updates/syncs
// the header.
func (sm *SkykeyManager) saveKey(pubaccesskey Pubaccesskey) error {
	keyID := pubaccesskey.ID()

	// Store the new key.
	sm.idsByName[pubaccesskey.Name] = keyID
	sm.keysByID[keyID] = pubaccesskey

	file, err := os.OpenFile(sm.staticPersistFile, os.O_RDWR, defaultFilePerm)
	if err != nil {
		return errors.AddContext(err, "Unable to open SkykeyManager persist file")
	}
	defer file.Close()

	// Seek to the end of the known-to-be-valid part of the file.
	_, err = file.Seek(int64(sm.fileLen), io.SeekStart)
	if err != nil {
		return err
	}

	writer := newCountingWriter(file)
	err = pubaccesskey.marshalSia(writer)
	if err != nil {
		return errors.AddContext(err, "Error writing pubaccesskey to file")
	}

	err = file.Sync()
	if err != nil {
		return err
	}

	// Update the header
	sm.fileLen += writer.BytesWritten()
	return sm.saveHeader(file)
}
