package main

// Expects blockartlib.go to be in the ./blockartlib/ dir, relative to
// this art-app.go file
import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"

	"flag"

	"../blockartlib"
)

func main() {
	flag.Parse()
	if len(flag.Args()) != 2 {
		fmt.Fprintln(os.Stderr, "ArtApp1a [privKey] [miner ip:port]")
		os.Exit(1)
	}

	privKeyToParse := flag.Arg(0)
	minerAddr := flag.Arg(1)
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
	fmt.Printf("[ArtApp_1a] CurrentInk: %d\n", ink)

	// AddShape blue transparent square
	shape1SvgStr := "M 50 50 L 70 50 L 70 70 L 50 70 Z" //inkReq: 80
	fill1 := "transparent"
	stroke1 := "blue"
	fmt.Printf("[ArtApp_1a] AddShape1[Incoming]: svgstr: %s, fill: %s, stroke: %s\n", shape1SvgStr, fill1, stroke1)
	shapeHash1, blockHash1, ink, err := canvas.AddShape(validateNum, blockartlib.PATH, shape1SvgStr, fill1, stroke1)
	if checkError(err) != nil {
		return
	}
	fmt.Printf("[ArtApp_1a] AddShape1[Return]: shapeHash: %s, blockHash: %s ,ink: %d\n", shapeHash1, blockHash1, ink)

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
		fmt.Fprintf(os.Stderr, "[ArtApp_1a] Error ", err.Error())
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
