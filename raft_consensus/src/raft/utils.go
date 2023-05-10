package raft

import (
	"log"
)

// `Activate` -- this method operates on your Raft peer struct and initiates functionality
// to allow the Raft peer to interact with others.  before the peer is activated, it can
// have internal algorithm state, but it cannot make remote calls using its stubs or receive
// remote calls using its underlying remote.Service interface.  in essence, when not activated,
// the Raft peer is "sleeping" from the perspective of any other Raft peer.
//
// this method is used exclusively by the Controller whenever it needs to "wake up" the Raft
// peer and allow it to start interacting with other Raft peers.  this is used to emulate
// connecting a new peer to the network or recovery of a previously failed peer.
//
// when this method is called, the Raft peer should do whatever is necessary to enable its
// remote.Service interface to support remote calls from other Raft peers as soon as the method
// returns (i.e., if it takes time for the remote.Service to start, this method should not
// return until that happens).  the method should not otherwise block the Controller, so it may
// be useful to spawn go routines from this method to handle the on-going operation of the Raft
// peer until the remote.Service stops.
//
// given an instance `rf` of your Raft peer struct, the Controller will call this method
// as `rf.Activate()`, so you should define this method accordingly. NOTE: this is _not_
// a remote call using the `remote.Service` interface of the Raft peer.  it uses direct
// method calls from the Controller, and is used purely for the purposes of the test code.
// you should not be using this method for any messaging between Raft peers.
func (peer *RaftPeer) Activate() {

	/* Start the peer's remote service */
	err := peer.service.Start()
	if err != nil {
		log.Printf("Error in Service.Start(): %s", err.Error())
		return
	}

	/* Start service and set peer's activated field to true (should be safe from deadlock the "activated" field in a peer) */
	peer.Mutex.Lock()
	/* Set peer's active field to true, to check before sending remote calls. */
	peer.active = true
	peer.Mutex.Unlock()

	prettyPrint(Client, "P%v activated", peer.ID)

	/* Wait for electionTimeout, in another thread. */
	go peer.Dispatcher()
}

// `Deactivate` -- this method performs the "inverse" operation to `Activate`, namely to emulate
// disconnection / failure of the Raft peer.  when called, the Raft peer should effectively "go
// to sleep", meaning it should stop its underlying remote.Service interface, including shutting
// down the listening socket, causing any further remote calls to this Raft peer to fail due to
// connection error.  when deactivated, a Raft peer should not make or receive any remote calls,
// and any execution of the Raft protocol should effectively pause.  however, local state should
// be maintained, meaning if a Raft node was the LEADER when it was deactivated, it should still
// believe it is the leader when it reactivates.
//
// given an instance `rf` of your Raft peer struct, the Controller will call this method
// as `rf.Deactivate()`, so you should define this method accordingly. Similar notes / details
// apply here as with `Activate`
//
// TODO: implement the `Deactivate` method
func (peer *RaftPeer) Deactivate() {
	/* Stop the peer's remote service. */
	peer.Mutex.Lock()

	/* Stop service and set peer's activated field to false */
	peer.service.Stop()

	peer.active = false // Set peer to inactive

	/* Send message to break out of loop */
	peer.killChannel <- true

	peer.Mutex.Unlock()

	prettyPrint(Client, "P%v deactivated", peer.ID)
}
