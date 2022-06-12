package utils

import (
	"crypto/sha256"
	"encoding/base64"
	"math/rand"
)

const randomCharPool = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func HashString(input string) string {
	hashed := sha256.Sum256([]byte(input))
	return base64.StdEncoding.EncodeToString(hashed[:])
}

func RandomString(length uint) string {
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = randomCharPool[rand.Intn(len(randomCharPool))]
	}
	return string(bytes)
}
