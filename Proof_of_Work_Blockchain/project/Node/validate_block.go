package node

import (
	"bytes"
	blk "project/Block"
)

/*
	   Return true if the block is valid and false otherwise.

	   A block is valid if:
			- it has a valid index,
			- it has a valid prevHash,
			- its Proof-of-Work is valid and
			- the block is not already in the chain.

Params: When passed 1, ValidateBlock only checks for matching indeces.
*/
func (node *Node) ValidateBlock(block blk.Block, i int) bool {
	/*
		A valid index is higher than the most recently committed block index.
		When a node receives a block that skipped an index or more,
		the node realizes it is missing a block and it should update its blockchain
		before accepting the block.
	*/
	prevIndex := len(node.Blockchain.Blocks) - 1
	prevBlock := node.Blockchain.Blocks[prevIndex]
	prevHash := prevBlock.SelfHash

	if i == 1 {
		return block.Index > prevIndex
	}

	return block.Index > prevIndex &&
		bytes.Equal(prevHash, block.PrevBlockHash) &&
		block.Validate() &&
		!node.IsDoubleSpend(block)
}

/*
Return true if the block is already in the chain,
else false.
*/
func (node *Node) IsDoubleSpend(block blk.Block) bool {
	// Iterate over all blocks in node's blockchain
	for _, b := range node.Blockchain.Blocks {
		// If the hashes equal, we have a double spend.
		if bytes.Equal(b.SelfHash, block.SelfHash) {
			return true
		}
	}

	return false
}
