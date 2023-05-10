// 14-736 Lab 2 Raft implementation in go

/*
	This is an implementation of the Raft consensus protocol. [https://raft.github.io/raft.pdf]
	By Parardha Kumar and David Mberingabo.

	We use channels and time.After to handle election timeouts and heartbeat timeouts. This is handled in Dispatcher.

	We use our remote library to run RPC calls, such as AppendEntries and RequestVotes.

	Potential Failures:
		1. This implementation was not tested or implemented for a live network with live, sovereign peers.
		2. It was also not tested to measure what its scalability is and what costs are incurred
		   as the numbers of peers grow.
		3. We also have not tested our code for malicious peers or any security in the consensus protocol.
*/

package raft

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	rpc "../remote"
)

// Given two ints a and b, return the smaller int
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

/* Start of RAFT Constants */
const RAFT_HEARTBEAT = 150 * time.Millisecond
const RAFT_IP_ADDRESS = "127.0.0.1:"
const LEADER = "LEADER"
const CANDIDATE = "CANDIDATE"
const FOLLOWER = "FOLLOWER"

var GlobalMutex sync.Mutex

/* End of RAFT Constants */

type logTopic string

const (
	Client    logTopic = "CLNT"
	Candidate logTopic = "CNDT"
	Commit    logTopic = "CMIT"
	Drop      logTopic = "DROP"
	Error     logTopic = "ERRO"
	Info      logTopic = "INFO"
	Leader    logTopic = "LEAD"
	Log       logTopic = "LOG1"
	Log2      logTopic = "LOG2"
	Control   logTopic = "CTRL"
	Response  logTopic = "RESP"
	Term      logTopic = "TERM"
	Test      logTopic = "TEST"
	Timer     logTopic = "TIMR"
	Trace     logTopic = "TRCE"
	Vote      logTopic = "VOTE"
	Peer      logTopic = "PEER"
)

/* PrettyPrint logic */
var debugStart time.Time
var debugVerbosity int

// Retrieve the verbosity level from an environment variable; Used by PrettyPrint
func getVerbosity() int {
	v := os.Getenv("VERBOSE")
	level := 0
	if v != "" {
		var err error
		level, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("Invalid verbosity %v", v)
		}
	}
	return level
}

func init() {
	debugStart = time.Now()
	debugVerbosity = getVerbosity()
	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
}

func prettyPrint(topic logTopic, format string, a ...interface{}) {
	if debugVerbosity < 1 {
		return
	}
	time := time.Since(debugStart).Microseconds()
	time /= 100
	prefix := fmt.Sprintf("\n%06d %v ", time, string(topic))
	format = prefix + format + "|pretty print"
	log.Printf(format, a...)
}

// StatusReport struct sent from Raft node to Controller in response to command and status requests.
// this is needed by the Controller, so do not change it. make sure you give it to the Controller
// when requested
type StatusReport struct {
	Index     int
	Term      int
	Leader    bool
	CallCount int
}

// This is an entry that is logged onto the Raft network
type LogEntry struct {
	Term        int // Term this entry was created
	Command     int
	Index       int // Starting from 1
	IsCommitted bool
	commitCount int
}

/*
RaftInterface -- this is the "service interface" that is implemented by each Raft peer using the
remote library from Lab 1.  it supports five remote methods that you must define and implement.
*/
type RaftInterface struct {
	RequestVote     func(term int, candidateId int, lastLogIndex int, lastLogTerm int) (int, bool, rpc.RemoteObjectError)                                        // TODO: define function type
	AppendEntries   func(leaderTerm int, leaderID int, prevLogIndex int, prevLogTerm int, entry []LogEntry, leaderCommit int) (int, bool, rpc.RemoteObjectError) // TODO: define function type
	GetCommittedCmd func(int) (int, rpc.RemoteObjectError)
	GetStatus       func() (StatusReport, rpc.RemoteObjectError)
	NewCommand      func(int) (StatusReport, rpc.RemoteObjectError)
}

/*
A struct defining a unique peer in Raft.
A RaftPeer implements all the functions in the RaftInterface.
A RaftPeer can be created, activated and deactivated by the controller.
*/
type RaftPeer struct {
	port     int
	ID       int
	numPeers int // Number of known peers
	role     string
	active   bool // True if peer is active
	service  *rpc.Service

	peerStubs map[int]*RaftInterface // Array of stub peers
	Mutex     sync.Mutex             // This peer's mutex

	/* Persistent state */
	currentTerm int
	votedFor    int // -1 if peer has not voted for anyone

	leaderId int // The ID of this peer's leader

	// Channel used to signal the end/start of a heartbeat timer
	heartbeatChannel chan bool
	// Channel used to signal deactivation of a peer
	killChannel chan bool

	logEntries []LogEntry // This peer's logs

	/* These fields are set/updated by AppendEntry() */
	lastLogTerm int

	/* This is volatile state, which can be inconsistent with agreed upon state */
	commitIndex int // Our latest known committed index

	lastLogIndex int
	lastCommit   int
	nextIndex    []int // Initialized as 1 for all peers
	matchIndex   []int
}

/*
Remove entries in peer's log, from prevIndex onward,
used in AppendEntries().
*/
func removeElements(slice []LogEntry, prevIndex int) []LogEntry {
	// Create a new slice with only the desired elements
	newSlice := append(slice[:prevIndex+1], []LogEntry{}...)
	return newSlice
}

/*
Dispatcher deals with a peer's deactivation, timeouts and corresponding role switches,
using channels.
*/
func (peer *RaftPeer) Dispatcher() {
	for { // Infinite forloop
		peer.Mutex.Lock()
		role := peer.role        // Get peer's role
		term := peer.currentTerm // Get peer's current term
		peer.Mutex.Unlock()
		prettyPrint(Info, "P%d current term: %v", peer.ID, term)

		switch role {
		case LEADER:
			select {
			case <-peer.killChannel: // If peer is deactivated, exit infinite loop
				return
			case <-time.After(RAFT_HEARTBEAT): // After heartbeat timeout
				// Send Empty Heartbeat
				go peer.SendHeartbeat(-1) // Send Empty Heartbeat
			}

		case FOLLOWER:
			select {
			case <-peer.killChannel: // If peer is deactivated, exit infinite loop
				return
			case <-peer.heartbeatChannel: // Peer received a heartbeat from legit leader
			// Do nothing, restart loop (i.e. restart Election Timer)
			case <-time.After(RandomElectionTimeoutDuration()): // After election timeout
				peer.Mutex.Lock()
				peer.currentTerm += 1 // Increment term
				peer.leaderId = -1    // Stop recognizing any leaders
				prettyPrint(Client, "P%d Role changed to CANDIDATE", peer.ID)
				peer.role = CANDIDATE   // Set peer's role to Candidate
				peer.votedFor = peer.ID // Vote for self
				peer.Mutex.Unlock()
				peer.LeaderElection() // Start leader election
			}

		case CANDIDATE:
			select {
			case <-peer.killChannel: // If peer is deactivated, exit infinite loop
				return
			case <-peer.heartbeatChannel: // Peer received a heartbeat from legit Leader
				peer.Mutex.Lock()
				prettyPrint(Timer, "P%d Heartbeat timeout ended in term %v", peer.ID, peer.currentTerm)
				peer.role = FOLLOWER // Candidate received heartbeat and acknowledged legit Leader
				peer.Mutex.Unlock()
			case <-time.After(RandomElectionTimeoutDuration()): // After election timeout -> re-election
				peer.Mutex.Lock()
				prettyPrint(Timer, "P%d Election timeout ended in term %v", peer.ID, peer.currentTerm)
				peer.currentTerm += 1   // Increment term
				peer.votedFor = peer.ID // Vote for self
				peer.Mutex.Unlock()
				peer.LeaderElection() // Start leader election, again
			}
		}
	}

}

/*
Wrapper function ran in go routines to call AppendEntries on stubs, repeatedly, until
leader gets a valid response from stub peer, gets deactivated or switches roles.
*/
func (leader *RaftPeer) CallAppendEntries(peerId int, entryIndex int) {

	/* Initialize some variables */
	entry := []LogEntry{}
	successful := false
	leader.Mutex.Lock()
	peerStub := leader.peerStubs[peerId] // Get stub peer

	// Calculate lastLogIndex from nextIndex of this stub peer
	prevLogIndex := leader.nextIndex[peerId] - 1
	// Get the length of the leader's logs
	lastLogIndex := len(leader.logEntries)
	leader.Mutex.Unlock()

	// While Leader has not received a Success reply
	for !successful && lastLogIndex >= prevLogIndex {
		leader.Mutex.Lock()
		leaderTerm := leader.currentTerm        // Get the Leader's term
		leaderCommitIndex := leader.commitIndex // Get the Leader's latest committed index

		prevLogIndex = leader.nextIndex[peerId] - 1         // Decrement prevLogindex
		prevLogTerm := leader.logEntries[prevLogIndex].Term // Get the term of the entry at prevLogIndex

		if entryIndex != -1 { // If this is not an empty heartbeat
			entry = leader.logEntries[prevLogIndex+1:] // Get the entry
		}
		leader.Mutex.Unlock()

		/* If candidate is not active or candidate is not a FOLLOWER, do not send remote calls. */
		leader.Mutex.Lock()
		if leader.role != LEADER || !leader.active {
			leader.Mutex.Unlock()
			return
		}
		leader.Mutex.Unlock()

		/* Send AppendEntrie RPC */
		term, success, roe := peerStub.AppendEntries(leaderTerm, leader.ID, prevLogIndex, prevLogTerm, entry, leaderCommitIndex)
		if (roe != rpc.RemoteObjectError{}) { // Handle Remote Object Error
			return
		}

		successful = success // If peet stub replied true, forloop will exit at end

		if !successful { // If peer stub retplied false
			leader.Mutex.Lock()

			/* If RPC request or response contains term T > currentTerm  (§5.1) */
			if term > leader.currentTerm {
				leader.currentTerm = term // set currentTerm = T
				leader.role = FOLLOWER    // and convert to Follower.
				leader.Mutex.Unlock()
				return
			} else {
				if entryIndex != -1 {
					leader.nextIndex[peerId]-- // Decrement nextIndex
				}
			}
			leader.Mutex.Unlock()
		}
	}
	leader.Mutex.Lock()

	// At this point, RPC was Successful

	// Set this peer stub's nextIndex to the Leader's last log entry
	leader.nextIndex[peerId] = len(leader.logEntries)
	// Set this peer stub's nextIndex to its nextIndex - 1
	leader.matchIndex[peerId] = leader.nextIndex[peerId] - 1

	leader.Mutex.Unlock()

}

// Append Entries to all the Followers
func (leader *RaftPeer) SendHeartbeat(entryIndex int) {
	prettyPrint(Leader, "P%d Sending HBs to all servers", leader.ID)

	for peerId := range leader.peerStubs {

		/* If candidate is not active or candidate is not a FOLLOWER, do not send remote calls. */
		leader.Mutex.Lock()
		if !leader.active || !(leader.role == LEADER) {
			leader.Mutex.Unlock()
			return
		}
		leader.Mutex.Unlock()

		prettyPrint(Client, "P%d Sending Heartbeat to %d", leader.ID, peerId)

		// Handle each peer's AppendEntries operations in a go routine
		leader.CallAppendEntries(peerId, entryIndex)
	}

	leader.Mutex.Lock()
	/* If candidate is not active or candidate is not a FOLLOWER, do not send remote calls. */
	if !leader.active || !(leader.role == LEADER) {
		leader.Mutex.Unlock()
		return
	}

	commitIndex := leader.commitIndex // Get the Leader's latest committed index
	commitCount := 1                  // Initialize the commit count

	for i := commitIndex + 1; i < len(leader.logEntries); i++ {

		for mIndex := 0; mIndex < leader.numPeers; mIndex++ {
			if leader.matchIndex[mIndex] >= i && leader.logEntries[i].Term == leader.currentTerm {
				commitCount++ // Increment commit count
			}
		}

		// If commit count is greater than majority, set new commit index for Leader
		if commitCount > leader.numPeers/2 {
			commitIndex = i
		}
	}

	leader.commitIndex = commitIndex

	leader.Mutex.Unlock()
}

// This function executes when election timer runs out
func (candidate *RaftPeer) LeaderElection() {

	prettyPrint(Client, "P%d is starting leader election", candidate.ID)

	votes := 1                                          // Total number of votes amassed
	for peerId, peerStub := range candidate.peerStubs { // For each peer stub

		/* If candidate is not active or candidate is not a FOLLOWER, do not send remote calls. */
		candidate.Mutex.Lock()
		if !candidate.active || !(candidate.role == CANDIDATE) {
			candidate.Mutex.Unlock()
			return
		}

		candidateCurrentTerm := candidate.currentTerm // Get the candidate's term

		// Get Candidate's lastLogIndex
		candidateLastLogIndex := len(candidate.logEntries) - 1 // 0
		// Get the Entry's term at lastLogIndex
		candidateLastLogTerm := candidate.logEntries[candidateLastLogIndex].Term
		candidate.Mutex.Unlock()

		/* Send RequestVote RPC */
		term, voteGranted, roe := peerStub.RequestVote(candidateCurrentTerm, candidate.ID, candidateLastLogIndex, candidateLastLogTerm)
		if (roe != rpc.RemoteObjectError{}) { // Handle errors
			prettyPrint(Client, "P%v:Error in calling RequestVote RPC for Peer %v", candidate.ID, peerId)
			continue
		}

		if voteGranted {
			votes++                           // Increment vote count
			if votes > candidate.numPeers/2 { // If votes received exceed majority
				candidate.Mutex.Lock()
				candidate.role = LEADER           // become leader
				candidate.leaderId = candidate.ID // enforce self as leader
				candidate.Mutex.Unlock()
				candidate.SendHeartbeat(-1) // Send Empty Heartbeats to all followers
				return
			}
		}

		candidate.Mutex.Lock()
		/* If RPC request or response contains term T > currentTerm  (§5.1) */
		if term > candidate.currentTerm {
			candidate.currentTerm = term // set currentTerm = T
			candidate.role = FOLLOWER    // and convert to follower.
			candidate.Mutex.Unlock()
			return
		}
		candidate.Mutex.Unlock()
	}

}

/* Returns a random time duration between 500-650ms */
func RandomElectionTimeoutDuration() time.Duration {
	return time.Duration(rand.Intn(150)+500) * time.Millisecond
}

// `NewRaftPeer` -- this method should create an instance of the above struct and return a pointer
// to it back to the Controller, which calls this method.  this allows the Controller to create,
// interact with, and control the configuration as needed.  this method takes three parameters:
// -- port: this is the service port number where this Raft peer will listen for incoming messages
// -- id: this is the ID (or index) of this Raft peer in the peer group, ranging from 0 to num-1
// -- num: this is the number of Raft peers in the peer group (num > id)
func NewRaftPeer(port int, id int, num int) *RaftPeer {
	rand.Seed(time.Now().UnixNano()) // Set the random seed

	/* Initialize new peer's fields */
	peer := RaftPeer{
		port:             port,
		ID:               id,
		numPeers:         num,
		active:           false,
		votedFor:         -1,
		role:             FOLLOWER,
		currentTerm:      0,
		killChannel:      make(chan bool),
		heartbeatChannel: make(chan bool),
		logEntries:       []LogEntry{},
		nextIndex:        make([]int, num),
		matchIndex:       make([]int, num),
	}
	for i := 0; i < num; i++ {
		peer.nextIndex[i] = 1 // Set each stub peer's nextIndex
	}

	// Initialize log with empty log at array position 0
	peer.logEntries = append(peer.logEntries, LogEntry{Term: 0, Index: 0})

	// Create a new remote service attached to this peer
	s, err := rpc.NewService(&RaftInterface{}, &peer, port, false, false)
	if err != nil {
		log.Printf(err.Error())
	}
	peer.service = s // set the peer's service

	/* Populate the peer stubs */
	peer.peerStubs = map[int]*RaftInterface{} // Initialize array of peer stubs
	for peerId := 0; peerId < num; peerId++ {
		if peerId == id {
			continue
		}
		peerStub := &RaftInterface{}
		PORT := port - id + peerId // Calculate peer's port
		peerAddress := RAFT_IP_ADDRESS + strconv.Itoa(PORT)
		err := rpc.StubFactory(peerStub, peerAddress, false, false) // Create a stub peer
		if err != nil {
			log.Printf("Error Creating Peer Stub for Peer %d", peerId)
			continue
		}
		peer.peerStubs[peerId] = peerStub
	}

	return &peer
}

/*
	RequestVote -- this is one of the remote calls defined in the Raft paper, and it should be
		supported as such.  you will need to include whatever argument types are needed per the Raft
		algorithm, and you can package the return values however you like, as long as the last return
		type is `remote.RemoteObjectError`, since that is required for the remote library use.
*/

func (peer *RaftPeer) RequestVote(candidateTerm int, candidateId int, lastLogIndex int, lastLogTerm int) (int, bool, rpc.RemoteObjectError) {

	prettyPrint(Client, "P%d is requesting vote from %d for Term %d", candidateId, peer.ID, candidateTerm)

	peer.Mutex.Lock()
	/*  Reply false if term < currentTerm (Figure 2) */
	if candidateTerm < peer.currentTerm {
		peerCurrentTerm := peer.currentTerm
		prettyPrint(Client, "P%d denied vote to %d. Reason : Candidate Term = %d  < Peer.CurrentTerm = %d", peer.ID, candidateId, candidateTerm, peer.currentTerm)
		peer.Mutex.Unlock()
		return peerCurrentTerm, false, rpc.RemoteObjectError{}
	}

	/*
		If RPC request contains term T > currentTerm: (decided to go with greater or equal)
	*/
	if candidateTerm > peer.currentTerm {
		peer.currentTerm = candidateTerm // Adopt Candidate's term
		peer.role = FOLLOWER             // Become a Follower
		peer.votedFor = -1               // Do not recognize previous votes
	}

	peerLastLogIndex := len(peer.logEntries) - 1
	// If peer's last entry's term is larger than the new entry's term
	// or if they are equal, but our lastLogIndex is larger, then reply false.
	if peer.logEntries[peerLastLogIndex].Term > lastLogTerm || (peer.logEntries[peerLastLogIndex].Term == lastLogTerm && peerLastLogIndex > lastLogIndex) {
		peerCurrentTerm := peer.currentTerm
		peer.Mutex.Unlock()
		return peerCurrentTerm, false, rpc.RemoteObjectError{} // reply false
	}

	// If the peer hasnt voted for candidate already
	// and if peer already has voted for someone else
	if peer.votedFor != candidateId && peer.votedFor != -1 {
		peerCurrentTerm := peer.currentTerm
		peer.Mutex.Unlock()
		return peerCurrentTerm, false, rpc.RemoteObjectError{} // Reply false
	}

	// At this point, Candidate is legit.
	peer.votedFor = candidateId // Vote for candidate
	prettyPrint(Client, "P%d denied vote to %d . Reason: Peer VotedFor -> %v", peer.ID, candidateId, peer.votedFor)
	peerCurrentTerm := peer.currentTerm
	peer.Mutex.Unlock()

	return peerCurrentTerm, true, rpc.RemoteObjectError{} // Reply true
}

/*
	AppendEntries -- this is one of the remote calls defined in the Raft paper, and it should be
		supported as such and defined in a similar manner to RequestVote above.

		Returns this peer's current term and a bool that is true if follower contained entry matching
		prevLogIndex and prevLogTerm
*/

func (peer *RaftPeer) AppendEntries(leaderTerm int, leaderID int, prevLogIndex int, prevLogTerm int, entry []LogEntry, commitIndex int) (int, bool, rpc.RemoteObjectError) {
	prettyPrint(Client, "P%d received HB from %v", peer.ID, leaderID)

	peer.Mutex.Lock()
	currentTerm := peer.currentTerm
	peer.Mutex.Unlock()

	/*
		Reply false if term < currentTerm (Figure 2 & §5.2 (b))
	*/
	if leaderTerm < currentTerm {
		prettyPrint(Client, "P%d timer not reset, hearbeat leader was not legitimate", peer.ID)
		return currentTerm, false, rpc.RemoteObjectError{}
	}

	/*
		At this point terms are either equal or Leader has larger term
	*/
	peer.Mutex.Lock()
	peer.currentTerm = leaderTerm // Enforce the leader's term (§5.1)
	peer.role = FOLLOWER          // Enforce that peer is a Follower
	peer.votedFor = leaderID      // Accept leader

	// If this is not an empty heartbeat
	if len(entry) != 0 {

		// Check that Leader's logs are at least as-up-to-date as peer's, before appending,
		// If not as-up-to-date (as defined in the Raft paper), then reply false.
		if prevLogIndex >= len(peer.logEntries) || prevLogTerm != peer.logEntries[prevLogIndex].Term {
			peerCurrentTerm := peer.currentTerm
			peer.Mutex.Unlock()
			return peerCurrentTerm, false, rpc.RemoteObjectError{}
		}

		// Delete anything from prevLogIndex + 1 onwards
		peer.logEntries = removeElements(peer.logEntries, prevLogIndex)

		// Append each of the entries sent by the leader
		for i := 0; i < len(entry); i++ {
			prettyPrint(Error, "P%d successfully appended Entry %v", peer.ID, entry)
			peer.logEntries = append(peer.logEntries, entry[i])
		}
	}

	peer.Mutex.Unlock()

	peer.heartbeatChannel <- true // Successful Heartbeat, reset timeout

	peer.Mutex.Lock()
	// If Leader's commit index is larger
	if commitIndex > peer.commitIndex {
		peer.commitIndex = min(len(peer.logEntries)-1, commitIndex) // Update peer's commit index
	}
	peerCurrentTerm := peer.currentTerm
	peer.Mutex.Unlock()

	return peerCurrentTerm, true, rpc.RemoteObjectError{}
}

/*
GetStatus -- this is a remote call that is used by the Controller to collect status information

	about the Raft peer.  the struct type that it returns is defined above, and it must be implemented
	as given, or the Controller and test code will not function correctly.  This method takes no arguments and is essentially
	a "getter" for the state of the Raft peer, including the Raft peer's current term, current last
	log index, role in the Raft algorithm, and total number of remote calls handled since starting.
	the method returns a `StatusReport` struct as defined at the top of this file.
*/
func (peer *RaftPeer) GetStatus() (StatusReport, rpc.RemoteObjectError) {
	peer.Mutex.Lock()
	index := len(peer.logEntries) - 1
	term := peer.currentTerm
	leader := peer.role == LEADER // Is peer Leader?
	numCallsReceived := peer.service.GetCount()
	peer.Mutex.Unlock()

	report := StatusReport{Index: index, Term: term, Leader: leader, CallCount: numCallsReceived}

	return report, rpc.RemoteObjectError{}
}

/*
NewCommand -- called (only) by the Controller.  this method emulates submission of a new command

	by a Raft client to this Raft peer, which should be handled and processed according to the rules
	of the Raft algorithm.  once handled, the Raft peer should return a `StatusReport` struct with
	the updated status after the new command was handled.
*/
func (peer *RaftPeer) NewCommand(command int) (StatusReport, rpc.RemoteObjectError) {

	/* If the peer is not a leader then redirect the command to the leader*/
	peer.Mutex.Lock()
	if peer.role != LEADER || !peer.active {
		peer.Mutex.Unlock()
		return peer.GetStatus()
	}
	peer.Mutex.Unlock()

	/* 1. Append Command to own log */
	peer.Mutex.Lock()

	/* Create LogEntry */
	index := len(peer.logEntries)

	entry := LogEntry{Term: peer.currentTerm, Command: command, IsCommitted: false, Index: index, commitCount: 1}

	/* Append of list of logEntries */
	peer.logEntries = append(peer.logEntries, entry)

	/* Update Leader's last commit index */
	if peer.lastCommit == 0 {
		peer.lastCommit = 1
	}
	peer.Mutex.Unlock()

	/* 2. Issue Append Entry (retry Append Entry Indefinetely) */
	peer.SendHeartbeat(index)

	return peer.GetStatus()
}

// GetCommittedCmd -- called (only) by the Controller.  this method provides an input argument
// `index`.  if the Raft peer has a log entry at the given `index`, and that log entry has been
// committed (per the Raft algorithm), then the command stored in the log entry should be returned
// to the Controller.  otherwise, the Raft peer should return the value 0, which is not a valid
// command number and indicates that no committed log entry exists at that index
func (peer *RaftPeer) GetCommittedCmd(index int) (int, rpc.RemoteObjectError) {
	peer.Mutex.Lock()
	if index <= peer.commitIndex {
		logEntry := peer.logEntries[index] // Get peer's log entry at index
		peer.Mutex.Unlock()
		return logEntry.Command, rpc.RemoteObjectError{} // Send command inside the entry
	} else {
		peer.Mutex.Unlock()
		return 0, rpc.RemoteObjectError{}
	}
}
