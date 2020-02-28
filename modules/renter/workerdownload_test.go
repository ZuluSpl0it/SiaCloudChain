package renter

import (
	"testing"

	"gitlab.com/NebulousLabs/fastrand"

	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/modules/renter/siafile"
	"gitlab.com/SiaPrime/SiaPrime/types"
)

// TestSegmentsForRecovery tests the segmentsForRecovery helper function.
func TestSegmentsForRecovery(t *testing.T) {
	// Test the legacy erasure coder first.
	rscOld, err := siafile.NewRSCode(10, 20)
	if err != nil {
		t.Fatal(err)
	}
	offset := fastrand.Intn(100)
	length := fastrand.Intn(100)
	startSeg, numSeg := segmentsForRecovery(uint64(offset), uint64(length), rscOld)
	if startSeg != 0 || numSeg != modules.SectorSize/crypto.SegmentSize {
		t.Fatal("segmentsForRecovery failed for legacy erasure coder")
	}

	// Get a new erasure coder and decoded segment size.
	rsc, err := siafile.NewRSSubCode(10, 20, 64)
	if err != nil {
		t.Fatal(err)
	}

	// Define a function for easier testing.
	assert := func(offset, length, expectedStartSeg, expectedNumSeg uint64) {
		startSeg, numSeg := segmentsForRecovery(offset, length, rsc)
		if startSeg != expectedStartSeg {
			t.Fatalf("wrong startSeg: expected %v but was %v", expectedStartSeg, startSeg)
		}
		if numSeg != expectedNumSeg {
			t.Fatalf("wrong numSeg: expected %v but was %v", expectedNumSeg, numSeg)
		}
	}

	// Test edge cases within the first segment.
	assert(0, 640, 0, 1)
	assert(1, 639, 0, 1)
	assert(639, 1, 0, 1)
	assert(1, 639, 0, 1)

	// Same lengths but different offset.
	assert(640, 640, 1, 1)
	assert(641, 639, 1, 1)
	assert(1279, 1, 1, 1)
	assert(641, 639, 1, 1)

	// Test fetching 2 segments.
	assert(0, 641, 0, 2)
	assert(1, 640, 0, 2)
	assert(640, 641, 1, 2)
	assert(641, 640, 1, 2)

	// Test fetching 3 segments.
	assert(0, 1281, 0, 3)
	assert(1, 1280, 0, 3)
	assert(1, 1281, 0, 3)
	assert(640, 1281, 1, 3)
	assert(641, 1280, 1, 3)
}

// TestSectorOffsetAndLength tests the sectorOffsetAndLength helper function.
func TestSectorOffsetAndLength(t *testing.T) {
	// Test the legacy erasure coder first.
	rscOld, err := siafile.NewRSCode(10, 20)
	if err != nil {
		t.Fatal(err)
	}
	offset := fastrand.Intn(100)
	length := fastrand.Intn(100)
	startSeg, numSeg := sectorOffsetAndLength(uint64(offset), uint64(length), rscOld)
	if startSeg != 0 || numSeg != modules.SectorSize {
		t.Fatal("sectorOffsetAndLength failed for legacy erasure coder")
	}

	// Get a new erasure coder and decoded segment size.
	rsc, err := siafile.NewRSSubCode(10, 20, 64)
	if err != nil {
		t.Fatal(err)
	}

	// Define a function for easier testing.
	assert := func(offset, length, expectedOffset, expectedLength uint64) {
		o, l := sectorOffsetAndLength(offset, length, rsc)
		if o != expectedOffset {
			t.Fatalf("wrong offset: expected %v but was %v", expectedOffset, o)
		}
		if l != expectedLength {
			t.Fatalf("wrong length: expected %v but was %v", expectedLength, l)
		}
	}

	// Test edge cases within the first segment.
	assert(0, 640, 0, 64)
	assert(1, 639, 0, 64)
	assert(639, 1, 0, 64)
	assert(1, 639, 0, 64)

	// Same lengths but different offset.
	assert(640, 640, 64, 64)
	assert(641, 639, 64, 64)
	assert(1279, 1, 64, 64)
	assert(641, 639, 64, 64)

	// Test fetching 2 segments.
	assert(0, 641, 0, 128)
	assert(1, 640, 0, 128)
	assert(640, 641, 64, 128)
	assert(641, 640, 64, 128)

	// Test fetching 3 segments.
	assert(0, 1281, 0, 192)
	assert(1, 1280, 0, 192)
	assert(1, 1281, 0, 192)
	assert(640, 1281, 64, 192)
	assert(641, 1280, 64, 192)
}

// TestCheckDownloadGouging checks that the fetch backups price gouging
// checker is correctly detecting price gouging from a host.
func TestCheckDownloadGouging(t *testing.T) {
	oneCurrency := types.NewCurrency64(1)

	// minAllowance contains only the fields necessary to test the price gouging
	// function. The min allowance doesn't include any of the max prices,
	// because the function will ignore them if they are not set.
	minAllowance := modules.Allowance{
		// Funds is set such that the tests come out to an easy, round number.
		// One siacoin is multiplied by the number of elements that are checked
		// for gouging, and then divided by the gounging denominator.
		Funds: types.SiacoinPrecision.Mul64(3).Div64(downloadGougingFractionDenom).Sub(oneCurrency),

		ExpectedDownload: modules.StreamDownloadSize, // 1 stream download operation.
	}
	// minHostSettings contains only the fields necessary to test the price
	// gouging function.
	//
	// The cost is set to be exactly equal to the price gouging limit, such that
	// slightly decreasing any of the values evades the price gouging detector.
	minHostSettings := modules.HostExternalSettings{
		BaseRPCPrice:           types.SiacoinPrecision,
		DownloadBandwidthPrice: types.SiacoinPrecision.Div64(modules.StreamDownloadSize),
		SectorAccessPrice:      types.SiacoinPrecision,
	}

	err := checkDownloadGouging(minAllowance, minHostSettings)
	if err == nil {
		t.Fatal("expecting price gouging check to fail:", err)
	}

	// Drop the host prices one field at a time.
	newHostSettings := minHostSettings
	newHostSettings.BaseRPCPrice = minHostSettings.BaseRPCPrice.Mul64(100).Div64(101)
	err = checkDownloadGouging(minAllowance, newHostSettings)
	if err != nil {
		t.Fatal(err)
	}
	newHostSettings = minHostSettings
	newHostSettings.DownloadBandwidthPrice = minHostSettings.DownloadBandwidthPrice.Mul64(100).Div64(101)
	err = checkDownloadGouging(minAllowance, newHostSettings)
	if err != nil {
		t.Fatal(err)
	}
	newHostSettings = minHostSettings
	newHostSettings.SectorAccessPrice = minHostSettings.SectorAccessPrice.Mul64(100).Div64(101)
	err = checkDownloadGouging(minAllowance, newHostSettings)
	if err != nil {
		t.Fatal(err)
	}

	// Set min settings on the allowance that are just below what should be
	// acceptable.
	maxAllowance := minAllowance
	maxAllowance.Funds = maxAllowance.Funds.Add(oneCurrency)
	maxAllowance.MaxRPCPrice = types.SiacoinPrecision.Add(oneCurrency)
	maxAllowance.MaxContractPrice = oneCurrency
	maxAllowance.MaxDownloadBandwidthPrice = types.SiacoinPrecision.Div64(modules.StreamDownloadSize).Add(oneCurrency)
	maxAllowance.MaxSectorAccessPrice = types.SiacoinPrecision.Add(oneCurrency)
	maxAllowance.MaxStoragePrice = oneCurrency
	maxAllowance.MaxUploadBandwidthPrice = oneCurrency

	// The max allowance should have no issues with price gouging.
	err = checkDownloadGouging(maxAllowance, minHostSettings)
	if err != nil {
		t.Fatal(err)
	}

	// Should fail if the MaxRPCPrice is dropped.
	failAllowance := maxAllowance
	failAllowance.MaxRPCPrice = types.SiacoinPrecision.Sub(oneCurrency)
	err = checkDownloadGouging(failAllowance, minHostSettings)
	if err == nil {
		t.Fatal("expecting price gouging check to fail")
	}

	// Should fail if the MaxDownloadBandwidthPrice is dropped.
	failAllowance = maxAllowance
	failAllowance.MaxDownloadBandwidthPrice = types.SiacoinPrecision.Div64(modules.StreamDownloadSize).Sub(oneCurrency)
	err = checkDownloadGouging(failAllowance, minHostSettings)
	if err == nil {
		t.Fatal("expecting price gouging check to fail")
	}

	// Should fail if the MaxSectorAccessPrice is dropped.
	failAllowance = maxAllowance
	failAllowance.MaxSectorAccessPrice = types.SiacoinPrecision.Sub(oneCurrency)
	err = checkDownloadGouging(failAllowance, minHostSettings)
	if err == nil {
		t.Fatal("expecting price gouging check to fail")
	}
}
