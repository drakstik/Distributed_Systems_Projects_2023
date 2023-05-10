package helpers

import (
	"os"
	"strings"
)

/*
	Attempt to read from the NodeList filepath.
*/
func GetPorts(filepath string) []string {

	_, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		file, err := os.Create(filepath)
		if err != nil {
			panic(err)
		}
		defer file.Close()
	}

	dat, err := os.ReadFile(filepath)
	if err != nil {
		panic(err)
	}

	known_ports := strings.Split(string(dat), " ")
	known_ports = known_ports[:len(known_ports)-1]

	return known_ports
}
