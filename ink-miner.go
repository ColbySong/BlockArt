package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/gob"
	"encoding/hex"
	"errors"
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
	"./util"
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
	all map[string]*args.Operation
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
type MArtNode struct {
	inkMiner *InkMiner // so artnode can get instance of ink miner
}

var ShapeByShapeHash map[string]*blockartlib.Shape

var (
	errLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	connectedMiners               = ConnectedMiners{all: make([]net.Addr, 0, 0)}
	pendingOperations             = PendingOperations{all: make(map[string]*args.Operation)}
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

	ShapeByShapeHash = make(map[string]*blockartlib.Shape)

	// Start listening for RPC calls from art & miner nodes
	mserver := new(MServer)
	mserver.inkMiner = miner

	mArtNode := new(MArtNode)
	mArtNode.inkMiner = miner

	minerServer := rpc.NewServer()
	minerServer.Register(mserver)
	minerServer.Register(mArtNode)

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

func (s *MServer) DisseminateOperation(op args.Operation, _ignore *bool) error {
	pendingOperations.Lock()

	if _, exists := pendingOperations.all[op.Hash]; !exists {
		// Add operation to pending transaction
		pendingOperations.all[op.Hash] = &args.Operation{op.Op, op.Hash}
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

// Give requesting art node the canvas settings
// Also check if the art node knows your private key
func (a *MArtNode) OpenCanvas(privKey ecdsa.PrivateKey, canvasSettings *blockartlib.CanvasSettings) error {
	if privKey == *a.inkMiner.privKey { //TODO: can use == to compare priv keys?
		*canvasSettings = a.inkMiner.settings.CanvasSettings
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDPRIVKEY]) // TODO: return error if priv keys do not match???
}

func (a *MArtNode) AddShape(shapeRequest blockartlib.AddShapeRequest, newShapeResp *blockartlib.NewShapeResponse) error {
	// check ink level
	// check shape overlap err (same app ok?)
	// check canvas outofbounds err
	inkRemaining := uint32(0) // stub
	svgPath, err := util.ConvertPathToPoints(shapeRequest.SvgString)
	isTransparent := false //use shape.Fill and check transparency
	isClosed := false      // use shape.Stroke and check if closed
	inkRequired := util.CalculateInkRequired(svgPath, isTransparent, isClosed)
	if inkRequired < inkRemaining {
		return errors.New(blockartlib.ErrorName[blockartlib.INSUFFICIENTINK])
	}

	canvasSettings := a.inkMiner.settings.CanvasSettings
	if isOutOfBounds := util.CheckOutOfBounds(svgPath, canvasSettings.CanvasXMax, canvasSettings.CanvasYMax); isOutOfBounds {
		return errors.New(blockartlib.ErrorName[blockartlib.OUTOFBOUNDS])
	}

	// check overlap with other shapes in block

	// TODO: add to pending operations? call to create block??

	// populate NewShapeResponse struct with shapeHash, blockHash and inkRemaining

	return nil
}

func (a *MArtNode) GetSvgString(shapeHash string, svgString *string) error {
	// TODO: rather than keep record of shape by shapeHash, go thru entire blockchain
	// to get svgstring by shapehash in op
	if shape, ok := ShapeByShapeHash[shapeHash]; ok {
		*svgString = shape.SvgString
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDSHAPEHASH])
}

func (a *MArtNode) GetInk(ignoredreq bool, inkRemaining *uint32) error {
	// TODO: inkRemaining, an attribute in InkMiner struct? or get info
	// by looking thru entire blockchain..
	return nil
}

func (a *MArtNode) DeleteShape(deleteShapeReq blockartlib.DeleteShapeReq, inkRemaining *uint8) error {
	// wait until delete is confirmed before updating ink remaining
	// TODO: traverse blockchain and look for operation with incoming shapeHash
	// and check if is owner (matching pub key)
	// then wait for validationNum to be fulfilled

	//return ShapeOwnerErr
	return nil
}

func (a *MArtNode) GetShapes(blockHash string, shapeHashes []string) error {
	// TODO: Can each key (blockhash) have more than 1 blocks??
	if block, ok := a.inkMiner.blockChain.Blocks[blockHash]; ok {

		// iterate through all operations stored on the block which should be shapehashes?
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDBLOCKHASH])
}

func (a *MArtNode) GetGenesisBlock(ignoredreq bool, blockHash *string) error {
	*blockHash = a.inkMiner.settings.GenesisBlockHash
	return nil
}

func (a *MArtNode) GetChildren(blockHash string, blockHashes []string) error {
	// TODO: traverse blockchain to find corresponding block and return it's children
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDBLOCKHASH])
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
