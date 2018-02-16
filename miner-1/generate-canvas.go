package main

// Expects blockartlib.go to be in the ./blockartlib/ dir, relative to
// this art-app.go file
import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"../blockartlib"
	"../util"
)

type block struct {
	blockHash string
	blockNum  int
	prevHash  string
}

func main() {
	priv := util.GetMinerPrivateKey()
	minerAddr := util.GetMinerAddr()
	fmt.Printf("Miner Private Key: %s\nMiner Address: %s\n", priv, minerAddr)

	privKey, _ := ParsePrivateKey(priv)

	// Open a canvas.
	canvas, canvasSettings, err := blockartlib.OpenCanvas(minerAddr, privKey)
	if checkError(err) != nil {
		return
	}

	// Create ./canvas.html
	err = generateCanvas(canvas, canvasSettings)
	if checkError(err) != nil {
		return
	}

	// Close the canvas.
	_, err = canvas.CloseCanvas()
	if checkError(err) != nil {
		return
	}
}

func generateCanvas(canvas blockartlib.Canvas, canvasSettings blockartlib.CanvasSettings) error {
	genesisBlockHash, err := canvas.GetGenesisBlock()
	if err != nil {
		return err
	}

	blockchain, err := buildBlockChain(canvas, genesisBlockHash)
	if err != nil {
		return err
	}

	// printBlockChain(blockchain)

	canvasShapes, err := getCanvasShapes(canvas, genesisBlockHash, blockchain)
	if err != nil {
		return err
	}

	err = createCanvasHtmlFile(canvasSettings.CanvasXMax, canvasSettings.CanvasYMax, canvasShapes)
	if err != nil {
		return err
	}

	return nil
}

func printBlockChain(blockchain map[string]*block) {
	for _, block := range blockchain {
		fmt.Println(block)
	}
}

func getCanvasShapes(canvas blockartlib.Canvas, genesisBlockHash string, blockchain map[string]*block) ([]string, error) {
	allShapes, err := getAllShapesOnLongestChain(canvas, genesisBlockHash, blockchain)
	if err != nil {
		return nil, err
	}

	// Get all deletedShapes and the number of times they appear
	deletedShapes := make(map[string]int)
	for i, shape := range allShapes {
		if strings.HasPrefix(shape, "delete ") {
			// If operation is a delete, add it to deletedShapes and update the count
			split := strings.SplitN(shape, " ", 2)
			deletedShapes[split[1]] = deletedShapes[split[1]] + 1

			// Remove delete shape operation from allShapes
			allShapes = append(allShapes[:i], allShapes[i+1:]...)
		}
	}

	// Delete deletedShapes from allShapes
	for deletedShape, count := range deletedShapes {
		for c := 0; c < count; c++ {
			for i, shapeHash := range allShapes {
				if deletedShape == shapeHash {
					allShapes = append(allShapes[:i], allShapes[i+1:]...)
					break
				}
			}
		}
	}

	return allShapes, nil
}

func createCanvasHtmlFile(canvasXMax uint32, canvasYMax uint32, canvasShapes []string) error {
	canvasHtml := "<html>\n\t<body>\n"
	canvasHtml += "\t\t<svg height=\"" + strconv.FormatUint(uint64(canvasYMax), 10) + "\" width=\"" + strconv.FormatUint(uint64(canvasXMax), 10) + "\">\n"
	for _, shape := range canvasShapes {
		canvasHtml += "\t\t\t" + shape + "\n"
	}
	canvasHtml += "\t\t</svg>\n"
	canvasHtml += "\t</body>\n</html>\n"

	err := ioutil.WriteFile("./canvas.html", []byte(canvasHtml), 0644)
	return err
}

func buildBlockChain(canvas blockartlib.Canvas, genesisBlockHash string) (map[string]*block, error) {
	blockchain := make(map[string]*block)

	blockchain[genesisBlockHash] = &block{genesisBlockHash, 0, ""}

	err := buildBlockChainHelper(canvas, genesisBlockHash, 1, &blockchain)
	if err != nil {
		return nil, err
	}

	return blockchain, nil
}

func buildBlockChainHelper(canvas blockartlib.Canvas, blockHash string, blockNum int, blockchain *map[string]*block) error {
	children, err := canvas.GetChildren(blockHash)
	if err != nil {
		return err
	}

	for _, child := range children {
		(*blockchain)[child] = &block{child, blockNum, blockHash}
		err = buildBlockChainHelper(canvas, child, blockNum+1, blockchain)
		if err != nil {
			return err
		}
	}

	return nil
}

func getAllShapesOnLongestChain(canvas blockartlib.Canvas, genesisBlockHash string, blockchain map[string]*block) ([]string, error) {
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
		shapes, err := canvas.GetShapes(blockHash)
		if err != nil {
			return nil, err
		}

		for _, shape := range shapes {
			allShapes = append(allShapes, shape)
		}
	}

	return allShapes, nil
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
