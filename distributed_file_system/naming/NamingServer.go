/*

This is the implementation of a Naming Server as part of a Distributed Filesystem
(DFS) in Golang. Our naming server follows the API as described in API_Naming_Service.md,
API_Naming_Registration.md and API_Storage_Command.md is shared with storage servers.

This Naming Server handles HTTP POST calls according to the API from Clients and
Storage Servers. The clients are simulated by the tests in Java. The whole system
communicates using JSON objects and is tested in such a manner that Naming Server
and Storage Server could be implemented in different languages as long as they follow the API.

Naming Server and the whole DFS, in fact, expect well behaved participants
and has low security and robustness against misbehaving or malicious
participants. This is meant as a demo and further limitations are described below.

To start the Naming Server simply run this pseudo command line:
	`go run NamingServer.go arg0 arg1`
where arg0 is the Service Port and arg1 is the Registration Port
and it will start listening for registering storage servers and
client requests. Outputs can be printed to console in a normal go run
however if running `make test`, then the java tests will run the Naming
Server in threads, so you will not be able to view comments, simply output to
the designated output files SERVICE_OUT and REGISTRATION_OUT. This is also
where all errors are printed.

---------------------------Location Structure of DFS: ---------------------------
The Naming Server holds a representation of the entire location structure of the DFS
and uses it to coordinate storage servers and clients. All client write and reads
should first be requested via the Naming Server, but nothing enforces this.

A location can be a directory or a file, distinguished in the name of the location.
A location contains sublocations that are also locations. This way, the NAMING_SERVER
only maintains the root Location, from which it can travers and explore all other
locations on the DFS.

---------------------------Design Limitations: ---------------------------
This DFS design for a Naming Server assumes well behaved clients and
storage servers. Clients and storage server implementations should not be
compromised and follow a strict policy. Some of the policies are discernable
from the "DANGER NOTE" comments over API request handling conditionals
and other functions.

Here are some suggestions for future design characteristics for the Naming Server:
	- The UnlockLocation() and LockLocation() functions assumes the lock's path
	  is a valid existing path.
	- UnlockLockation() and LockLocation() functions utilize defensive locking.
	- All the DANGER NOTEs are opportunities to make this more secure and trustless
	  and robust against malicious clients.
	- Clients and storage servers should be vetted using somekind of secure boot
	  and attestation using a trusted PKI.
	- Economic incentives could be used alongside consensus algorithms to fully decentralize
	  the system and make it robust against attacks on Naming Server as single source of failure.
	- Ensure that clients and storage servers are well behaved, punish those who misbehave.
	- Locks/Unlocks should be able to be transmitted to a storage server, to stop a client from
	  replaying writes and reads without getting a lock from Naming Server first.

*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

/* Global Variables and Constants */
var NAMING_SERVER *NamingServer
var mu sync.Mutex
var access_mu sync.Mutex

/* Output files for logs */
var SERVICE_OUT os.File
var REGISTRATION_OUT os.File

/* API Commands for Naming Server*/
const PROTOCOL string = "tcp"
const IS_VALID_PATH string = "/is_valid_path"
const REGISTER string = "/register"
const LIST string = "/list"
const IS_DIRECTORY string = "/is_directory"
const CREATE_DIRECTORY string = "/create_directory"
const CREATE_FILE string = "/create_file"
const GET_STORAGE string = "/get_storage"
const LOCK string = "/lock"
const UNLOCK string = "/unlock"
const DELETE string = "/delete"

// This is a Helper function used for easy printing of &Location
// nested in arrays and structs.
func (l *Location) String() string {
	subLocs := make([]string, len(l.subLocations))
	for i, subLoc := range l.subLocations {
		subLocs[i] = subLoc.name
	}
	return fmt.Sprintf("%s{%s}", l.name, strings.Join(subLocs, ", "))
}

/*
This is a Naming Server, only one instance NAMING_SERVER is
created in the main function, and used throughout this file
in managing Client requests and Storage Server registration.
*/
type NamingServer struct {

	/* Is Server running? */
	running bool

	/* Network listeners */
	serviceListener net.Listener // For client requests
	servicePort     string

	// For Storage Server registration requests
	registrationListener net.Listener
	registrationPort     string

	/* List of storage servers that registered */
	registry []StorageServer

	/* Root of DFS directory tree. */
	root *Location

	/* A map of all files on system and how many times they have been accessed */
	access_counts map[string]int
}

/* Functions Related to File System, paths and locations */

// Represents a location in the DFS
// A file or directory are differentiated by name
type Location struct {
	name         string      // Location's name
	subLocations []*Location // Directory or file, distinguished by their name
	locks        []Lock      // List of locks currently held by location

	/*
		List of locks currently held and waiting to hold location.
		All locks are added to this queue in a first-com, first serve fashion.
	*/
	lock_queue []Lock

	/* Used to index the locks as they are added to the queue. Only incremented. */
	num_locks int

	/* Array of EL indeces that are waiting to lock this location */
	exclusive_locks_waiting []int //
}

/*
Increment the access count. Create the map entry if it does not exist
Then call /storage_copy on all storage server's except file owner.

File owner is the storage server that registered with the file.
*/
func Increment_Access_Count(file string) {
	access_mu.Lock()

	// check if the map contains a certain string key
	if _, ok := NAMING_SERVER.access_counts[file]; ok {

		// The map contains the key '$file'
		// Increment access count
		NAMING_SERVER.access_counts[file]++
	} else {
		// The map does not contain the key '$file'"
		// Create the entry.
		NAMING_SERVER.access_counts[file] = 1
	}

	if NAMING_SERVER.access_counts[file] >= 20 {
		NAMING_SERVER.access_counts[file] = 0 // Reset access count
		access_mu.Unlock()
		CallStorageCopy(file) // Call storage copy on all storage servers, except file owner
		return
	}
	access_mu.Unlock()
}

/*
Send the delete command to all Storage Servers,
but when all == false, send to storage servers that are not
the file's owner.
*/
func SendDelete(file string, all bool) {
	fmt.Fprintf(&SERVICE_OUT, "Sending /storage_delete here\n")

	owner_command_port := 0
	ports := []int{}

	/* Find which Storage Server owns which file */
	for _, ss := range NAMING_SERVER.registry {
		ports = append(ports, ss.CommandPort)

		for _, f := range ss.Files {
			if f == file {
				owner_command_port = ss.CommandPort
				fmt.Fprintf(&SERVICE_OUT, "Found owner: %v\n", owner_command_port)
			}
		}
	}

	if len(NAMING_SERVER.registry) > 1 {

		/* Create a PathRequest an object */
		req_obj := PathRequest{PathString: file}

		/* Marshall request object */
		jsonBytes, err := json.Marshal(req_obj)
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error encoding JSON: %v\n", err)
			return
		}

		// For each port
		for _, port := range ports {

			// if all == true, include owner's port
			// else all == false, skip owner's port
			if port == owner_command_port && !all {
				continue
			}

			// Store the command port of ever storage server
			requestURL := fmt.Sprintf("http://localhost:%d", port)

			// Create the request
			req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonBytes))
			if err != nil {
				fmt.Fprintf(&SERVICE_OUT, "Error creating HTTP request: %v\n", err)
				return
			}

			// Set the request's header to JSON
			req.Header.Set("Content-Type", "application/json")

			// Set the request's URI
			req.URL.Path = "/storage_delete"

			client := &http.Client{}
			// Send request, then wait for a response
			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(&SERVICE_OUT, "Error sending HTTP request: %v\n", err)
				return
			}
			fmt.Fprintf(&SERVICE_OUT, "Sent /storage_delete to %d\n", port)
			resp.Body.Close()
		}
	}
}

/* Call storage copy as per API */
func CallStorageCopy(file string) {

	owner_port := 0 // Port of storage server that owns file
	owner_command_port := 0
	ports := []int{}

	/* Find which Storage Server owns which file */
	for _, ss := range NAMING_SERVER.registry {
		ports = append(ports, ss.CommandPort)

		for _, f := range ss.Files {
			if f == file {
				owner_port = ss.ClientPort          // Get Storage Server's Client Port
				owner_command_port = ss.CommandPort // Get Storage Server's Command Port
			}
		}
	}

	/* If there are storage servers in the registry and owner's port exists*/
	if len(NAMING_SERVER.registry) > 1 && owner_port != 0 {

		/* Get the storage copy as an object */
		req_obj := StorageCopy{Path: file, ServerIP: "127.0.0.1", ServerPort: owner_port}

		/* Marshall request object */
		jsonBytes, err := json.Marshal(req_obj)
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error encoding JSON: %v\n", err)
			return
		}

		// For each storage server
		for _, port := range ports {

			if port == owner_command_port {
				continue
			}

			// Store the command port of ever storage server
			requestURL := fmt.Sprintf("http://localhost:%d", port)

			// Create the request
			req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonBytes))
			if err != nil {
				fmt.Fprintf(&SERVICE_OUT, "Error creating HTTP request: %v\n", err)
				return
			}

			// Set the request's header to JSON
			req.Header.Set("Content-Type", "application/json")

			// Set the request's URI
			req.URL.Path = "/storage_copy"

			client := &http.Client{}
			// Send request, then wait for a response
			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(&SERVICE_OUT, "Error sending HTTP request: %v\n", err)
				return
			}
			resp.Body.Close()
		}
	}
}

// return false if path.Path is empty string,
// doesnt start with delimiter or string contains a colon.
func IsPathValid(path string) bool {
	return len(path) > 0 && path[0] == '/' && !strings.Contains(path, ":")
}

/*
Returns true if the given parent directory contains a file.
*/
func ContainsX(parentDirectory []string, x string) bool {
	for _, location := range parentDirectory {
		if strings.Contains(location, x) {
			return true
		}
	}

	return false
}

/* Pop a lock off this location's queue. */
func (currentLocation *Location) Pop(unlock Lock) {
	if len(currentLocation.lock_queue) == 0 {
		return
	}

	currentLock := currentLocation.locks[0]     // Get lock on current location
	topOfQueue := currentLocation.lock_queue[0] // Get top of the current location's lock queue

	/*Can safely pop SL from queue IF:
	unlock, currentLock and topOfQueue are SL
	*/
	if !unlock.Exclusive && !currentLock.Exclusive && !topOfQueue.Exclusive {
		currentLocation.lock_queue = currentLocation.lock_queue[1:]
		// fmt.Fprintf(&SERVICE_OUT, "Popped lock%v from Queue: %v\n", unlock, currentLocation.lock_queue)
		return
	}

	if unlock.Exclusive && topOfQueue.Exclusive && currentLock.Exclusive {
		currentLocation.lock_queue = currentLocation.lock_queue[1:]
		currentLocation.exclusive_locks_waiting = currentLocation.exclusive_locks_waiting[1:]
		// fmt.Fprintf(&SERVICE_OUT, "Popped lock%v from Queue: %v\n", unlock, currentLocation.lock_queue)
		return
	}
}

/*
Return a list of files found at this Directory location.
*/
func (currentLocation *Location) GetContentsAt(locationNames []string, ret *[]string) {

	isFinalLocation := len(locationNames) == 1
	// Base case, we are the location
	if isFinalLocation {
		lastLocation := locationNames[0]

		// If we at the root
		if currentLocation.name == "/" {
			// If we want to list the root contents
			if lastLocation == currentLocation.name {
				for _, sub := range currentLocation.subLocations {
					// Collect root sublocation names
					*ret = append(*ret, sub.name)
				}

				return // Exit as we have listed root
			}
		}

		// If we are not listing root
		for _, sub := range currentLocation.subLocations {
			// If the location we are listing is a currentLocation's sublocation
			if sub.name == lastLocation {
				for _, subSub := range sub.subLocations {
					// Collect all sublocations of the currentLocation sub
					*ret = append(*ret, subSub.name)
				}
			}
		}

		return // Exit because this is final location
	}

	// Else we have not reached final location
	for _, sub := range currentLocation.subLocations {
		midwayLocation := locationNames[0]

		if sub.name == midwayLocation {
			// Get the rest of the locations, without this midwayLocation
			restOfLocations := append(locationNames[:0], locationNames[1:]...)
			sub.GetContentsAt(restOfLocations, ret)
		}
	}
}

/*
Returns true if location already exists; false otherwise.
This function expects the input ret to be false.
*/
func (currentLocation *Location) LocationExists(locationNames []string, ret *bool) {

	// Base case, final location on path was reached
	isFinalLocation := len(locationNames) == 1
	if isFinalLocation && len(currentLocation.subLocations) > 0 {
		finalLocation := locationNames[0] // Get the final location
		// For each sub location
		for _, sub := range currentLocation.subLocations {
			if finalLocation == sub.name {
				*ret = true // Set ret to true (because this is a recursive func in golang)
				return      // Found location
			}
		}
		*ret = false // Exited forloop, could not find location
		return       // Location does not exist
	}

	// else, this is a midway location
	midwayLocation := locationNames[0]

	// For each sublocation
	for _, sub := range currentLocation.subLocations {
		// If midway is in sublocations
		if midwayLocation == sub.name {
			// Get the rest of the locations, without this midwayLocation
			restOfLocations := append(locationNames[:0], locationNames[1:]...)
			sub.LocationExists(restOfLocations, ret) // Recurse over midway location
		}
	}
}

/*
Returns true if file or directory path does not already exist
and creates a new location;

Return false if path already exists.
*/
func (currentLocation *Location) CheckNewPath(locationNames []string, idx int) bool {
	// If no sub locations,
	if len(currentLocation.subLocations) == 0 {
		// just add new locations
		currentLocation.AppendNewLocation(locationNames)
		return true // No Conflict
	}

	// Check if we have reached the final location on path
	isFinalLocation := (idx == (len(locationNames) - 1))

	// Base case, reached final location in "new path"
	if isFinalLocation {
		// Get final location of the "new path"
		finalLocation := locationNames[len(locationNames)-1]

		/*
			Itterate over current sublocations
		*/
		for _, sub := range currentLocation.subLocations {
			// Is sublocation conflicting with finalLocation in "new path"?
			if finalLocation == sub.name {
				return false // finalLocation already exists
			}
		}
		// fmt.Fprintf(&REGISTRATION_OUT, "Final Location; No Conflicts\n")
		// No location conflicts were found from root to finalLocation
		currentLocation.AppendNewLocation([]string{finalLocation})

		return true // No Conflics were found
	}

	/* Not finalLocation, so get the midway location */
	midwayLocation := locationNames[idx]

	/*
		Itterate over current sublocations
	*/
	for _, sub := range currentLocation.subLocations {
		// Is there a sublocation with the same name as the midwayLocation?
		if midwayLocation == sub.name {
			// Yes, then increase index and run again
			return sub.CheckNewPath(locationNames, idx+1)
		}
	}

	/*
		Exited forloop without finding a location with the same
		name as the midway location. No conflicting locations.
	*/

	// Path starts from midway location
	locationNames = locationNames[idx:]
	// Recursively create a new path without worrying about conflicts
	currentLocation.AppendNewLocation(locationNames)

	// Midway location did not exist, so No Conflicts.
	return true
}

/* Create a new path, without worrying about conflicts */
func (currentLocation *Location) AppendNewLocation(locationNames []string) {

	// Base case, last location to append
	if len(locationNames) == 1 {
		// Append final location
		newFinalLocation := &Location{name: locationNames[0], locks: []Lock{}}
		currentLocation.subLocations = append(currentLocation.subLocations, newFinalLocation)
		fmt.Fprintf(&SERVICE_OUT, "Appending location %v\n", newFinalLocation)
		return // Return
	}

	/* If not last location, then we are midway */
	midwayLocation := locationNames[0]

	if currentLocation.name == midwayLocation {
		otherLocations := append(locationNames[:0], locationNames[1:]...) // Remove 0th element
		currentLocation.AppendNewLocation(otherLocations)                 // Recursive call on the rest of the path
		return                                                            // Exit
	}

	/* Run recursive call on a sub location with the same name. */
	for _, sub := range currentLocation.subLocations {
		// If there exists a sublocation with the same name
		if sub.name == midwayLocation {
			otherLocations := append(locationNames[:0], locationNames[1:]...)
			sub.AppendNewLocation(otherLocations) // Recursive call on the rest of the path
			return                                // Exit
		}
	}

	// Outside of the loop, there is no midway location with the same name
	// So create it.
	newMidwayLocation := &Location{name: midwayLocation, locks: []Lock{}}
	currentLocation.subLocations = append(currentLocation.subLocations, newMidwayLocation)

	/* Run recursive call on a sub location with the same name. */
	for _, sub := range currentLocation.subLocations {
		// If there exists a sublocation with the same name
		if sub.name == midwayLocation {
			// fmt.Fprintf(&REGISTRATION_OUT, "Midway Location /%s already exists\n", midwayLocation)
			otherLocations := append(locationNames[:0], locationNames[1:]...)
			sub.AppendNewLocation(otherLocations) // Recursive call on the rest of the path
			return                                // Exit
		}
	}
}

/*
This function sends a create file command to the first storage server
in NAMING_SERVER's registry.

Returns true if API call responded with success == true, false otherwise
*/
func (naming_server *NamingServer) CreateFileOnStorage(path PathRequest) bool {

	// If there are storage servers in the NAMING_SERVER's registry
	if len(naming_server.registry) > 0 {

		// Get storage server's command port & create request url
		command_port := naming_server.registry[0].CommandPort
		requestURL := fmt.Sprintf("http://localhost:%d", command_port)

		// JSON encode the path object
		jsonBytes, err := json.Marshal(path)
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error encoding JSON: %v", err)
			return false
		}

		// Create the request
		req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error creating HTTP request: %v", err)
			return false
		}

		// Set the request's header to JSON
		req.Header.Set("Content-Type", "application/json")

		// Set the request's URI
		req.URL.Path = "/storage_create"

		client := &http.Client{} // Initialize an http client
		// Send request, then wait for a response
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error sending HTTP request: %v", err)
			return false
		}
		fmt.Fprintf(&SERVICE_OUT, "Sent request to storage server: %v", req)

		// Close the connection once done
		defer resp.Body.Close()

		var response ServiceResponse
		err = json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "Error decoding JSON: %v", err)
			return false
		}

		return response.Success // Return whether response was successful or not
	}
	return false
}

/*
This function starts at root, navigates to the path in the lock,
then locks that location.

Locations can only be locked if they are not already locked exclusively (write lock),
otherwise the lock is queued until location is available for locking.

A single location can have multiple read locks, but only one write lock
at a time.

When a location gets locked for any kind of access,
all objects along the path to that object, including the root directory,
must be locked for shared access.

ret is set to true if this function is successful.
*/
func (currentLocation *Location) LockLocation(lock Lock, idx int, ret *bool) {
	// Split path string into locations, remove the empty element at beginning of arr
	locations := strings.Split(lock.PathString, "/")[1:]

	// Base case, found location to lock
	if currentLocation.name == lock.PathString || locations[len(locations)-1] == currentLocation.name {

		currentLocation.num_locks++                  // Increment the number of locks
		lock.queue_index = currentLocation.num_locks // Set the lock's queue index

		/* Append lock to queue */
		mu.Lock()
		currentLocation.lock_queue = append(currentLocation.lock_queue, lock)

		/* Store the EL's index */
		if lock.Exclusive {
			currentLocation.exclusive_locks_waiting = append(currentLocation.exclusive_locks_waiting, lock.queue_index)
		}

		/* Handle Read lock encounter first. */
		// If there is one lock or more on this location
		// and it's not an exclusive locks
		if !lock.Exclusive && len(currentLocation.exclusive_locks_waiting) < 1 && len(currentLocation.locks) >= 1 && !currentLocation.locks[0].Exclusive {
			// Append new read lock
			currentLocation.locks = append(currentLocation.locks, lock)
			mu.Unlock()
			*ret = true
			return
		}

		// If this location already has a lock
		if len(currentLocation.locks) > 0 {
			mu.Unlock()

			for {
				// If there are no more locks on this location
				mu.Lock()
				if len(currentLocation.locks) == 0 {
					topOfQueue := lock // Initialize top of queue variable

					if len(currentLocation.lock_queue) > 0 {
						topOfQueue = currentLocation.lock_queue[0]
					}

					// Only top of the queue ELs are allowed.
					if topOfQueue.Exclusive && lock.queue_index != topOfQueue.queue_index {
						mu.Unlock()
						continue
					}

					// If top of the queue is SL and EL is in queue
					// Do not surpass EL's index
					if !topOfQueue.Exclusive && len(currentLocation.exclusive_locks_waiting) > 0 && lock.queue_index >= currentLocation.exclusive_locks_waiting[0] {
						mu.Unlock()
						continue
					}

					// Append new read or write lock
					currentLocation.locks = append(currentLocation.locks, lock)
					mu.Unlock()
					*ret = true
					return
				}

				// If there is an SL on this location
				if len(currentLocation.locks) > 0 && !currentLocation.locks[0].Exclusive {

					// Ensure they do no surpass index of EL in queue
					if len(currentLocation.exclusive_locks_waiting) > 0 && lock.queue_index >= currentLocation.exclusive_locks_waiting[0] {
						mu.Unlock()
						continue
					}

					// Append new read or write lock
					currentLocation.locks = append(currentLocation.locks, lock)
					mu.Unlock()
					*ret = true
					return
				}
				mu.Unlock()
			}
		}

		// Append new read or write lock
		currentLocation.locks = append(currentLocation.locks, lock)
		mu.Unlock()
		*ret = true
		return
	}

	nextLocation := locations[idx]

	// For each sublocation
	for _, sub := range currentLocation.subLocations {

		// Find the sublocation with the same name as next location
		if nextLocation == sub.name {

			// If current location is exclusively locked,
			// wait until it becomes free
			mu.Lock()
			if len(currentLocation.locks) > 0 && len(currentLocation.locks) == 1 && currentLocation.locks[0].Exclusive {
				mu.Unlock()

				fmt.Fprintf(&SERVICE_OUT, "Waiting for %v to be free...", currentLocation.name)

				for {
					mu.Lock()
					if len(currentLocation.locks) == 0 {
						mu.Unlock()
						break
					}
					mu.Unlock()
				}

				sl := Lock{PathString: currentLocation.name, Exclusive: false}

				currentLocation.num_locks++                // Increment the number of locks
				sl.queue_index = currentLocation.num_locks // Set the lock's queue index

				/* Append lock to queue */
				currentLocation.lock_queue = append(currentLocation.lock_queue, sl)
				fmt.Fprintf(&SERVICE_OUT, "\nAdded lock%v to Queue\n\n", sl)

				/* Actually lock this location */
				currentLocation.locks = append(currentLocation.locks, sl) // Locking all locations along the path
				fmt.Fprintf(&SERVICE_OUT, "Midway location %v has New Read Lock appended\n", currentLocation.name)

				/* Recurse */
				sub.LockLocation(lock, idx+1, ret)
				return // return after recursing
			}
			mu.Unlock()

			/*
				API:
				"When a client requests that any object be locked for any kind of access,
				all objects along the path to that object, including the root directory,
				must be locked for shared access."
			*/

			/* Create shared lock */
			sl := Lock{PathString: currentLocation.name, Exclusive: false}

			currentLocation.num_locks++                // Increment the number of locks
			sl.queue_index = currentLocation.num_locks // Set the lock's queue index

			/* Append lock to queue */
			currentLocation.lock_queue = append(currentLocation.lock_queue, sl)
			fmt.Fprintf(&SERVICE_OUT, "\nAdded lock%v to Queue\n\n", sl)

			/* Actually lock this location */
			currentLocation.locks = append(currentLocation.locks, sl) // Locking all locations along the path
			fmt.Fprintf(&SERVICE_OUT, "Midway location %v has New Read Lock appended\n", currentLocation.name)

			/* Recurse */
			sub.LockLocation(lock, idx+1, ret)
			return // return after recursing
		}
	}
}

/*
This function starts at root, recursively navigates to the path to be unlocked,
then unlocks it if it is the correct unlock. Assumes location exists.

Along the way, it removes a single read lock per midway location, assuming it finds
a read lock there.

ret is set to true if this function is successful.
*/
func (currentLocation *Location) UnlockLocation(unlock Lock, idx int, ret *bool) {

	// Split path string into locations, remove the empty element at beginning of arr
	locations := strings.Split(unlock.PathString, "/")[1:]

	/* Case 1:
	-> pathString is root or reached the final location to unlock
	*/
	if currentLocation.name == unlock.PathString || currentLocation.name == locations[len(locations)-1] {

		/* Handle Non-Exclusive unlocks first */
		// Read unlock and final location has One or multiple read locks
		mu.Lock()
		if !unlock.Exclusive && len(currentLocation.locks) >= 1 && !currentLocation.locks[0].Exclusive {

			currentLocation.Pop(unlock) // Pop thhis lock off the queue

			currentLocation.locks = currentLocation.locks[1:] // Unlock this location
			mu.Unlock()
			*ret = true
			return
		}

		/* Handle Exclusive unlocks */
		// Exclusive unlock and final location has one exclusive lock
		if unlock.Exclusive && len(currentLocation.locks) == 1 && currentLocation.locks[0].Exclusive {
			currentLocation.Pop(unlock)                       // Pop this lock off the queue
			currentLocation.locks = currentLocation.locks[1:] // Unlock location
			mu.Unlock()
			*ret = true
			return
		}
		mu.Unlock()

		return // Exit
	}

	/* Case 2:
	-> Search for next location, remove one lock from current location
	*/
	nextLocation := locations[idx]
	// For each sublocation
	for _, sub := range currentLocation.subLocations {
		// Search and find the next sub location
		if sub.name == nextLocation {

			/* Handle current location before recursing on sub location. */

			// If current location has a read lock on it, remove it
			mu.Lock()
			if len(currentLocation.locks) >= 1 && !currentLocation.locks[0].Exclusive {
				currentLocation.Pop(currentLocation.locks[0])     // XD Pop lock
				currentLocation.locks = currentLocation.locks[1:] // Remove one read lock from this location
				mu.Unlock()
				/* Recurse to next sublocation */
				sub.UnlockLocation(unlock, idx+1, ret)
				return // return after recursing
			}
			mu.Unlock()

			/* Recurse to next sublocation */
			sub.UnlockLocation(unlock, idx+1, ret)
			return // return after recursing
		}
	}
}

/* JSON structs according to API */

type PathRequest struct {
	PathString string `json:"path"`
}

type StorageServer struct {
	StorageIP   string   `json:"storage_ip"`
	ClientPort  int      `json:"client_port"`
	CommandPort int      `json:"command_port"`
	Files       []string `json:"files"`
}

type StorageCopy struct {
	Path       string `json:"path"`
	ServerIP   string `json:"server_ip"`
	ServerPort int    `json:"server_port"`
}

type RegistrationResponse struct {
	Files []string `json:"files"`
}

type ListSuccessfulResponse struct {
	Files []string `json:"files"`
}

type ExceptionResponse struct {
	ExceptionType string `json:"exception_type"`
	ExceptionInfo string `json:"exception_info"`
}

type ServiceResponse struct {
	Success bool `json:"success"`
}

type StorageInfo struct {
	ServerIP   string `json:"server_ip"`
	ServerPort int    `json:"server_port"`
}

type Lock struct {
	PathString  string `json:"path"`
	Exclusive   bool   `json:"exclusive"`
	queue_index int
}

/* The next set of functions all deal with handling requests and running the server */

/*
Start a NamingServer.
*/
func (serv *NamingServer) Start() {
	// Return an error if the service is already started
	if serv.running {
		return
	}

	/* Otherwise, start the service. */

	// Set the Service's running boolean to true
	serv.running = true
	// Create a listener
	ln1, err := net.Listen(PROTOCOL, serv.servicePort)
	if err != nil { // Catch error while creating listener
		log.Println("Error listening on PORT", serv.servicePort, err)
		return
	}
	// Set the server's serviceListener
	serv.serviceListener = ln1

	// Create a second listener
	ln2, err := net.Listen(PROTOCOL, serv.registrationPort)
	// Catch error while creating listener
	if err != nil {
		log.Println("Error listening on PORT", serv.registrationPort, err)
		return
	}
	// Set the server's registrationListener
	serv.registrationListener = ln2

	// Start serving registration requests
	go StartRegistration(serv)

	// Start serving client requests
	StartService(serv)

	return
}

/*
Start the NamingServer's Registration Listener and server http requests.
*/
func StartRegistration(serv *NamingServer) {

	/* Define a request handler function using HandleServiceConnection */
	handler := func(w http.ResponseWriter, r *http.Request) {
		HandleRegistration(w, r)
	}

	fmt.Fprintf(&REGISTRATION_OUT, "Listening on "+serv.registrationPort+" for Registration Requests...\n")

	// Serve the HTTP request using the registration listener and handler function
	err := http.Serve(serv.registrationListener, http.HandlerFunc(handler))
	if err != nil {
		fmt.Fprintln(&REGISTRATION_OUT, "ERROR:", err)
	}
}

/*
Handler function for http registration requests.

DANGER NOTE: Registration assumes well behaved storage servers.
Registration is best done when there is not heavy usage of the file system.
*/
func HandleRegistration(w http.ResponseWriter, r *http.Request) {
	/* Check if valid Register command was sent */
	if r.RequestURI == REGISTER {

		/* Get the StorageServer object from the json request */
		var storage_server StorageServer
		err := json.NewDecoder(r.Body).Decode(&storage_server) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&REGISTRATION_OUT, "ERROR: %v\n", err)
		}

		// For each currently registered server
		for _, ss := range NAMING_SERVER.registry {
			// If StorageServer is already registerd,
			if ss.ClientPort == storage_server.ClientPort || ss.CommandPort == storage_server.CommandPort {
				w.Header().Set("Content-Type", "application/json")
				// TODO: Make sure code is 409
				w.WriteHeader(http.StatusConflict)
				// Send a bad registration response, in accordance with API.
				response := ExceptionResponse{
					ExceptionType: "IllegalStateException",
					ExceptionInfo: "This storage server is already registered.",
				}
				// fmt.Fprintf(&REGISTRATION_OUT, "409 Conflict %v\n", response)
				json.NewEncoder(w).Encode(response)
				return
			}
		}

		filesToDelete := []string{}

		// If files are not empty,
		if len(storage_server.Files) > 0 {
			// For each file path in the storage server's files
			for _, filePath := range storage_server.Files {

				filePath = strings.TrimLeft(filePath, "/") // trim first slash
				locations := strings.Split(filePath, "/")  // split locations by delimiter

				// Check if filePath is a new path or not
				// isNewPath := NAMING_SERVER.root.CheckNewPath(locations, 0)
				isValidPath := NAMING_SERVER.root.CheckNewPath(locations, 0)

				if !isValidPath {
					filePath := "/" + filePath
					filesToDelete = append(filesToDelete, filePath)
					fmt.Fprintf(&REGISTRATION_OUT, "Invalid Path: %s\n", filePath)
				} else {
					fmt.Fprint(&REGISTRATION_OUT, "NEW ROOT: ", NAMING_SERVER.root.subLocations, "\n")
				}
			}

		}

		NAMING_SERVER.registry = append(NAMING_SERVER.registry, storage_server) // Register storage server

		/* Handle response */
		w.Header().Set("Content-Type", "application/json")
		response := RegistrationResponse{Files: filesToDelete}
		fmt.Fprintf(&REGISTRATION_OUT, "Response to registration: %v\n", response)
		json.NewEncoder(w).Encode(response)
		return // Exit, 200, No files to delete
	}

	// Respond with 400 Bad Request, if the command is unknown.
	http.Error(w, "Unknown Command", http.StatusBadRequest)
}

/*
Start the NamingServer's Service Listener and handle client http requests.
*/
func StartService(serv *NamingServer) {

	// Define a request handler function using HandleServiceConnection
	handler := func(w http.ResponseWriter, r *http.Request) {
		HandleServiceCommand(w, r)
	}

	fmt.Fprintf(&SERVICE_OUT, "Listening on "+serv.servicePort+" for Service Requests...\n")

	// Serve the HTTP request using the service listener and handler function
	err := http.Serve(serv.serviceListener, http.HandlerFunc(handler))
	if err != nil {
		fmt.Fprintln(&SERVICE_OUT, "ERROR:", err)
	}
}

/*
Handler function for http client requests.
DANGER NOTE: Registration assumes well behaved clients.
*/
func HandleServiceCommand(w http.ResponseWriter, r *http.Request) {

	fmt.Fprintf(&SERVICE_OUT, "\n---------------Received %v command---------------\n", r.RequestURI)

	// If the command is /is_valid_path
	if r.RequestURI == IS_VALID_PATH {
		/* Get the path from the request */
		var req PathRequest
		err := json.NewDecoder(r.Body).Decode(&req) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR! %s\n", err)
			return
		}
		path := req.PathString // The path string sent by client

		// Check if NOT valid path or contains a colon
		if !IsPathValid(path) {
			/* Respond to invalid path request */
			fmt.Fprintf(&SERVICE_OUT, "Invalid path:%v\n", req)
			w.Header().Set("Content-Type", "application/json")
			response := ServiceResponse{Success: false} // Success == false
			json.NewEncoder(w).Encode(response)         // json encoding the response object
			return
		}

		/* Respond to valid path request */
		w.Header().Set("Content-Type", "application/json")
		response := ServiceResponse{Success: true} // Success == true
		json.NewEncoder(w).Encode(response)        // json encoding the response object
		return
	}

	// If the command is /is_directory
	// DANGER NOTE: The parent directory should be locked for shared access before
	// this operation is performed, to prevent the file/directory in question
	// from being deleted or created while this call is in progress.
	if r.RequestURI == IS_DIRECTORY {

		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		pathString := strings.TrimLeft(path.PathString, "/") // trim first slash
		locations := strings.Split(pathString, "/")          // split locations by delimiter

		/* Check if path is valid */
		if !IsPathValid(path.PathString) {
			fmt.Fprintf(&SERVICE_OUT, "Invalid path:%v\n", path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory is not a valid path.",
			}
			fmt.Fprintf(&SERVICE_OUT, "Sending: %v\n", w)
			json.NewEncoder(w).Encode(response)
			return
		}

		/* Check if path exists */
		locationExists := false
		NAMING_SERVER.root.LocationExists(locations, &locationExists)
		if locationExists || path.PathString == "/" {
			// If path leads to a file, then respond with {success: false}
			if strings.Contains(locations[len(locations)-1], "file") {
				/* Object exists but is NOT directory*/
				fmt.Fprintf(&SERVICE_OUT, "Not a directory!: %v\n", path)
				w.Header().Set("Content-Type", "application/json")
				response := ServiceResponse{Success: false} // Success == false
				json.NewEncoder(w).Encode(response)
				return
			}

			/* Object exists and is directory*/
			fmt.Fprintf(&SERVICE_OUT, "Location %v Exists!\n", path)
			w.Header().Set("Content-Type", "application/json")
			response := ServiceResponse{Success: true} // Response{Success == true}
			json.NewEncoder(w).Encode(response)
			return
		} else {
			/* Directory does NOT exist*/
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "FileNotFoundException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			fmt.Fprintf(&SERVICE_OUT, "Sending: %v\n", w)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// If the command is /list
	// DANGER NOTE: The directory should be locked for shared access before this
	// operation is performed, to allow for safe reading of the directory contents.
	if r.RequestURI == LIST {
		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(path.PathString) {
			fmt.Fprintf(&SERVICE_OUT, "Invalid path: %v\n", path)
			// respond with success = false
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		pathString := strings.TrimLeft(path.PathString, "/") // trim first slash
		locations := strings.Split(pathString, "/")          // split locations by delimiter

		locationExists := false // Initialize to false

		// This will set the locationExists bool to true if location exists
		NAMING_SERVER.root.LocationExists(locations, &locationExists)

		/* If the location exists or the path requested is root*/
		if locationExists || path.PathString == "/" {

			finalLocation := locations[len(locations)-1] // The end of the path

			/* If final location is NOT a Directory */
			if !strings.Contains(finalLocation, "directory") && path.PathString != "/" {
				fmt.Fprintf(&SERVICE_OUT, "File is not Directory: %v\n", path)
				// respond with {Success = false}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound) // 404
				response := ExceptionResponse{
					ExceptionType: "FileNotFoundException",
					ExceptionInfo: "the file/directory or parent directory does not exist.",
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			content := []string{} // Populate with content names

			// If client is asking to list root contents
			if path.PathString == "/" {
				// This is because of the way we handle delimiters
				NAMING_SERVER.root.GetContentsAt([]string{"/"}, &content)
			} else { // If client is not asking to list root contents
				NAMING_SERVER.root.GetContentsAt(locations, &content)
			}

			fmt.Fprintf(&SERVICE_OUT, "Files at %s are: %v", path.PathString, content)

			/* Path requested is existing directory respond with contents */
			w.Header().Set("Content-Type", "application/json")
			response := ListSuccessfulResponse{Files: content}
			json.NewEncoder(w).Encode(response)
			return
		} else {
			/*Location does not exist; path string is not root.*/
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "FileNotFoundException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// If the command is /create_directory
	// DANGER NOTE: The parent directory of the new directory should be locked for
	// exclusive access before this operation is performed.
	if r.RequestURI == CREATE_DIRECTORY {
		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		pathStringTrimmed := strings.TrimLeft(path.PathString, "/") // trim first slash
		locations := strings.Split(pathStringTrimmed, "/")          // split locations by delimiter

		/* Handle an invalid pathString */
		if !IsPathValid(path.PathString) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		isRoot := false // Initialize as false

		// If parent directory exists.
		if len(locations) > 1 {
			parentExists := false // Initialize to false

			var parentDirectory []string // Initialize the parent directory variable

			// Get a copy of the parent directory
			parentDirectory = make([]string, len(locations)-1)
			copy(parentDirectory, locations[:len(locations)-1])

			// Set the parentDirectoryExists bool to true if parent directory exists
			NAMING_SERVER.root.LocationExists(parentDirectory, &parentExists)

			// If the parentDirectory does not exist or contains a file.
			if !parentExists || ContainsX(parentDirectory, "file") {
				fmt.Fprintf(&SERVICE_OUT, "File Not Found: %v\n", path)
				// Respond with {ExceptionType: "FileNotFoundException"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound) // 404
				response := ExceptionResponse{
					ExceptionType: "FileNotFoundException",
					ExceptionInfo: "the parent directory does not exist.",
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		} else {
			isRoot = path.PathString == "/" // Check if path string is root
		}

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		directoryExists := false // Initialize to false

		// Set the directoryExists bool to true if directory exists
		NAMING_SERVER.root.LocationExists(locs, &directoryExists)

		// If directory already exists
		if directoryExists || isRoot {
			fmt.Fprintf(&SERVICE_OUT, "Directory already exists!: %v\n", path)
			/* Respond with {Success: false} */
			w.Header().Set("Content-Type", "application/json")
			response := ServiceResponse{Success: false}
			json.NewEncoder(w).Encode(response)
			return
		}

		/* At this point we are free to create a new directory */

		// Create a new path, if it does not already exist.
		success := NAMING_SERVER.root.CheckNewPath(locations, 0)

		/* Respond with {Success: success}, probably true */
		w.Header().Set("Content-Type", "application/json")
		response := ServiceResponse{Success: success}
		json.NewEncoder(w).Encode(response)
		return
	}

	// DANGER NOTE: The parent directory of the new file should be locked for exclusive
	// access before this operation is performed.
	if r.RequestURI == CREATE_FILE {
		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(path.PathString) {
			fmt.Fprintf(&SERVICE_OUT, "Invalid path: %v\n", path)
			// Respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		pathString := strings.TrimLeft(path.PathString, "/") // trim first slash
		locations := strings.Split(pathString, "/")          // split locations by delimiter

		isRoot := false // Initialize as false

		// If parent directory exists.
		if len(locations) > 1 {
			parentExists := false // Initialize to false

			var parentDirectory []string // Initialize the parent directory variable

			// Get parent directory
			parentDirectory = make([]string, len(locations)-1)
			copy(parentDirectory, locations[:len(locations)-1])

			// This will set the parentDirectoryExists bool to true if parent directory exists
			NAMING_SERVER.root.LocationExists(parentDirectory, &parentExists)

			// If parent directory does not exist
			// or parent directory contains a file
			if !parentExists || ContainsX(parentDirectory, "file") {
				// Respond with {ExceptionType: "FileNotFoundException"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound) // 404
				response := ExceptionResponse{
					ExceptionType: "FileNotFoundException",
					ExceptionInfo: "the parent directory does not exist.",
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		} else {
			isRoot = path.PathString == "/" // Check if path string is root
		}

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		directoryExists := false // Initialize to false
		// This will set the directoryExists bool to true if directory exists
		NAMING_SERVER.root.LocationExists(locs, &directoryExists)

		// If directory already exists
		if directoryExists || isRoot {
			/* Respond with {Success: false} */
			w.Header().Set("Content-Type", "application/json")
			response := ServiceResponse{Success: false}
			json.NewEncoder(w).Encode(response)
			return
		}

		/* At this point we are free to create a new directory */
		createdNewPath := NAMING_SERVER.root.CheckNewPath(locations, 0)

		if createdNewPath {
			if NAMING_SERVER.CreateFileOnStorage(path) {
				//TODO: send /storage_copy to all other StorageServers
			}
		}

		/* Respond with {Success: success}, probably true */
		w.Header().Set("Content-Type", "application/json")
		response := ServiceResponse{Success: createdNewPath}
		json.NewEncoder(w).Encode(response)
		return
	}

	// If client requests a storage server's port.
	// DANGER NOTE: If the client intends to perform `read` or `size` commands,
	// it should lock the file for shared access before making this call;
	// if it intends to perform a `write` command, it should lock the file
	// for exclusive access.
	if r.RequestURI == GET_STORAGE {

		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(path.PathString) {
			// Respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		locations := strings.Split(path.PathString, "/")[1:] // split locations by delimiter

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		locationExists := false // Initialize as false

		// Recursively check if location exists
		NAMING_SERVER.root.LocationExists(locs, &locationExists)

		// If the location does not exist or the final location is a directory
		if !locationExists || strings.Contains(locations[len(locations)-1], "directory") {
			fmt.Fprintf(&SERVICE_OUT, "Location not found: %v\n", path)
			// respond with {Success = false}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "FileNotFoundException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		for _, storage_server := range NAMING_SERVER.registry {
			for _, file := range storage_server.Files {
				if file == path.PathString {
					fmt.Fprintf(&SERVICE_OUT, "Storage %d owns %s", storage_server.ClientPort, path.PathString)
					w.Header().Set("Content-Type", "application/json")
					response := StorageInfo{
						ServerIP:   storage_server.StorageIP,
						ServerPort: storage_server.ClientPort,
					}
					json.NewEncoder(w).Encode(response)
					return
				}
			}
		}
	}

	// Handle locking
	// This is a blocking request call, it only responds back when lock is acquired
	if r.RequestURI == LOCK {
		/* Get the StorageServer object from the json request */
		var lock Lock
		err := json.NewDecoder(r.Body).Decode(&lock) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(lock.PathString) {
			// Respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		locations := strings.Split(lock.PathString, "/")[1:] // split locations by delimiter

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		locationExists := false // Initialize as false

		// Check if location exists
		NAMING_SERVER.root.LocationExists(locs, &locationExists)

		// If location does not exist
		if !locationExists && lock.PathString != "/" {
			fmt.Fprintf(&SERVICE_OUT, "Location not found: %v\n", lock)
			// respond with {ExceptionType: "FileNotFoundException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "FileNotFoundException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		successfullyLocked := false // Initialize boolean

		// Set boolean above to true if location is successfully locked
		NAMING_SERVER.root.LockLocation(lock, 0, &successfullyLocked)

		if successfullyLocked {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(&SERVICE_OUT, "Successfully locked!\n")
			return
		} else {
			return // Unlikely outcome, since function is blocking
		}
	}

	// Handle unlocking
	if r.RequestURI == UNLOCK {
		/* Get the StorageServer object from the json request */
		var lock Lock
		err := json.NewDecoder(r.Body).Decode(&lock) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(lock.PathString) {
			// Respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		locations := strings.Split(lock.PathString, "/")[1:] // split locations by delimiter

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		locationExists := false // Initialize as false

		NAMING_SERVER.root.LocationExists(locs, &locationExists)

		// If location does not exist
		if !locationExists && lock.PathString != "/" {
			// respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		successfullyUnlocked := false

		NAMING_SERVER.root.UnlockLocation(lock, 0, &successfullyUnlocked)

		if successfullyUnlocked {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Increment access counts and send deletes if access count >= 20
			Increment_Access_Count(lock.PathString)

			// If lock was exclusive
			if lock.Exclusive {
				// Delete it from all storage servers,
				// except owner's.
				SendDelete(lock.PathString, false)
			}
			return // Exit
		} else {
			// Unlikely to fail to unlock.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Handle delete command
	if r.RequestURI == DELETE {
		/* Get the StorageServer object from the json request */
		var path PathRequest
		err := json.NewDecoder(r.Body).Decode(&path) // Decode the request's body
		if err != nil {
			fmt.Fprintf(&SERVICE_OUT, "ERROR: %v\n", err)
		}

		/* Handle an invalid pathString */
		if !IsPathValid(path.PathString) {
			fmt.Fprintf(&SERVICE_OUT, "Invalid path: %v\n", path)
			// Respond with {ExceptionType: "IllegalArgumentException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "IllegalArgumentException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		locations := strings.Split(path.PathString, "/")[1:] // split locations by delimiter

		// Get a copy of locations
		locs := make([]string, len(locations))
		copy(locs, locations)

		locationExists := false // Initialize as false

		NAMING_SERVER.root.LocationExists(locs, &locationExists)

		// If location does not exist
		if !locationExists && path.PathString != "/" {
			fmt.Fprintf(&SERVICE_OUT, "Location not found: %v\n", path)
			// respond with {ExceptionType: "FileNotFoundException"}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			response := ExceptionResponse{
				ExceptionType: "FileNotFoundException",
				ExceptionInfo: "the file/directory or parent directory does not exist.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Send delete to all storage servers
		SendDelete(path.PathString, true)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := ServiceResponse{Success: true}
		json.NewEncoder(w).Encode(response)
		return
	}

	/* Respond with 400 Bad Request, if the command is unknown. */
	http.Error(w, "Unknown Command", http.StatusBadRequest)
}

func main() {
	/* Create a new file output to log service logs. */
	file, err := os.OpenFile("output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()
	SERVICE_OUT = *file

	/* Create a new file output to log registration logs. */
	file2, err := os.OpenFile("output2.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer file2.Close()
	REGISTRATION_OUT = *file2

	/*
		Get arguments in the form `go run NamingServer.go arg0 arg1`,
		where arg0 is the Service Port and arg1 is the Registration Port
	*/
	args := os.Args[1:]

	// Create a NamingServer struct
	NAMING_SERVER = &NamingServer{
		servicePort:      "127.0.0.1:" + args[0],
		registrationPort: "127.0.0.1:" + args[1],
		running:          false,
		root:             &Location{name: "/", locks: []Lock{}},
		access_counts:    map[string]int{},
	}

	fmt.Fprint(&SERVICE_OUT, "\n----------------------------**Starting a NamingServer**----------------------------\n")
	fmt.Fprint(&REGISTRATION_OUT, "\n----------------------------**Starting a NamingServer**----------------------------\n")

	NAMING_SERVER.Start() // Start the naming server
}
