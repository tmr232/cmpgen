package main

import (
	"cmp"

	"github.com/tmr232/cmpgen"
)

func init() {
	cmpgen.Register[Person](
		func(a, b Person) int {
			return cmp.Or(
				cmp.Compare(a.Name, b.Name),
				cmp.Compare(a.Age, b.Age),
			)
		},
		"Name", "Age",
	)
}
