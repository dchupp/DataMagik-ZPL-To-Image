package datamatrix

import (
	"math/rand"
	"testing"
)

func BenchmarkEncodeMaxBinary(b *testing.B) {
	data := make([]byte, 1550)
	rand.New(rand.NewSource(1)).Read(data)
	for range b.N {
		if _, err := Encode(data, Options{Quality: 200}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMaxText(b *testing.B) {
	data := make([]byte, 2335)
	for i := range data {
		data[i] = 'A' + byte(i%26)
	}
	for range b.N {
		if _, err := Encode(data, Options{Quality: 200}); err != nil {
			b.Fatal(err)
		}
	}
}
