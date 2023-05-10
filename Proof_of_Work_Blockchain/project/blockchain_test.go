package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	blockchainBlock "project/Block"
	test_helper "project/Helpers"
	blockchainNode "project/Node"
	blockchainUser "project/User"
	"testing"
	"time"
)

const NODE_DIR = "/tmp/NodeList.txt"
const USER_DIR = "/tmp/UserList.txt"

/* The entire journey is logged here */
const TEST_OUT_DIR = "/tmp/test_output.txt"

/*
 * Delete the Registered Users and Registered Nodes in their repective dirs and kill the processes associated with them.
 */
func cleanup() {
	if len(test_helper.GetPorts(NODE_DIR)) >= 0 {
		err := os.Remove(NODE_DIR)
		if test_helper.Check(err) {
			fmt.Println("ERR: Could not delete NodeList successfully")
		} else {
			fmt.Printf("File %s deleted successfully!\n", NODE_DIR)
		}
	}
}

/* Happy Journeys */

/*
Check if a new node is successfully able to register on the blockchain
*/
func TestRegisterUser(t *testing.T) {
	fmt.Println("Testing New Blockchain User Registration...")
	/* Clean anything that may have been reminiscent in the previous run */
	cleanup()
	testNode := blockchainUser.User{}
	testNode.RegisterUser(NODE_DIR, USER_DIR)
	port := testNode.Port
	if port == "" {
		t.Errorf("User Registration Failed\n")
		return
	}
	fmt.Printf("Successfully got port %v for registered node\n", port)
}

func TestRegisterNode(t *testing.T) {
	fmt.Println("Testing New Blockchain Node Registration...")
	/* Clean anything that may have been reminiscent in the previous run */
	cleanup()
	testNode := blockchainNode.Node{}
	LogFile, _ := os.OpenFile(TEST_OUT_DIR, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	testNode.RegisterNode(NODE_DIR, USER_DIR, *LogFile)
	port := testNode.Port
	if port == "" {
		t.Errorf("Node Registration Failed\n")
		return
	}
	fmt.Printf("Successfully got port %v for registered user\n", port)
}

/* Check that a blockchain is not created until there are atleaset 5 nodes in the network */
func TestBlockChainCreationFailure(t *testing.T) {
	fmt.Println("Testing New Blockchain Creation Failure...")
	cleanup()
	blockChainNodes := make([]blockchainNode.Node, 3)
	LogFile, _ := os.OpenFile(TEST_OUT_DIR, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	for _, blockchainNode := range blockChainNodes {
		blockchainNode.RegisterNode(NODE_DIR, USER_DIR, *LogFile)
		if blockchainNode.Port == "" {
			t.Errorf("Node Registration Failed\n")
		}
		fmt.Printf("Successfully got port %v for registered node\n", blockchainNode.Port)
	}
	_, blockchain := blockchainNode.GetBlockchain(NODE_DIR)
	if len(blockchain.Blocks) != 0 {
		t.Errorf("Shouldn't create a blockchain network unless there are atleast 4 nodes\n")
	}
}

/*
Four successful node registrations on the network should trigger a the blockchain creation genesis
*/
func TestBlockchainCreationSuccess(t *testing.T) {
	fmt.Println("Testing New Blockchain Creation Success...")
	cleanup()
	blockChainNodes := make([]blockchainNode.Node, 5)
	LogFile, _ := os.OpenFile(TEST_OUT_DIR, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	for _, blockchainNode := range blockChainNodes {
		blockchainNode.RegisterNode(NODE_DIR, USER_DIR, *LogFile)
		if blockchainNode.Port == "" {
			t.Errorf("Node Registration Failed\n")
		}
		fmt.Printf("Successfully got port %v for registered node\n", blockchainNode.Port)
	}

	/* Wait for blockchain to be replicated */
	/* WARNING: Increase this if Blockchain Creation Fails */
	time.Sleep(2000 * time.Millisecond)

	/* The 4th node triggers the blockchain creation */
	_, blockchain := blockchainNode.GetBlockchain(NODE_DIR)
	blockchain_len := len(blockchain.Blocks)

	if len(blockchain.Blocks) == 0 {
		t.Errorf("Blockchain creation failed\n")
		return
	}

	for i := 0; i < 3; i++ {
		blockchain_found, blockchain_ := blockchainNode.GetBlockchain(NODE_DIR)
		if !blockchain_found || blockchain_len != len(blockchain_.Blocks) {
			t.Errorf("Length of blockchains differ\n")
			return
		}
		/* Comparing the current and previous hashes of each block in the blockchain */
		for j, block := range blockchain_.Blocks {
			if (hex.EncodeToString(block.SelfHash) != hex.EncodeToString(blockchain.Blocks[j].SelfHash)) ||
				(hex.EncodeToString(block.PrevBlockHash) != hex.EncodeToString(blockchain.Blocks[j].PrevBlockHash)) {
				t.Errorf("Blockchain Creation Failed: hash of block %d differs\n", i)
			}
		}
	}
	fmt.Printf("Successfully matched all hashes\n")
}

func ValidateTest(pow *blockchainBlock.ProofOfWork) bool {
	var hashInt big.Int

	data := pow.MergeBlockNonce(pow.Block.Nonce)
	hash := sha256.Sum256(data)
	hashInt.SetBytes(hash[:])

	isValid := hashInt.Cmp(pow.Target) == -1
	return isValid

}

func TestNewProofOfWork(t *testing.T) {
	fmt.Println("Testing New Proof of Work ...")

	/* Create New Block with no prior block information */
	testBlock := blockchainBlock.NewBlock("Test Block", []byte{}, -1)

	newProofOfWork := blockchainBlock.NewProofOfWork(testBlock)

	valid := ValidateTest(newProofOfWork)
	if !valid {
		t.Errorf("Invalid Proof of Work")
		return
	}
	fmt.Printf("Successfully completed Proof of Work!\n")
}

/*
Test that peers can accept content from users and does not accept content from non-registered users.
*/
func TestContentAcceptance(t *testing.T) {
	/* Create a new file output for logs. */
	LogFile, err := os.OpenFile("output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	OUT = *LogFile

	if len(test_helper.GetPorts(NODE_LIST)) >= 0 {
		err = os.Remove(NODE_LIST)
		if test_helper.Check(err) {
			fmt.Println("ERR: Could not delete NodeList successfully")
		} else {
			fmt.Printf("File %s deleted successfully!\n", NODE_LIST)
		}
	}

	if len(test_helper.GetPorts(USER_LIST)) >= 0 {
		err = os.Remove(USER_LIST)
		if test_helper.Check(err) {
			fmt.Println("ERR: Could not delete UserList successfully")
		} else {
			fmt.Printf("File %s deleted successfully!\n", USER_LIST)
		}
	}

	// Initialize 5 empty nodes
	node1 := blockchainNode.Node{}
	node2 := blockchainNode.Node{}
	node3 := blockchainNode.Node{}
	node4 := blockchainNode.Node{}
	node5 := blockchainNode.Node{}

	// Register 5 nodes
	go node1.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node2.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node3.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node4.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node5.RegisterNode(NODE_LIST, USER_LIST, OUT)

	// Wait for registration to be processed
	time.Sleep(wait_time)

	/*
		Sending content non-concurrently should result in all users getting their
		content added.

		Depends on DIFFICULTY and content processing time.
	*/
	bob := blockchainUser.User{}
	bob.RegisterUser(USER_LIST, NODE_LIST)
	bob.SendContent("Test content")

	// Wait for content to be processed
	time.Sleep(wait_time * 3)

	// Get and print majority blockchain
	success, blockchain := blockchainNode.GetBlockchain(NODE_LIST)
	if !success {
		t.Errorf("Blockchain creation failed\n")
		return
	}

	if len(blockchain.Blocks) < 2 {
		t.Errorf("Expected 2 blocks in blockchain but there was %d\n", len(blockchain.Blocks))
		return
	}

	// Check for the right content in each block
	for idx, block := range blockchain.Blocks {
		if (idx == 0 && string(block.Content) != "Genesis Block") ||
			(idx == 1 && string(block.Content) != "Test content") {
			t.Errorf("Content in blockchain is not as expected.\n")
			return
		}
	}

}
