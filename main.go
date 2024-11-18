package main

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type RegistryKey struct {
	Type   reflect.Type
	Fields string
}

var Registry = make(map[RegistryKey]any)

func NewRegistryKey[T any](fields ...string) RegistryKey {
	return RegistryKey{Type: reflect.TypeOf(*new(T)), Fields: strings.Join(fields, "|")}
}

func CmpFields[T any](names ...string) func(a, b T) int {
	regKey := NewRegistryKey[T](names...)
	if regFunc, ok := Registry[regKey]; ok {
		return regFunc.(func(T, T) int)
	}
	cmp := func(a, b T) int {
		aVal := reflect.ValueOf(a)
		bVal := reflect.ValueOf(b)

		for _, name := range names {
			aField := aVal.FieldByName(name)
			bField := bVal.FieldByName(name)

			if aField.CanInt() {
				aInt := aField.Int()
				bInt := bField.Int()
				if aInt == bInt {
					continue
				} else if aInt < bInt {
					return -1
				} else {
					return 1
				}
			} else if aField.CanUint() {
				aUint := aField.Uint()
				bUint := bField.Uint()
				if aUint == bUint {
					continue
				} else if aUint < bUint {
					return -1
				} else {
					return 1
				}
			} else if aField.CanFloat() {
				aFloat := aField.Float()
				bFloat := bField.Float()
				if aFloat == bFloat {
					continue
				} else if aFloat < bFloat {
					return -1
				} else {
					return 1
				}
			} else if aField.Kind() == reflect.String {
				aString := aField.String()
				bString := bField.String()
				if aString == bString {
					continue
				} else if aString < bString {
					return -1
				} else {
					return 1
				}
			}
		}

		return 0
	}
	return cmp
}

func CmpBy[T any, C cmp.Ordered](key func(T) C) func(T, T) int {
	return func(t1, t2 T) int {
		return cmp.Compare(key(t1), key(t2))
	}
}

func CmpByString[T any](key func(T) string) func(T, T) int {
	return func(t1, t2 T) int {
		return strings.Compare(key(t1), key(t2))
	}
}

func Chain[T any](comparators ...func(T, T) int) func(T, T) int {
	return func(a, b T) int {
		for _, cmp := range comparators {
			r := cmp(a, b)
			if r != 0 {
				return r
			}
		}
		return 0
	}
}

type Person struct {
	Name string
	Age  int
}

func PersonCmp(a, b Person) int {
	return cmp.Or(
		strings.Compare(a.Name, b.Name),
		cmp.Compare(a.Age, b.Age),
	)
}

func main() {
	people := []Person{
		{"Gopher", 13},
		{"Alice", 55},
		{"Bob", 24},
		{"Alice", 20},
	}
	slices.SortFunc(people, CmpFields[Person]("Name", "Age"))

	fmt.Println(people)

	cmpKey := Chain(
		CmpBy(func(p Person) string { return p.Name }),
		CmpBy(func(p Person) int { return p.Age }),
	)

	slices.SortFunc(people, cmpKey)
	fmt.Println(people)
}
