package remote

import (
	"bytes"
	"encoding/gob"
	"log"
	"net"
	"reflect"
)

/*
Transform the given interfaces into reflect values

Return a list of reflected values.
*/
func interfaceSliceToReflectValue(inputs []interface{}) []reflect.Value {
	// This will be the return variable
	var values []reflect.Value
	// Populate the return variable with relfection values of each input in the give interface (inputs)
	for _, input := range inputs {
		values = append(values, reflect.ValueOf(input))
	}
	// Return the reflection values
	return values
}

/*
Start the given service.
*/
func StartService(serv *Service) {
	// Infinite loop
	for {
		// Listen until a stub connection is attempted, then accept the connection.
		conn, err := serv.ln.Accept()
		// Return if there was an error accepting the connection.
		if err != nil {
			return
		}

		// Increment the remote calls served by this service.
		serv.mu.Lock()
		serv.remote_calls_served++
		serv.mu.Unlock()

		// handle the connection in its own thread.
		go HandleConnection(serv, conn)
	}
}

/*
Handle a stub's remote call to this service.
*/
func HandleConnection(serv *Service, conn net.Conn) {
	// Convert the connection to a Leaky Connection
	ls := NewLeakySocket(conn, serv.lossy, serv.delayed)
	// Receive the encoded request message
	request, err := ls.RecvObject()

	// Create a ReplyMsg object to send back to stub.
	reply := ReplyMsg{Success: false}

	// If message was not received properly:
	if err != nil {
		log.Println(err)
		// Set the reply to an error reply.
		reply.Err = RemoteObjectError{Err: error_message[UNABLE_TO_READ_REQUEST_SERVICE]}
		/* Gob encoding reply for transmission */
		var reply_bytes bytes.Buffer
		gob.Register(RemoteObjectError{})
		enc := gob.NewEncoder(&reply_bytes)
		err = enc.Encode(&reply)
		if err != nil {
			log.Fatalf("Error in encoding %v", err)
		}

		// Send the error reply back to the stub.
		SendBytes(ls, reply_bytes.Bytes())
		return
	}

	/* Decode the received request */
	dec := gob.NewDecoder(bytes.NewReader(request))
	var request_message RequestMsg
	err = dec.Decode(&request_message)
	if err != nil {
		log.Fatal("decode error 1:", err)
	}

	// Translate the method call arguments into their reflected values
	params := interfaceSliceToReflectValue(request_message.Args)

	// True if the method exists for this service.
	method_exists := DoesMethodExist(
		serv.ifc,
		serv.sobj,
		request_message.Method,
		params,
		request_message.ExpectedReturnValues,
	)

	// If requested method does not exist
	if !method_exists {
		log.Println(error_message[REQUEST_METHOD_DOESNOT_EXIST])
		// Set the reply to an error reply.
		reply.Err = RemoteObjectError{Err: error_message[REQUEST_METHOD_DOESNOT_EXIST]}
		/* Gob encoding reply for transmission */
		var reply_bytes bytes.Buffer
		gob.Register(RemoteObjectError{})
		enc := gob.NewEncoder(&reply_bytes)
		err = enc.Encode(&reply)
		if err != nil {
			log.Fatalf("Error in encoding %v", err)
		}

		// Send the error reply back to the stub.
		SendBytes(ls, reply_bytes.Bytes())
		return
	}

	// Get the service object's requested method
	method := serv.sobj.MethodByName(request_message.Method)

	// Call the service object's requested method with the provided arguments
	out := method.Call(params)

	// Create a list of interfaces for containing the method call outputs.
	output := make([]interface{}, len(out))
	// Populate the list of outputs with the actual method call outputs.
	for i, v := range out {
		output[i] = v.Interface()
	}

	// Set the reply to the method call outputs.
	reply.Reply = output
	// Set an empty error in the reply
	reply.Err = RemoteObjectError{}

	/* Gob encode the reply object */
	var reply_bytes bytes.Buffer
	for i := 0; i < method.Type().NumOut(); i++ {
		t := method.Type().Out(i)
		v := reflect.New(t).Elem().Interface()
		gob.Register(v)
	}
	enc := gob.NewEncoder(&reply_bytes)
	err = enc.Encode(&reply)
	if err != nil {
		log.Fatalf("Error in encoding %v", err)
	}

	// Send the encoded reply back to the stub.
	SendBytes(ls, reply_bytes.Bytes())
}
