package node

import (
	"fmt"
	blk "project/Block"
	"time"
)

/*
Mine content into a block with a PoW,
then accept the block once majority of peers accept it.

The mining can get interrupted by a block sent by a peer.
In that case, cease mining, validate the block.
If the block is valid, then stop mining,
if it is not valid then continue mining.

Once the mining ends and node awaits peers to accept the
block, then no interruptions are acceptable from validation
requests.

Before mining and a node should update its blockchain to
the most recent version.
*/
func (node *Node) MineContent(content string) bool {
	// node.Acceptance_mu.Lock()
	// Update blockchain before mining
	node.UpdateBlockchain()
	// node.Acceptance_mu.Unlock()

	// Get the previous block
	prevBlock := node.Blockchain.Blocks[len(node.Blockchain.Blocks)-1]

	// Get the new block (this process is interruptible)
	success, newBlock := node.MineNewBlock(content, prevBlock.SelfHash, prevBlock.Index)
	if !success {
		// Could not mine new block
		// Either due to interruption or errors while mining
		return false
	}

	// Acceptance is non-interruptible
	// node.Acceptance_mu.Lock()
	// Check if the block's index is still valid.
	if node.ValidateBlock(*newBlock, 1) {
		/* Ask peers if this block is valid before accepting it. */
		success = node.AcceptBlock(*newBlock, 0)
		// node.Acceptance_mu.Unlock()

		// If acceptance is denied by majority of peers
		if !success {
			/*
				Update blockchain if acceptance is denied by majority. Means our blockchain might be
				outdated.
			*/
			node.UpdateBlockchain()
		}
	} else {
		fmt.Printf("%s got interrupted by valid block\n", node.Port)
		// node.Acceptance_mu.Unlock() // Unlock before exiting
		return false
	}

	return success
}

/*
Create and return a new block.
*/
func (node *Node) MineNewBlock(data string, prevBlockHash []byte, prevIndex int) (bool, *blk.Block) {
	// Create a new block using given data, prevBlockHash and the current time.
	// 		Initialize the block's SelfHash as an empty array of bytes
	block := &blk.Block{
		PrevBlockHash: prevBlockHash,
		Index:         prevIndex + 1, Timestamp: time.Now().UnixNano(),
		Content:  []byte(data),
		Nonce:    0,
		SelfHash: []byte{}}

	// Create a new PoW object using the recently created block.
	pow := blk.NewProofOfWork(block)

	// Run proof of work
	// Returns a hash proof and corresponding nonce.
	nonce, hash := node.RunPoW(*pow)

	// If nonce == -1, then the Run function was unsuccessful
	// Due to interruptions or errors
	if nonce == -1 {
		return false, nil
	}

	/* Set the block's PoW hash and corresponding nonce*/
	block.SelfHash = hash[:]
	block.Nonce = nonce

	fmt.Printf("%s successfully mined block{ %s }\n", node.Port, block.Content)
	return true, block
}
