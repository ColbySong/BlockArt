package blockchain

import (
	"crypto/ecdsa"
	"sync"
)

type Block struct {
	BlockNum    uint32
	PrevHash    string // MD5 hash of [prevHash, opSig, minerPubKey, nonce]
	OpRecords   map[string]*OpRecord
	MinerPubKey *ecdsa.PublicKey
	Nonce       uint32
}

type OpRecord struct {
	Op           string
	OpSig        string // signed with private key of art node
	InkUsed      uint32
	AuthorPubKey ecdsa.PublicKey
}

type BlockChain struct {
	sync.RWMutex
	Blocks     map[string]*Block // Map of block hashes to blocks
	NewestHash string            // The tip of the longest branch
}
