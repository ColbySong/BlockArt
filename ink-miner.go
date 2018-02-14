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
	"strings"
	"sync"
	"time"

	"crypto/md5"
	"encoding/json"

	"bytes"
	"crypto/rand"

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

// Broadcast the new operation
func (m InkMiner) broadcastNewOperation(op blockchain.OpRecord, opRecordHash string) error {
	pendingOperations.Lock()
	if _, exists := pendingOperations.all[opRecordHash]; !exists {
		// Add operation to pending transaction
		// TODO : get ink for op
		pendingOperations.all[opRecordHash] = &blockchain.OpRecord{
			Op:           op.Op,
			InkUsed:      op.InkUsed,
			OpSigS:       op.OpSigS,
			OpSigR:       op.OpSigR,
			AuthorPubKey: op.AuthorPubKey,
		}
		pendingOperations.Unlock()

		// Send operation to all connected miners
		sendToAllConnectedMiners("MServer.DisseminateOperation", op, nil)
		return nil
	}
	pendingOperations.Unlock()

	return nil
}

// This method does not acquire lock; To use this function, acquire lock and then call function
func saveBlockToBlockChain(block blockchain.Block) {
	blockHash := ComputeBlockHash(block)

	blockChain.Blocks[blockHash] = &block

	// Update if the block is new tip
	if block.BlockNum > blockChain.Blocks[blockChain.NewestHash].BlockNum {
		blockChain.NewestHash = blockHash
	}

	removeOperationsFromPendingOperations(block.OpRecords)
}

func getBlockChainsFromNeighbours() []*blockchain.BlockChain {
	var bcs []*blockchain.BlockChain

	connectedMiners.Lock()
	for _, minerAddr := range connectedMiners.all {
		miner, err := rpc.Dial("tcp", minerAddr.String())
		handleError("Could not dial miner: "+minerAddr.String(), err)

		var resp blockchain.BlockChain
		err = miner.Call("MServer.GetBlockChain", nil, &resp)
		handleError("Could not call RPC method: MServer.GetBlockChain", err)

		bcs = append(bcs, &resp)
	}
	connectedMiners.Unlock()

	return bcs
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

		hash := ComputeBlockHash(*block)
		blockChain.Blocks[hash] = block
		blockChain.NewestHash = hash

		broadcastNewBlock(*block)

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
		hash := ComputeBlockHash(*block)

		if verifyTrailingZeros(hash, numZeros) {
			outLog.Printf("Successfully mined a block. Hash: %s with nonce: %d\n", hash, block.Nonce)
			return block
		}

		nonce = nonce + 1

		pendingOperations.Unlock()
	}
}

// Broadcast the newly-mined block to the miner network, and clear the operations that were included in it.
func broadcastNewBlock(block blockchain.Block) error {
	removeOperationsFromPendingOperations(block.OpRecords)

	sendToAllConnectedMiners("MServer.DisseminateBlock", block, nil)
	return nil
}

func removeOperationsFromPendingOperations(opRecords map[string]*blockchain.OpRecord) {
	pendingOperations.Lock()
	for opHash := range opRecords {
		delete(pendingOperations.all, opHash)
	}
	pendingOperations.Unlock()
}

// Generic method to send RPC to all peers
func sendToAllConnectedMiners(remoteProcedure string, request interface{}, resp interface{}) {
	connectedMiners.RLock()
	for _, minerAddr := range connectedMiners.all {
		miner, err := rpc.Dial("tcp", minerAddr.String())
		handleError("Could not dial miner: "+minerAddr.String(), err)
		err = miner.Call(remoteProcedure, request, &resp)
		handleError("Could not call RPC method: "+remoteProcedure, err)
	}
	connectedMiners.RUnlock()
}

// Compute the MD5 hash of a Block
func ComputeBlockHash(block blockchain.Block) string {
	bytes, err := json.Marshal(block)
	handleError("Could not marshal block to JSON", err)

	hash := md5.New()
	hash.Write(bytes)
	return hex.EncodeToString(hash.Sum(nil))
}

// Compute the MD5 hash of a OpRecord
func ComputeOpRecordHash(opRecord blockchain.OpRecord) string {
	bytes, err := json.Marshal(opRecord)
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
	inkRemaining := GetInkTraversal(a.inkMiner, a.inkMiner.pubKey)
	if inkRemaining <= 0 {
		return errors.New(blockartlib.ErrorName[blockartlib.INSUFFICIENTINK])
	}
	requestedSVGPath, _ := util.ConvertPathToPoints(shapeRequest.SvgString)
	isTransparent := shapeRequest.IsTransparent
	isClosed := shapeRequest.IsClosed

	// check if shape is in bound
	canvasSettings := a.inkMiner.settings.CanvasSettings
	if util.CheckOutOfBounds(requestedSVGPath, canvasSettings.CanvasXMax, canvasSettings.CanvasYMax) != nil {
		return errors.New(util.ShapeErrorName[util.OUTOFBOUNDS])
	}

	// check if shape overlaps with shapes from OTHER application
	currentSVGStringsOnCanvas := GetShapeTraversal(a.inkMiner, a.inkMiner.pubKey)
	for _, svgPathString := range currentSVGStringsOnCanvas {
		svgPath, _ := util.ConvertPathToPoints(svgPathString)
		if util.CheckOverlap(svgPath, requestedSVGPath) != nil {
			return errors.New(util.ShapeErrorName[util.SHAPEOVERLAP])
		}
	}

	// if shape is inbound and does not overlap, then calculate the ink required
	inkRequired := util.CalculateInkRequired(requestedSVGPath, isTransparent, isClosed)
	if inkRequired < uint32(inkRemaining) {
		return errors.New(blockartlib.ErrorName[blockartlib.INSUFFICIENTINK])
	}

	// create svg path
	shapeSvgPathString := util.ConvertToSvgPathString(shapeRequest.SvgString, shapeRequest.Stroke, shapeRequest.Fill)

	// sign the shape
	r, s, err := ecdsa.Sign(rand.Reader, a.inkMiner.privKey, []byte(shapeSvgPathString))
	handleError("unable to sign shape", err)

	opRecord := blockchain.OpRecord{
		Op:           shapeSvgPathString,
		OpSigS:       s,
		OpSigR:       r,
		InkUsed:      inkRequired,
		AuthorPubKey: *a.inkMiner.pubKey,
	}

	opRecordHash := ComputeOpRecordHash(opRecord)
	a.inkMiner.broadcastNewOperation(opRecord, opRecordHash)

	// wait until return from validateNum validation
	if blockHash, validated := IsValidatedByValidateNum(opRecordHash, shapeRequest.ValidateNum, a.inkMiner.settings.GenesisBlockHash, a.inkMiner.pubKey); validated {
		newShapeResp.ShapeHash = opRecordHash
		newShapeResp.BlockHash = blockHash
		inkRemaining := GetInkTraversal(a.inkMiner, a.inkMiner.pubKey)
		if inkRemaining < 0 {
			return miscErr("AddShape: Shouldn't have negative ink after successful implementation of block")
		}
		newShapeResp.InkRemaining = uint32(inkRemaining)
		return nil
	}
	return miscErr("AddShape was unsuccessful")
}

func (a *MArtNode) GetSvgString(shapeHash string, svgString *string) error {
	outLog.Printf("Reached GetSvgString\n")
	if opRecord, _, exists := GetOpRecordTraversal(shapeHash, a.inkMiner.settings.GenesisBlockHash); exists {
		*svgString = opRecord.Op
		return nil
	}
	return errors.New(blockartlib.ErrorName[blockartlib.INVALIDSHAPEHASH])
}

func (a *MArtNode) GetInk(ignoredreq bool, inkRemaining *uint32) error {
	outLog.Printf("Reached GetInk\n")
	ink := GetInkTraversal(a.inkMiner, a.inkMiner.pubKey)
	if ink < 0 {
		fmt.Printf("Get ink got back negative ink %d", *inkRemaining)
	}
	*inkRemaining = uint32(ink)
	return nil
}

func concatStrings(strArray []string) string {
	var buf bytes.Buffer
	for i := 0; i < len(strArray); i++ {
		buf.WriteString(strArray[i])
	}
	return buf.String()
}

func (a *MArtNode) DeleteShape(deleteShapeReq blockartlib.DeleteShapeReq, inkRemaining *uint32) error {
	outLog.Printf("Reached DeleteShape\n")

	if opRecord, _, exists := GetOpRecordTraversal(deleteShapeReq.ShapeHash, a.inkMiner.settings.GenesisBlockHash); exists {
		if VerifyOpRecordAuthor(*a.inkMiner.pubKey, opRecord) {
			newOp := concatStrings([]string{"delete ", opRecord.Op})

			// sign the shape
			r, s, err := ecdsa.Sign(rand.Reader, a.inkMiner.privKey, []byte(newOp))
			handleError("unable to sign shape", err)

			inkRefunded := opRecord.InkUsed

			newOpRecord := blockchain.OpRecord{
				Op:           newOp,
				InkUsed:      inkRefunded,
				OpSigS:       s,
				OpSigR:       r,
				AuthorPubKey: *a.inkMiner.pubKey,
			}
			opRecordHash := ComputeOpRecordHash(newOpRecord)
			a.inkMiner.broadcastNewOperation(newOpRecord, opRecordHash)

			// wait until return from validateNum validation
			if _, validated := IsValidatedByValidateNum(opRecordHash, deleteShapeReq.ValidateNum, a.inkMiner.settings.GenesisBlockHash, a.inkMiner.pubKey); !validated {
				newInkRemaining := GetInkTraversal(a.inkMiner, a.inkMiner.pubKey)

				if newInkRemaining < 0 {
					return miscErr("DeleteShape: Shouldn't have negative ink after successful implementation of block")
				}
				*inkRemaining = uint32(newInkRemaining)
				return nil
			}
			return miscErr("Delete Shape was unsuccessful")
		}
	}
	return errors.New(blockartlib.ErrorName[blockartlib.SHAPEOWNER])

}

// 1) Wait until op is taken off pending list => this means op has been incorporated into a block
// 2) Find the opRecord in the longest chain (of the artnode's miner),
// 3) and check if it has at least validateNum # of blocks following it
// 4) if it doesn't meet validateNum # of blocks following it yet, periodically repeat steps 2-3
// case 0: if during a check, it does have validateNum # of blocks following it, return the blockHash of the block
//         the op was incorporated in AND return true
// case 1: if during a check, the op is no longer found in the longest chain, then it means it was
//    	   rejected because either the artnode's miner is malicious or was building off the wrong chain to begin with.
//    	   In this case, the op is lost and we return false
func IsValidatedByValidateNum(opRecordHash string, validateNum uint8, genesisBlockHash string, pubKey *ecdsa.PublicKey) (string, bool) {
	//TODO: need to lock when periodically checking blockchain?
	for {
		if _, exists := pendingOperations.all[opRecordHash]; !exists {
			for {
				if opRecord, blockHash, exists := GetOpRecordTraversal(opRecordHash, genesisBlockHash); exists {
					blockNumOfOp := blockChain.Blocks[blockHash].BlockNum
					newestBlockNum := blockChain.Blocks[blockChain.NewestHash].BlockNum
					if newestBlockNum-blockNumOfOp >= uint32(validateNum) {
						if VerifyOpRecordAuthor(*pubKey, opRecord) {
							return blockHash, true
						}
					}
				} else {
					return "", false
				}
				time.Sleep(2 * time.Second) //TODO: what's an optimal time to check?
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", false
}

// Return true if the miner's public key matches author's public key of the OpRecord
// and also decodes the opSigS and opSigR of the opRecord to verify it was signed by the author
// listed in the OpRecord
func VerifyOpRecordAuthor(requestorPublicKey ecdsa.PublicKey, opRecord blockchain.OpRecord) bool {
	return reflect.DeepEqual(requestorPublicKey, opRecord.AuthorPubKey) &&
		ecdsa.Verify(&opRecord.AuthorPubKey, []byte(opRecord.Op), opRecord.OpSigR, opRecord.OpSigS)
}

// given the shapeHash, return true if it is in the longest chain of the blockchain
// if true, also return the opRecord and the corresponding blockHash of the block that the shapeHash is contained in
func GetOpRecordTraversal(shapeHash string, genesisBlockHash string) (blockchain.OpRecord, string, bool) {
	newestHash := blockChain.NewestHash
	for blockHash := newestHash; blockHash != genesisBlockHash; blockHash = blockChain.Blocks[blockHash].PrevHash {
		block := blockChain.Blocks[blockHash]
		if len(block.OpRecords) > 0 {
			if opRecord, exists := block.OpRecords[shapeHash]; exists {
				return *opRecord, blockHash, true
			}
		}
	}
	return blockchain.OpRecord{}, "", false
}

// returns the amount of ink owned by @param pubKey
func GetInkTraversal(inkMiner *InkMiner, pubKey *ecdsa.PublicKey) int {
	inkRemaining := 0
	newestHash := blockChain.NewestHash
	for blockHash := newestHash; blockHash != inkMiner.settings.GenesisBlockHash; blockHash = blockChain.Blocks[blockHash].PrevHash {
		block := blockChain.Blocks[blockHash]
		if len(block.OpRecords) == 0 { // NoOp block
			if reflect.DeepEqual(*block.MinerPubKey, *pubKey) {
				inkRemaining += int(inkMiner.settings.InkPerNoOpBlock)
			}
		} else { // Op Block
			if reflect.DeepEqual(*block.MinerPubKey, *pubKey) {
				inkRemaining += int(inkMiner.settings.InkPerOpBlock)
			}
			for _, opRecord := range block.OpRecords {
				if reflect.DeepEqual(opRecord.AuthorPubKey, *pubKey) {
					if isOpDelete(opRecord.Op) {
						inkRemaining += int(opRecord.InkUsed)
					} else { // Add block
						inkRemaining -= int(opRecord.InkUsed)
					}
				}
			}
		}
	}
	return inkRemaining
}

// returns all the shapes on the canvas EXCEPT the ones drawn by @param pubKey
// strings are in the form of "M 0 0 L 50 50"
func GetShapeTraversal(inkMiner *InkMiner, pubKey *ecdsa.PublicKey) []string {
	newestHash := blockChain.NewestHash
	var shapesDrawnByOtherApps []string
	for blockHash := newestHash; blockHash != inkMiner.settings.GenesisBlockHash; blockHash = blockChain.Blocks[blockHash].PrevHash {
		block := blockChain.Blocks[blockHash]
		if len(block.OpRecords) != 0 {
			shapesDrawnByOtherApps = append(shapesDrawnByOtherApps, getShapesFromOpRecords(block.OpRecords, pubKey)...)
		}
	}

	return shapesDrawnByOtherApps
}

// returns all the shapes in the opRecords EXCEPT the ones drawn by @param pubKey
func getShapesFromOpRecords(opRecords map[string]*blockchain.OpRecord, pubKey *ecdsa.PublicKey) []string {
	var shapesDrawnByOtherApps []string
	var shapesToDelete []string
	for _, opRecord := range opRecords {
		if !reflect.DeepEqual(opRecord.AuthorPubKey, *pubKey) {
			svgPath := parsePath(opRecord.Op)
			if isOpDelete(opRecord.Op) {
				shapesToDelete = append(shapesToDelete, svgPath)
			} else {
				shapesDrawnByOtherApps = append(shapesDrawnByOtherApps, svgPath)
			}
		}
	}

	// remove shapes that was deleted
	shapesDrawnByOtherApps = removeShapesDeleted(shapesDrawnByOtherApps, shapesToDelete)

	return shapesDrawnByOtherApps
}

// Returns all operations in the given blockchain
// Must supply valid corresponding genesisBlockHash
func GetAllOperationsFromBlockChain(bc blockchain.BlockChain, genesisBlockHash string) map[string]*blockchain.OpRecord {
	allOps := make(map[string]*blockchain.OpRecord)
	for blockHash := bc.NewestHash; blockHash != genesisBlockHash; blockHash = bc.Blocks[blockHash].PrevHash {
		blockOpRecords := bc.Blocks[blockHash].OpRecords
		if len(blockOpRecords) != 0 {
			for opHash, op := range blockOpRecords {
				allOps[opHash] = op
			}
		}
	}
	return allOps
}

func (a *MArtNode) GetShapes(blockHash string, shapeHashes *[]string) error {
	outLog.Printf("Reached GetShapes\n")
	// TODO: Can each key (blockhash) have more than 1 blocks??
	blockChain.RLock()
	defer blockChain.RUnlock()

	if block, ok := blockChain.Blocks[blockHash]; ok {
		shapeHashes := make([]string, len(block.OpRecords))
		var i = 0
		for _, v := range block.OpRecords {
			shapeHashes[i] = v.Op
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

// removes all strings in shapesToDelete from allShapes
func removeShapesDeleted(allShapes []string, shapesToDelete []string) []string {
	for i, svgShape := range allShapes {
		for _, shapesToDelete := range shapesToDelete {
			if svgShape == shapesToDelete {
				allShapes = append(allShapes[:i], allShapes[i+1:]...)
			}
		}
	}
	return allShapes
}

func parsePath(shapeSVGString string) string {
	buf := strings.Split(shapeSVGString, "d=\"")
	bufTwo := strings.Split(buf[1], "\" s")
	return bufTwo[0]
}

func isOpDelete(shapeSvgString string) bool {
	buf := strings.Split(shapeSvgString, " ")
	return strings.EqualFold(buf[0], "delete")
}

func miscErr(msg string) error {
	var buf bytes.Buffer
	buf.WriteString(blockartlib.ErrorName[blockartlib.MISC])
	buf.WriteString(" ")
	buf.WriteString(msg)
	return errors.New(buf.String())
}
