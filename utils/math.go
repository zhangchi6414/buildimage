package utils

import (
	"math/rand"
	"time"
)

const LETTERS = "abcdefghijklmnopqrstuvwxyz0123456789"

func Randow() string {
	rand.Seed(time.Now().UnixNano())

	b := make([]byte, 5)
	for i := range b {
		b[i] = LETTERS[rand.Intn(len(LETTERS))]
	}
	return string(b)

}
