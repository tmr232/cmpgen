# CmpGen

Generate comparison functions for Go

## Usage

To create a cmp function, call `cmpgen.CmpByFields`.
It takes the type to compare as a type argument, and the field names to compare by as strings.

```go
// main.go
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
```

Running `go generate .` will create a file named `main_cmpgen.go`.
That file will contain the implementation of the comparison functions you use.
