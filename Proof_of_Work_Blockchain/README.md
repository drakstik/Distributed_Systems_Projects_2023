# Proof of Work Blockchain Demo
This is a Go implementation of PoW Blockchain. 

See the PDF (Proof of Work Blockchain in Go.pdf File) for more detailed description.

### Registration
Successful registration of nodes results in a /tmp/NodeList.txt being created, which is a simple representation of a PKI.
Nodes are represented by their port. Each node picks a port number above 5000.
Nodes achieve consensus via broadcast messages, so the list of nodes is always checked before broadcasting.

Once the node list reaches a minimum non-trivial number of nodes (4 nodes), then a new blockchain is created by the 4th node with the function NewBlockchain(), which spawns a genesis block at position 0. This function does not work if there are fewer than 4 nodes. This blockchain is automatically broadcasted to all peers as the init blockchain. All other nodes from that point must copy the blockchain from peers and adopt the majority blockchain.

### Using the Blockchain
A user also registers in order to access the network by adding its port to /tmp/UserList.txt. This could be useful in the future if content is addressed to other users or to track users' actions across time (like a wallet). Once registered, users can send content to a random set of nodes, which must race to build a block, find the block's nonce and appropriate hash, in the Proof of Work procedure. Once a node completes a Proof of Work, it can send it to peers to validate the blockchain and accept it or reject it. A block is accepted when received for validation, if it is valid. A valid block has: 
a. an index greater than the current blockchain's last index and 
b. a valid Proof of Work, and 
c. it must be a new block, never seen before by the network. 
If one of these features is not there, then the block must be rejected. 

## Instructions to Run

git clone git@github.com:cmu14736/s23-lab4-commitcrew.git

git checkout final

cd s23-lab4-commitcrew/project/

go run main (You might want to use sudo if you face permission issues)

Run the Test Cases:
go test

Note: We have implemented actual block mining which is a resource intensive process even for small blockchains (One of the reasons that tilts POW toward a practically Byzantine Fault Tolerant System). Sometimes the blockchain takes a longer time to mine depending on the available resources on the machine, so either closing demanding tasks on the OS and/or increasing the timeout in the test cases helps resolve the error. 



### References
Our proposal:
https://docs.google.com/document/d/1FXXuP_DX8Ubkcmd217RoNdkVYJa7BgwpQ7U5dQnsVyw/edit?usp=sharing
   


