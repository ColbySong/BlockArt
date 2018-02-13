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
	OpSig        string // signed with private key of art node
	InkUsed      uint32
	OpSigS       *big.Int // signed with private key of art node
	OpSigR	     *big.Int // edsca.Sign returns R, S which is both needed to verify
	AuthorPubKey ecdsa.PublicKey
}

type BlockChain struct {
	sync.RWMutex
	Blocks     map[string]*Block // Map of block hashes to blocks
	NewestHash string            // The tip of the longest branch
}
