package main

// Expects blockartlib.go to be in the ./blockartlib/ dir, relative to
// this art-app.go file
import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"

	"../blockartlib"
	"../util"
)

func main() {
	privKeyToParse := util.GetMinerPrivateKey()
	minerAddr := util.GetMinerAddr()
	fmt.Printf("Miner Private Key: %s\nMiner Address: %s\n", privKeyToParse, minerAddr)
	privKey, _ := ParsePrivateKey(privKeyToParse)
	validateNum := uint8(2)

	// Open a canvas.
	canvas, _, err := blockartlib.OpenCanvas(minerAddr, privKey)
	if checkError(err) != nil {
		return
	}

	// Get current ink (after NoOp blocks mined)
	ink, err := canvas.GetInk()
	if err != nil {
		return
	}
	fmt.Printf("[ArtApp_1b] CurrentInk: %d\n", ink)

	// AddShape blue filled square
	shape2SvgStr := "M 80 80 L 100 80 L 100 100 L 80 100 Z" //inkReq: 400
	fill2 := "blue"
	stroke2 := "blue"
	fmt.Printf("[ArtApp_1b] AddShape1[Incoming]: svgstr: %s, fill: %s, stroke: %s", shape2SvgStr, fill2, stroke2)
	shapeHash2, blockHash2, ink, err := canvas.AddShape(validateNum, blockartlib.PATH, shape2SvgStr, fill2, stroke2)
	if checkError(err) != nil {
		return
	}
	fmt.Printf("[ArtApp_1b] AddShape1[Return]: shapeHash: %s, blockHash: %s ,ink: %d\n", shapeHash2, blockHash2, ink)

	// Close the canvas.
	_, err = canvas.CloseCanvas()
	if checkError(err) != nil {
		return
	}

	fmt.Println("END")
}

// If error is non-nil, print it out and return it.
func checkError(err error) error {
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ArtApp_1b] Error ", err.Error())
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
