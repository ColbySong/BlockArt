package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/gob"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"

	"crypto/md5"
	"encoding/json"

	"./args"
	"./blockartlib"
	"./blockchain"
)

const HeartbeatMultiplier = 2
const FirstNonce = 0 // the first uint32
const FirstBlockNum = 1

type ConnectedMiners struct {
	sync.RWMutex
	all []net.Addr
}

type PendingOperations struct {
	sync.RWMutex
	all map[string]*blockchain.OpRecord
}

type InkMiner struct {
	addr       net.Addr
	server     *rpc.Client
	pubKey     *ecdsa.PublicKey
	privKey    *ecdsa.PrivateKey
	settings   *blockartlib.MinerNetSettings
	blockChain *blockchain.BlockChain
}

type MServer struct {
	inkMiner *InkMiner // TODO: Not sure if MServer needs to know about InkMiner
}

var (
	errLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	connectedMiners               = ConnectedMiners{all: make([]net.Addr, 0, 0)}
	pendingOperations             = PendingOperations{all: make(map[string]*blockchain.OpRecord)}
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

	inbound, err := net.ListenTCP("tcp", addr)

	bc := &blockchain.BlockChain{
		Blocks: make(map[string]*blockchain.Block),
	}

	// Create InkMiner instance
	miner := &InkMiner{
		addr:       inbound.Addr(),
		server:     server,
		pubKey:     &pub,
		privKey:    priv,
		blockChain: bc,
	}

	settings := miner.register()
	miner.settings = &settings

	miner.blockChain.NewestHash = settings.GenesisBlockHash

	go miner.startSendingHeartbeats()
	go miner.maintainMinerConnections()
	go miner.startMiningBlocks()

	// Start listening for RPC calls from art & miner nodes
	mserver := new(MServer)
	mserver.inkMiner = miner

	minerServer := rpc.NewServer()
	minerServer.Register(mserver)

	handleError("Listen error", err)
	outLog.Printf("Server started. Receiving on %s\n", inbound.Addr().String())

	for {
		conn, _ := inbound.Accept()
		go minerServer.ServeConn(conn)
	}
}

// Keep track of minimum number of miners at all times (MinNumMinerConnections)
func (m InkMiner) maintainMinerConnections() {
	connectedMiners.Lock()
	connectedMiners.all = m.getNodesFromServer()
	connectedMiners.Unlock()

	for {
		connectedMiners.Lock()
		if uint8(len(connectedMiners.all)) < m.settings.MinNumMinerConnections {
			connectedMiners.all = m.getNodesFromServer()
		}
		connectedMiners.Unlock()

		time.Sleep(time.Duration(m.settings.HeartBeat) * time.Millisecond)
	}
}

func (s *MServer) DisseminateOperation(op blockchain.OpRecord, _ignore *bool) error {
	pendingOperations.Lock()

	if _, exists := pendingOperations.all[op.OpSig]; !exists {
		// Add operation to pending transaction
		pendingOperations.all[op.OpSig] = &blockchain.OpRecord{op.Op, op.OpSig, op.AuthorPubKey}
		pendingOperations.Unlock()

		// Send operation to all connected miners
		connectedMiners.Lock()
		for _, minerAddr := range connectedMiners.all {
			miner, err := rpc.Dial("tcp", minerAddr.String())
			handleError("Could not dial miner", err)
			err = miner.Call("MServer.DisseminateOperation", op, nil)
			handleError("Could not call RPC method MServer.DisseminateOperation", err)
		}
		connectedMiners.Unlock()

		return nil
	}

	pendingOperations.Unlock()

	return nil
}

func (m InkMiner) getNodesFromServer() []net.Addr {
	var nodes []net.Addr
	err := m.server.Call("RServer.GetNodes", m.pubKey, &nodes)
	handleError("Could not get nodes from server", err)
	return nodes
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
func (m InkMiner) startSendingHeartbeats() {
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

func (m InkMiner) startMiningBlocks() {
	for {
		block := m.computeBlock()

		hash := computeBlockHash(*block)
		m.blockChain.Blocks[hash] = block
		m.blockChain.NewestHash = hash

		m.broadcastNewBlock(block)
	}
}

// Mine a single block that includes a set of operations.
func (m InkMiner) computeBlock() *blockchain.Block {
	defer pendingOperations.Unlock()

	var nonce uint32 = FirstNonce
	for {
		pendingOperations.Lock()

		var numZeros uint8

		// todo - may also need to lock m.blockChain

		if len(pendingOperations.all) == 0 {
			numZeros = m.settings.PoWDifficultyNoOpBlock
		} else {
			numZeros = m.settings.PoWDifficultyOpBlock
		}

		var nextBlockNum uint32

		if len(m.blockChain.Blocks) == 0 {
			nextBlockNum = FirstBlockNum
		} else {
			nextBlockNum = m.blockChain.Blocks[m.blockChain.NewestHash].BlockNum + 1
		}

		block := &blockchain.Block{
			BlockNum:    nextBlockNum,
			PrevHash:    m.blockChain.NewestHash,
			OpRecords:   pendingOperations.all,
			MinerPubKey: m.pubKey,
			Nonce:       nonce,
		}
		hash := computeBlockHash(*block)

		if verifyTrailingZeros(hash, numZeros) {
			outLog.Printf("Successfully mined a block. Hash: %s with nonce: %d\n", hash, block.Nonce)
			return block
		}

		nonce = nonce + 1

		pendingOperations.Unlock()
	}
}

// Broadcast the newly-mined block to the miner network
func (m InkMiner) broadcastNewBlock(block *blockchain.Block) error {
	// TODO - stub
	// TODO - clear ops that are included in this block, but only if confident that they
	// TODO   will be part of the main chain
	return nil
}

// Compute the MD5 hash of a Block
func computeBlockHash(block blockchain.Block) string {
	bytes, err := json.Marshal(block)
	handleError("Could not marshal block to JSON", err)

	hash := md5.New()
	hash.Write(bytes)
	return hex.EncodeToString(hash.Sum(nil))
}

// Verify that a hash ends with some number of zeros
func verifyTrailingZeros(hash string, numZeros uint8) bool {
	for i := uint8(0); i < numZeros; i++ {
		if hash[31-i] != '0' {
			return false
		}
	}
	return true
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
