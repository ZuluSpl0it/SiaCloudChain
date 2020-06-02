package pubaccesskey

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aead/chacha20/chacha"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
)

// TestSkykeyManager tests the basic functionality of the skykeyManager.
func TestSkykeyManager(t *testing.T) {
	// Create a key manager.
	persistDir := build.TempDir("pubaccesskey", t.Name())
	keyMan, err := NewSkykeyManager(persistDir)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the header values are set.
	if keyMan.staticVersion != skykeyVersion {
		t.Fatal("Expected version to be set")
	}
	if int(keyMan.fileLen) < headerLen {
		t.Fatal("Expected at file to be at least headerLen bytes")
	}

	// Creating a key with name longer than the max allowed should fail.
	var longName [MaxKeyNameLen + 1]byte
	for i := 0; i < len(longName); i++ {
		longName[i] = 0x41 // "A"
	}
	_, err = keyMan.CreateKey(string(longName[:]), TypePublicID)
	if !errors.Contains(err, errSkykeyNameToolong) {
		t.Fatal(err)
	}

	// Creating a key with name less than or equal to max len should be ok.
	_, err = keyMan.CreateKey(string(longName[:len(longName)-1]), TypePublicID)
	if err != nil {
		t.Fatal(err)
	}

	// Unsupported cipher types should cause an error.
	_, err = keyMan.CreateKey("test_key1", SkykeyType(0x00))
	if !errors.Contains(err, errUnsupportedSkykeyType) {
		t.Fatal(err)
	}
	_, err = keyMan.CreateKey("test_key1", SkykeyType(0xFF))
	if !errors.Contains(err, errUnsupportedSkykeyType) {
		t.Fatal(err)
	}

	pubaccesskey, err := keyMan.CreateKey("test_key1", TypePublicID)
	if err != nil {
		t.Fatal(err)
	}

	// Simple encoding/decoding test.
	var buf bytes.Buffer
	err = pubaccesskey.marshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}

	var decodedSkykey Pubaccesskey
	err = decodedSkykey.unmarshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !decodedSkykey.equals(pubaccesskey) {
		t.Log(pubaccesskey)
		t.Log(decodedSkykey)
		t.Fatal("Expected decoded pubaccesskey to be the same")
	}

	// Check duplicate name errors.
	_, err = keyMan.CreateKey("test_key1", TypePublicID)
	if !errors.Contains(err, ErrSkykeyWithNameAlreadyExists) {
		t.Fatal("Expected pubaccesskey name to already exist", err)
	}

	// Check the correct ID is returned.
	id, err := keyMan.IDByName("test_key1")
	if err != nil {
		t.Fatal(err)
	}
	if id != pubaccesskey.ID() {
		t.Fatal("Expected matching keyID")
	}

	// Check that the correct error for a random unknown key is given.
	randomNameBytes := fastrand.Bytes(24)
	randomName := string(randomNameBytes)
	id, err = keyMan.IDByName(randomName)
	if err != errNoSkykeysWithThatName {
		t.Fatal(err)
	}

	// Check that the correct error for a random unknown key is given.
	var randomID SkykeyID
	fastrand.Read(randomID[:])
	_, err = keyMan.KeyByID(randomID)
	if err != errNoSkykeysWithThatID {
		t.Fatal(err)
	}

	// Create a second test key and check that it's different than the first.
	skykey2, err := keyMan.CreateKey("test_key2", TypePublicID)
	if err != nil {
		t.Fatal(err)
	}
	if skykey2.equals(pubaccesskey) {
		t.Fatal("Expected different pubaccesskey to be created")
	}
	if len(keyMan.keysByID) != 3 {
		t.Fatal("Wrong number of keys", len(keyMan.keysByID))
	}
	if len(keyMan.idsByName) != 3 {
		t.Fatal("Wrong number of keys", len(keyMan.idsByName))
	}

	// Check KeyByName returns the keys with the expected ID.
	key1Copy, err := keyMan.KeyByName("test_key1")
	if err != nil {
		t.Fatal(err)
	}
	if !key1Copy.equals(pubaccesskey) {
		t.Fatal("Expected key ID to match")
	}

	key2Copy, err := keyMan.KeyByName("test_key2")
	if err != nil {
		t.Fatal(err)
	}
	if !key2Copy.equals(skykey2) {
		t.Fatal("Expected key ID to match")
	}
	fileLen := keyMan.fileLen

	// Load a new keymanager from the same persistDir.
	keyMan2, err := NewSkykeyManager(persistDir)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the header values are set.
	if keyMan2.staticVersion != skykeyVersion {
		t.Fatal("Expected version to be set")
	}
	if keyMan2.fileLen != fileLen {
		t.Fatal("Expected file len to match previous keyMan", fileLen, keyMan2.fileLen)
	}

	if len(keyMan.keysByID) != len(keyMan2.keysByID) {
		t.Fatal("Expected same number of keys")
	}
	for id, key := range keyMan.keysByID {
		if !key.equals(keyMan2.keysByID[id]) {
			t.Fatal("Expected same keys")
		}
	}

	// Check that AddKey works properly by re-adding all the keys from the first
	// 2 key managers into a new one.
	persistDir = build.TempDir(t.Name(), "add-only-keyman")
	addKeyMan, err := NewSkykeyManager(persistDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range keyMan.keysByID {
		err := addKeyMan.AddKey(key)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check for the correct number of keys.
	if len(addKeyMan.keysByID) != 3 {
		t.Fatal("Wrong number of keys", len(addKeyMan.keysByID))
	}
	if len(addKeyMan.idsByName) != 3 {
		t.Fatal("Wrong number of keys", len(addKeyMan.idsByName))
	}

	// Try re-adding the same keys, and check that the duplicate name error is
	// shown.
	for _, key := range keyMan.keysByID {
		err := addKeyMan.AddKey(key)
		if !errors.Contains(err, ErrSkykeyWithIDAlreadyExists) {
			t.Fatal(err)
		}
	}
}

// TestSkykeyDerivation tests pubaccesskey derivation methods used in pubfile
// encryption.
func TestSkykeyDerivations(t *testing.T) {
	// Create a key manager.
	persistDir := build.TempDir("pubaccesskey", t.Name())
	keyMan, err := NewSkykeyManager(persistDir)
	if err != nil {
		t.Fatal(err)
	}

	pubaccesskey, err := keyMan.CreateKey("derivation_test_key", TypePublicID)
	if err != nil {
		t.Fatal(err)
	}
	masterNonce := pubaccesskey.Nonce()

	derivationPath1 := []byte("derivationtest1")
	derivationPath2 := []byte("path2")

	// Create file-specific keys.
	numDerivedSkykeys := 5
	derivedSkykeys := make([]Pubaccesskey, 0)
	for i := 0; i < numDerivedSkykeys; i++ {
		fsKey, err := pubaccesskey.GenerateFileSpecificSubkey()
		if err != nil {
			t.Fatal(err)
		}
		derivedSkykeys = append(derivedSkykeys, fsKey)

		// Further derive subkeys along the 2 test paths.
		dk1, err := fsKey.DeriveSubkey(derivationPath1)
		if err != nil {
			t.Fatal(err)
		}
		dk2, err := fsKey.DeriveSubkey(derivationPath2)
		if err != nil {
			t.Fatal(err)
		}
		derivedSkykeys = append(derivedSkykeys, dk1)
		derivedSkykeys = append(derivedSkykeys, dk2)
	}

	// Include all keys.
	numDerivedSkykeys *= 3

	// Check that all keys have the same Key data.
	for i := 0; i < numDerivedSkykeys; i++ {
		if !bytes.Equal(pubaccesskey.Entropy[:chacha.KeySize], derivedSkykeys[i].Entropy[:chacha.KeySize]) {
			t.Fatal("Expected each derived pubaccesskey to have the same key as the master pubaccesskey")
		}
		// Sanity check by checking ID equality also.
		if pubaccesskey.ID() != derivedSkykeys[i].ID() {
			t.Fatal("Expected each derived pubaccesskey to have the same ID as the master pubaccesskey")
		}
	}

	// Check that all nonces have a different nonce, and are not considered equal.
	for i := 0; i < numDerivedSkykeys; i++ {
		ithNonce := derivedSkykeys[i].Nonce()
		if bytes.Equal(ithNonce[:], masterNonce[:]) {
			t.Fatal("Expected nonce different from master nonce", i)
		}
		for j := i + 1; j < numDerivedSkykeys; j++ {
			jthNonce := derivedSkykeys[j].Nonce()
			if bytes.Equal(ithNonce[:], jthNonce[:]) {
				t.Fatal("Expected different nonces", ithNonce, jthNonce)
			}
			// Sanity check our definition of equals.
			if derivedSkykeys[i].equals(derivedSkykeys[j]) {
				t.Fatal("Expected pubaccesskey to be different", i, j)
			}
		}
	}
}

// TestSkykeyFormatCompat tests compatibility code for the old pubaccesskey format.
func TestSkykeyFormatCompat(t *testing.T) {
	badOldKeyString := "BAAAAAAAAABrZXkxAAAAAAAAAAQgAAAAAAAAADiObVg49-0juJ8udAx4qMW-TEHgDxfjA0fjJSNBuJ4a"
	oldKeyString := "CAAAAAAAAAB0ZXN0a2V5MQAAAAAAAAAEOAAAAAAAAADJfmSVAo2HGDfBpPrDr1CoqiqXAMYG9FaaHBwxKL6lNVEysSVY65et5zdFmwCMb7HibTE8LlRR5Q=="

	var oldSkykey compatSkykeyV148
	err := oldSkykey.fromString(badOldKeyString)
	if err == nil {
		t.Fatal("Expected error decoding incorrectly formatted old key")
	}

	err = oldSkykey.fromString(oldKeyString)
	if err != nil {
		t.Fatal(err)
	}
	if oldSkykey.name != "testkey1" {
		t.Fatal("Incorrect pubaccesskey name", oldSkykey.name)
	}
	if oldSkykey.ciphertype != crypto.TypeXChaCha20 {
		t.Fatal("Incorrect pubaccesskey name", oldSkykey.name)
	}

	// Sanity check: the pubaccesskey can be used to create a cipherkey still
	_, err = crypto.NewSiaKey(oldSkykey.ciphertype, oldSkykey.entropy)
	if err != nil {
		t.Log(len(oldSkykey.entropy))
		t.Fatal(err)
	}

	// Test a marshal and unmarshal of a new key.
	oldSkykey2 := compatSkykeyV148{
		name:       "oldkey2",
		ciphertype: crypto.TypeXChaCha20,
		entropy:    make([]byte, 56),
	}
	fastrand.Read(oldSkykey2.entropy)

	var buf bytes.Buffer
	err = oldSkykey2.marshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}

	var decodedOK2 compatSkykeyV148
	err = decodedOK2.unmarshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if decodedOK2.name != oldSkykey2.name {
		t.Fatal("Expected key names to match", decodedOK2.name)
	}
	if decodedOK2.ciphertype != oldSkykey2.ciphertype {
		t.Fatal("Expected key ciphertypes to match", decodedOK2.ciphertype)
	}
	if !bytes.Equal(decodedOK2.entropy, oldSkykey2.entropy) {
		t.Log(decodedOK2)
		t.Log(oldSkykey2)
		t.Fatal("Expected entropy to match")
	}

	// Write an old key to the buffer again.
	err = oldSkykey2.marshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Test conversion to updated key format.
	var sk Pubaccesskey
	err = sk.unmarshalAndConvertFromOldFormat(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Name != oldSkykey2.name {
		t.Fatal("Incorrect pubaccesskey name", sk.Name)
	}
	if sk.Type != TypePublicID {
		t.Fatal("Incorrect pubaccesskey name", sk.Type)
	}
	if sk.CipherType() != crypto.TypeXChaCha20 {
		t.Fatal("Incorrect pubaccesskey ciphertype", sk.CipherType())
	}
	if !bytes.Equal(sk.Entropy, oldSkykey2.entropy) {
		t.Log(sk)
		t.Log(oldSkykey)
		t.Fatal("Expected entropy to match")
	}
}

// TestSkykeyURIFormatting checks the ToString and FromString pubaccesskey methods
// that use URI formatting.
func TestSkykeyURIFormatting(t *testing.T) {
	testKeyName := "FormattingTestKey"
	keyDataString := "AT7-P751d_SEBhXvbOQTfswB62n2mqMe0Q89cQ911KGeuTIV2ci6GjG3Aj5CuVZUDS6hkG7pHXXZ"
	nameParam := "?name=" + testKeyName

	testStrings := []string{
		SkykeyScheme + ":" + keyDataString + nameParam, // pubaccesskey with scheme and name
		keyDataString + nameParam,                      // pubaccesskey with name and no scheme
		SkykeyScheme + ":" + keyDataString,             // pubaccesskey with scheme and no name
		keyDataString,                                  // pubaccesskey with no scheme and no name
	}
	skykeys := make([]Pubaccesskey, len(testStrings))

	// Check that we can load from string and recreate the input string from the
	// pubaccesskey.
	for i, testString := range testStrings {
		err := skykeys[i].FromString(testString)
		if err != nil {
			t.Fatal(err)
		}
		s, err := skykeys[i].ToString()
		if err != nil {
			t.Fatal(err)
		}

		// ToString should always output the "pubaccesskey:" scheme even if the input did
		// not.
		withScheme := strings.Contains(testString, SkykeyScheme)
		if withScheme && s != testString {
			t.Fatal("Expected string to match test string", i, s, testString)
		} else if !withScheme && s != SkykeyScheme+":"+testString {
			t.Fatal("Expected string to match test string", i, s, testString)
		}
	}

	// The first 2 keys should have names and the rest should not.
	for i, sk := range skykeys {
		if i <= 1 && sk.Name != testKeyName {
			t.Log(sk)
			t.Log("Expected testKeyName in pubaccesskey")
		}
		if i > 1 && sk.Name != "" {
			t.Log(sk)
			t.Log("Expected testKeyName in pubaccesskey")
		}
	}

	// All skykeys should have the same ID for each pubaccesskey.
	for i := 1; i < len(skykeys); i++ {
		if skykeys[i].ID() != skykeys[i-1].ID() {
			t.Fatal("Expected same ID", i)
		}
	}
}

// TestSkyeyMarshalling tests edges cases in marshalling and unmarshalling.
func TestSkykeyMarshalling(t *testing.T) {
	skykeyType := TypePublicID
	cipherKey := crypto.GenerateSiaKey(skykeyType.CipherType())
	pubaccesskey := Pubaccesskey{
		Type:    skykeyType,
		Entropy: cipherKey.Key(),
	}

	// marshal/unmarshal a good key.
	var buf bytes.Buffer
	err := pubaccesskey.marshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}
	sk := Pubaccesskey{}
	err = sk.unmarshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Add a name that is too long.
	for i := 0; i < MaxKeyNameLen+1; i++ {
		pubaccesskey.Name += "L"
	}
	buf.Reset()
	err = pubaccesskey.marshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshaling a Pubaccesskey with a long name should throw an error.
	sk = Pubaccesskey{}
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errSkykeyNameToolong) {
		t.Fatal("Expected error for long name", err)
	}
	// Forcefully marshal the pubaccesskey
	e := encoding.NewEncoder(&buf)
	e.WriteByte(byte(pubaccesskey.Type))
	e.Write(sk.Entropy[:])
	e.Encode(sk.Name)
	if err = e.Err(); err != nil {
		t.Fatal(err)
	}
	// Check for the unmarshal error.
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errSkykeyNameToolong) {
		t.Fatal("Expected error for trying to unmarshal pubaccesskey with a name that is too long", err)
	}

	// Fix the name length and use the (default) invalid type.
	pubaccesskey = Pubaccesskey{
		Name:    "a-reasonably-sized-name",
		Entropy: pubaccesskey.Entropy,
	}
	buf.Reset()
	err = pubaccesskey.marshalSia(&buf)
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errCannotMarshalTypeInvalidSkykey) {
		t.Fatal("Expected error for trying to marshal an invalid pubaccesskey type", err)
	}

	// Forcefully marshal a pubaccesskey with type invalid.
	e = encoding.NewEncoder(&buf)
	e.WriteByte(byte(TypeInvalid))
	e.Write(sk.Entropy[:])
	e.Encode(sk.Name)
	if err = e.Err(); err != nil {
		t.Fatal(err)
	}
	// Check for the unmarshal error.
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errCannotMarshalTypeInvalidSkykey) {
		t.Fatal("Expected error for trying to unmarshal an invalid pubaccesskey type", err)
	}

	// Use an unknown type and check for the marshal error.
	pubaccesskey.Type = SkykeyType(0xF0)
	buf.Reset()
	err = pubaccesskey.marshalSia(&buf)
	if !errors.Contains(err, errUnsupportedSkykeyType) {
		t.Fatal("Expected error for trying to marshal an unknown pubaccesskey type", err)
	}

	// Forcefully marshal a bad pubaccesskey.
	buf.Reset()
	e = encoding.NewEncoder(&buf)
	e.WriteByte(0xF0)
	e.Write(sk.Entropy[:])
	e.Encode(sk.Name)
	if err = e.Err(); err != nil {
		t.Fatal(err)
	}
	// Check for the unmarshal error.
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errUnsupportedSkykeyType) {
		t.Fatal("Expected error for trying to unmarshal an unknown pubaccesskey type", err)
	}

	// Create a pubaccesskey with small Entropy slice.
	pubaccesskey = Pubaccesskey{
		Name:    "aname",
		Type:    TypePublicID,
		Entropy: make([]byte, 5),
	}
	buf.Reset()
	err = pubaccesskey.marshalSia(&buf)
	if !errors.Contains(err, errInvalidEntropyLength) {
		t.Fatal(err)
	}

	// Forcefully marshal a bad pubaccesskey.
	buf.Reset()
	e = encoding.NewEncoder(&buf)
	e.WriteByte(byte(pubaccesskey.Type))
	e.Write(pubaccesskey.Entropy[:])
	e.Encode(pubaccesskey.Name)
	if err = e.Err(); err != nil {
		t.Fatal(err)
	}
	// Check for the unmarshal error.
	err = sk.unmarshalSia(&buf)
	if !errors.Contains(err, errUnmarshalDataErr) {
		t.Fatal("Expected error for trying to unmarshal pubaccesskey with small Entropy slice", err)
	}

	// Create a pubaccesskey with too large of an Entropy slice.
	pubaccesskey = Pubaccesskey{
		Name:    "aname",
		Type:    TypePublicID,
		Entropy: make([]byte, 500),
	}
	buf.Reset()
	err = pubaccesskey.marshalSia(&buf)
	if !errors.Contains(err, errInvalidEntropyLength) {
		t.Fatal(err)
	}

	// Forcefully marshal a bad pubaccesskey.
	buf.Reset()
	e = encoding.NewEncoder(&buf)
	e.WriteByte(byte(pubaccesskey.Type))
	e.Write(pubaccesskey.Entropy[:])
	e.Encode(pubaccesskey.Name)
	if err = e.Err(); err != nil {
		t.Fatal(err)
	}
	// There should be no error, because we only try to unmarshal the correct
	// (smaller) number of bytes.
	err = sk.unmarshalSia(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if sk.Type != TypePublicID {
		t.Fatal("Expected correct pubaccesskey type")
	}
	if sk.Name != "" {
		t.Fatal("Expected no pubaccesskey name")
	}
	if len(sk.Entropy) != chacha.XNonceSize+chacha.KeySize {
		t.Fatal("Expected entropy with correct size.")
	}

	// Unmarshaling a Pubaccesskey with a long name should throw an error.
	sk = Pubaccesskey{}
	err = sk.unmarshalSia(&buf)
	// Try unmarshalling small random byte slices.
	for i := 0; i < 10; i++ {
		buf.Reset()
		buf.Write(fastrand.Bytes(fastrand.Intn(20)))
		sk = Pubaccesskey{}

		err = sk.unmarshalSia(&buf)
		if err == nil {
			t.Log(buf)
			t.Log(sk)
			t.Fatal("Expected random byte unmarshaling to fail")
		}
	}

	// Try unmarshalling larger random byte slices.
	for i := 0; i < 10; i++ {
		buf.Reset()
		buf.Write(fastrand.Bytes(100 * fastrand.Intn(20)))
		sk = Pubaccesskey{}

		err = sk.unmarshalSia(&buf)
		if err == nil {
			t.Log(buf)
			t.Log(sk)
			t.Fatal("Expected random byte unmarshaling to fail")
		}
	}
}

// TestSkykeyTypeStrings tests FromString and ToString methods for SkykeyTypes
func TestSkykeyTypeStrings(t *testing.T) {
	publicIDString := TypePublicID.ToString()
	if publicIDString != "public-id" {
		t.Fatal("Incorrect skykeytype name", publicIDString)
	}

	var st SkykeyType
	err := st.FromString(publicIDString)
	if err != nil {
		t.Fatal(err)
	}
	if st != TypePublicID {
		t.Fatal("Wrong SkykeyType", st)
	}

	invalidTypeString := TypeInvalid.ToString()
	if invalidTypeString != "invalid" {
		t.Fatal("Incorrect skykeytype name", invalidTypeString)
	}

	var invalidSt SkykeyType
	err = invalidSt.FromString(invalidTypeString)
	if err != ErrInvalidSkykeyType {
		t.Fatal(err)
	}
}
