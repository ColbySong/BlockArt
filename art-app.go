/*

A trivial application to illustrate how the blockartlib library can be
used from an application in project 1 for UBC CS 416 2017W2.

Usage:
go run art-app.go
*/

package main

// Expects blockartlib.go to be in the ./blockartlib/ dir, relative to
// this art-app.go file
import "./blockartlib"

import "fmt"
import "os"
import (
	"crypto/ecdsa"
	"encoding/hex"
	"crypto/x509"
)

func main() {
	minerAddr := "127.0.0.1:35374"
	privKey := "3081a40201010430aeb7b244cf5ee8a952ff378a140275a0d7f98a7c44faca12357867c667b860fa2aaf7bf9039d3b481479bf0fd512097fa00706052b81040022a1640362000449e30da789d5b12a9487a96d70d69b6b8cbd6821d7a647f35c18a8d5f0969054ae3130e7a2a813363eb578747bc77048b700badea328df20ce68a58fcd0e4166f538f9393e0b4072d069cc4cc631271660dc5ebebb20531f11eeb4bd5aa6a5ca"
	privKeyParsed, _ := ParsePrivateKey(privKey)
	// Open a canvas.
	canvas, settings, err := blockartlib.OpenCanvas(minerAddr, privKeyParsed)
	fmt.Printf("Canvas X max: %d, Canvas Y max: %d\n", settings.CanvasXMax, settings.CanvasYMax)
	if checkError(err) != nil {
		return
	}

        validateNum := uint8(3)

	// Add a line.
	shapeHash, blockHash, ink, err := canvas.AddShape(validateNum, blockartlib.PATH, "M 0 0 L 0 5", "transparent", "red")
	fmt.Println("ShapeHash: ", shapeHash)
	fmt.Println("Blockhash: ", blockHash)
	fmt.Println("Ink after drawing M 0 0 L 0 5: ", ink)
	if checkError(err) != nil {
		return
	}

	// Add another line.
	shapeHash2, blockHash2, ink2, err := canvas.AddShape(validateNum, blockartlib.PATH, "M 0 0 L 10 0", "transparent", "blue")
	fmt.Println("ShapeHash2: ", shapeHash2)
	fmt.Println("Blockhash2: ", blockHash2)
	fmt.Println("Ink 2 after drawing M 0 0 L 10 0: ", ink2)
	if checkError(err) != nil {
		return
	}

	// Delete the first line.
	ink3, err := canvas.DeleteShape(validateNum, shapeHash)
	fmt.Println("Ink 3 after deleting M 0 0 L 0 5: ", ink3)
	if checkError(err) != nil {
		return
	}

	svgString, err := canvas.GetSvgString(shapeHash)
	svgString2, err := canvas.GetSvgString(shapeHash2)
	fmt.Println("First svg string: ", svgString)
	fmt.Println("First svg string 2: ", svgString2)

	// assert ink3 > ink2

	// Get genesis block hash
	genesisBlockHash, err := canvas.GetGenesisBlock()
	if checkError(err) != nil {
		return
	}

	// Build block chain
	err = buildBlockChain(canvas, genesisBlockHash)
	if checkError(err) != nil {
		return
	}

	for _, block := range blockchain {
		fmt.Println(block)
	}

	//// Retrieve shapes on longest chain
	//allShapes := getShapesOnLongestChain(canvas, genesisBlockHash)
	//fmt.Println(allShapes)

	// Close the canvas.
	ink4, err := canvas.CloseCanvas()
	fmt.Println("Final ink after drawing 2 and deleting one: ", ink4)
	if checkError(err) != nil {
		return
	}
}

// If error is non-nil, print it out and return it.
func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error ", err.Error())
		return err
	}
	return nil
}

func ParsePrivateKey(privKey string) (priv ecdsa.PrivateKey, pub ecdsa.PublicKey) {
	privKeyBytesRestored, _ := hex.DecodeString(privKey)
	privPointer, err := x509.ParseECPrivateKey(privKeyBytesRestored)
	checkError(err)
	pub = priv.PublicKey
	return *privPointer, pub
}

type block struct {
	blockHash string
	blockNum  int
	prevHash  string
}

var blockchain = make(map[string]*block)

func buildBlockChain(canvas blockartlib.Canvas, genesisBlockHash string) error {
	blockchain = make(map[string]*block)

	blockchain[genesisBlockHash] = &block{genesisBlockHash, 0, ""}

	err := buildBlockChainHelper(canvas, genesisBlockHash, 1)
	if checkError(err) != nil {
		return err
	}

	return nil
}

func buildBlockChainHelper(canvas blockartlib.Canvas, blockHash string, blockNum int) error {
	children, err := canvas.GetChildren(blockHash)
	if checkError(err) != nil {
		return err
	}

	for _, child := range children {
		blockchain[child] = &block{child, blockNum, blockHash}
		err = buildBlockChainHelper(canvas, child, blockNum+1)
		if checkError(err) != nil {
			return err
		}
	}

	return nil
}

func getShapesOnLongestChain(canvas blockartlib.Canvas, genesisBlockHash string) []string {
	maxBlockNum := 0
	newestBlockHash := genesisBlockHash
	for _, block := range blockchain {
		if block.blockNum > maxBlockNum {
			maxBlockNum = block.blockNum
			newestBlockHash = block.blockHash
		}
	}

	allShapes := make([]string, 0, 0)
	for blockHash := newestBlockHash; blockHash != genesisBlockHash; blockHash = blockchain[blockHash].prevHash {
		shapes, _ := canvas.GetShapes(blockHash)
		for _, shape := range shapes {
			allShapes = append(allShapes, shape)
		}
	}

	return allShapes
}