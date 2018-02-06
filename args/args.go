package args

import (
	"net"
	"crypto/ecdsa"
)

// todo - might be able to move this into blockartlib.go - follow piazza @382
type MinerInfo struct {
	Address net.Addr
	Key     ecdsa.PublicKey
}