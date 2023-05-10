/*
  # Limitations & Improvements Suggestions:

	- The system can only move forward if a majority of Nodes are active. Otherwise, no consensus can be achieved,
	and no new nodes can register. To improve this, Nodes that do not answer to some threshold number of API calls should be voted to
	be removed from the list, and once a majority of its peers agree, then the Node should be removed.

	- Our blockchain can only accept content from one user at a time. Nodes can be modified to bundle data together over time.
*/

package node

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	blk "project/Block"
	bc "project/Blockchain"
	help "project/Helpers"
	usr "project/User"
	"sync"
	"time"
)

/* Output files for logs */
var OUT os.File
var wait10_time time.Duration = 10 * time.Millisecond

var NODE_LIST string
var USER_LIST string

const PROTOCOL string = "tcp"
const LOCALHOST string = "http://localhost:"
const LOCALHOST_IP string = "127.0.0.1:"

const NEW_CHAIN string = "/new_chain"
const COPY_CHAIN string = "/copy_chain"
const CONTENT string = "/content"
const VALIDATE string = "/validate"

/*
A Node is referenced to by its port and holds a copy of the blockchain.

It is in charge of listening for users and peers, and handling their
requests according to the agreed upon Proof of Work consensus protocol.

Nodes first registers to the network before they can validate blocks to
the blockchain.
*/
type Node struct {
	Port       string
	Blockchain bc.Blockchain
	Listener   net.Listener
	Running    bool

	Validated []blk.Block

	Acceptance_mu *sync.Mutex
}

/*
This function creates an http listener for both users and peers.
*/
func (node *Node) StartListening(out os.File) {

	OUT = out

	// Declare a new mutex variable
	var myMutex sync.Mutex

	node.Acceptance_mu = &myMutex

	/* Otherwise, start the service. */

	NODE_ADDRESS := LOCALHOST_IP + node.Port

	listener, err := net.Listen(PROTOCOL, NODE_ADDRESS)
	if help.Check(err) {
		return
	}

	/* Wrapper Function to Handle HTTP Requests */
	handler := func(w http.ResponseWriter, r *http.Request) {
		node.HandleRequests(w, r)
	}

	if help.Check(http.Serve(listener, http.HandlerFunc(handler))) {
		fmt.Fprintf(&OUT, "%s Error Serving HTTP on CLT PORT", node.Port)
	}

	fmt.Fprintf(&OUT, "Client Interface has started on %v", NODE_ADDRESS)
}

/*
This function handles requests to the node. Depending on the URI, another
function will be called to handle the request made by either a user or
a peer.
*/
func (node *Node) HandleRequests(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(&OUT, "\n---------------%s Received %v command from %v---------------\n", node.Port, r.RequestURI, r.RemoteAddr)

	// A new chain was created by the 4th node
	// Handle this by accepting it.
	if r.RequestURI == NEW_CHAIN {
		if len(node.Blockchain.Blocks) == 0 {
			/* Decode the blockchain object from the json request */
			var blockchain bc.Blockchain
			err := json.NewDecoder(r.Body).Decode(&blockchain)
			if help.Check(err) {
				fmt.Fprintln(&OUT, "ERROR: Could not decode JSON")
			}

			/*
				Limitation: Before accepting blockchain, we should verify that the 5th node is
				the one that sent this chain.
			*/
			node.Blockchain = blockchain

			fmt.Fprintln(&OUT, "New blockchain accepted!")
		}

		return
	}

	// A request for a copy of the currently committed blockchain,
	// Reply back with this node's copy of a committed blockchain.
	if r.RequestURI == COPY_CHAIN {
		blockchain := node.Blockchain

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(blockchain)
		return
	}

	// A request for content to be mined and accepted on the blockchain.
	// User's must be registered to get their content accepted.
	if r.RequestURI == CONTENT {
		/* Unmarshal the content */
		var content usr.Content
		err := json.NewDecoder(r.Body).Decode(&content) // Decode the request's body
		help.Check(err)

		// Check if user is registered
		if content.User.IsUserRegistered() {
			// Only mine content coming from registered users.
			node.MineContent(content.Content)
		}
	}

	// When a block is sent for validation,
	// validate it and accept it if it is valid.
	// Otherwise remove it from the validated list.
	if r.RequestURI == VALIDATE {
		/* Unmarshal the block */
		var block blk.Block
		err := json.NewDecoder(r.Body).Decode(&block) // Decode the request's body
		help.Check(err)

		fmt.Fprintf(&OUT, "block{ %s } received for validation\n", block.Content)

		// Check if block is fully valid
		if node.ValidateBlock(block, 0) {
			// Add block to the list of validated blocks (this stops mining)
			node.Validated = append(node.Validated, block)

			/* This sleep call is purely for demonstration purposes.

			We are essentially waiting for nodes to create potential conflicts
			more often. Recall that conflicts are blocks that are validated (i.e. added
			to the node's array of validated blocks) during the same period of time.
			Although rare, conflicts can occur frequently at scale.
			To demonstrate this, we add the short sleep for the node.Validated array
			to fill up, thereby forcing	conflicts.
			Conflicts will still occur randomly, but the longer the wait time the more
			likely they will occur.
			*/
			time.Sleep(wait10_time)

			// Acceptance is non-interruptible
			// node.Acceptance_mu.Lock()

			// If there are conflicting blocks then this will be false
			conflict := len(node.Validated) > 1

			// If there are no conflicting blocks
			if !conflict {
				// Accept the block; no conflicts
				node.acceptValidatedBlock(w, block)
				// node.Acceptance_mu.Unlock() // unlock

				// Reset the node's list of validated
				node.Validated = []blk.Block{}

				fmt.Fprintf(&OUT, "Node %s validated and accepted Block{ %s }\n", node.Port, block.Content)
				return
			} else { // There exists at least one conflict
				fmt.Fprintln(&OUT, "Conflict detected!!!")

				// This process is ran as part of pre-acceptance, so
				// should be thread safe as it accesses the node's Validated array
				// node.Acceptance_mu.Lock()
				// Check if the blocks are tied, before validating.
				if node.ValidatedBlocksTied() {
					// Accept earliest block
					node.acceptValidatedBlock(w, node.Validated[0])
				} else {
					// Get the block with the longest index
					block = node.FindLongestBranch()
					node.acceptValidatedBlock(w, block)
				}
			}
			// node.Acceptance_mu.Unlock() // unlock

		} else {
			fmt.Fprintf(&OUT, "Node %s could not validate Block{ %s }\n", node.Port, block.Content)
			// respond with 403
			w.WriteHeader(403)
		}
	}
}

/*
Accept the given block. Only check if the block was validated for its index.

If the block's index is invalid, then do not accept and respond with a 403.
If block's index is greater than the blockchain's last index + 1, then block was
sent to be validated with a skipped index, then node should update blockchain.
*/
func (node *Node) acceptValidatedBlock(w http.ResponseWriter, block blk.Block) {
	node.Acceptance_mu.Lock()
	// Check the index is still valid on the block
	if !node.ValidateBlock(block, 0) {
		fmt.Fprintf(&OUT, "Node %s could not validate Block{ %s }\n", node.Port, block.Content)
		// respond with 403
		w.WriteHeader(403)
		return
	}

	// Check no missing blocks.
	if block.Index == len(node.Blockchain.Blocks) {
		// Accept block
		// node.Acceptance_mu.Lock()
		node.Blockchain.Blocks = append(node.Blockchain.Blocks, &block)
	}
	node.Acceptance_mu.Unlock()

	/*
		Handle missing blocks via simple broadcast update

		Limitations: This requires a complete blockchain message, instead of a
		few missing blocks. Improvements would require asking for specific blocks,
		or have peers respond with missing blocks when they respond 403, and node can then
		update itself if majority send the same missing blocks.
	*/

	// If block's index is greater than the blockchain's last index + 1
	// Then the node knows it has skipped a block so it should update its blockchain
	if block.Index > len(node.Blockchain.Blocks) {
		node.UpdateBlockchain()
	}

}

/*
Check if this node's validated blocks are tied. If so, return true, else
return false.
*/
func (node *Node) ValidatedBlocksTied() bool {

	idx := node.Validated[0].Index

	/* Iterate over each block */
	for _, block := range node.Validated {
		if idx != block.Index {
			// Not a tied list of validated blocks
			return false
		}
	}

	// List of validated blocks is indeed tied.
	return true
}

/*
Get the block with the highest index in the node's list of validated blocks.
*/
func (node *Node) FindLongestBranch() blk.Block {
	longestIdx := 0 // Initialize longest index
	blockIdx := 0   // Initialize index of block in the validated list

	/* Iterate over list of validated blocks */
	for idx, block := range node.Validated {
		if block.Index > longestIdx {
			longestIdx = block.Index // update longest Index field
			blockIdx = idx           // update block index with longest Index
		}
	}

	// Return the block with the longest index field.
	return node.Validated[blockIdx]
}
