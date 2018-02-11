/*

This package specifies the application's interface to the the BlockArt
library (blockartlib) to be used in project 1 of UBC CS 416 2017W2.

*/

package blockartlib

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"strings"
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
	INVALIDSHAPESVGSTRING
	SHAPESVGSTRINGTOOLONG
	SHAPEOVERLAP
	OUTOFBOUNDS
	INVALIDPRIVKEY
)

var errorName = []string{
	INSUFFICIENTINK:       "INSUFFICIENTINK",
	INVALIDSHAPESVGSTRING: "INVALIDSHAPESVGSTRING",
	SHAPESVGSTRINGTOOLONG: "SHAPESVGSTRINGTOOLONG",
	SHAPEOVERLAP:          "SHAPEOVERLAP",
	OUTOFBOUNDS:           "OUTOFBOUNDS",
	INVALIDPRIVKEY:        "INVALIDPRIVKEY",
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
	blockHash    string
	inkRemaining uint32
}

type Shape struct {
	ShapeType ShapeType
	SvgString string
	Fill      string
	Stroke    string
}

func (c CanvasStruct) AddShape(validateNum uint8, shapeType ShapeType, shapeSvgString string, fill string, stroke string) (shapeHash string, blockHash string, inkRemaining uint32, err error) {

	shape := Shape{
		ShapeType: shapeType,
		SvgString: shapeSvgString,
		Fill:      fill,
		Stroke:    stroke}

	resp := NewShapeResponse{}
	c.MinerRPC.Call("MArtNode.AddShape", shape, &resp)

	// - DisconnectedError
	// - InsufficientInkError
	// - InvalidShapeSvgStringError
	// - ShapeSvgStringTooLongError
	// - ShapeOverlapError
	// - OutOfBoundsError

	return resp.ShapeHash, resp.blockHash, resp.inkRemaining, nil
}

func (c CanvasStruct) GetSvgString(shapeHash string) (svgString string, err error) {

	//TODO: miner side: implement map[shapeHash]Shape where Shape has shapeType, svgstring, fill, stroke
	return "", InvalidShapeHashError(shapeHash)
}

func (c CanvasStruct) GetInk() (inkRemaining uint32, err error) {
	var ignoredreq = true
	var ink uint32
	err = c.MinerRPC.Call("MArtNode.GetInk", ignoredreq, &ink)
	if err != nil {
		return 0, DisconnectedError(c.MinerAddr)
	}
	return ink, nil
}

func (c CanvasStruct) DeleteShape(validateNum uint8, shapeHash string) (inkRemaining uint32, err error) {

	return 1, nil
}

func (c CanvasStruct) GetShapes(blockHash string) (shapeHashes []string, err error) {

	return []string{""}, nil
}

func (c CanvasStruct) GetGenesisBlock() (blockHash string, err error) {

	return "", nil
}

func (c CanvasStruct) GetChildren(blockHash string) (blockHashes []string, err error) {

	return []string{""}, nil
}

func (c CanvasStruct) CloseCanvas() (inkRemaining uint32, err error) {
	//TODO: so far, can't see what info miner needs to know about artnode upon disconnect
	var ignoredreq = true
	var resp uint32
	err = c.MinerRPC.Call("MArtNode.ArtNodeDisconnecting", ignoredreq, &resp)
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

	minerRPC, err := rpc.Dial("tcp", minerAddr)

	canvasSettings := CanvasSettings{}
	err = minerRPC.Call("MArtNode.registerArtNode", privKey, canvasSettings)
	handleError("Could not connect to miner", err)

	canvasStruct := CanvasStruct{MinerRPC: minerRPC, MinerAddr: minerAddr}
	if err != nil {
		if strings.EqualFold(err.Error(), errorName[INVALIDPRIVKEY]) {
			return nil, canvasSettings, InvalidPrivKey(InvalidPrivKey{})
		}
		return nil, canvasSettings, DisconnectedError(minerAddr)
	}

	return canvasStruct, canvasSettings, nil
}

func handleError(msg string, e error) {
	if e != nil {
		errLog.Fatalf("%s, err = %s\n", msg, e.Error())
	}
}
