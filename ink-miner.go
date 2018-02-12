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
	"reflect"
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
	all map[string]*blockchain.OpRecord
}

type InkMiner struct {
	addr     net.Addr
	server   *rpc.Client
	pubKey   *ecdsa.PublicKey
	privKey  *ecdsa.PrivateKey
	settings *blockartlib.MinerNetSettings
}

type MServer struct {
	inkMiner *InkMiner // TODO: Not sure if MServer needs to know about InkMiner
}
type MArtNode struct {
	inkMiner *InkMiner // so artnode can get instance of ink miner
}

var (
	errLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog            *log.Logger = log.New(os.Stderr, "[miner] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	connectedMiners               = ConnectedMiners{all: make([]net.Addr, 0, 0)}
	pendingOperations             = PendingOperations{all: make(map[string]*blockchain.OpRecord)}
	blockChain                    = blockchain.BlockChain{Blocks: make(map[string]*blockchain.Block)}
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

	// Create InkMiner instance
	miner := &InkMiner{
		addr:    inbound.Addr(),
		server:  server,
		pubKey:  &pub,
		privKey: priv,
	}

	settings := miner.register()
	miner.settings = &settings

	blockChain.Lock()
	blockChain.NewestHash = settings.GenesisBlockHash
	blockChain.Unlock()

	go miner.startSendingHeartbeats()
	go miner.maintainMinerConnections()
	go miner.startMiningBlocks()

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

func (s *MServer) DisseminateOperation(op blockchain.OpRecord, _ignore *bool) error {
	pendingOperations.Lock()

	if _, exists := pendingOperations.all[op.OpSig]; !exists {
		// Add operation to pending transaction
		pendingOperations.all[op.OpSig] = &blockchain.OpRecord{op.Op, op.OpSig, op.AuthorPubKey}
		pendingOperations.Unlock()

		// Send operation to all connected miners
		sendToAllConnectedMiners("MServer.DisseminateOperation", op)

		return nil
	}

	pendingOperations.Unlock()

	return nil
}

// Disseminate Block
//
// If block number is greater than the local blockchain's latest block number by 1:
// 1) Validate this block
//		a) Verify all operations within the block are valid
//		b) Verify that it used a valid prevHash
//		c) Verify that the blockhash contains a valid number of zeroes at the end
// 2) Add this block to the blockchain and start build off this newest block
//
// If block number is greater than the local blockchain's latest block number by more than 1:
// 1) Fetch all block numbers between local blockchain's latest block and this block number
// 		a) Verify all operations within the block are valid
//		b) Verify that it used a valid prevHash
//		c) Verify that the blockhash contains a valid number of zeroes at the end
// 2) Validate this block
//		a) Verify all operations within the block are valid
//		b) Verify that it used a valid prevHash
//		c) Verify that the blockhash contains a valid number of zeroes at the end
// 3) Add all fetched blocks and this block to the blockchain and build off this newest block
//
// When to disseminate:
// 1) If the block is valid AND
// 2) If blockHash does not exist in local blockchain AND
// 3) If block number is greater than local blockchain's latest block number
// Otherwise, do not disseminate
func (s *MServer) DisseminateBlock(block blockchain.Block, _ignore *bool) error {
	// TODO: May need to change locking semantics
	blockChain.Lock()
	defer blockChain.Unlock()

	newestHash := blockChain.NewestHash
	newestBlock := blockChain.Blocks[newestHash]

	blockHash := computeBlockHash(block)

	if block.BlockNum == newestBlock.BlockNum+1 {
		// Receieved a block with block number equal to the one miner is currently mining

		// Block's PrevHash matches newest block's PrevHash, add to block chain and disseminate
		if block.PrevHash == newestHash {
			// TODO: Verify block's operations, if not valid, stop and don't disseminate

			blockChain.Blocks[blockHash] = &block
			blockChain.NewestHash = blockHash
			// sendToAllConnectedMiners("MServer.DisseminateBlock", block)
		} else {
			// TODO: What should we do if block.PrevHash != newestHash?
			//       Save it anyway incase it becomes longest, don't work off it, and don't disseminate?
			//       But if the PrevHash doesn't match, then it won't be valid in the miner's local blockchain
		}

		return nil
	} else if block.BlockNum > newestBlock.BlockNum+1 {
		// TODO: Receieved a block with block number greater than the one miner is currently mining
		// TODO: Fetch entire blockchain from neighbours?
	} else {
		// TODO: Received a block with block number less than the one miner is currently mining
		//       Save it anyway,
	}

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
		// Lock entire blockchain while computing hash so that if you receive
		// disseminated blocks from other miners, you don't update the blockchain
		// while computing current hash
		blockChain.Lock()

		block := m.computeBlock()

		hash := computeBlockHash(*block)
		blockChain.Blocks[hash] = block
		blockChain.NewestHash = hash

		m.broadcastNewBlock(block)

		blockChain.Unlock()
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

		if len(blockChain.Blocks) == 0 {
			nextBlockNum = FirstBlockNum
		} else {
			nextBlockNum = blockChain.Blocks[blockChain.NewestHash].BlockNum + 1
		}

		block := &blockchain.Block{
			BlockNum:    nextBlockNum,
			PrevHash:    blockChain.NewestHash,
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
	// sendToAllConnectedMiners("MServer.DisseminateBlock", *block)
	return nil
}

func sendToAllConnectedMiners(remoteProcedure string, payload interface{}) {
	connectedMiners.Lock()
	for _, minerAddr := range connectedMiners.all {
		miner, err := rpc.Dial("tcp", minerAddr.String())
		handleError("Could not dial miner: "+minerAddr.String(), err)
		err = miner.Call(remoteProcedure, payload, nil)
		handleError("Could not call RPC method: "+remoteProcedure, err)
	}
	connectedMiners.Unlock()
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
	outLog.Printf("Reached OpenCanvas")
	if reflect.DeepEqual(privKey, *a.inkMiner.privKey) {
		*canvasSettings = a.inkMiner.settings.CanvasSettings
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDPRIVKEY])
}

func (a *MArtNode) AddShape(shapeRequest blockartlib.AddShapeRequest, newShapeResp *blockartlib.NewShapeResponse) error {
	outLog.Printf("Reached AddShape \n")
	inkRemaining := uint32(0) // TODO: get how much ink the miner has
	requestedSVGPath, _ := util.ConvertPathToPoints(shapeRequest.SvgString)
	isTransparent := shapeRequest.IsTransparent
	isClosed := shapeRequest.IsClosed

	// check if shape is in bound
	canvasSettings := a.inkMiner.settings.CanvasSettings
	if util.CheckOutOfBounds(requestedSVGPath, canvasSettings.CanvasXMax, canvasSettings.CanvasYMax) != nil {
		return errors.New(util.ShapeErrorName[util.OUTOFBOUNDS])
	}

	// check if shape overlaps with shapes from OTHER application
	// TODO: need a way to get all the current shapes on canvas excluding its own shapes
	currentSVGStringsOnCanvas := []string{} // TODO: requires a list of SVG strings that are currently on the canvas in the form of "M 0 10 H 20"
	for _, svgPathString := range currentSVGStringsOnCanvas {
		svgPath, _ := util.ConvertPathToPoints(svgPathString)
		if util.CheckOverlap(svgPath, requestedSVGPath) != nil {
			return errors.New(util.ShapeErrorName[util.SHAPEOVERLAP])
		}
	}

	// if shape is inbound and does not overlap, then calculate the ink required
	inkRequired := util.CalculateInkRequired(requestedSVGPath, isTransparent, isClosed)
	if inkRequired < inkRemaining {
		return errors.New(blockartlib.ErrorName[blockartlib.INSUFFICIENTINK])
	}

	// TODO: add to pending operations? call to create block??
	// populate NewShapeResponse struct with shapeHash, blockHash and inkRemaining
	// TODO: MINER MUST USE VALIDATENUM AND ONLY RETURN THE SHAPEHASH AND BLOCKHASH WHEN VALIDATE NUM IS SATISFIED

	return nil
}

func (a *MArtNode) GetSvgString(shapeHash string, svgString *string) error {
	outLog.Printf("Reached GetSvgString\n")
	// TODO: traverse blockchain to get svgstring by shapehash
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDSHAPEHASH])
}

func (a *MArtNode) GetInk(ignoredreq bool, inkRemaining *uint32) error {
	outLog.Printf("Reached GetInk\n")
	// TODO: inkRemaining, an attribute in InkMiner struct? or get info
	// by looking thru entire blockchain..
	return nil
}

func (a *MArtNode) DeleteShape(deleteShapeReq blockartlib.DeleteShapeReq, inkRemaining *uint8) error {
	outLog.Printf("Reached DeleteShape\n")
	// wait until delete is confirmed before updating ink remaining
	// TODO: traverse blockchain and look for operation with incoming shapeHash
	// and check if is owner (matching pub key)
	// then wait for validationNum to be fulfilled

	//return ShapeOwnerErr
	return nil
}

func (a *MArtNode) GetShapes(blockHash string, shapeHashes *[]string) error {
	outLog.Printf("Reached GetShapes\n")
	// TODO: Can each key (blockhash) have more than 1 blocks??
	blockChain.RLock()
	defer blockChain.RUnlock()

	if block, ok := blockChain.Blocks[blockHash]; ok {
		shapeHashes := make([]string, len(block.OpRecords))
		var i = 0
		for hash := range block.OpRecords {
			shapeHashes[i] = hash
			i++
		}
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDBLOCKHASH])
}

func (a *MArtNode) GetGenesisBlock(ignoredreq bool, blockHash *string) error {
	outLog.Printf("Reached GetGenesisBlock\n")
	*blockHash = a.inkMiner.settings.GenesisBlockHash
	return nil
}

func (a *MArtNode) GetChildren(blockHash string, blockHashes *[]string) error {
	outLog.Printf("Reached GetChildren\n")
	// TODO: traverse blockchain to find corresponding block and return it's children
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDBLOCKHASH])
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
