package blockchain

import (
	"crypto/ecdsa"
	"sync"
	"math/big"
)

type Block struct {
	BlockNum    uint32
	PrevHash    string // MD5 hash of [prevHash, opSig, minerPubKey, nonce]
	// TODO-dc: [IMPORTANT] none of these fields should be publicly accessible! Causes concurrent read/write problems
	OpRecords   map[string]*OpRecord // key for opRecords is the hash the whole opRecord Struct,
	                                 // it is also the shapeHash that is returned to users
	MinerPubKey *ecdsa.PublicKey
	Nonce       uint32
}

type OpRecord struct {
	Op           string
	InkUsed      uint32
	OpSigS       *big.Int // signed with private key of art node
	OpSigR	     *big.Int // edsca.Sign returns R, S which is both needed to verify
	AuthorPubKey ecdsa.PublicKey
}

type BlockChain struct {
	mutex sync.RWMutex
	// TODO-dc: [IMPORTANT] none of these fields should be publicly accessible! Causes concurrent read/write problems
	Blocks     map[string]*Block // Map of block hashes to blocks
	// TODO - perhaps 'newest' isn't the best term
	newestHash string            // The tip of the longest branch
}

func (b *BlockChain) GetSize() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return len(b.Blocks)
}

// Return the length of the longest chain.
// Assumes that newestHash points at the tip of the longest chain.
func (b *BlockChain) GetLen() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	chainLen := 0
	nextHash := b.newestHash
	for {
		block, exists := b.Blocks[nextHash]
		if !exists {
			return chainLen
		} else {
			chainLen = chainLen + 1
			nextHash = block.PrevHash
		}
	}
}

func (b *BlockChain) AddBlockAndUpdateTip(block *Block, hash string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.Blocks) == 0 || block.BlockNum > b.Blocks[b.newestHash].BlockNum {
		b.newestHash = hash
	}
	b.Blocks[hash] = block
}

// TODO - perhaps 'newest' isn't the best term
func (b *BlockChain) GetNewestBlockNum() uint32 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	newestBlock, exists := b.Blocks[b.newestHash]
	if exists {
		return newestBlock.BlockNum
	} else {
		return 1
	}
}

// TODO - perhaps 'newest' isn't the best term
func (b *BlockChain) GetNewestHash() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.newestHash
}

func (b *BlockChain) GetPrevHash(hash string) string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.Blocks[hash].PrevHash
}

func (b *BlockChain) GetBlockNum(hash string) uint32 {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	block, exists := b.Blocks[hash]
	if exists {
		return block.BlockNum
	} else {
		return 0
	}
}

func (b *BlockChain) GetBlockByHash(hash string) *Block {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.Blocks[hash]
}

func (b *BlockChain) DoesBlockExist(hash string) bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	_, exists := b.Blocks[hash]
	return exists
}

func (b *BlockChain) SetNewestHash(hash string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.newestHash = hash
}
