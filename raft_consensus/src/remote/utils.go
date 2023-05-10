package remote

import (
	"log"
	"reflect"
)

/*
An interface is bad if it is nil or if one of its outputs is NOT a RemoteObjectError{}.

Return true if interface is bad, and false otherwise.
*/
func IsBadInterface(ifc interface{}) bool {

	// ifc is bad if it is nil
	if ifc == nil {
		return true
	}

	// Get the interface's value
	val := reflect.ValueOf(ifc).Elem()

	// For each field in ifc.Value
	for i := 0; i < val.NumField(); i++ {
		// Get the method in the field
		method := val.Field(i)
		// For each method output
		for j := 0; j < method.Type().NumOut(); j++ {
			// Return false if the method outputs a RemoteObjectError{}
			// i.e. ifc is NOT bad.
			if (method.Type().Out(j) == reflect.TypeOf(RemoteObjectError{})) {
				return false
			}
		}
	}

	// Otherwise, field methods in ifc do not contain RemoteObjectError{} as an output,
	// so ifc is bad.
	return true
}

/*
Checks if a Method exists in the interface.

Return true if method exists, and false otherwise.
*/
func DoesMethodExist(ifc reflect.Type, sobj reflect.Value, target_method string, params []reflect.Value, expectedReturnValues int) bool {
	// Get the target method from the service object.
	method := sobj.MethodByName(target_method)

	// Return false if the method is a zero Value.
	if !method.IsValid() {
		return false
	}

	// Return false if the number of inputs do not match the target method
	if method.Type().NumIn() != len(params) {
		return false
	}

	// Return false if the number of outputs do not match the target method
	if method.Type().NumOut() != expectedReturnValues {
		return false
	}

	// For each input in the method
	for i := 0; i < method.Type().NumIn(); i++ {
		// Return false if the input types do not match.
		if method.Type().In(i) != params[i].Type() {
			log.Printf("Error (Service): Input type mismatch; %v given, instead of %v ", method.Type().In(i), params[i].Type())
			return false
		}
	}
	return true
}

/*
Attempt to send bytes over a socket until they are successfuly sent.
*/
func SendBytes(ls *LeakySocket, msg []byte) {
	// Infinite loop
	for {
		// Try sending
		sent, _ := ls.SendObject(msg)
		if sent {
			// Message was sent, exit loop.
			break
		}
	}
}
