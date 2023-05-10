// support for generic Remote Object services over sockets
// including a socket wrapper that can drop and/or delay messages arbitrarily
// works with any* objects that can be gob-encoded for serialization
//
// the LeakySocket wrapper for net.Conn is provided in its entirety, and should
// not be changed, though you may extend it with additional helper functions as
// desired.  it is used directly by the test code.
//
// the RemoteObjectError type is also provided in its entirety, and should not
// be changed.
//
// suggested RequestMsg and ReplyMsg types are included to get you started,
// but they are only used internally to the remote library, so you can use
// something else if you prefer
//
// the Service type represents the callee that manages remote objects, invokes
// calls from callers, and returns suitable results and/or remote errors
//
// the StubFactory converts a struct of function declarations into a functional
// caller stub by automatically populating the function definitions.
//
// USAGE:
// the desired usage of this library is as follows (not showing all error-checking
// for clarity and brevity):
//
//	example ServiceInterface known to both client and server, defined as
//	type ServiceInterface struct {
//	    ExampleMethod func(int, int) (int, remote.RemoteObjectError)
//	}
//
//	1. server-side program calls NewService with interface and connection details, e.g.,
//	   obj := &ServiceObject{}
//	   srvc, err := remote.NewService(&ServiceInterface{}, obj, 9999, true, true)
//
//	2. client-side program calls StubFactory, e.g.,
//	   stub := &ServiceInterface{}
//	   err := StubFactory(stub, 9999, true, true)
//
//	3. client makes calls, e.g.,
//	   n, roe := stub.ExampleMethod(7, 14736)
//
// TODO *** here's what needs to be done for Lab 2:
//
//  1. create the Service type and supporting functions, including but not
//     limited to: NewService, Start, Stop, IsRunning, and GetCount (see below)
//
//  2. create the StubFactory which uses reflection to transparently define each
//     method call in the client-side stub (see below)
package remote

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"log"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// LeakySocket
//
// LeakySocket is a wrapper for a net.Conn connection that emulates
// transmission delays and random packet loss. it has its own send
// and receive functions that together mimic an unreliable connection
// that can be customized to stress-test remote service interactions.
type LeakySocket struct {
	s         net.Conn
	isLossy   bool
	lossRate  float32
	msTimeout int
	usTimeout int
	isDelayed bool
	msDelay   int
	usDelay   int
}

// builder for a LeakySocket given a normal socket and indicators
// of whether the connection should experience loss and delay.
// uses default loss and delay values that can be changed using setters.
func NewLeakySocket(conn net.Conn, lossy bool, delayed bool) *LeakySocket {
	ls := &LeakySocket{}
	ls.s = conn
	ls.isLossy = lossy
	ls.isDelayed = delayed
	ls.msDelay = 2
	ls.usDelay = 0
	ls.msTimeout = 500
	ls.usTimeout = 0
	ls.lossRate = 0.05

	return ls
}

// send a byte-string over the socket mimicking unreliability.
// delay is emulated using time.Sleep, packet loss is emulated using RNG
// coupled with time.Sleep to emulate a timeout
func (ls *LeakySocket) SendObject(obj []byte) (bool, error) {
	if obj == nil {
		return true, nil
	}

	if ls.s != nil {
		rand.Seed(time.Now().UnixNano())
		if ls.isLossy && rand.Float32() < ls.lossRate {
			time.Sleep(time.Duration(ls.msTimeout)*time.Millisecond + time.Duration(ls.usTimeout)*time.Microsecond)
			return false, nil
		} else {
			if ls.isDelayed {
				time.Sleep(time.Duration(ls.msDelay)*time.Millisecond + time.Duration(ls.usDelay)*time.Microsecond)
			}
			_, err := ls.s.Write(obj)
			if err != nil {
				return false, errors.New("SendObject Write error: " + err.Error())
			}
			return true, nil
		}
	}
	return false, errors.New("SendObject failed, nil socket")
}

// receive a byte-string over the socket connection.
// no significant change to normal socket receive.
func (ls *LeakySocket) RecvObject() ([]byte, error) {
	if ls.s != nil {
		buf := make([]byte, 4096)
		n := 0
		var err error
		for n <= 0 {
			n, err = ls.s.Read(buf)
			if n > 0 {
				return buf[:n], nil
			}
			if err != nil {
				if err != io.EOF {
					return nil, errors.New("RecvObject Read error: " + err.Error())
				}
			}
		}
	}
	return nil, errors.New("RecvObject failed, nil socket")
}

// enable/disable emulated transmission delay and/or change the delay parameter
func (ls *LeakySocket) SetDelay(delayed bool, ms int, us int) {
	ls.isDelayed = delayed
	ls.msDelay = ms
	ls.usDelay = us
}

// change the emulated timeout period used with packet loss
func (ls *LeakySocket) SetTimeout(ms int, us int) {
	ls.msTimeout = ms
	ls.usTimeout = us
}

// enable/disable emulated packet loss and/or change the loss rate
func (ls *LeakySocket) SetLossRate(lossy bool, rate float32) {
	ls.isLossy = lossy
	ls.lossRate = rate
}

// close the socket (can also be done on original net.Conn passed to builder)
func (ls *LeakySocket) Close() error {
	return ls.s.Close()
}

// RemoteObjectError
//
// RemoteObjectError is a custom error type used for this library to identify remote methods.
// it is used by both caller and callee endpoints.
type RemoteObjectError struct {
	Err string
}

// getter for the error message included inside the custom error type
func (e *RemoteObjectError) Error() string { return e.Err }

// RequestMsg (this is only a suggestion, can be changed)
//
// RequestMsg represents the request message sent from caller to callee.
// it is used by both endpoints, and uses the reflect package to carry
// arbitrary argument types across the network.
type RequestMsg struct {
	Method               string
	Args                 []interface{}
	ExpectedReturnValues int
}

// ReplyMsg (this is only a suggestion, can be changed)
//
// ReplyMsg represents the reply message sent from callee back to caller
// in response to a RequestMsg. it similarly uses reflection to carry
// arbitrary return types along with a success indicator to tell the caller
// whether the call was correctly handled by the callee. also includes
// a RemoteObjectError to specify details of any encountered failure.
type ReplyMsg struct {
	Success bool
	Reply   []interface{}
	Err     RemoteObjectError
}

// Service -- server side stub/skeleton
//
// A Service encapsulates a multithreaded TCP server that manages a single
// remote object on a single TCP port, which is a simplification to ease management
// of remote objects and interaction with callers.  Each Service is built
// around a single struct of function declarations. All remote calls are
// handled synchronously, meaning the lifetime of a connection is that of a
// sinngle method call.  A Service can encounter a number of different issues,
// and most of them will result in sending a failure response to the caller,
// including a RemoteObjectError with suitable details.
type Service struct {
	ln                  net.Listener  // Network listener
	ifc                 reflect.Type  // Service interface type
	ifc_val             reflect.Value // Service interface value
	sobj                reflect.Value // Service remote object
	running             bool          // True, if service is running?
	port                string        // Address to communicate on
	lossy               bool          // True if this Service communicates over a leaky socket
	delayed             bool          // True if this Service communicates over a leaky socket
	remote_calls_served int           // Number of remote calls served
	mu                  sync.Mutex
}

// build a new Service instance around a given struct of supported functions,
// a local instance of a corresponding object that supports these functions,
// and arguments to support creation and use of LeakySocket-wrapped connections.
// performs the following:
// -- returns a local error if function struct or object is nil
// -- returns a local error if any function in the struct is not a remote function
// -- if neither error, creates and populates a Service and returns a pointer
func NewService(ifc interface{}, sobj interface{}, port int, lossy bool, delayed bool) (*Service, error) {
	/* Return an error if service object (sobj) is nil */
	if sobj == nil {
		return nil, errors.New(error_message[INVALID_SERVICE_OBJECT])
	}

	// Check if interface ifc is bad.
	bad_interface := IsBadInterface(ifc)

	// Return an error if given interface ifc is "bad".
	if bad_interface {
		return nil, errors.New(error_message[INTERFACE_REMOTE_ERROR_OBJECT_NOT_FOUND])
	}

	// Create an address using the given port
	port_str := "127.0.0.1:" + strconv.Itoa(port)

	// Create a new Service
	service := Service{
		ifc:                 reflect.TypeOf(ifc),
		ifc_val:             reflect.ValueOf(ifc),
		sobj:                reflect.ValueOf(sobj),
		running:             false,
		port:                port_str,
		lossy:               lossy,
		delayed:             delayed,
		remote_calls_served: 0,
	}

	// Return the new service without errors
	return &service, nil

}

/*
start the Service's tcp listening connection, update the Service status,
and start receiving caller connections.

Return nil if there were no errors, or return the error.
*/
func (serv *Service) Start() error {
	// Return an error if the service is already started
	if serv.running {
		return errors.New(error_message[SERVICE_ALREADY_RUNNING])
	}

	/* Otherwise, start the service. */

	// Set the Service's running boolean to true
	serv.running = true
	// Create a listener
	ln, err := net.Listen(PROTOCOL, serv.port)
	// Catch error while creating listener
	if err != nil {
		log.Println("Error listening on PORT", serv.port, err)
		return err
	}

	// Set the service's listener
	serv.ln = ln

	// Start the service.
	go StartService(serv)

	// No errors encountered while starting the service.
	return nil
}

/*
Return this service's number of remote calls served.
*/
func (serv *Service) GetCount() int {
	serv.mu.Lock()
	calls_served := serv.remote_calls_served
	serv.mu.Unlock()
	return calls_served
}

/*
Return true if this service is running, and false otherwise.
*/
func (serv *Service) IsRunning() bool {
	// Return false if the service is nil
	if serv == nil {
		return false
	}

	// Otherwise return the service's "running" field
	return serv.running
}

/*
Stop this service.
*/
func (serv *Service) Stop() {
	if serv != nil {
		// Close the service's network listener
		serv.ln.Close()
		// Set the service to NOT running
		serv.running = false
	}
}

/*
	 StubFactory -- make a client-side stub

	 	StubFactory uses reflection to populate the interface functions to create the
	 	caller's stub interface. Only works if all functions are exported/public.
		Once created, the interface masks remote calls to a Service that hosts the
		object instance that the functions are invoked on.  The network address of the
		remote Service must be provided with the stub is created, and it may not change later.

		A call to StubFactory requires the following inputs:
		-- a struct of function declarations to act as the stub's interface/proxy
		-- the remote address of the Service as "<ip-address>:<port-number>"
		-- indicator of whether caller-to-callee channel has emulated packet loss
		-- indicator of whether caller-to-callee channel has emulated propagation delay
		   performs the following:
		-- returns a local error if function struct is nil
		-- returns a local error if any function in the struct is not a remote function
		-- otherwise, uses relection to access the functions in the given struct and
		   populate their function definitions with the required stub functionality
*/
func StubFactory(ifc interface{}, adr string, lossy bool, delayed bool) error {

	// Check if the given stub interface ifc is "bad"
	bad_interface := IsBadInterface(ifc)

	// If ifc is bad, then return an error.
	if bad_interface {
		return errors.New(error_message[INVALID_INTERFACE])
	}

	// Get the the stub's value
	ifc_reflection := reflect.ValueOf(ifc).Elem()

	// For each field in the stub
	for i := 0; i < ifc_reflection.NumField(); i++ {

		// Get the field's method name
		method_name := ifc_reflection.Type().Field(i).Name

		/*
				Define a "stub function" that makes a connection to the remote object,
			 	makes a method call and returns the outputs.

				Return an array of reflection values.
		*/
		method_def := reflect.MakeFunc(ifc_reflection.Field(i).Type(), func(args []reflect.Value) []reflect.Value {

			// Create an array of reflection values to return
			returnval := []reflect.Value{}

			// Start a TCP connection with the service's address.
			conn, err := net.DialTimeout("tcp", adr, 5*time.Second)

			// If there was an error making the connection:
			if err != nil {
				// Append the zero value of each of the method's output to an array of reflection values,
				// except the last output in the method.
				for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut()-1; j++ {
					returnval = append(returnval, reflect.Zero(ifc_reflection.FieldByName(method_name).Type().Out(j)))
				}

				// Append the final reflection value as a RemoteObjectError{},
				// Thereby fulfilling the output requirements of the method being called.
				returnval = append(returnval, reflect.ValueOf(RemoteObjectError{
					Err: error_message[UNABLE_TO_SEND_CONNECTION_TO_SERVER]}))

				// Return the list of reflection values containing output values that have been
				// zeroed out and a RemoteObjectError{}.
				return returnval
			}

			// Create a new leaky socket using the connection to the service
			ls := NewLeakySocket(conn, lossy, delayed)

			/* Convert given arguments values into interfaces, for easy transmission. */
			var req_ifc []interface{}
			for _, arg := range args {
				req_ifc = append(req_ifc, arg.Interface())
			}

			// Cretae a request message to send to the service as a method call.
			request_message := RequestMsg{
				Method:               method_name,
				Args:                 req_ifc,
				ExpectedReturnValues: ifc_reflection.FieldByName(method_name).Type().NumOut()}

			/* Gob encode the request message */
			var req_bytes bytes.Buffer

			/* Encode */
			for i := 0; i < ifc_reflection.FieldByName(method_name).Type().NumIn(); i++ {
				t := ifc_reflection.FieldByName(method_name).Type().In(i)
				v := reflect.New(t).Elem().Interface()
				gob.Register(v)
			}

			enc := gob.NewEncoder(&req_bytes)
			err = enc.Encode(&request_message)
			// If encoding results in an error:
			if err != nil {
				log.Printf("%v %v", error_message[ENCODING_ERROR], err)
				// Append the zero value of each of the method's output to an array of reflection values,
				// except the last output in the method.
				for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut()-1; j++ {
					returnval = append(returnval, reflect.Zero(ifc_reflection.FieldByName(method_name).Type().Out(j)))
				}
				// Append the final reflection value as a RemoteObjectError{},
				// Thereby fulfilling the output requirements of the method being called.
				returnval = append(returnval, reflect.ValueOf(RemoteObjectError{
					Err: error_message[ENCODING_ERROR]}))
				// Return the list of reflection values containing output values that have been
				// zeroed out and a RemoteObjectError{}.
				return returnval
			}

			/* Try sending the encoded request to the service, until it is successfully sent */
			for {
				sent, _ := ls.SendObject(req_bytes.Bytes())
				if sent {
					break // Message successfully sent, so escape infinite loop
				}
			}

			/* Wait to receive response from service */
			response, err := ls.RecvObject() // Blocking call
			// If receiving results in an error:
			if err != nil {
				log.Printf(error_message[LEAKY_SOCKET_READ_ERROR_CLIENT], method_name, err)
				// Append the zero value of each of the method's output to an array of reflection values,
				// except the last output in the method.
				for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut()-1; j++ {
					returnval = append(returnval, reflect.Zero(ifc_reflection.FieldByName(method_name).Type().Out(j)))
				}
				// Append the final reflection value as a RemoteObjectError{},
				// Thereby fulfilling the output requirements of the method being called.
				returnval = append(returnval, reflect.ValueOf(RemoteObjectError{
					Err: error_message[LEAKY_SOCKET_READ_ERROR_CLIENT]}))
				// Return the list of reflection values containing output values that have been
				// zeroed out and a RemoteObjectError{}.
				return returnval
			}

			// Create an empty reply object to contain the remote service's response
			res := ReplyMsg{}

			/* Gob decode the service's response to the remote call */
			dec := gob.NewDecoder(bytes.NewReader(response))
			err = dec.Decode(&res)
			// If there was an error while decoding:
			if err != nil {
				log.Println(error_message[DECODING_ERROR])
				// Append the zero value of each of the method's output to an array of reflection values,
				// except the last output in the method.
				for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut()-1; j++ {
					returnval = append(returnval, reflect.Zero(ifc_reflection.FieldByName(method_name).Type().Out(j)))
				}
				// Append the final reflection value as a RemoteObjectError{},
				// Thereby fulfilling the output requirements of the method being called.
				returnval = append(returnval, reflect.ValueOf(RemoteObjectError{
					Err: error_message[DECODING_ERROR]}))
				// Return the list of reflection values containing output values that have been
				// zeroed out and a RemoteObjectError{}.
				return returnval
			}

			// If the reply's Err field is NOT an empty RemoteObjectError,
			// then the reply object contains an error.
			if (res.Err != RemoteObjectError{}) {
				// Append the zero value of each of the method's output to an array of reflection values,
				// except the last output in the method.
				for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut()-1; j++ {
					returnval = append(returnval, reflect.Zero(ifc_reflection.FieldByName(method_name).Type().Out(j)))
				}
				// Append the final reflection value as a RemoteObjectError{},
				// Thereby fulfilling the output requirements of the method being called.
				returnval = append(returnval, reflect.ValueOf(RemoteObjectError{
					Err: res.Err.Err}))
				// Return the list of reflection values containing output values that have been
				// zeroed out and a RemoteObjectError{}.
				return returnval
			}

			// Convert the returned outputs from a list of interfaces into a list of reflection values
			result := interfaceSliceToReflectValue(res.Reply)

			// Add each returned output result[j] to the final return variable,
			// which is a list of reflection values
			for j := 0; j < ifc_reflection.FieldByName(method_name).Type().NumOut(); j++ {
				returnval = append(returnval, result[j])
			}

			// Return the outputs from the remote call.
			return returnval
		})

		// Set the given stub's field to be the stub remote function defined above,
		// so when method_name is called on this stub, the remote function will be called instead.
		ifc_reflection.FieldByName(method_name).Set(method_def)
	}

	// Successfully ran StubFactory with no errors.
	return nil
}
