/*

A trivial application to illustrate how the blockartlib library can be
used from an application in project 1 for UBC CS 416 2017W2.

Usage:
go run art-app.go
*/

package main

// Expects blockartlib.go to be in the ./blockartlib/ dir, relative to
// this art-app.go file
import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"

	"./blockartlib"
)

func main() {
	minerAddr := "127.0.0.1:8080"
	privKeyToParse := "3081a40201010430aeb7b244cf5ee8a952ff378a140275a0d7f98a7c44faca12357867c667b860fa2aaf7bf9039d3b481479bf0fd512097fa00706052b81040022a1640362000449e30da789d5b12a9487a96d70d69b6b8cbd6821d7a647f35c18a8d5f0969054ae3130e7a2a813363eb578747bc77048b700badea328df20ce68a58fcd0e4166f538f9393e0b4072d069cc4cc631271660dc5ebebb20531f11eeb4bd5aa6a5ca" // TODO: use crypto/ecdsa to read pub/priv keys from a file argument.

	privKey, _ := ParsePrivateKey(privKeyToParse)
	// Open a canvas.
	canvas, _, err := blockartlib.OpenCanvas(minerAddr, privKey)
	if checkError(err) != nil {
		return
	}

	// validateNum := uint8(2)

	// // Add a line.
	// shapeHash, blockHash, ink, err := canvas.AddShape(validateNum, blockartlib.PATH, "M 0 0 L 0 5", "transparent", "red")
	// if checkError(err) != nil {
	// 	return
	// }

	// // Add another line.
	// shapeHash2, blockHash2, ink2, err := canvas.AddShape(validateNum, blockartlib.PATH, "M 0 0 L 5 0", "transparent", "blue")
	// if checkError(err) != nil {
	// 	return
	// }

	// // Delete the first line.
	// ink3, err := canvas.DeleteShape(validateNum, shapeHash)
	// if checkError(err) != nil {
	// 	return
	// }

	// assert ink3 > ink2

	// Close the canvas.
	_, err = canvas.CloseCanvas()
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
