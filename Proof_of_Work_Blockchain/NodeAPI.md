This API describes JSON messages and commands that a blockchain node can handle using HTTP.

# Overview

A User proposes new data to a random set of Nodes in the system. 

Upon receiving new data, the Node adds the data to a block then starts mining for a nonce and corresponding block hash, or Proof of Work. While it mines, the Node may receive a request from its peer to add a block to the blockchain. The Node must validate the block and if the block gets committed, then the block that was being mined should be rejected, otherwise the Node should continue mining.

TODO: What happens if a malicious block sends a bunch of non-valid block? Right now, a validation is blocking. Must work for a hash+nonce but very fast to check. May get away with a few blocks, but not many. Malicious node could send the same hash+nonce over and over again.

A node can verify a mined block by checking that the nonce matches the corresponding valid hash.
A node can check that the hash is not repeated in the blockchain; When a node validates a block, it checks with a 2/3rd majority of peers if it can commit the block.
A node can commit a block to its blockchain once it is successfully validated/committed by a majority.

TODO: What if a node crashes during voting and majority cannot be reached?
        Periodic checkup on peers and updating of counts?
            Discount peers that are innactive for too long or that do not respond to a certain number of requests?
                How long should a Node be waiting for a response, before counting the request as failed?
                    Broadcasts should return 200 Ok, otherwise it's a fail? 3 strikes and out?

# API Summary

**Register**: Request to register as a Node or User.
**BroadcastRegistry**: Broadcast the registry of Nodes and Users to all peers.

**NewData**: Request from a user for new data to be mined into a PoW block.

**CopyBlockchain**: Request for a copy of the blockchain.
**CopyBlock**: Request for a copy of a block.
**ValidateBlock**: Request from a peer to verify and validate a mined block.


# API Definitions

## Register
The Node that receives a registration request adds the Node or User to its registry and broadcasts its registry to its peers.

## BroadcastRegistry
The Node that receives a registry broadcast should ensure that its own registry is up to date.

## NewData
A User sends a request to the network containing new data. A Node mines the data into a block and broadcasts it to validate it into the blockchain. Nodes will respond after the block containing the data has been validated into the blockchain.

TODO: Creating a block should require updated blockchain tip, or else it might constantly get rejected. Should call /copychain before validating. Should we return failure after first attempt to verify & validate, or keep trying? If we keep trying, wont we need to implement somekind of mechanism to stop Nodes from trying forever?

TODO: Increase difficulty over "time" and only accept blocks with that difficulty.

### Request
**URI**: `/newdata`
**Method**: `POST`
**Body**:
```json
{
    "data": "Alice sent 1 BTC to Bob"
}
```

### Response (Successful)
**Status** : `200 OK`
**Body** :
```json
{
    "block": {
            "prev_hash": "002a04c22e1f3639d1261e029975911f599b5fbb99ec3c7adf47523dbd7ecb6c",
            "index": "1",
            "timestamp": "1681539282306497400",
            "data": "416c6963652073656e7420312042544320746f20426f62",
            "nonce": "1439",
            "hash": "000b6dbc34b21b2b8dea526281762e6fff9723ffeb7d2cdbd932ed15f9341c9d"
        }
}
```


## CopyBlockchain
A request for a copy of the blockchain. Nodes should respond with the latest validated blockchain it is aware of.

### Request
**URI**: `/copychain`
**Method**: `POST`

### Response (Successful)
**Status** : `200 OK`
**Body** : 
```json
{
    "blocks": [
        {
            "prev_hash": "000b6dbc34b21b2b8dea526281762e6fff9723ffeb7d2cdbd932ed15f9341c9d",
            "index": "2",
            "timestamp": "1681539282311034400",
            "data": "416c6963652073656e7420332042544320746f20426f62",
            "nonce": "2973",
            "hash": "0015558a4fae5123965fd1118e99fef4387a43252aa0b6cce72a660d317e9658"
        },
        {
            "prev_hash": "002a04c22e1f3639d1261e029975911f599b5fbb99ec3c7adf47523dbd7ecb6c",
            "index": "1",
            "timestamp": "1681539282306497400",
            "data": "416c6963652073656e7420312042544320746f20426f62",
            "nonce": "1439",
            "hash": "000b6dbc34b21b2b8dea526281762e6fff9723ffeb7d2cdbd932ed15f9341c9d"
        },
        {
            "prev_hash": "",
            "index": "0",
            "timestamp": "1681539282302972200",
            "data": "47656e6573697320426c6f636b",
            "nonce": "662",
            "hash": "002a04c22e1f3639d1261e029975911f599b5fbb99ec3c7adf47523dbd7ecb6c"
        }
    ]
}
```

### Error Response
Blockchain was not found.
**Status** : `404 Not Found`

## CopyBlock

## ValidateBlock
A peer may request a Node to validate a block. The Node first verifies the nonce and block data hash appropriately. Then it checks that the hash is not being repeated in the blockchain. If the block has already been committed, the Node responds with committed=true. When these checks pass, then node validates the block and responds successfully. 

Once validated, the Node makes sure the block has indeed been committed by trying to validate the block with majority of its peers. Once the Node is sure the block is committed, it may commit it as well. 

A Node should always accept the block with the largest index. 

A Node realizes it has missed a block when the block index received for validation is greater than the length of the blockchain. It should immediately request the missing blocks from the peer that sent the block with the larger index.

Tied blocks are same index blocks, with different content, that are received before one is successfully committed. They should all be validated as potential branches, but only one should be accepted. The tie will be resolved by the next mined block when one branch becomes longer than the others.For example, if block A is received and while running Step 3, block B is received as well. Then A is accepted and built upon, while B is validated and kept as a potential branch. When block B-C is received, the node should switch back to B and accept B-C, and discards branch A. If A-C was received or mined instead of B-C, then the node discards branch B.

When the branches have the same content, they are duplicate branches, and only one of the branchs is accepted and the rest are be rejected and not stored as potential branches.

### Request
**URI**: `/validate`
**Method**: `POST`
**Body**:
```json
{
    "block": {
            "prev_hash": "002a04c22e1f3639d1261e029975911f599b5fbb99ec3c7adf47523dbd7ecb6c",
            "index": "1",
            "timestamp": "1681539282306497400",
            "data": "416c6963652073656e7420312042544320746f20426f62",
            "nonce": "1439",
            "hash": "000b6dbc34b21b2b8dea526281762e6fff9723ffeb7d2cdbd932ed15f9341c9d"
        }
}
```

### Response (Successful)
Respond with a 200 OK and the validated block.
**Status** : `200 OK`
**Body**:
```json
{
    "committed": false
}
```

### Error Response
Validation attempt was unsuccessful.
**Status**: `403 Forbidden`

### Error Response
Block was not verifiable.
**Status**: `400 Bad Request`
