package modules

import (
	"encoding/binary"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/types"
)

type (
	// Instruction specifies a generic instruction used as an input to
	// `mdm.ExecuteProgram`.
	Instruction struct {
		Specifier InstructionSpecifier
		Args      []byte
	}
	// InstructionSpecifier specifies the type of the instruction.
	InstructionSpecifier types.Specifier
)

const (
	// MDMProgramInitTime is the time it takes to execute a program. This is a
	// hardcoded value which is meant to be replaced in the future. TODO: The
	// time is hardcoded to 10 for now until we add time management in the
	// future.
	MDMProgramInitTime = 10

	// MDMTimeAppend is the time for executing an 'Append' instruction.
	MDMTimeAppend = 10000

	// MDMTimeCommit is the time used for executing managedFinalize.
	MDMTimeCommit = 50e3

	// MDMTimeHasSector is the time for executing a 'HasSector' instruction.
	MDMTimeHasSector = 1

	// MDMTimeReadSector is the time for executing a 'ReadSector' instruction.
	MDMTimeReadSector = 1000

	// MDMTimeWriteSector is the time for executing a 'WriteSector' instruction.
	MDMTimeWriteSector = 10000

	// RPCIAppendLen is the expected length of the 'Args' of an Append
	// instructon.
	RPCIAppendLen = 9

	// RPCIDropSectorsLen is the expected length of the 'Args' of a DropSectors
	// Instruction.
	RPCIDropSectorsLen = 9

	// RPCIHasSectorLen is the expected length of the 'Args' of a HasSector
	// instruction.
	RPCIHasSectorLen = 8

	// RPCIReadSectorLen is the expected length of the 'Args' of a ReadSector
	// instruction.
	RPCIReadSectorLen = 25
)

var (
	// SpecifierAppend is the specifier for the Append instruction.
	SpecifierAppend = InstructionSpecifier{'A', 'p', 'p', 'e', 'n', 'd'}

	// SpecifierDropSectors is the specifier for the DropSectors instruction.
	SpecifierDropSectors = InstructionSpecifier{'D', 'r', 'o', 'p', 'S', 'e', 'c', 't', 'o', 'r', 's'}

	// SpecifierHasSector is the specifier for the HasSector instruction.
	SpecifierHasSector = InstructionSpecifier{'H', 'a', 's', 'S', 'e', 'c', 't', 'o', 'r'}

	// SpecifierReadSector is the specifier for the ReadSector instruction.
	SpecifierReadSector = InstructionSpecifier{'R', 'e', 'a', 'd', 'S', 'e', 'c', 't', 'o', 'r'}

	// ErrMDMInsufficientBudget is the error returned if the remaining budget of
	// an MDM program is not sufficient to execute the next instruction.
	ErrMDMInsufficientBudget = errors.New("remaining budget is insufficient")

	// ErrMDMInsufficientCollateralBudget is the error returned if the remaining
	// collateral budget of an MDM program is not sufficient to execute the next
	// instruction.
	ErrMDMInsufficientCollateralBudget = errors.New("remaining collateral budget is insufficient")
)

// RPCIReadSector is a convenience method to create an Instruction of type 'ReadSector'.
func RPCIReadSector(rootOff, offsetOff, lengthOff uint64, merkleProof bool) Instruction {
	args := make([]byte, RPCIReadSectorLen)
	binary.LittleEndian.PutUint64(args[:8], rootOff)
	binary.LittleEndian.PutUint64(args[8:16], offsetOff)
	binary.LittleEndian.PutUint64(args[16:24], lengthOff)
	if merkleProof {
		args[24] = 1
	}
	return Instruction{
		Args:      args,
		Specifier: SpecifierReadSector,
	}
}

// MDMAppendCost is the cost of executing an 'Append' instruction.
func MDMAppendCost(pt RPCPriceTable) (types.Currency, types.Currency) {
	writeCost := pt.WriteLengthCost.Mul64(SectorSize).Add(pt.WriteBaseCost)
	storeCost := pt.WriteStoreCost.Mul64(SectorSize) // potential refund
	return writeCost.Add(storeCost), storeCost
}

// MDMInitCost is the cost of instantiatine the MDM. It is defined as:
// 'InitBaseCost' + 'MemoryTimeCost' * 'programLen' * Time
func MDMInitCost(pt RPCPriceTable, programLen uint64) types.Currency {
	return pt.MemoryTimeCost.Mul64(programLen).Mul64(MDMProgramInitTime).Add(pt.InitBaseCost)
}

// MDMHasSectorCost is the cost of executing a 'HasSector' instruction.
func MDMHasSectorCost(pt RPCPriceTable) (types.Currency, types.Currency) {
	cost := pt.MemoryTimeCost.Mul64(1 << 20).Mul64(MDMTimeHasSector)
	refund := types.ZeroCurrency // no refund
	return cost, refund
}

// MDMReadCost is the cost of executing a 'Read' instruction. It is defined as:
// 'readBaseCost' + 'readLengthCost' * `readLength`
func MDMReadCost(pt RPCPriceTable, readLength uint64) (types.Currency, types.Currency) {
	cost := pt.ReadLengthCost.Mul64(readLength).Add(pt.ReadBaseCost)
	refund := types.ZeroCurrency // no refund
	return cost, refund
}

// MDMWriteCost is the cost of executing a 'Write' instruction of a certain length.
func MDMWriteCost(pt RPCPriceTable, writeLength uint64) (types.Currency, types.Currency) {
	writeCost := pt.WriteLengthCost.Mul64(writeLength).Add(pt.WriteBaseCost)
	storeCost := types.ZeroCurrency // no refund since we overwrite existing storage
	return writeCost, storeCost
}

// MDMCopyCost is the cost of executing a 'Copy' instruction.
func MDMCopyCost(pt RPCPriceTable, contractSize uint64) types.Currency {
	return types.SiacoinPrecision // TODO: figure out good cost
}

// MDMDropSectorsCost is the cost of executing a 'DropSectors' instruction for a
// certain number of dropped sectors.
func MDMDropSectorsCost(pt RPCPriceTable, numSectorsDropped uint64) (types.Currency, types.Currency) {
	cost := pt.DropSectorsLengthCost.Mul64(numSectorsDropped).Add(pt.DropSectorsBaseCost)
	refund := types.ZeroCurrency
	return cost, refund
}

// MDMSwapCost is the cost of executing a 'Swap' instruction.
func MDMSwapCost(pt RPCPriceTable, contractSize uint64) types.Currency {
	return types.SiacoinPrecision // TODO: figure out good cost
}

// MDMTruncateCost is the cost of executing a 'Truncate' instruction.
func MDMTruncateCost(pt RPCPriceTable, contractSize uint64) types.Currency {
	return types.SiacoinPrecision // TODO: figure out good cost
}

// MDMAppendMemory returns the additional memory consumption of a 'Append'
// instruction.
func MDMAppendMemory() uint64 {
	return SectorSize // A full sector is added to the program's memory until the program is finalized.
}

// MDMDropSectorsMemory returns the additional memory consumption of a
// `DropSectors` instruction
func MDMDropSectorsMemory() uint64 {
	return 0 // 'DropSectors' doesn't hold on to any memory beyond the lifetime of the instruction.
}

// MDMHasSectorMemory returns the additional memory consumption of a 'HasSector'
// instruction.
func MDMHasSectorMemory() uint64 {
	return 0 // 'HasSector' doesn't hold on to any memory beyond the lifetime of the instruction.
}

// MDMReadMemory returns the additional memory consumption of a 'Read' instruction.
func MDMReadMemory() uint64 {
	return 0 // 'Read' doesn't hold on to any memory beyond the lifetime of the instruction.
}

// MDMMemoryCost computes the memory cost given a price table, memory and time.
func MDMMemoryCost(pt RPCPriceTable, usedMemory, time uint64) types.Currency {
	return pt.MemoryTimeCost.Mul64(usedMemory * time)
}

// MDMAppendCollateral returns the additional collateral a 'Append' instruction
// requires the host to put up.
func MDMAppendCollateral(pt RPCPriceTable) types.Currency {
	return pt.CollateralCost.Mul64(SectorSize)
}

// MDMDropSectorsCollateral returns the additional collateral a 'DropSectors'
// instruction requires the host to put up.
func MDMDropSectorsCollateral() types.Currency {
	return types.ZeroCurrency
}

// MDMHasSectorCollateral returns the additional collateral a 'HasSector'
// instruction requires the host to put up.
func MDMHasSectorCollateral() types.Currency {
	return types.ZeroCurrency
}

// MDMReadCollateral returns the additional collateral a 'Read' instruction
// requires the host to put up.
func MDMReadCollateral() types.Currency {
	return types.ZeroCurrency
}
