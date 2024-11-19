package main

import (
	"go/types"
)

func targetFunc[T any](t T) {}

type global struct{}

var x = 2

func main() {
	type MyType struct {
		X int
	}

	targetFunc(1)
	targetFunc("A")
	targetFunc(global{})
	targetFunc(struct{}{})
	a := 1
	targetFunc(a)
	targetFunc(x)
	targetFunc(types.Universe)
	targetFunc(&x)
	targetFunc(MyType{x})
	targetFunc[MyType](MyType{x})
}
