package main

/*
	User: An entity that sends data across the network.
		  If the blockchain network is the decentralized server, the user is the client.
		  eg: User David sends money to User Parardha, David is using the blockchain to send content (money in this case)

	Node: A Machine on the Blockchain

*/

import (
	"fmt"
	"log"
	"os"
	blk "project/Block"
	help "project/Helpers"
	nd "project/Node"
	usr "project/User"
	"sync"
	"time"
)

var OUT os.File
var wait_time time.Duration = 2000 * time.Millisecond

/* Global Constants */
const NODE_LIST = "/tmp/NodeList.txt"
const USER_LIST = "/tmp/UserList.txt"

var USER_LIST_MUTEX sync.Mutex
var NODE_LIST_MUTEX sync.Mutex

/* Random Function for Sanity Test */
func Hello() string {
	return "Hello World"
}

/*
	Entry point of application
	We register 5 nodes that constitute the blockchain network
*/

func main() {

	/* Create a new file output for logs. */
	LogFile, err := os.OpenFile("output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	OUT = *LogFile

	if len(help.GetPorts(NODE_LIST)) >= 0 {
		err = os.Remove(NODE_LIST)
		if help.Check(err) {
			fmt.Println("ERR: Could not delete NodeList successfully")
		} else {
			fmt.Printf("File %s deleted successfully!\n", NODE_LIST)
		}
	}

	if len(help.GetPorts(USER_LIST)) >= 0 {
		err = os.Remove(USER_LIST)
		if help.Check(err) {
			fmt.Println("ERR: Could not delete UserList successfully")
		} else {
			fmt.Printf("File %s deleted successfully!\n", USER_LIST)
		}
	}

	// Initialize 5 empty nodes
	node1 := nd.Node{}
	node2 := nd.Node{}
	node3 := nd.Node{}
	node4 := nd.Node{}
	node5 := nd.Node{}

	// Register 5 nodes
	go node1.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node2.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node3.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node4.RegisterNode(NODE_LIST, USER_LIST, OUT)
	go node5.RegisterNode(NODE_LIST, USER_LIST, OUT)

	// Wait for registration to be processed
	time.Sleep(wait_time)

	// Get and print majority blockchain
	success, blockchain := nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Sending content non-concurrently should result in all users getting their
		content added.

		Depends on DIFFICULTY and content processing time.
	*/
	bob := usr.User{}
	bob.RegisterUser(USER_LIST, NODE_LIST)
	bob.SendContent("First content")
	bob.SendContent("Second content")
	bob.SendContent("Third content")

	// Wait for content to be processed
	time.Sleep(wait_time * 3)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Sending content concurrently should result in the users interrupting eachother,
		so only one of the content should be accepted.
		This is because they race to the block acceptance.

		Depends on DIFFICULTY and content processing time.
	*/
	go bob.SendContent("Concurrent content")
	go bob.SendContent("Concurrent content")
	go bob.SendContent("Concurrent content")

	// Wait for content to be processed
	time.Sleep(wait_time * 3)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Sending content concurrently. One in a go routine, the other should be an interupting
		valid block sent using AcceptBlock() to simulate a /validate request while mining.
	*/

	// Get the previous block on the most recent blockchain
	prevBlock := blockchain.Blocks[len(blockchain.Blocks)-1]

	// Basically mining a block for testing purposes
	validBlock := blk.NewBlock("Interception", prevBlock.SelfHash, prevBlock.Index)

	go bob.SendContent("Do not accept")

	node := nd.Node{} // Instantiate unregistered node

	// Sent interuption block via /validate
	node.AcceptBlock(*validBlock, 1)

	// Wait for content to be processed
	time.Sleep(wait_time)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Same thing as above, but this time send an invalid interruption. The network should accept the content sent
		by bob instead.
	*/
	prevBlock = blockchain.Blocks[len(blockchain.Blocks)-1]
	// Invalid interruption block
	invalidBlock := blk.NewBlock("Do not accept", prevBlock.PrevBlockHash, prevBlock.Index)

	go bob.SendContent("Fourth content")

	// Sent interuption block via /validate
	node.AcceptBlock(*invalidBlock, 1)

	// Wait for content to be processed
	time.Sleep(wait_time)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Send concurrent content like above, but this time cause a conflict by sending multiple /validate requests.
		We are sending tied blocks with the same Index, nodes will accept whichever reaches them.

		Recall a conflict is simple blocks that are concurrently received for validation.
		Also recall that we are causing conflicts by waiting for conflicting blocks to occur,
		using a sleep function call to simulate conflicts resulting from scaling users. This is done instead
		of actually testing it at scale.
	*/
	prevBlock = blockchain.Blocks[len(blockchain.Blocks)-1]
	validBlock = blk.NewBlock("Interception", prevBlock.SelfHash, prevBlock.Index)

	// Tied valid interruption block
	validBlockTied := blk.NewBlock("Interception 2", prevBlock.SelfHash, prevBlock.Index)

	go bob.SendContent("Do not accept")

	// Send interuption block via /validate
	go node.AcceptBlock(*validBlock, 1)
	// Send a tied interuption block via /validate
	go node.AcceptBlock(*validBlockTied, 1)

	// Wait for content to be processed
	time.Sleep(wait_time)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
		Send concurrent content like above, but this time cause a conflict by sending multiple /validate requests.
		We are sending blocks with different Index (one higher than the other, but both valid).

		The higher block should be validated and instead of being accepted,
		the node should request /copy_chain from its peers.

		Due to the nature of the code, this test should result in no changes to the
		blockchain, since the block being sent is not an actual block from a user.

		This can viewed at the end of the output where the nodes receive /copy_chain/.

		Limitation & Future Work:
			- Demonstrate and test our codebase at scale, especially for conflicting content.
			- With enough users and over a long enough period, the nodes could grow
			  out of sync enough to create conflicts that are not tied blocks, which would
			  trigger the node to update and receive missing blocks.

	*/

	prevBlock = blockchain.Blocks[len(blockchain.Blocks)-1]
	validBlock = blk.NewBlock("Do not accept", prevBlock.SelfHash, prevBlock.Index)

	// Valid interruption block with a valid index + 1
	validBlockDiff := blk.NewBlock("Highest Index block", prevBlock.SelfHash, prevBlock.Index+1)

	go bob.SendContent("Do not accept")

	// Send interuption block via /validate
	go node.AcceptBlock(*validBlock, 1)
	// Send a tied interuption block via /validate
	go node.AcceptBlock(*validBlockDiff, 1)

	// Wait for content to be processed
	time.Sleep(wait_time)

	// Get and print majority blockchain
	success, blockchain = nd.GetBlockchain(NODE_LIST)
	if success {
		nd.PrintBlockchain(blockchain)
	}

	/*
			The final blockchain should look something like this:

			---------------------------------**Blockchain**---------------------------------
		Block 0:
		  PrevBlockHash:
		  Index: 0
		  Timestamp: 1683257165757555400
		  Data: Genesis Block
		  Nonce: 61744
		  SelfHash: 000023f33275e73b7dd560dcb4dfb892c609c5225f5d2845bf8f966040606482
		Block 1:
		  PrevBlockHash: 000023f33275e73b7dd560dcb4dfb892c609c5225f5d2845bf8f966040606482
		  Index: 1
		  Timestamp: 1683257167467456700
		  Data: First content
		  Nonce: 114289
		  SelfHash: 00001722f1933b39d3f44c3db9ba6fdcfbbc614c37df72c57a0bf48c33eed1d0
		Block 2:
		  PrevBlockHash: 00001722f1933b39d3f44c3db9ba6fdcfbbc614c37df72c57a0bf48c33eed1d0
		  Index: 2
		  Timestamp: 1683257167628935100
		  Data: Second content
		  Nonce: 132389
		  SelfHash: 000009da6bd0a43c795090424d0c9bea55d37706e8f3a6b6e7b07117281b2099
		Block 3:
		  PrevBlockHash: 000009da6bd0a43c795090424d0c9bea55d37706e8f3a6b6e7b07117281b2099
		  Index: 3
		  Timestamp: 1683257167786835900
		  Data: Third content
		  Nonce: 64310
		  SelfHash: 00003c230667621bb097c1b19a750ac5cdfd0dd00a6d579155b9120fbf5d1741
		Block 4:
		  PrevBlockHash: 00003c230667621bb097c1b19a750ac5cdfd0dd00a6d579155b9120fbf5d1741
		  Index: 4
		  Timestamp: 1683257173099325800
		  Data: Concurrent content
		  Nonce: 43141
		  SelfHash: 000026bbea7598039ad27f04af7f5a2a23ca412ffb074f3bb944915e69d017fc
		Block 5:
		  PrevBlockHash: 000026bbea7598039ad27f04af7f5a2a23ca412ffb074f3bb944915e69d017fc
		  Index: 5
		  Timestamp: 1683257178305923100
		  Data: Interception
		  Nonce: 48616
		  SelfHash: 000012888c643c1b8fb4ff16268f8ca5793cad41bbb0a22be794c49118f0c649
		Block 6:
		  PrevBlockHash: 000012888c643c1b8fb4ff16268f8ca5793cad41bbb0a22be794c49118f0c649
		  Index: 6
		  Timestamp: 1683257180186994000
		  Data: Fourth content
		  Nonce: 3030
		  SelfHash: 000008916f8f337a823e6b5891af9e1282d82ddf719f170fb8904ca682988a54
		Block 7:
		  PrevBlockHash: 000008916f8f337a823e6b5891af9e1282d82ddf719f170fb8904ca682988a54
		  Index: 7
		  Timestamp: 1683257182083840000
		  Data: Interception (or Interception 2)
		  Nonce: 399095
		  SelfHash: 000010f5a3e1177951e0492e916a4e4f90edf5496e3ce59feb9fb9baaf3fb35d
		---------------------------------*****---------------------------------
	*/
}
