package helpers

import (
	"os"
)

/* 
	1. Open the file containing the list of ports registered in the Blockchain
	2. Add the new port to the list
	3. Close the file
*/
func RegisterPort(port string, filepath string) {
	file, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	Check(err)
	text := port + " "
	_, err = file.WriteString(text)
	Check(err)
	file.Close()
}
