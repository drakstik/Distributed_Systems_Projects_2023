package remote

/* Start of Error Message Codes */

const INVALID_INTERFACE = 0
const INVALID_SERVICE_OBJECT = 1
const INTERFACE_REMOTE_ERROR_OBJECT_NOT_FOUND = 2
const INVALID_SERVICE = 3
const SERVICE_ALREADY_RUNNING = 4
const UNABLE_TO_ACCEPT_CONNECTION_ON_SERVER = 5
const UNABLE_TO_READ_REQUEST_SERVICE = 6
const JSON_UNMARSHAL_FAILED = 7
const REQUEST_METHOD_DOESNOT_EXIST = 8
const UNABLE_TO_SEND_CONNECTION_TO_SERVER = 9
const ENCODING_ERROR = 10
const LEAKY_SOCKET_READ_ERROR_CLIENT = 11
const DECODING_ERROR = 11

/* End of Constant Global Variables */

var error_message = []string{
	"Error (NewService): Invalid Interface",
	"Error (NewService): Invalid SOBJ",
	"Error (NewService): Non-remote service interface and instance",
	"Error (Service): Invalid Service",
	"Error (Service): Start Request sent to an already running service",
	"Error (Service): Unable to accept tcp connections service",
	"Error (Service): Could not read request from Client Stub",
	"Error (Service): JSON Unmarshal failed",
	"Error (Service): Request Method doesn't exist on Remote",
	"Error (Client): Unable to send tcp connections to service",
	"Error (Client): Error in encoding",
	"Error (Client): Unable to read from LS on client for Method : %v Error: %v",
	"Error (Client): Error in decoding",
}
