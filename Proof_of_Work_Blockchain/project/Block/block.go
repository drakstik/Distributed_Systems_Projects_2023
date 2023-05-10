package block

import (
	"crypto/sha256"
	"encoding/json"
	"time"
)

type Block struct {
	PrevBlockHash []byte `json:"prev_hash"`
	Index         int    `json:"index"`

	Timestamp int64  `json:"timestamp"`
	Content   []byte `json:"data"`

	Nonce    int    `json:"nonce"`
	SelfHash []byte `json:"hash"`
}

/*
Set this block's hash
*/
func (b *Block) SetHash() {
	jsonBlock, err := json.Marshal(b) // json encode the block
	if err != nil {
		panic(err)
	}

	hash := sha256.Sum256(jsonBlock)

	// Set the block's SelfHash
	b.SelfHash = hash[:]
}

/*
Create and return a new block
*/
func NewBlock(content string, prevBlockHash []byte, prevIndex int) *Block {
	// Create a new block using given data, prevBlockHash and the current time.
	// 		Initialize the block's SelfHash as an empty array of bytes
	block := &Block{prevBlockHash, prevIndex + 1, time.Now().UnixNano(), []byte(content), 0, []byte{}}
	pow := NewProofOfWork(block)

	// Run proof of work
	// Returns a hash proof when correct nonce is found.
	nonce, hash := pow.Run()

	block.SelfHash = hash[:]
	block.Nonce = nonce

	return block
}

/*
Turn the block into a PoW, then validate it.
*/
func (block *Block) Validate() bool {

	// Turn block into a PoW
	pow := NewProofOfWork(block)

	// Return the validate result for this PoW.
	return pow.ValidatePoW()
}
