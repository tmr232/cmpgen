package cmpgen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type registryKey struct {
	Type   reflect.Type
	Fields string
}

var registry = make(map[registryKey]any)

func newRegistryKey[T any](fields ...string) registryKey {
	return registryKey{Type: reflect.TypeOf(*new(T)), Fields: strings.Join(fields, ", ")}
}

// Register is meant to be used in the generated code only.
func Register[T any](fn any, fields ...string) {
	registry[newRegistryKey[T](fields...)] = fn
}

// CmpByFields provides a cmp function to be used with sorting functions.
// The type parameter is the type to compare, and the fields are the names
// of the fields to compare.
func CmpByFields[T any](field string, fields ...string) func(T, T) int {
	regKey := newRegistryKey[T](slices.Insert(fields, 0, field)...)
	if regFunc, ok := registry[regKey]; ok {
		return regFunc.(func(T, T) int)
	}
	panic(fmt.Sprintf("No comparator registered for CmpByFields[%s](%s)", regKey.Type.Name(), regKey.Fields))
}
