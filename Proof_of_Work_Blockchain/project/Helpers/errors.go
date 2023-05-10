package helpers

import "fmt"

/*
Returns true if the error exists and false if it does not
*/
func Check(err error) bool {
	if err != nil {
		fmt.Println(err)
		return true
	}
	return false
}
