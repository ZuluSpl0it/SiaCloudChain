package pubaccesskey

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"

	//	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// SkykeyIDLen is the length of a SkykeyID
	SkykeyIDLen = 16

	// MaxKeyNameLen is the maximum length of a pubaccesskey's name.
	MaxKeyNameLen = 128

	// headerLen is the length of the pubaccesskey file header.
	// It is the length of the magic, the version, and and the file length.
	headerLen = types.SpecifierLen + types.SpecifierLen + 8

	// Permissions match those in modules/renter.go
	// Redefined here to avoid an import cycle.
	defaultFilePerm = 0644
	defaultDirPerm  = 0755
)

var (
	skykeyVersionString = "1.4.3"
	skykeyVersion       = types.NewSpecifier(skykeyVersionString)

	// SkykeySpecifier is used as a prefix when hashing Pubaccesskeys to compute their
	// ID.
	SkykeySpecifier = types.NewSpecifier("Pubaccesskey")

	// SkykeyFileMagic is the first piece of data found in a Pubaccesskey file.
	SkykeyFileMagic = types.NewSpecifier("SkykeyFile")

	errUnsupportedSkykeyCipherType = errors.New("Unsupported Pubaccesskey ciphertype")
	errNoSkykeysWithThatName       = errors.New("No Pubaccesskey with that name")
	errNoSkykeysWithThatID         = errors.New("No Pubaccesskey is assocated with that ID")
	errSkykeyNameAlreadyExists     = errors.New("Pubaccesskey name already exists.")
	errSkykeyWithIDAlreadyExists   = errors.New("Pubaccesskey ID already exists.")
	errSkykeyNameToolong           = errors.New("Pubaccesskey name exceeds max length")

	// SkykeyPersistFilename is the name of the pubaccesskey persistence file.
	SkykeyPersistFilename = "pubaccesskeys.dat"
)

// SkykeyID is the identifier of a pubaccesskey.
type SkykeyID [SkykeyIDLen]byte

// Pubaccesskey is a key used to encrypt/decrypt pubfiles.
type Pubaccesskey struct {
	Name       string
	CipherType crypto.CipherType
	Entropy    []byte
}

// SkykeyManager manages the creation and handling of new pubaccesskeys which can be
// referenced by their unique name or identifier.
type SkykeyManager struct {
	idsByName map[string]SkykeyID
	keysByID  map[SkykeyID]Pubaccesskey

	version types.Specifier
	fileLen uint64 // Invariant: fileLen is at least headerLen

	persistFile string
	mu          sync.Mutex
}

// countingWriter is a wrapper of an io.Writer that keep track of the total
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

func (cw countingWriter) BytesWritten() uint64 {
	return uint64(cw.count)
}

// unmarshalSia decodes the Pubaccesskey into the reader.
func (sk *Pubaccesskey) unmarshalSia(r io.Reader) error {
	d := encoding.NewDecoder(r, encoding.DefaultAllocLimit)
	d.Decode(&sk.Name)
	d.Decode(&sk.CipherType)
	d.Decode(&sk.Entropy)
	return d.Err()
}

// marshalSia encodes the Pubaccesskey into the writer.
func (sk Pubaccesskey) marshalSia(w io.Writer) error {
	e := encoding.NewEncoder(w)
	e.Encode(sk.Name)
	e.Encode(sk.CipherType)
	e.Encode(sk.Entropy)
	return e.Err()
}

// ToString encodes the Pubaccesskey as a base64 string.
func (sk Pubaccesskey) ToString() (string, error) {
	var b bytes.Buffer
	err := sk.marshalSia(&b)
	return base64.URLEncoding.EncodeToString(b.Bytes()), err
}

// FromString decodes the base64 string into a Pubaccesskey.
func (sk *Pubaccesskey) FromString(s string) error {
	keyBytes, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return sk.unmarshalSia(bytes.NewReader(keyBytes))
}

// ID returns the ID for the Pubaccesskey.
func (sk Pubaccesskey) ID() (keyID SkykeyID) {
	h := crypto.HashAll(SkykeySpecifier, sk.CipherType, sk.Entropy)
	copy(keyID[:], h[:SkykeyIDLen])
	return keyID
}

// ToString encodes the SkykeyID as a base64 string.
func (id SkykeyID) ToString() string {
	return base64.URLEncoding.EncodeToString(id[:])
}

// FromString decodes the base64 string into a Pubaccesskey ID.
func (id *SkykeyID) FromString(s string) error {
	idBytes, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	if len(idBytes) != SkykeyIDLen {
		return errors.New("Pubaccesskey ID has invalid length")
	}
	copy(id[:], idBytes[:])
	return nil
}

// equals returns true if and only if the two Pubaccesskeys are equal.
func (sk *Pubaccesskey) equals(otherKey Pubaccesskey) bool {
	return sk.Name == otherKey.Name && sk.ID() == otherKey.ID() && sk.CipherType.String() == otherKey.CipherType.String()
}

// SupportsCipherType returns true if and only if the SkykeyManager supports
// keys with the given cipher type.
func (sm *SkykeyManager) SupportsCipherType(ct crypto.CipherType) bool {
	return ct == crypto.TypeXChaCha20
}

// CreateKey creates a new Pubaccesskey under the given name and cipherType.
func (sm *SkykeyManager) CreateKey(name string, cipherType crypto.CipherType) (Pubaccesskey, error) {
	if len(name) > MaxKeyNameLen {
		return Pubaccesskey{}, errSkykeyNameToolong
	}
	if !sm.SupportsCipherType(cipherType) {
		return Pubaccesskey{}, errUnsupportedSkykeyCipherType
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.idsByName[name]
	if ok {
		return Pubaccesskey{}, errSkykeyNameAlreadyExists
	}

	// Generate the new key.
	cipherKey := crypto.GenerateSiaKey(cipherType)
	pubaccesskey := Pubaccesskey{name, cipherType, cipherKey.Key()}

	err := sm.saveKey(pubaccesskey)
	if err != nil {
		return Pubaccesskey{}, err
	}
	return pubaccesskey, nil
}

// AddKey creates a key with the given name, cipherType, and entropy and adds it
// to the key file.
func (sm *SkykeyManager) AddKey(sk Pubaccesskey) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.idsByName[sk.Name]
	if ok {
		return errSkykeyNameAlreadyExists
	}

	_, ok = sm.keysByID[sk.ID()]
	if ok {
		return errSkykeyWithIDAlreadyExists
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

// NewSkykeyManager creates a SkykeyManager for managing pubaccesskeys.
func NewSkykeyManager(persistDir string) (*SkykeyManager, error) {
	sm := &SkykeyManager{
		idsByName:   make(map[string]SkykeyID),
		keysByID:    make(map[SkykeyID]Pubaccesskey),
		fileLen:     0,
		persistFile: filepath.Join(persistDir, SkykeyPersistFilename),
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

	dec.Decode(&sm.version)
	if dec.Err() != nil {
		return errors.AddContext(dec.Err(), "Error decoding pubaccesskey file version")
	}

	versionBytes, err := sm.version.MarshalText()
	if err != nil {
		return err
	}
	version := strings.ReplaceAll(string(versionBytes), string(0x0), "")

	if !build.IsVersion(version) {
		return errors.New("pubaccesskey file header missing version")
	}
	if build.VersionCmp(skykeyVersionString, version) < 0 {
		return errors.New("Unknown pubaccesskey version")
	}

	// Read the length of the file into the key manager.
	dec.Decode(&sm.fileLen)
	return dec.Err()
}

// saveHeader saves the header data of the pubaccesskey file to disk and syncs the
// file.
func (sm *SkykeyManager) saveHeader(file *os.File) error {
	_, err := file.Seek(0, 0)
	if err != nil {
		return errors.AddContext(err, "Unable to save pubaccesskey header")
	}

	e := encoding.NewEncoder(file)
	e.Encode(SkykeyFileMagic)
	e.Encode(sm.version)
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
	file, err := os.OpenFile(sm.persistFile, os.O_RDWR|os.O_CREATE, defaultFilePerm)
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
		sm.version = skykeyVersion
		sm.fileLen = uint64(headerLen)
		return sm.saveHeader(file)
	}

	// Otherwise load the existing header and all the pubaccesskeys in the file.
	err = sm.loadHeader(file)
	if err != nil {
		return errors.AddContext(err, "Error loading header")
	}

	_, err = file.Seek(int64(headerLen), io.SeekStart)
	if err != nil {
		return err
	}

	// Read all the pubaccesskeys up to the length set in the header.
	n := headerLen
	for n < int(sm.fileLen) {
		var sk Pubaccesskey
		err = sk.unmarshalSia(file)
		if err != nil {
			return errors.AddContext(err, "Error unmarshaling Pubaccesskey")
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
	return nil
}

// saveKey saves the key and appends it to the pubaccesskey file and updates/syncs
// the header.
func (sm *SkykeyManager) saveKey(pubaccesskey Pubaccesskey) error {
	keyID := pubaccesskey.ID()

	// Store the new key.
	sm.idsByName[pubaccesskey.Name] = keyID
	sm.keysByID[keyID] = pubaccesskey

	file, err := os.OpenFile(sm.persistFile, os.O_RDWR, defaultFilePerm)
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
