package main

import (
	"flag"
	"fmt"
	"os"
	"log"
	"net/rpc"
	"net"
	"time"
	"crypto/ecdsa"
	"encoding/gob"
	"crypto/elliptic"
	"encoding/hex"
	"crypto/x509"
	"./args"
	"./blockartlib"
)

const HeartbeatMultiplier = 2

type InkMiner struct {
	addr net.Addr
	server *rpc.Client
	pubKey *ecdsa.PublicKey
	privKey *ecdsa.PrivateKey
	settings *blockartlib.MinerNetSettings
}

var (
	errLog *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	//outLog *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
)

// Start the miner.
func main() {
	gob.Register(&net.TCPAddr{})
	gob.Register(&elliptic.CurveParams{})


	// Command line input parsing
	flag.Parse()
	if len(flag.Args()) != 3 {
		fmt.Fprintln(os.Stderr, "./server [server ip:port] [pubKey] [privKey]")
		os.Exit(1)
	}
	serverAddr := flag.Arg(0)
	//pubKey := flag.Arg(1) // do we even need this? follow @367 on piazza
	privKey := flag.Arg(2)


	// Decode keys from strings
	privKeyBytesRestored, _ := hex.DecodeString(privKey)
	priv, err := x509.ParseECPrivateKey(privKeyBytesRestored)
	handleError("Couldn't parse private key", err)
	pub := priv.PublicKey


	// Establish RPC channel to server
	server, err := rpc.Dial("tcp", serverAddr)
	handleError("Could not dial server", err)
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	handleError("Could not resolve miner address", err)


	// Create InkMiner instance
	// todo - replace with TCP listener addr?
	miner := &InkMiner{addr, server, &pub, priv, nil}
	settings := miner.register()
	miner.settings = &settings

	go miner.sendHeartBeats()

	// todo - start listening for RPC calls from art & miner nodes
}


// Registers the miner node on the server by making an RPC call.
// Returns the miner network settings retrieved from the server.
func (m InkMiner) register() blockartlib.MinerNetSettings {
	req := args.MinerInfo{
		Address: m.addr,
		Key:     *m.pubKey,
	}
	var resp blockartlib.MinerNetSettings
	err := m.server.Call("RServer.Register", req, &resp)
	handleError("Could not register miner", err)
	return resp
}


// Periodically send heartbeats to the server at period defined by server times a frequency multiplier
func (m InkMiner) sendHeartBeats() {
	for {
		m.sendHeartBeat()
		time.Sleep(time.Duration(m.settings.HeartBeat) / HeartbeatMultiplier * time.Millisecond)
	}
}

// Send a single heartbeat to the server
func (m InkMiner) sendHeartBeat() {
	var ignoredResp bool // there is no response for this RPC call
	err := m.server.Call("RServer.HeartBeat", *m.pubKey, &ignoredResp)
	handleError("Could not send heartbeat to server", err)
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
