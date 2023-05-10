package node

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	blk "project/Block"
	"time"
)

func (node *Node) RunPoW(pow blk.ProofOfWork) (int, []byte) {

	var hash [32]byte   // Stores PoW hash
	var hashInt big.Int // Wraps poW hash for fast verification
	nonce := 0          // Initialize the nonce

	// Start measuring time (useful for testing/calculations/tuning).
	start := time.Now()

	for nonce < math.MaxInt64 && len(node.Validated) == 0 {
		// Merge the block and the nonce into
		// and represent them as an array of bytes
		data := pow.MergeBlockNonce(nonce)

		// Hash the array of bytes representing block+nonce
		hash = sha256.Sum256(data)

		// Convert the hash into a Big Int
		hashInt.SetBytes(hash[:])

		/*
			Given x.Cmp(y), return:
			->  -1 if x is less than y.
				0 if x is equal to y.
				1 if x is greater than y.

			PoW is legit if hashInt is less than pow.Target.
		*/
		if hashInt.Cmp(pow.Target) == -1 {
			break // Found the nonce, because the hash is less than target=0000...1...000
		} else {
			nonce++ // increment nonce if hash is equal or greater than target=0000...1...000
		}
	}

	elapsed := time.Since(start)

	// If this is a case of interruption by peer sending a valid block
	if len(node.Validated) != 0 {

		return -1, []byte{}
	}

	fmt.Printf("Block mining elapsed time: %s\n", elapsed)
	return nonce, hash[:]
}
