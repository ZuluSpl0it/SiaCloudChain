package modules

import (
	"encoding/base32"
	"testing"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
)

// TestPublinkManualExamples checks a pile of manual examples using table driven
// tests.
func TestPublinkManualExamples(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Good Examples.
	var publinkExamples = []struct {
		offset         uint64
		length         uint64
		expectedLength uint64
	}{
		// Try a valid offset for each mode.
		{4096, 1, 4096},
		{4096 * 2, (32 * 1024) + 1, 32*1024 + 4096},
		{4096 * 4, (64 * 1024) + 1, 64*1024 + 4096*2},
		{4096 * 8, (128 * 1024) + 1, 128*1024 + 4096*4},
		{4096 * 16, (256 * 1024) + 1, 256*1024 + 4096*8},
		{4096 * 32, (512 * 1024) + 1, 512*1024 + 4096*16},
		{4096 * 64, (1024 * 1024) + 1, 1024*1024 + 4096*32},
		{4096 * 128, (2048 * 1024) + 1, 2048*1024 + 4096*64},
		// Smattering of random examples.
		{4096, 0, 4096},
		{4096 * 2, 0, 4096},
		{4096 * 3, 0, 4096},
		{4096 * 3, 4096 * 8, 4096 * 8},
		{0, 1, 4096},
		{0, 4095, 4096},
		{0, 4096, 4096},
		{0, 4097, 8192},
		{0, 8192, 8192},
		{4096 * 45, 0, 4096},
		{0, 10e3, 4096 * 3},
		{0, 33e3, 4096 * 9},
		{0, 39e3, 4096 * 10},
		{8192 * 350, 39e3, 4096 * 10},
		{0, 71 * 1024, 72 * 1024},
		{0, (32 * 1024) - 1, 32 * 1024},
		{0, 32 * 1024, 32 * 1024},
		{0, (32 * 1024) + 1, 36 * 1024},
		{0, (64 * 1024) - 1, 64 * 1024},
		{8 * 1024, (64 * 1024) - 1, 64 * 1024},
		{16 * 1024, (64 * 1024) - 1, 64 * 1024},
		{0, (64 * 1024), 64 * 1024},
		{24 * 1024, (64 * 1024), 64 * 1024},
		{56 * 1024, (64 * 1024), 64 * 1024},
		{0, (64 * 1024) + 1, 72 * 1024},
		{16 * 1024, (64 * 1024) - 1, 64 * 1024},
		{48 * 1024, (64 * 1024) - 1, 64 * 1024},
		{16 * 1024, (64 * 1024), 64 * 1024},
		{48 * 1024, (64 * 1024), 64 * 1024},
		{16 * 1024, (64 * 1024) + 1, 72 * 1024},
		{48 * 1024, (64 * 1024) + 1, 72 * 1024},
		{16 * 1024, (72 * 1024) - 1, 72 * 1024},
		{48 * 1024, (72 * 1024) - 1, 72 * 1024},
		{16 * 1024, (72 * 1024), 72 * 1024},
		{48 * 1024, (72 * 1024), 72 * 1024},
		{16 * 1024, (72 * 1024) + 1, 80 * 1024},
		{48 * 1024, (72 * 1024) + 1, 80 * 1024},
		{192 * 1024, (288 * 1024) - 1, 288 * 1024},
		{128 * 2 * 1024, 1025 * 1024, (1024 + 128) * 1024},
		{512 * 1024, 2050 * 1024, (2048 + 256) * 1024},
	}
	// Try each example.
	for _, example := range publinkExamples {
		sl, err := NewPublinkV1(crypto.Hash{}, example.offset, example.length)
		if err != nil {
			t.Error(err)
		}
		offset, length, err := sl.OffsetAndFetchSize()
		if err != nil {
			t.Fatal(err)
		}
		if offset != example.offset {
			t.Error("bad offset:", example.offset, example.length, example.expectedLength, offset)
		}
		if length != example.expectedLength {
			t.Error("bad length:", example.offset, example.length, example.expectedLength, length)
		}
		if sl.Version() != 1 {
			t.Error("bad version:", sl.Version())
		}
	}

	// Invalid Examples.
	var badPublinkExamples = []struct {
		offset uint64
		length uint64
	}{
		// Try an invalid offset for each mode.
		{2048, 4096},
		{4096, (4096 * 8) + 1},
		{4096 * 2, (4096 * 2 * 8) + 1},
		{4096 * 4, (4096 * 4 * 8) + 1},
		{4096 * 8, (4096 * 8 * 8) + 1},
		{4096 * 16, (4096 * 16 * 8) + 1},
		{4096 * 32, (4096 * 32 * 8) + 1},
		{4096 * 64, (4096 * 64 * 8) + 1},
		// Try some invalid inputs.
		{1024 * 1024 * 3, 1024 * 1024 * 2},
	}
	// Try each example.
	for _, example := range badPublinkExamples {
		_, err := NewPublinkV1(crypto.Hash{}, example.offset, example.length)
		if err == nil {
			t.Error("expecting a failure:", example.offset, example.length)
		}
	}
}

// TestPublink checks that the linkformat is correctly encoding to and decoding
// from a string.
func TestPublink(t *testing.T) {
	// Create a linkdata struct that is all 0's, check that the resulting
	// publink is the right size, and check that the struct encodes and decodes
	// without problems.
	var slMin Publink
	str := slMin.String()
	if len(str) != base64EncodedPublinkSize {
		t.Error("publink is not the right size")
	}
	var slMinDecoded Publink
	err := slMinDecoded.LoadString(str)
	if err != nil {
		t.Fatal(err)
	}
	if slMinDecoded != slMin {
		t.Error("encoding and decoding is not symmetric")
	}

	// Create a linkdata struct that is all 1's, check that the resulting
	// publink is the right size, and check that the struct encodes and decodes
	// without problems.
	slMax := Publink{
		bitfield: 65535,
	}
	slMax.bitfield -= 7175 // set the final three bits to 0, and also bits 10, 11, 12 to zer oto make this a valid publink.
	for i := 0; i < len(slMax.merkleRoot); i++ {
		slMax.merkleRoot[i] = 255
	}
	str = slMax.String()
	if len(str) != base64EncodedPublinkSize {
		t.Error("str is not the right size")
	}
	var slMaxDecoded Publink
	err = slMaxDecoded.LoadString(str)
	if err != nil {
		t.Fatal(err)
	}
	if slMaxDecoded != slMax {
		t.Error("encoding and decoding is not symmetric")
	}

	// Verify the base32 encoded representation of the Publink
	b32 := slMax.Base32EncodedString()
	if len(b32) != base32EncodedPublinkSize {
		t.Error("encoded base32 string is not the right size")
	}
	var slMaxB32Decoded Publink
	err = slMaxB32Decoded.LoadString(b32)
	if err != nil {
		t.Error("should be no issues loading a base32 encoded publink")
	}
	if slMaxB32Decoded != slMax {
		t.Error("base32 encoding and decoding is not symmetric")
	}
	if slMaxB32Decoded.String() != slMax.String() {
		t.Error("base32 encoding and decoding is not symmetric")
	}

	// Try loading a base32 encoded string that has an incorrect size
	b32OffByOne := b32[1:]
	err = slMaxB32Decoded.LoadString(b32OffByOne)
	if !errors.Contains(err, ErrPublinkIncorrectSize) {
		t.Error("expecting 'ErrPublinkIncorrectSize' when loading string that is too small")
	}

	// Try loading a base32 encoded string that has an incorrect size
	b32OffByOne = b32 + "a"
	err = slMaxB32Decoded.LoadString(b32OffByOne)
	if !errors.Contains(err, ErrPublinkIncorrectSize) {
		t.Error("expecting 'ErrPublinkIncorrectSize' when loading string that is too large")
	}

	// Try loading a base32 encoded string that has an illegal character
	b32IllegalChar := "_" + b32[1:]
	err = slMaxB32Decoded.LoadString(b32IllegalChar)
	if err == nil {
		t.Error("expecting error when loading a string containing an illegal character")
	}

	// Try loading a base32 encoded string with invalid bitfield
	var slInvalidBitfield Publink
	slInvalidBitfield.bitfield = 1
	b32BadBitfield := slInvalidBitfield.Base32EncodedString()
	err = slMaxB32Decoded.LoadString(b32BadBitfield)
	if err == nil {
		t.Error("expecting error when loading a string representing a publink with an illegal bitfield")
	}

	// Try loading an arbitrary string that is too small.
	var sl Publink
	var arb string
	for i := 0; i < base64EncodedPublinkSize-1; i++ {
		arb = arb + "a"
	}
	err = sl.LoadString(arb)
	if !errors.Contains(err, ErrPublinkIncorrectSize) {
		t.Error("expecting 'ErrPublinkIncorrectSize' when loading string that is too small")
	}
	// Try loading a siafile that's just arbitrary/meaningless data.
	arb = arb + "a"
	err = sl.LoadString(arb)
	if err == nil {
		t.Error("arbitrary string should not decode")
	}
	// Try loading a siafile that's too large.
	long := arb + "a"
	err = sl.LoadString(long)
	if err == nil {
		t.Error("expecting error when loading string that is too large")
	}
	// Try loading a blank siafile.
	blank := ""
	err = sl.LoadString(blank)
	if !errors.Contains(err, ErrPublinkIncorrectSize) {
		t.Error("expecting 'ErrPublinkIncorrectSize' when loading a blank string")
	}

	// Try giving a publink extra params and loading that.
	slStr := sl.String()
	params := slStr + "?fdsafdsafdsa"
	err = sl.LoadString(params)
	if err != nil {
		t.Error("should be no issues loading a publink with params")
	}
	// Add more params, separated by ampersands, per URL standards
	params = params + "&fffffdsafdsafdsa"
	err = sl.LoadString(params)
	if err != nil {
		t.Error("should be no issues loading a publink with params")
	}

	// Try loading a non base64 string.
	nonb64 := "sia://%" + slStr
	err = sl.LoadString(nonb64[:len(slStr)])
	if err == nil {
		t.Error("should not be able to load non base64 string")
	}

	// Try parsing a pubfile that's got a bad version.
	var slBad Publink
	slBad.bitfield = 1
	str = slBad.String()
	_, _, err = slBad.OffsetAndFetchSize()
	if err == nil {
		t.Error("should not be able to get offset and fetch size of bad publink")
	}
	// Try setting invalid mode bits.
	slBad.bitfield = ^uint16(0) - 3
	_, _, err = slBad.OffsetAndFetchSize()
	if err == nil {
		t.Error("should not be able to get offset and fetch size of bad publink")
	}

	// Check the MerkleRoot() function.
	mr := crypto.HashObject("fdsa")
	sl, err = NewPublinkV1(mr, 4096, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if sl.MerkleRoot() != mr {
		t.Fatal("root mismatch")
	}
}

// TestPublinkAutoExamples performs a brute force test over lots of values for
// the publink bitfield to ensure correctness.
func TestPublinkAutoExamples(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Helper function to try some values.
	tryValues := func(offset, length, expectedLength uint64) {
		sl, err := NewPublinkV1(crypto.Hash{}, offset, length)
		if err != nil {
			t.Error(err)
		}
		offsetOut, lengthOut, err := sl.OffsetAndFetchSize()
		if err != nil {
			t.Fatal(err)
		}
		if offset != offsetOut {
			t.Error("bad offset:", offset, length, expectedLength, offsetOut)
		}
		if expectedLength != lengthOut {
			t.Error("bad length:", offset, length, expectedLength, lengthOut)
		}

		// Encode the publink and then decode the publink. There should be no
		// errors in doing so, and the result should equal the initial.
		str := sl.String()
		var slDecode Publink
		err = slDecode.LoadString(str)
		if err != nil {
			t.Error(err)
		}
		if slDecode != sl {
			t.Log(sl)
			t.Error("linkdata does not maintain its fields when encoded and decoded")
		}
	}

	// Check every length in the first row. The first row must be offset by 4
	// kib.
	for i := uint64(0); i < 8; i++ {
		// Check every possible offset for each length.
		for j := uint64(0); j < 1024-i; j++ {
			// Try the edge cases. One byte into the length, one byte before the
			// end of the length, the very end of the length.
			shift := uint64(0)
			offsetAlign := uint64(4096)
			lengthAlign := uint64(4096)
			tryValues(offsetAlign*j, shift+((lengthAlign*i)+1), shift+(lengthAlign*(i+1)))
			tryValues(offsetAlign*j, shift+((lengthAlign*(i+1))-1), shift+(lengthAlign*(i+1)))
			tryValues(offsetAlign*j, shift+(lengthAlign*(i+1)), shift+(lengthAlign*(i+1)))

			// Try some random values.
			for k := uint64(0); k < 5; k++ {
				rand := uint64(fastrand.Intn(int(lengthAlign)))
				rand++                            // move range from [0, lengthAlign) to [1, lengthAlign].
				rand += shift + (lengthAlign * i) // Move range into the range being tested.
				tryValues(offsetAlign*j, rand, shift+(lengthAlign*(i+1)))
			}
		}
	}

	// The first row is a special case, a general loop can be used for the
	// remaining 7 rows.
	for r := uint64(1); r < 7; r++ {
		// Check every length in the second row.
		for i := uint64(0); i < 8; i++ {
			// Check every possible offset for each length.
			offsets := uint64(1024 >> r)
			for j := uint64(0); j < offsets-4-(i/2); j++ {
				// Try the edge cases. One byte into the length, one byte before the
				// end of the length, the very end of the length.
				shift := uint64(1 << (14 + r))
				offsetAlign := uint64(1 << (12 + r))
				lengthAlign := uint64(1 << (11 + r))
				tryValues(offsetAlign*j, shift+((lengthAlign*i)+1), shift+(lengthAlign*(i+1)))
				tryValues(offsetAlign*j, shift+((lengthAlign*(i+1))-1), shift+(lengthAlign*(i+1)))
				tryValues(offsetAlign*j, shift+(lengthAlign*(i+1)), shift+(lengthAlign*(i+1)))

				// Try some random values for the length.
				for k := uint64(0); k < 25; k++ {
					rand := uint64(fastrand.Intn(int(lengthAlign)))
					rand++                            // move range from [0, lengthAlign) to [1, lengthAlign].
					rand += shift + (lengthAlign * i) // Move range into the range being tested.
					tryValues(offsetAlign*j, rand, shift+(lengthAlign*(i+1)))
				}
			}
		}
	}
}

// Base32EncodedString converts Publink to a base32 encoded string.
func (sl Publink) Base32EncodedString() string {
	// Encode the raw bytes to base32
	return base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sl.Bytes())
}
