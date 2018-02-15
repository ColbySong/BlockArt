package blockchain

import (
	"crypto/ecdsa"
	"sync"
	"math/big"
)

type Block struct {
	BlockNum    uint32
	PrevHash    string // MD5 hash of [prevHash, opSig, minerPubKey, nonce]
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
	Blocks     map[string]*Block // Map of block hashes to blocks
	NewestHash string            // The tip of the longest branch TODO - confusing name?
}

func (b BlockChain) Size() int {
	return len(b.Blocks)
}

// Return the length of the longest chain.
// Assumes that NewestHash points at the tip of the longest chain.
func (b BlockChain) Len() int {
	chainLen := 0
	nextHash := b.NewestHash
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

func (b BlockChain) Lock() {
	//b.mutex.Lock()
}

func (b BlockChain) Unlock() {
	//b.mutex.Unlock()
}

func (b BlockChain) RLock() {
	//b.mutex.RLock()
}

func (b BlockChain) RUnlock() {
	//b.mutex.RUnlock()
}