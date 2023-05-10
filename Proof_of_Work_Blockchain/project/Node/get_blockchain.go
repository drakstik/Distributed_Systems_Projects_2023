package node

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	bc "project/Blockchain"
	help "project/Helpers"
)

/*
Send /copychain to all known ports and return the majority blockchain.
*/
func GetBlockchain(filepath string) (bool, bc.Blockchain) {
	// Get all known ports
	known_ports := help.GetPorts(filepath)

	// Array of response bodies
	responses := []string{}

	/* Iterate over each port */
	for _, port := range known_ports {
		// Create url using the node's port
		url := LOCALHOST + port + COPY_CHAIN

		// Create a new HTTP client
		client := &http.Client{}

		// Send a GET request to http://localhost:known_port/copychain
		resp, err := client.Get(url)
		if !help.Check(err) {
			// Read the response body
			body, err := io.ReadAll(resp.Body)
			help.Check(err)
			// Append the response for later
			responses = append(responses, string(body))
		}
	}

	chosenChain := ""

	// For each received responseA
	for _, responseA := range responses {
		count := 0
		// For each received responseA
		for _, responseB := range responses {
			// Compare responseA with all other responses
			if responseB == responseA {
				count++ // Count how many responses are similar
			}

			// If the response count is greater than majority,
			// then adopt it as the right blockchain
			if count >= (len(known_ports)/3)*2 {
				chosenChain = responseA
			}
		}
	}

	if chosenChain == "" {
		return false, bc.Blockchain{}
	}

	/* Get the blockchain object from the json request */
	var blockchain bc.Blockchain
	if err := json.Unmarshal([]byte(chosenChain), &blockchain); err != nil {
		panic(err)
	}
	fmt.Println("Successfully got a blockchain")
	return true, blockchain
}

/*
Print a given blockchain
*/
func PrintBlockchain(blockchain bc.Blockchain) {
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

/*
Update this node's blockchain. This function is not thread safe and should be
called in a thread safe manner using Acceptance_mu since it updates the blockchain.
*/
func (node *Node) UpdateBlockchain() bool {
	success, blockchain := GetBlockchain(NODE_LIST)
	if success {
		node.Acceptance_mu.Lock()
		node.Blockchain = blockchain
		node.Acceptance_mu.Unlock()
	} else {
		return false // Could not update blockchain
	}

	return true
}
