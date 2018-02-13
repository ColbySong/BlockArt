package main

import "./blockchain"

// RPC Target
// Disseminate Block to connected miners, if it passes validation.
// TODO - I think we can delete these steps or at least move them to isValidBlock()
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
func (s *MServer) DisseminateBlock(block blockchain.Block, _ignore *bool) {
	// TODO: May need to change locking semantics
	blockChain.Lock()
	defer blockChain.Unlock()

	if s.isValidBlock(block) {
		saveBlockToBlockChain(block)
		sendToAllConnectedMiners("MServer.DisseminateBlock", block, nil)
		switchToLongestBranch()
	} else {
		errLog.Printf("Rejecting invalid block.\n")
	}
}


// RPC Target
func (s *MServer) DisseminateOperation(op blockchain.OpRecord, _ignore *bool) error {
	pendingOperations.Lock()

	opRecordHash := computeOpRecordHash(op)
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

// RPC Target
// Return entire block chain
func (s *MServer) GetBlockChain(_ignore bool, bc *blockchain.BlockChain) error {
	blockChain.RLock()
	defer blockChain.RUnlock()

	*bc = blockChain

	return nil
}


// Checks if a block is valid, including its operations.
func (s *MServer) isValidBlock(block blockchain.Block) bool {
	blockChain.Lock() // TODO - this is also locked by the caller, what will happen?
	defer blockChain.Unlock()

	hash := computeBlockHash(block)

	// 0. Check that this block isn't already part of the local blockChain
	_, alreadyExists := blockChain.Blocks[hash]
	if alreadyExists {
		errLog.Printf("Invalid block received: block with hash already exists: %s\n", hash)
		return false
	}


	// 1. Check for valid block num
	prevBlock, prevBlockExistsLocally := blockChain.Blocks[block.PrevHash]
	if !prevBlockExistsLocally {updateBlockChain()}

	prevBlock, prevBlockExistsLocally = blockChain.Blocks[block.PrevHash]
	if !prevBlockExistsLocally {
		errLog.Printf("Invalid block received: no previous block found\n")
		return false
	}

	isNextBlock := block.BlockNum == prevBlock.BlockNum + 1
	if !isNextBlock {
		errLog.Printf("Invalid block received: invalid BlockNum [%d]\n", block.BlockNum)
		return false
	}

	// 2. Check hash for valid proof-of-work
	var proofDifficulty uint8
	if len(block.OpRecords) == 0 {
		proofDifficulty = s.inkMiner.settings.PoWDifficultyNoOpBlock
	} else {
		proofDifficulty = s.inkMiner.settings.PoWDifficultyOpBlock
	}

	hasValidPoW := verifyTrailingZeros(hash, proofDifficulty)
	if !hasValidPoW {
		errLog.Printf("Invalid block received: invalid proof-of-work\n")
		return false
	}

	// 3. Check operations for validity
	if !hasValidOperations(block.OpRecords) {
		errLog.Printf("Invalid block received: invalid operations\n")
		return false
	}

	return true
}


func switchToLongestBranch() string {
	// TODO - how are we gonna handle locking this?
	blockChain.Lock()
	defer blockChain.Unlock()

	maxBlockNum := uint32(0)
	var newestHash string

	for hash, block := range blockChain.Blocks {
		if block.BlockNum > maxBlockNum {
			maxBlockNum = block.BlockNum
			newestHash = hash
		}
	}

	blockChain.NewestHash = newestHash
	return newestHash
}
// Checks if ALL operations as a set can be executed.
// Must check for ink level and shape overlap.
func hasValidOperations(ops map[string]*blockchain.OpRecord) bool {
	// todo - stub
	return true
}

// Downloads the entire BlockChain from all connected miners and updates the local
// version with the majority copy.
func updateBlockChain() {
	_ = getBlockChainsFromNeighbours()
	// TODO - update global variable to match majority value from the response
}