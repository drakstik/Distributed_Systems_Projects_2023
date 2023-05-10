/*
This is an implementation of a Proof of Work Blockchain in Golang.

By David Mberingabo and Parardha Kumar

Proof of Work is computed over the hash of a block and a nonce (an integer added
to the block before hashing). Starting the nonce at 0, a miner checks if the hash
of the block+nonce begins with a certain number of zeroes. This number of zeroes
is defined by the network as a difficulty level that can be increased over time
by the network in order to maintain difficulty when computing power increases
over time.


Reference: https://jeiwan.net/posts/building-blockchain-in-go-part-2/
*/

package block

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/big"
	"time"
)

/*
target bits in BTC is the difficulty level.
This constant is used in calculating the hex representation of
the target. In this demo, difficulty is 24 which would look like
this in hex -> 0x10000000000000000000000000000000000000000000000000000000000
*/
const DIFFICULTY = 18 // Current difficulty

/**/
type ProofOfWork struct {
	Block  *Block
	Target *big.Int
}

// IntToHex converts an int64 to a byte array
func IntToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

/* Turns a block into a proof of work object that can then be validated. */
func NewProofOfWork(b *Block) *ProofOfWork {

	target := big.NewInt(1) // 000.....0001

	// Bitwise left-shift target by (256 - DIFFICULTY) positions
	// 000.....0001 -> 000...1...0000
	target.Lsh(target, uint(256-DIFFICULTY))

	pow := &ProofOfWork{b, target}

	return pow
}

/*
Return a block+nonce.

A block+nonce is a byte array containing all the fields
in the pow's block merged with the targetBits and nonce.
*/
func (pow *ProofOfWork) MergeBlockNonce(nonce int) []byte {
	data := bytes.Join(
		[][]byte{
			pow.Block.PrevBlockHash,
			pow.Block.Content,
			IntToHex(pow.Block.Timestamp),
			IntToHex(int64(DIFFICULTY)),
			IntToHex(int64(nonce)),
		},
		[]byte{},
	)

	return data
}

// Run proof of work.
// Return a nonce and the corresponding hash of the block+nonce
func (pow *ProofOfWork) Run() (int, []byte) {
	/* Initialize variables */

	// hashInt is used to store hash of block+nonce
	// and to compute the
	var hashInt big.Int
	var hash [32]byte // Hash of the block+nonce
	nonce := 0

	// fmt.Printf("Mining the block containing \"%s\"\n", pow.block.Data)

	// Measure time to find nonce.
	start := time.Now()

	for nonce < math.MaxInt64 {
		data := pow.MergeBlockNonce(nonce)
		hash = sha256.Sum256(data)
		//
		hashInt.SetBytes(hash[:])

		/*
			Given x.Cmp(y), return:
			->  -1 if x is less than y.
				0 if x is equal to y.
				1 if x is greater than y.
		*/
		if hashInt.Cmp(pow.Target) == -1 {
			break // Found the nonce, because the hash is less than target=0000...1...000
		} else {
			nonce++ // increment nonce if hash is equal or greater than target=0000...1...000
		}
	}

	elapsed := time.Since(start)
	// fmt.Printf("Finished work! : %x\n", hash)
	fmt.Printf("Block mining elapsed time: %s\n\n", elapsed)

	return nonce, hash[:]
}

func (pow *ProofOfWork) ValidatePoW() bool {
	var hashInt big.Int

	data := pow.MergeBlockNonce(pow.Block.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	/*
		Given x.Cmp(y), return:
		->  -1 if x is less than y.
			0 if x is equal to y.
			1 if x is greater than y.
	*/
	isValid := hashInt.Cmp(pow.Target) == -1

	return isValid
}
