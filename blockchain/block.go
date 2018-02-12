package blockchain

import "crypto/ecdsa"

// todo - include ink level of every miner in the network
// todo - include all shapes on the canvas
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
	AuthorPubKey ecdsa.PublicKey
}

type BlockChain struct {
	Blocks     map[string]*Block // Map of block hashes to blocks
	NewestHash string            // The tip of the longest branch
}
