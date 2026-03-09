// Package id generates short, URL-friendly random identifiers.
//
// Each ID is 8 characters drawn from a base62 alphabet (0-9, A-Z, a-z),
// giving ~47.6 bits of entropy. This is collision-resistant for typical
// application workloads without requiring coordination.
package id

import "crypto/rand"

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// New generates a short random ID (8 chars, base62-encoded).
func New() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	out := make([]byte, 8)
	for i, b := range buf {
		out[i] = base62[int(b)%len(base62)]
	}
	return string(out)
}
