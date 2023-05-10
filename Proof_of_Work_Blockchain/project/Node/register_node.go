package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	bc "project/Blockchain"
	help "project/Helpers"
	"strconv"
	"sync"
)

var registration_mutex sync.Mutex

/*
	Register a node to the blockchain
	RegisterNode may be called concurrently and should be thread safe.
*/
func (node *Node) RegisterNode(NodeList string, UserList string, OUT os.File) {
	
	/*
		Nodes read the NodeList and choose a port number that
		has not already	been used.

		Once there are a non-trivial number of nodes on the network
		a NewBlockchain() may be created.
	*/

	registration_mutex.Lock()

	// Read from the NodeList
	known_ports := help.GetPorts(NodeList)

	// Initial port choice
	chosen_port := 1234

	// If there are nodes already registered
	if len(known_ports) > 0 {
		/* Choose a port number */
		init := known_ports[len(known_ports)-1] // Initialize port choice to last known port
		chosen_port, err := strconv.Atoi(init)  // Convert initial port choice to int
		if help.Check(err) {
			return // Error while converting to int
		}
		chosen_port++ // Increment chosen port by 1

		/* Set the node's port field */
		node.Port = strconv.Itoa(chosen_port)

		// Only fourth node will create a new chain
		blockchain, success := bc.NewBlockchain(known_ports) // Create new blockchain
		if success {
			fmt.Println("Successfully created a new Blockchain")
			// Once a blockchain is created, broadcast it to all peers.
			if !node.BroadcastNewChain(known_ports, blockchain) {
				fmt.Printf("Node %s could not broadcast to peers. Stop registration.\n", node.Port)
				return // could not broadcast to peers. Stop registration.
			}
		}

		// NEXT: If more than 4 peers exist, call CopyBlockchain instead.

		/* Set the node's blockchain field */
		node.Blockchain = *blockchain
	} else { // First node
		node.Port = strconv.Itoa(chosen_port) // Set the node's port to initial port 5000
	}

	// Add the node to the list
	help.RegisterPort(node.Port, NodeList)

	registration_mutex.Unlock()

	fmt.Fprintf(&OUT, "Successfully registered a Node at port %s\n", node.Port)

	// Set the NodeList and UserList constants
	NODE_LIST = NodeList
	USER_LIST = UserList

	go node.StartListening(OUT)
}

/* Send the new chain to all peers */
func (node *Node) BroadcastNewChain(known_ports []string, chain *bc.Blockchain) bool {

	/* Marshall blockchain into JSON */
	jsonBytes, err := json.Marshal(chain)
	if help.Check(err) {
		return false
	}

	/* Iterate over all known nodes */
	for _, peer_port := range known_ports {

		// Skip the sending node's port
		if peer_port == node.Port {
			continue
		}

		// Create url using the peer's port
		url := LOCALHOST + peer_port

		// Create the request
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
		if help.Check(err) {
			return false
		}

		// Set the request's header to JSON
		req.Header.Set("Content-Type", "application/json")

		// Set the request's URI to /new_chain
		req.URL.Path = NEW_CHAIN

		client := &http.Client{}
		// Send request, and wait for a response
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error getting response from : %v\n", url)
			return false
		}

		fmt.Printf("Sent /new_chain to %s\n Received %d from %s\n", url, resp.StatusCode, url)

		if help.Check(err) {
			return false
		} else {
			// No errors, so close the connection.
			resp.Body.Close()
		}
	}

	return true
}
