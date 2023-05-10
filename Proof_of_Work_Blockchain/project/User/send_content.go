package user

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	bc "project/Blockchain"
	help "project/Helpers"
)

/*
	A user can send content (as a string) to a random set of nodes.
*/
func (user *User) SendContent(content string) bool {
	known_nodes := help.GetPorts(NODE_LIST)

	/* Ensure there is a non-trivial number of registered nodes */
	if len(known_nodes) > bc.NON_TRIVIAL {
		// Select a set of random registered nodes
		rand_indeces := RandomSet(0, len(known_nodes)-1, numOfNodes)
		for _, rand_idx := range rand_indeces {
			// Send the content and wait for the response.
			// Continue sending to rest of nodes if the response is false.
			user.SendContentToNode(known_nodes[rand_idx], content)
		}
	} else {
		fmt.Println("User requires non-trivial number of nodes to be registered")
		return false
	}

	return true
}

/*
	Send an http request containing content to a single node.
*/
func (user *User) SendContentToNode(random_port string, content string) bool {
	// Store the command port of ever storage server
	requestURL := "http://localhost:" + random_port

	// Create Content Message
	message := Content{Content: content, User: *user}

	/* Marshall request object */
	jsonBytes, err := json.Marshal(message)
	help.Check(err)

	// Create the request
	req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonBytes))
	help.Check(err)

	// Set the request's header to JSON
	req.Header.Set("Content-Type", "application/json")

	// Set the request's URI to /content
	req.URL.Path = CONTENT

	client := &http.Client{}
	// Send request, then wait for a response
	resp, err := client.Do(req)
	notActive := help.Check(err)

	if !notActive {
		fmt.Printf("Sent /content to %s\n", random_port)
		resp.Body.Close()
	}

	return false // Response was not 200 OK
}

/*
Return a set of random numbers that are chosen from a range, without any repeating numbers in the set.

Inputs define the range and the number of random numbers to generate
*/
func RandomSet(start int, end int, count int) []int {

	// Initialize a slice with the numbers in the range
	numbers := make([]int, end-start+1)
	for i := start; i <= end; i++ {
		numbers[i-start] = i
	}

	// Shuffle the slice using the Fisher-Yates shuffle algorithm
	for i := len(numbers) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}

	// Take the first `count` numbers from the shuffled slice
	return numbers[:count]
}
