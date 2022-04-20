package types

// block.go defines the Block type for ScPrime, and provides some helper functions
// for working with blocks.

import (
	"bytes"

	"gitlab.com/NebulousLabs/encoding"
	"gitlab.com/scpcorp/ScPrime/crypto"
)

const (
	// BlockHeaderSize is the size, in bytes, of a block header.
	// 32 (ParentID) + 8 (Nonce) + 8 (Timestamp) + 32 (MerkleRoot)
	BlockHeaderSize = 80
)

type (
	// A Block is a summary of changes to the state that have occurred since the
	// previous block. Blocks reference the ID of the previous block (their
	// "parent"), creating the linked-list commonly known as the blockchain. Their
	// primary function is to bundle together transactions on the network. Blocks
	// are created by "miners," who collect transactions from other nodes, and
	// then try to pick a Nonce that results in a block whose BlockID is below a
	// given Target.
	Block struct {
		ParentID     BlockID         `json:"parentid"`
		Nonce        BlockNonce      `json:"nonce"`
		Timestamp    Timestamp       `json:"timestamp"`
		MinerPayouts []SiacoinOutput `json:"minerpayouts"`
		Transactions []Transaction   `json:"transactions"`
	}

	// A BlockHeader contains the data that, when hashed, produces the Block's ID.
	BlockHeader struct {
		ParentID   BlockID     `json:"parentid"`
		Nonce      BlockNonce  `json:"nonce"`
		Timestamp  Timestamp   `json:"timestamp"`
		MerkleRoot crypto.Hash `json:"merkleroot"`
	}

	// BlockHeight is the number of blocks that exist after the genesis block.
	BlockHeight uint64
	// A BlockID is the hash of a BlockHeader. A BlockID uniquely
	// identifies a Block, and indicates the amount of work performed
	// to mine that Block. The more leading zeros in the BlockID, the
	// more work was performed.
	BlockID crypto.Hash
	// The BlockNonce is a "scratch space" that miners can freely alter to produce
	// a BlockID that satisfies a given Target.
	BlockNonce [8]byte
)

// CalculateCoinbase calculates the coinbase for a given height. The coinbase
// equation is:
//
//     old coinbase := max(InitialCoinbase - height, MinimumCoinbase) * ScPrimecoinPrecision uint64
//     new coinbase := static @ 30 SCC
func CalculateCoinbase(height BlockHeight) Currency {
	base := uint64(30) //30e3 SC = 30 SCC
	return NewCurrency64(base).Mul(ScPrimecoinPrecision)
}

// CalculateNumSiacoins calculates the number of siacoins in circulation at a
// given height.
func CalculateNumSiacoins(height BlockHeight) Currency {
	airdropCoins := AirdropCommunityValue.Add(AirdropPoolValue).Add(AirdropSiaPrimeValue).Add(AirdropSiaCloudValue)

	deflationBlocks := BlockHeight(InitialCoinbase - MinimumCoinbase)
	avgDeflationSiacoins := CalculateCoinbase(0).Add(CalculateCoinbase(height)).Div(NewCurrency64(2))
	if height <= deflationBlocks {
		deflationSiacoins := avgDeflationSiacoins.Mul(NewCurrency64(uint64(height + 1)))
		return airdropCoins.Add(deflationSiacoins)
	}
	deflationSiacoins := avgDeflationSiacoins.Mul(NewCurrency64(uint64(deflationBlocks + 1)))
	trailingSiacoins := NewCurrency64(uint64(height - deflationBlocks)).Mul(CalculateCoinbase(height))
	return airdropCoins.Add(deflationSiacoins).Add(trailingSiacoins)
}

var numGenesisSiacoins = func() Currency {
	// Sum all the values for the genesis siacoin outputs.
	numGenesisSiacoins := NewCurrency64(0)
	for _, transaction := range GenesisBlock.Transactions {
		for _, siacoinOutput := range transaction.SiacoinOutputs {
			numGenesisSiacoins = numGenesisSiacoins.Add(siacoinOutput.Value)
		}
	}
	return numGenesisSiacoins
}()

// ID returns the ID of a Block, which is calculated by hashing the header.
func (h BlockHeader) ID() BlockID {
	return BlockID(crypto.HashObject(h))
}

// CalculateSubsidy takes a block and a height and determines the block
// subsidy.
func (b Block) CalculateSubsidy(height BlockHeight) Currency {
	subsidy := CalculateCoinbase(height)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			subsidy = subsidy.Add(fee)
		}
	}
	return subsidy
}

// Header returns the header of a block.
func (b Block) Header() BlockHeader {
	return BlockHeader{
		ParentID:   b.ParentID,
		Nonce:      b.Nonce,
		Timestamp:  b.Timestamp,
		MerkleRoot: b.MerkleRoot(),
	}
}

// ID returns the ID of a Block, which is calculated by hashing the
// concatenation of the block's parent's ID, nonce, and the result of the
// b.MerkleRoot(). It is equivalent to calling block.Header().ID()
func (b Block) ID() BlockID {
	return b.Header().ID()
}

// MerkleRoot calculates the Merkle root of a Block. The leaves of the Merkle
// tree are composed of the miner outputs (one leaf per payout), and the
// transactions (one leaf per transaction).
func (b Block) MerkleRoot() crypto.Hash {
	tree := crypto.NewTree()
	var buf bytes.Buffer
	e := encoding.NewEncoder(&buf)
	for _, payout := range b.MinerPayouts {
		payout.MarshalSia(e)
		tree.Push(buf.Bytes())
		buf.Reset()
	}
	for _, txn := range b.Transactions {
		txn.MarshalSia(e)
		tree.Push(buf.Bytes())
		buf.Reset()
	}
	return tree.Root()
}

// MinerPayoutID returns the ID of the miner payout at the given index, which
// is calculated by hashing the concatenation of the BlockID and the payout
// index.
func (b Block) MinerPayoutID(i uint64) SiacoinOutputID {
	return SiacoinOutputID(crypto.HashAll(
		b.ID(),
		i,
	))
}

// CalculateMinerFees calculates the sum of a block's miner transaction fees
func (b Block) CalculateMinerFees() Currency {
	fees := NewCurrency64(0)
	for _, txn := range b.Transactions {
		for _, fee := range txn.MinerFees {
			fees = fees.Add(fee)
		}
	}
	return fees
}

// CalculateDevSubsidy takes a block and a height and determines the block
// subsidies for the dev fund.
func CalculateDevSubsidy(height BlockHeight) Currency {
	coinbase := CalculateCoinbase(height)

	devSubsidy := NewCurrency64(0)
	if DevFundEnabled && (height >= DevFundInitialBlockHeight) {
		devFundDecayPercentage := uint64(100)
		if height >= DevFundDecayEndBlockHeight {
			devFundDecayPercentage = uint64(0)
		} else if height >= DevFundDecayStartBlockHeight {
			devFundDecayStartBlockHeight := uint64(DevFundDecayStartBlockHeight)
			devFundDecayEndBlockHeight := uint64(DevFundDecayEndBlockHeight)
			devFundDecayPercentage = uint64(100) - (uint64(height)-devFundDecayStartBlockHeight)*uint64(100)/(devFundDecayEndBlockHeight-devFundDecayStartBlockHeight)
		}

		devFundPercentageRange := DevFundInitialPercentage - DevFundFinalPercentage
		devFundPercentage := DevFundFinalPercentage*uint64(100) + devFundPercentageRange*devFundDecayPercentage

		devSubsidy = coinbase.Mul(NewCurrency64(devFundPercentage)).Div(NewCurrency64(10000))
	}

	return devSubsidy
}

// CalculateSubsidies takes a block and a height and determines the block
// subsidies for miners and the dev fund.
func (b Block) CalculateSubsidies(height BlockHeight) (Currency, Currency) {
	coinbase := CalculateCoinbase(height)
	devSubsidy := CalculateDevSubsidy(height)
	minerSubsidy := coinbase.Sub(devSubsidy).Add(b.CalculateMinerFees())
	return minerSubsidy, devSubsidy
}
