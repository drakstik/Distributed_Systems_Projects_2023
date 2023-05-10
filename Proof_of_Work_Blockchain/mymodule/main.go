package main

import (
	"fmt"
	"mymodule/mypackage"
)

func main() {
	fmt.Println("Hello from main")
	mypackage.PrintHello() // using mypackage

	/*
		This does not work when we try it with next2main.go,
		so everything should be under packages and main.go remains on top
		it's the only one we can run with go run anyways.
	*/
	// n := Node{}
	// n.PrintBye("main.go is saying Hello from node!")
}
