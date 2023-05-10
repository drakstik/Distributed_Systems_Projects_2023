package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	blk "project/Block"
	help "project/Helpers"
)

/*
This function requests peers to accept a block,
if majority of peers accept it, this node too can accept it.
*/
func (node *Node) AcceptBlock(newBlock blk.Block, i int) bool {
	/* Marshall request object */
	jsonBytes, err := json.Marshal(newBlock)
	help.Check(err)

	// Get the known ports
	known_ports := help.GetPorts(NODE_LIST)

	// Initialize the vote count
	count_votes := 0

	/* Iterate over all known nodes */
	for _, port := range known_ports {
		// Skip this node
		if i != 1 && port == node.Port {
			continue
		}

		url := LOCALHOST + port

		// Create the request
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
		help.Check(err)

		// Set the request's header to JSON
		req.Header.Set("Content-Type", "application/json")

		// Set the request's URI to /validate
		req.URL.Path = VALIDATE

		client := &http.Client{}
		// Send request, then wait for a response
		resp, err := client.Do(req)
		if help.Check(err) {
			fmt.Printf("%s could not send /validate to %s", node.Port, port)
			return false
		} else if resp.StatusCode == 200 {
			count_votes++ // increment count_vote for every 200 code received
		}

		fmt.Printf("%s Sent /validate{ %s } to %s\n", node.Port, newBlock.Content, port)

		resp.Body.Close()
	}

	if i == 1 {
		// Check if count_votes is majority
		if count_votes >= ((len(known_ports) / 3) * 2) {
			return true
		}
	}

	// Check if count_votes is majority
	if count_votes >= ((len(known_ports) / 3) * 2) {
		// Accept the block.
		node.Blockchain.Blocks = append(node.Blockchain.Blocks, &newBlock)
		fmt.Printf("Node %s accepted block{ %s }\n", node.Port, newBlock.Content)
		return true
	}

	return false

}
