// Package passwords generates cryptographically secure random passwords.
package passwords

import (
	"crypto/rand"
	"math/big"
)

const (
	MinLength = 4
	MaxLength = 128

	alnum   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	symbols = "!@#$%^&*()-_=+"
)

// Generate returns a random password drawn with crypto/rand (never math/rand).
// Alphanumeric by default; useSymbols adds a small, URL/shell-safe symbol set.
// Length is clamped to [MinLength, MaxLength] regardless of what's requested.
func Generate(length int, useSymbols bool) (string, error) {
	length = max(MinLength, min(length, MaxLength))

	alphabet := alnum
	if useSymbols {
		alphabet += symbols
	}

	n := big.NewInt(int64(len(alphabet)))
	out := make([]byte, length)
	for i := range out {
		idx, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}
