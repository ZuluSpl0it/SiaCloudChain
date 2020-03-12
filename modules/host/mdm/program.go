package mdm

import (
	"context"
	"errors"
	"fmt"
	"io"

	"gitlab.com/NebulousLabs/threadgroup"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

// The following errors are returned by `cost.Sub` in case of an underflow.
// ErrInsufficientBudget is always returned in combination with the error of the
// corresponding resource.
var (
	ErrInsufficientBudget             = errors.New("program has insufficient budget to execute")
	ErrInsufficientComputeBudget      = errors.New("insufficient 'compute' budget")
	ErrInsufficientDiskAccessesBudget = errors.New("insufficient 'diskaccess' budget")
	ErrInsufficientDiskReadBudget     = errors.New("insufficient 'diskread' budget")
	ErrInsufficientDiskWriteBudget    = errors.New("insufficient 'diskwrite' budget")
	ErrInsufficientMemoryBudget       = errors.New("insufficient 'memory' budget")
)

var (
	// ErrInterrupted indicates that the program was interrupted during
	// execution and couldn't finish.
	ErrInterrupted = errors.New("execution of program was interrupted")
)

// programState contains some fields needed for the execution of instructions.
// The program's state is captured when the program is created and remains the
// same during the execution of the program.
type programState struct {
	// mdm related fields.
	remainingBudget Cost

	// host related fields
	blockHeight types.BlockHeight
	host        Host

	// storage obligation related fields
	sectorsRemoved   []crypto.Hash
	sectorsGained    []crypto.Hash
	gainedSectorData [][]byte
}

// Program is a collection of instructions. Within a program, each instruction
// will potentially modify the size and merkle root of a file contract. After the
// final instruction is executed, the MDM will create an updated revision of the
// FileContract which has to be signed by the renter and the host.
type Program struct {
	so                 StorageObligation
	instructions       []instruction
	staticData         *programData
	staticProgramState *programState

	finalContractSize uint64 // contract size after executing all instructions
	budget            Cost

	renterSig  types.TransactionSignature
	outputChan chan Output

	tg *threadgroup.ThreadGroup
}

// ExecuteProgram initializes a new program from a set of instructions and a reader
// which can be used to fetch the program's data and executes it.
func (mdm *MDM) ExecuteProgram(ctx context.Context, instructions []modules.Instruction, budget Cost, so StorageObligation, initialContractSize uint64, initialMerkleRoot crypto.Hash, programDataLen uint64, data io.Reader) (func() error, <-chan Output, error) {
	p := &Program{
		budget:            budget,
		finalContractSize: initialContractSize,
		outputChan:        make(chan Output, len(instructions)),
		staticProgramState: &programState{
			blockHeight:     mdm.host.BlockHeight(),
			host:            mdm.host,
			remainingBudget: budget,
		},
		staticData: openProgramData(data, programDataLen),
		so:         so,
		tg:         &mdm.tg,
	}
	defer p.staticData.Close()

	// Convert the instructions.
	var err error
	var instruction instruction
	for _, i := range instructions {
		switch i.Specifier {
		case modules.SpecifierReadSector:
			instruction, err = p.staticDecodeReadSectorInstruction(i)
		default:
			err = fmt.Errorf("unknown instruction specifier: %v", i.Specifier)
		}
		if err != nil {
			return nil, nil, err
		}
		p.instructions = append(p.instructions, instruction)
	}
	// Make sure that the contract is locked unless the program we're executing
	// is a readonly program.
	if !p.readOnly() && !p.so.Locked() {
		return nil, nil, errors.New("contract needs to be locked for a program with one or more write instructions")
	}
	// Make sure the budget covers the initial cost.
	ps := p.staticProgramState
	ps.remainingBudget, err = ps.remainingBudget.Sub(InitCost(p.staticData.Len()))
	if err != nil {
		return nil, nil, err
	}

	// Execute all the instructions.
	if err := p.tg.Add(); err != nil {
		return nil, nil, err
	}
	go func() {
		defer p.tg.Done()
		defer close(p.outputChan)
		p.executeInstructions(ctx, initialMerkleRoot)
	}()
	// If the program is readonly there is no need to finalize it.
	if p.readOnly() {
		return nil, p.outputChan, nil
	}
	return p.managedFinalize, p.outputChan, nil
}

// executeInstructions executes the programs instructions sequentially while
// returning the results to the caller using outputChan.
func (p *Program) executeInstructions(ctx context.Context, fcRoot crypto.Hash) {
	for _, i := range p.instructions {
		select {
		case <-ctx.Done(): // Check for interrupt
			p.outputChan <- outputFromError(ErrInterrupted)
			break
		default:
		}
		// Execute next instruction.
		output := i.Execute(fcRoot)
		fcRoot = output.NewMerkleRoot
		p.outputChan <- output
		// Abort if the last output contained an error.
		if output.Error != nil {
			break
		}
	}
}

// managedFinalize commits the changes made by the program to disk. It should
// only be called after the channel returned by Execute is closed.
func (p *Program) managedFinalize() error {
	// Commit the changes to the storage obligation.
	ps := p.staticProgramState
	err := p.so.Update(ps.sectorsRemoved, ps.sectorsGained, ps.gainedSectorData)
	if err != nil {
		return err
	}
	return nil
}

// readOnly returns 'true' if all of the instructions executed by a program are
// readonly.
func (p *Program) readOnly() bool {
	for _, i := range p.instructions {
		if !i.ReadOnly() {
			return false
		}
	}
	return true
}
