package cmpgen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type RegistryKey struct {
	Type   reflect.Type
	Fields string
}

var registry = make(map[RegistryKey]any)

func newRegistryKey[T any](fields ...string) RegistryKey {
	return RegistryKey{Type: reflect.TypeOf(*new(T)), Fields: strings.Join(fields, ", ")}
}

func Register[T any](fn any, fields ...string) {
	registry[newRegistryKey[T](fields...)] = fn
}

func CmpByFields[T any](field string, fields ...string) func(T, T) int {
	regKey := newRegistryKey[T](slices.Insert(fields, 0, field)...)
	if regFunc, ok := registry[regKey]; ok {
		return regFunc.(func(T, T) int)
	}
	panic(fmt.Sprintf("No comparator registered for CmpByFields[%s](%s)", regKey.Type.Name(), regKey.Fields))
}
