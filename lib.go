package main

// #include <stdio.h>
import "C"
import "fmt"

//export printgo
func printgo(x float64) {
	fmt.Println(x)
}
