package main

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

const (
	letters    = "abcdefghijklmnopqrstuvwxyz"
	minNameLen = 3
	maxNameLen = 10
)

// generateRandomName creates a random string of letters
func generateRandomName() string {
	length := rand.Intn(maxNameLen-minNameLen+1) + minNameLen
	var sb strings.Builder

	// Capitalize first letter
	sb.WriteByte(byte(letters[rand.Intn(len(letters))] - 32))

	// Add random lowercase letters
	for i := 1; i < length; i++ {
		sb.WriteByte(letters[rand.Intn(len(letters))])
	}

	return sb.String()
}

// GenerateRandomPeople creates a slice of random Person structs
func GenerateRandomPeople(count int) []Person {
	people := make([]Person, count)

	for i := 0; i < count; i++ {
		people[i] = Person{
			Name: generateRandomName(),
			Age:  rand.Intn(80) + 18, // Random age between 18 and 97
		}
	}

	return people
}

func BenchmarkClassic(b *testing.B) {
	people := GenerateRandomPeople(1000)
	for range b.N {
		slices.SortFunc(people, PersonCmp)
	}
}

func BenchmarkReflection(b *testing.B) {
	people := GenerateRandomPeople(1000)
	cmp := CmpFields[Person]("Name", "Age")
	for range b.N {
		slices.SortFunc(people, cmp)
	}
}
func BenchmarkKey(b *testing.B) {
	people := GenerateRandomPeople(1000)
	cmp := Chain(
		CmpBy(func(p Person) string { return p.Name }),
		CmpBy(func(p Person) int { return p.Age }),
	)
	for range b.N {
		slices.SortFunc(people, cmp)
	}
}

func BenchmarkKeyString(b *testing.B) {
	people := GenerateRandomPeople(1000)
	cmp := Chain(
		CmpByString(func(p Person) string { return p.Name }),
		CmpBy(func(p Person) int { return p.Age }),
	)
	for range b.N {
		slices.SortFunc(people, cmp)
	}
}
