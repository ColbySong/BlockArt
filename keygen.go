package main

import (
	"fmt"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"

)

// To encode publicKey use:
// publicKeyBytes, _ = x509.MarshalPKIXPublicKey(&private_key.PublicKey)

func main() {
	p384 := elliptic.P384()
	priv1, _ := ecdsa.GenerateKey(p384, rand.Reader)


	privateKeyBytes, _ := x509.MarshalECPrivateKey(priv1)

	encodedBytes := hex.EncodeToString(privateKeyBytes)
	fmt.Printf("Private key: %s\n", encodedBytes)

	publicKeyBytes, _ := x509.MarshalPKIXPublicKey(&priv1.PublicKey)
	encodedPubKeyBytes := hex.EncodeToString(publicKeyBytes)
	fmt.Printf("Public key: %s\n", encodedPubKeyBytes)

	privateKeyBytesRestored, _ := hex.DecodeString(encodedBytes)
	priv2, _ := x509.ParseECPrivateKey(privateKeyBytesRestored)

	data := []byte("data")
	// Signing by priv1
	r, s, _ := ecdsa.Sign(rand.Reader, priv1, data)


	// Verifying against priv2 (restored from priv1)
	if !ecdsa.Verify(&priv2.PublicKey, data, r, s) {
		fmt.Printf("Error")
		return
	}

	fmt.Printf("Key was restored from string successfully")
}