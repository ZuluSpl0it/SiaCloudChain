package renter

// pubfile_encryption.go provides utilities for encrypting and decrypting
// skyfiles.

import (
	"github.com/aead/chacha20/chacha"
	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
	"gitlab.com/scpcorp/ScPrime/types"
)

// baseSectorNonceDerivation is the specifier used to derive a nonce for base
// sector encryption
var baseSectorNonceDerivation = types.NewSpecifier("BaseSectorNonce")

// fanoutNonceDerivation is the specifier used to derive a nonce for
// fanout encryption.
var fanoutNonceDerivation = types.NewSpecifier("FanoutNonce")

var errNoSkykeyMatchesSkyfileEncryptionID = errors.New("Unable to find matching pubaccesskey for public ID encryption")

// deriveFanoutKey returns the crypto.CipherKey that should be used for
// decrypting the fanout stream from the pubfile stored using this layout.
func (r *Renter) deriveFanoutKey(sl *skyfileLayout, fileSkykey pubaccesskey.Pubaccesskey) (crypto.CipherKey, error) {
	if sl.cipherType != crypto.TypeXChaCha20 {
		return crypto.NewSiaKey(sl.cipherType, sl.keyData[:])
	}

	// Derive the fanout key.
	fanoutSkykey, err := fileSkykey.DeriveSubkey(fanoutNonceDerivation[:])
	if err != nil {
		return nil, errors.AddContext(err, "Error deriving pubaccesskey subkey")
	}
	return fanoutSkykey.CipherKey()
}

// checkSkyfileEncryptionIDMatch tries to find a Pubaccesskey that can decrypt the
// identifier and be used for decrypting the associated pubfile. It returns an
// error if it is not found.
func (r *Renter) checkSkyfileEncryptionIDMatch(encryptionIdentifer []byte, nonce []byte) (pubaccesskey.Pubaccesskey, error) {
	allSkykeys := r.staticSkykeyManager.Skykeys()
	for _, sk := range allSkykeys {
		matches, err := sk.MatchesSkyfileEncryptionID(encryptionIdentifer, nonce)
		if err != nil {
			r.log.Debugln("SkykeyEncryptionID match err", err)
			continue
		}
		if matches {
			return sk, nil
		}
	}
	return pubaccesskey.Pubaccesskey{}, errNoSkykeyMatchesSkyfileEncryptionID
}

// decryptBaseSector attempts to decrypt the baseSector. If it has the necessary
// Pubaccesskey, it will decrypt the baseSector in-place. It returns the file-specific
// pubaccesskey to be used for decrypting the rest of the associated pubfile.
func (r *Renter) decryptBaseSector(baseSector []byte) (pubaccesskey.Pubaccesskey, error) {
	// Sanity check - baseSector should not be more than modules.SectorSize.
	// Note that the base sector may be smaller in the event of a packed
	// pubfile.
	if uint64(len(baseSector)) > modules.SectorSize {
		build.Critical("decryptBaseSector given a baseSector that is too large")
		return pubaccesskey.Pubaccesskey{}, errors.New("baseSector too large")
	}
	var sl skyfileLayout
	sl.decode(baseSector)

	if !isEncryptedLayout(sl) {
		build.Critical("Expected layout to be marked as encrypted!")
	}

	// Get the nonce to be used for getting private-id skykeys, and for deriving the
	// file-specific pubaccesskey.
	nonce := make([]byte, chacha.XNonceSize)
	copy(nonce[:], sl.keyData[pubaccesskey.SkykeyIDLen:pubaccesskey.SkykeyIDLen+chacha.XNonceSize])

	// Grab the key ID from the layout.
	var keyID pubaccesskey.PubaccesskeyID
	copy(keyID[:], sl.keyData[:pubaccesskey.SkykeyIDLen])

	// Try to get the pubaccesskey associated with that ID.
	masterSkykey, err := r.staticSkykeyManager.KeyByID(keyID)
	// If the ID is unknown, use the key ID as an encryption identifier and try
	// finding the associated pubaccesskey.
	if errors.Contains(err, pubaccesskey.ErrNoSkykeysWithThatID) {
		masterSkykey, err = r.checkSkyfileEncryptionIDMatch(keyID[:], nonce)
	}
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "Unable to find associated pubaccesskey")
	}

	// Derive the file-specific key.
	fileSkykey, err := masterSkykey.SubkeyWithNonce(nonce)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "Unable to derive file-specific subkey")
	}

	// Derive the base sector subkey and use it to decrypt the base sector.
	baseSectorKey, err := fileSkykey.DeriveSubkey(baseSectorNonceDerivation[:])
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "Unable to derive baseSector subkey")
	}

	// Get the cipherkey.
	ck, err := baseSectorKey.CipherKey()
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "Unable to get baseSector cipherkey")
	}

	_, err = ck.DecryptBytesInPlace(baseSector, 0)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.New("Error decrypting baseSector for download")
	}

	// Save the visible-by-default fields of the baseSector's layout.
	version := sl.version
	cipherType := sl.cipherType
	var keyData [64]byte
	copy(keyData[:], sl.keyData[:])

	// Decode the now decrypted layout.
	sl.decode(baseSector)

	// Reset the visible-by-default fields.
	// (They were turned into random values by the decryption)
	sl.version = version
	sl.cipherType = cipherType
	copy(sl.keyData[:], keyData[:])

	// Now re-copy the decrypted layout into the decrypted baseSector.
	copy(baseSector[:SkyfileLayoutSize], sl.encode())

	return fileSkykey, nil
}

// encryptBaseSectorWithSkykey encrypts the baseSector in place using the given
// Pubaccesskey. Certain fields of the layout are restored in plaintext into the
// encrypted baseSector to indicate to downloaders what Pubaccesskey was used.
func encryptBaseSectorWithSkykey(baseSector []byte, plaintextLayout skyfileLayout, sk pubaccesskey.Pubaccesskey) error {
	baseSectorKey, err := sk.DeriveSubkey(baseSectorNonceDerivation[:])
	if err != nil {
		return errors.AddContext(err, "Unable to derive baseSector subkey")
	}

	// Get the cipherkey.
	ck, err := baseSectorKey.CipherKey()
	if err != nil {
		return errors.AddContext(err, "Unable to get baseSector cipherkey")
	}

	_, err = ck.DecryptBytesInPlace(baseSector, 0)
	if err != nil {
		return errors.New("Error decrypting baseSector for download")
	}

	// Re-add the visible-by-default fields of the baseSector.
	var encryptedLayout skyfileLayout
	encryptedLayout.decode(baseSector)
	encryptedLayout.version = plaintextLayout.version
	encryptedLayout.cipherType = baseSectorKey.CipherType()

	// Add the key ID or the encrypted pubfile identifier, depending on the key
	// type.
	switch sk.Type {
	case pubaccesskey.TypePublicID:
		keyID := sk.ID()
		copy(encryptedLayout.keyData[:pubaccesskey.SkykeyIDLen], keyID[:])

	case pubaccesskey.TypePrivateID:
		encryptedIdentifier, err := sk.GenerateSkyfileEncryptionID()
		if err != nil {
			return errors.AddContext(err, "Unable to generate encrypted pubfile ID")
		}
		copy(encryptedLayout.keyData[:pubaccesskey.SkykeyIDLen], encryptedIdentifier[:])

	default:
		build.Critical("No encryption implemented for this pubaccesskey type")
		return errors.AddContext(errors.New("No encryption implemented for pubaccesskey type"), string(sk.Type))
	}

	// Add the nonce to the base sector, in plaintext.
	nonce := sk.Nonce()
	copy(encryptedLayout.keyData[pubaccesskey.SkykeyIDLen:pubaccesskey.SkykeyIDLen+len(nonce)], nonce[:])

	// Now re-copy the encrypted layout into the baseSector.
	copy(baseSector[:SkyfileLayoutSize], encryptedLayout.encode())
	return nil
}

// isEncryptedBaseSector returns true if and only if the the baseSector is
// encrypted.
func isEncryptedBaseSector(baseSector []byte) bool {
	var sl skyfileLayout
	sl.decode(baseSector)
	return isEncryptedLayout(sl)
}

// isEncryptedLayout returns true if and only if the the layout indicates that
// it is from an encrypted base sector.
func isEncryptedLayout(sl skyfileLayout) bool {
	return sl.version == 1 && sl.cipherType == crypto.TypeXChaCha20
}

func encryptionEnabled(sup modules.PubfileUploadParameters) bool {
	return sup.SkykeyName != "" || sup.PubaccesskeyID != pubaccesskey.PubaccesskeyID{}
}
