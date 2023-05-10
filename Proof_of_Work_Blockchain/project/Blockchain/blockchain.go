package blockchain

import (
	"fmt"
	block "project/Block"
)

const NON_TRIVIAL int = 4

/*
A blockchain is an array of blocks.

Blocks are logically chained by the prevBlock has in each new block,
with the Genesis block as as the root of the blockchain.
*/
type Blockchain struct {
	Blocks []*block.Block `json:"blocks"`
}

// /*
// Creates a new block using the given data and appends it to the blockchain.
// */
func (bc *Blockchain) AddBlock(data string) {
	prevBlock := bc.Blocks[len(bc.Blocks)-1]
	newBlock := block.NewBlock(data, prevBlock.SelfHash, prevBlock.Index)
	bc.Blocks = append(bc.Blocks, newBlock)
}

/*
When there are 4 peers, a new blockchain should be automatically created.
*/
func NewBlockchain(known_ports []string) (*Blockchain, bool) {

	if len(known_ports) == NON_TRIVIAL {
		genesisBlock := []*block.Block{block.NewBlock("Genesis Block", []byte{}, -1)}
		return &Blockchain{Blocks: genesisBlock}, true
	}

	return &Blockchain{}, false
}

/* Print a blockchain's blocks and fields to the console */
func PrintBlockchain(blockchain Blockchain) {
	fmt.Println("---------------------------------**Blockchain**---------------------------------")

	for _, block := range blockchain.Blocks {
		fmt.Printf("Block %d:\n", block.Index)
		fmt.Printf("  PrevBlockHash: %x\n", string(block.PrevBlockHash))
		fmt.Printf("  Index: %d\n", block.Index)
		fmt.Printf("  Timestamp: %d\n", block.Timestamp)
		fmt.Printf("  Data: %s\n", string(block.Content))
		fmt.Printf("  Nonce: %d\n", block.Nonce)
		fmt.Printf("  SelfHash: %x\n", string(block.SelfHash))
	}

	fmt.Println("---------------------------------*****---------------------------------")

}
