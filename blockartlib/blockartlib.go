/*

This package specifies the application's interface to the the BlockArt
library (blockartlib) to be used in project 1 of UBC CS 416 2017W2.

*/

package blockartlib

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strings"

	"../util"
)

var (
	errLog *log.Logger = log.New(os.Stderr, "[blockartlib] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
	outLog *log.Logger = log.New(os.Stderr, "[blockartlib] ", log.Lshortfile|log.LUTC|log.Lmicroseconds)
)

// Represents a type of shape in the BlockArt system.
type ShapeType int

const (
	// Path shape.
	PATH ShapeType = iota

	// Circle shape (extra credit).
	// CIRCLE
)

type CanvasStruct struct {
	MinerRPC  *rpc.Client
	MinerAddr string
}

// Settings for a canvas in BlockArt.
type CanvasSettings struct {
	// Canvas dimensions
	CanvasXMax uint32
	CanvasYMax uint32
}

// Settings for an instance of the BlockArt project/network.
type MinerNetSettings struct {
	// Hash of the very first (empty) block in the chain.
	GenesisBlockHash string

	// The minimum number of ink miners that an ink miner should be
	// connected to. If the ink miner dips below this number, then
	// they have to retrieve more nodes from the server using
	// GetNodes().
	MinNumMinerConnections uint8

	// Mining ink reward per op and no-op blocks (>= 1)
	InkPerOpBlock   uint32
	InkPerNoOpBlock uint32

	// Number of milliseconds between heartbeat messages to the server.
	HeartBeat uint32

	// Proof of work difficulty: number of zeroes in prefix (>=0)
	PoWDifficultyOpBlock   uint8
	PoWDifficultyNoOpBlock uint8

	// Canvas settings
	CanvasSettings CanvasSettings
}

////////////////////////////////////////////////////////////////////////////////////////////
// <ERROR DEFINITIONS>

// These type definitions allow the application to explicitly check
// for the kind of error that occurred. Each API call below lists the
// errors that it is allowed to raise.
//
// Also see:
// https://blog.golang.org/error-handling-and-go
// https://blog.golang.org/errors-are-values

// Contains address IP:port that art node cannot connect to.
type DisconnectedError string

func (e DisconnectedError) Error() string {
	return fmt.Sprintf("BlockArt: cannot connect to [%s]", string(e))
}

// Contains amount of ink remaining.
type InsufficientInkError uint32

func (e InsufficientInkError) Error() string {
	return fmt.Sprintf("BlockArt: Not enough ink to addShape [%d]", uint32(e))
}

// Contains the offending svg string.
type InvalidShapeSvgStringError string

func (e InvalidShapeSvgStringError) Error() string {
	return fmt.Sprintf("BlockArt: Bad shape svg string [%s]", string(e))
}

// Contains the offending svg string.
type ShapeSvgStringTooLongError string

func (e ShapeSvgStringTooLongError) Error() string {
	return fmt.Sprintf("BlockArt: Shape svg string too long [%s]", string(e))
}

// Contains the bad shape hash string.
type InvalidShapeHashError string

func (e InvalidShapeHashError) Error() string {
	return fmt.Sprintf("BlockArt: Invalid shape hash [%s]", string(e))
}

// Contains the bad shape hash string.
type ShapeOwnerError string

func (e ShapeOwnerError) Error() string {
	return fmt.Sprintf("BlockArt: Shape owned by someone else [%s]", string(e))
}

// Empty
type OutOfBoundsError struct{}

func (e OutOfBoundsError) Error() string {
	return fmt.Sprintf("BlockArt: Shape is outside the bounds of the canvas")
}

// Contains the hash of the shape that this shape overlaps with.
type ShapeOverlapError string

func (e ShapeOverlapError) Error() string {
	return fmt.Sprintf("BlockArt: Shape overlaps with a previously added shape [%s]", string(e))
}

// Contains the invalid block hash.
type InvalidBlockHashError string

func (e InvalidBlockHashError) Error() string {
	return fmt.Sprintf("BlockArt: Invalid block hash [%s]", string(e))

}

// </ERROR DEFINITIONS>
type InvalidPrivKey struct{}

func (InvalidPrivKey) Error() string {
	return fmt.Sprintf("BlockArt: The given private key does not match the private key at the miner address given")
}

// CUSTOM ERROR DEFINITIONS
type ErrorEnum int

const (
	INSUFFICIENTINK ErrorEnum = iota
	INVALIDSHAPEHASH
	INVALIDPRIVKEY
	INVALIDBLOCKHASH
	SHAPEOWNER
	MISC
)

var ErrorName = []string{
	INSUFFICIENTINK:  "INSUFFICIENTINK",
	INVALIDSHAPEHASH: "INVALIDSHAPEHASH",
	INVALIDPRIVKEY:   "INVALIDPRIVKEY",
	INVALIDBLOCKHASH: "INVALIDBLOCKHASH",
	SHAPEOWNER:       "SHAPEOWNER",
	MISC: "MISCERROR:",
}

////////////////////////////////////////////////////////////////////////////////////////////

// Represents a canvas in the system.
type Canvas interface {
	// Adds a new shape to the canvas.
	// Can return the following errors:
	// - DisconnectedError
	// - InsufficientInkError
	// - InvalidShapeSvgStringError
	// - ShapeSvgStringTooLongError
	// - ShapeOverlapError
	// - OutOfBoundsError
	AddShape(validateNum uint8, shapeType ShapeType, shapeSvgString string, fill string, stroke string) (shapeHash string, blockHash string, inkRemaining uint32, err error)

	// Returns the encoding of the shape as an svg string.
	// Can return the following errors:
	// - DisconnectedError
	// - InvalidShapeHashError
	GetSvgString(shapeHash string) (svgString string, err error)

	// Returns the amount of ink currently available.
	// Can return the following errors:
	// - DisconnectedError
	GetInk() (inkRemaining uint32, err error)

	// Removes a shape from the canvas.
	// Can return the following errors:
	// - DisconnectedError
	// - ShapeOwnerError
	DeleteShape(validateNum uint8, shapeHash string) (inkRemaining uint32, err error)

	// Retrieves hashes contained by a specific block.
	// Can return the following errors:
	// - DisconnectedError
	// - InvalidBlockHashError
	GetShapes(blockHash string) (shapeHashes []string, err error)

	// Returns the block hash of the genesis block.
	// Can return the following errors:
	// - DisconnectedError
	GetGenesisBlock() (blockHash string, err error)

	// Retrieves the children blocks of the block identified by blockHash.
	// Can return the following errors:
	// - DisconnectedError
	// - InvalidBlockHashError
	GetChildren(blockHash string) (blockHashes []string, err error)

	// Closes the canvas/connection to the BlockArt network.
	// - DisconnectedError
	CloseCanvas() (inkRemaining uint32, err error)
}

type NewShapeResponse struct {
	ShapeHash    string
	BlockHash    string
	InkRemaining uint32
}

type DeleteShapeReq struct {
	ValidateNum uint8
	ShapeHash   string
}

type AddShapeRequest struct {
	ValidateNum   uint8
	ShapeType     ShapeType
	SvgString     string
	Fill          string
	Stroke        string
	IsTransparent bool
	IsClosed      bool
}

func (c CanvasStruct) AddShape(validateNum uint8, shapeType ShapeType, shapeSvgString string, fill string, stroke string) (shapeHash string, blockHash string, inkRemaining uint32, err error) {
	if shapeType != PATH {
		return "", "", 0, InvalidShapeSvgStringError("Only PATH shape is supported")
	}

	// ValidateShapeSVGString will return the right type of error
	if _, err := util.ValidateShapeSVGString(shapeSvgString); err != nil {
		switch errorStr := err.Error(); errorStr {
		case util.ShapeErrorName[util.INVALIDSHAPESVGSTRING]:
			return "", "", 0, InvalidShapeSvgStringError(shapeSvgString)
		case util.ShapeErrorName[util.SHAPESVGSTRINGTOOLONG]:
			return "", "", 0, ShapeSvgStringTooLongError(shapeSvgString)
		default:
			return "", "", 0, err
		}
	}

	isTransparent := false
	isClosed := false

	if fill == "transparent" {
		isTransparent = true
	}

	lastSVGChar := string(shapeSvgString[len(shapeSvgString)-1])

	if lastSVGChar == "Z" || lastSVGChar == "z" {
		isClosed = true
	}

	addShapeRequest := AddShapeRequest{
		ValidateNum:   validateNum,
		ShapeType:     shapeType,
		SvgString:     shapeSvgString,
		Fill:          fill,
		Stroke:        stroke,
		IsTransparent: isTransparent,
		IsClosed:      isClosed,
	}

	resp := NewShapeResponse{}
	if err = c.MinerRPC.Call("MArtNode.AddShape", addShapeRequest, &resp); err != nil {
		errorStr := err.Error()
		if strings.HasPrefix(errorStr, ErrorName[MISC]) {
			return "", "", 0, err
		}
		switch errorStr {
		case ErrorName[INSUFFICIENTINK]:
			return "", "", 0, InsufficientInkError(resp.InkRemaining)
		case util.ShapeErrorName[util.SHAPEOVERLAP]:
			//The returning shape hash is the overlapping one
			return "", "", 0, ShapeOverlapError(resp.ShapeHash)
		case util.ShapeErrorName[util.OUTOFBOUNDS]:
			return "", "", 0, OutOfBoundsError(OutOfBoundsError{})
		default:
			return "", "", 0, DisconnectedError(c.MinerAddr)
		}
	}
	return resp.ShapeHash, resp.BlockHash, resp.InkRemaining, nil
}

func (c CanvasStruct) GetSvgString(shapeHash string) (svgString string, err error) {
	err = c.MinerRPC.Call("MArtNode.GetSvgString", shapeHash, &svgString)

	if err != nil {
		if strings.EqualFold(err.Error(), ErrorName[INVALIDSHAPEHASH]) {
			return "", InvalidShapeHashError(shapeHash)
		}
	}
	
	return svgString, InvalidShapeHashError(shapeHash)
}

func (c CanvasStruct) GetInk() (inkRemaining uint32, err error) {
	var ignoredreq = true
	err = c.MinerRPC.Call("MArtNode.GetInk", ignoredreq, &inkRemaining)
	if err != nil {
		return 0, DisconnectedError(c.MinerAddr)
	}
	return inkRemaining, nil
}

func (c CanvasStruct) DeleteShape(validateNum uint8, shapeHash string) (inkRemaining uint32, err error) {
	req := DeleteShapeReq{
		ValidateNum: validateNum,
		ShapeHash:   shapeHash}
	err = c.MinerRPC.Call("MArtNode.DeleteShape", req, &inkRemaining)
	if err != nil {
		errorStr := err.Error()
		if strings.HasPrefix(errorStr, ErrorName[MISC]) {
			return 0, err
		}
		if strings.EqualFold(errorStr, ErrorName[SHAPEOWNER]) {
			return 0, ShapeOwnerError(shapeHash)
		}
		return 0, DisconnectedError(c.MinerAddr)
	}
	return inkRemaining, nil
}

func (c CanvasStruct) GetShapes(blockHash string) (shapeHashes []string, err error) {
	err = c.MinerRPC.Call("MArtNode.GetShapes", blockHash, &shapeHashes)
	if err != nil {
		if strings.EqualFold(err.Error(), ErrorName[INVALIDBLOCKHASH]) {
			return []string{""}, InvalidBlockHashError(blockHash)
		}
		return []string{""}, DisconnectedError(c.MinerAddr)
	}
	return shapeHashes, nil
}

func (c CanvasStruct) GetGenesisBlock() (blockHash string, err error) {
	var ignoredreq = true
	err = c.MinerRPC.Call("MArtNode.GetGenesisBlock", ignoredreq, &blockHash)
	if err != nil {
		return "", DisconnectedError(c.MinerAddr)
	}
	return blockHash, nil
}

func (c CanvasStruct) GetChildren(blockHash string) (blockHashes []string, err error) {
	err = c.MinerRPC.Call("MArtNode.GetChildren", blockHash, &blockHashes)
	if err != nil {
		if strings.EqualFold(err.Error(), ErrorName[INVALIDBLOCKHASH]) {
			return []string{""}, InvalidBlockHashError(blockHash)
		}
		return []string{""}, DisconnectedError(c.MinerAddr)
	}
	return blockHashes, nil
}

func (c CanvasStruct) CloseCanvas() (inkRemaining uint32, err error) {
	var ignoredreq = true
	var resp uint32
	err = c.MinerRPC.Call("MArtNode.GetInk", ignoredreq, &resp)
	if err = c.MinerRPC.Close(); err != nil {
		return 0, DisconnectedError(c.MinerAddr)
	}
	return resp, nil
}

// The constructor for a new Canvas object instance. Takes the miner's
// IP:port address string and a public-private key pair (ecdsa private
// key type contains the public key). Returns a Canvas instance that
// can be used for all future interactions with blockartlib.
//
// The returned Canvas instance is a singleton: an application is
// expected to interact with just one Canvas instance at a time.
//
// Can return the following errors:
// - DisconnectedError
func OpenCanvas(minerAddr string, privKey ecdsa.PrivateKey) (canvas Canvas, setting CanvasSettings, err error) {
	gob.Register(&net.TCPAddr{})
	gob.Register(&elliptic.CurveParams{})

	minerRPC, err := rpc.Dial("tcp", minerAddr)
	handleError("Could not make RPC connection to miner", err)

	canvasSettings := CanvasSettings{}
	err = minerRPC.Call("MArtNode.OpenCanvas", privKey, &canvasSettings)
	if err != nil {
		if strings.EqualFold(err.Error(), ErrorName[INVALIDPRIVKEY]) {
			return nil, canvasSettings, InvalidPrivKey(InvalidPrivKey{})
		}
		return nil, canvasSettings, DisconnectedError(minerAddr)
	}
	canvasStruct := CanvasStruct{MinerRPC: minerRPC, MinerAddr: minerAddr}

	return canvasStruct, canvasSettings, nil
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
