package args

import (
	"crypto/ecdsa"
	"net"
)

// todo - might be able to move this into blockartlib.go - follow piazza @382
type MinerInfo struct {
	Address net.Addr
	Key     ecdsa.PublicKey
}

type Operation struct {
	Op   string
	Hash string
}
