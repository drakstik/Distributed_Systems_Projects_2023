package user

import (
	"fmt"
	help "project/Helpers"
	"strconv"
)

/*
RegisterUser a user to the given user list. Must be thread safe.

Limitation:
Registration is required so users can be distinguished and only
known users can send content to the blockchain. Usually, a blockchain
would use a Private/Public key infrastructure to track user's across
time (for example, when counting wallet amounts). Instead, we use
a simple registration file on our local machine. This file is accessible
by nodes who check if the user is on the list before accepting their content.
*/
func (user *User) RegisterUser(UserList string, NodeList string) {
	registration_mutex.Lock()

	// Read from the UserList
	known_ports := help.GetPorts(UserList)

	// Initial port choice
	chosen_port := 10000

	// If there are users already registered
	if len(known_ports) > 0 {
		/* Choose a port number */
		init := known_ports[len(known_ports)-1] // Initialize port choice to last known port
		chosen_port, err := strconv.Atoi(init)  // Convert initial port choice to int
		if help.Check(err) {
			return // Error while converting to int
		}
		chosen_port++ // Increment chosen port by 1

		/* Set the user's port field */
		user.Port = strconv.Itoa(chosen_port)
	} else { // First user
		user.Port = strconv.Itoa(chosen_port) // Set the user's port to initial port 5000
	}

	// Add the user to the list
	help.RegisterPort(user.Port, UserList)

	registration_mutex.Unlock()

	// Set the NodeList and UserList constants
	USER_LIST = UserList
	NODE_LIST = NodeList

	fmt.Printf("Successfully registered a User at port %s\n", user.Port)
}

/*
Returns true if the user is registered on the UserList
*/
func (user *User) IsUserRegistered() bool {
	// Read from the NodeList
	known_ports := help.GetPorts(USER_LIST)

	for _, port := range known_ports {
		if port == user.Port {
			return true
		}
	}

	return false
}
