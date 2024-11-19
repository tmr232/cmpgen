package main

import (
	"fmt"
	"slices"

	"github.com/tmr232/cmpgen"
)

type Person struct {
	Name string
	Age  int
}

//go:generate go run github.com/tmr232/cmpgen/cmd/cmpgen
func main() {
	people := []Person{
		{"Gopher", 13},
		{"Alice", 55},
		{"Bob", 24},
		{"Alice", 20},
	}
	slices.SortFunc(people, cmpgen.CmpByFields[Person]("Name", "Age"))

	fmt.Println(people)
}
