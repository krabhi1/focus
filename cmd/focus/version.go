package main

import "fmt"

var version = "dev"

func printVersion() {
	fmt.Println(colorInfo(version))
}
