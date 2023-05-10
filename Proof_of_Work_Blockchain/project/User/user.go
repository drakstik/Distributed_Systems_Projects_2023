package user

import (
	"sync"
)

var USER_LIST string
var NODE_LIST string

const CONTENT string = "/content"
const numOfNodes int = 1 // Number of nodes to send to

var registration_mutex sync.Mutex

type User struct {
	Port string `json:"port"`
	Name string `json:"name"`
}

type Content struct {
	Content string `json:"content"`
	User    User   `json:"user"`
}
