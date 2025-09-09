package util

import (
	"crypto/rand"
	"math/big"
)

const base36Chars = "0123456789abcdefghijklmnopqrstuvwxyz"

func GenerateShortID(length int) (string, error) {
	id := make([]byte, length)
	for i := range id {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(base36Chars))))
		if err != nil {
			return "", err
		}
		id[i] = base36Chars[n.Int64()]
	}
	return string(id), nil
}