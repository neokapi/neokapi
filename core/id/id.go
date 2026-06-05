// Package id generates short, URL-friendly random identifiers.
//
// Each ID is 8 characters drawn from a base62 alphabet (0-9, A-Z, a-z),
// giving ~47.6 bits of entropy. This is collision-resistant for typical
// application workloads without requiring coordination.
package id

import "crypto/rand"

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// rejectThreshold is the largest byte value that still divides evenly into
// the base62 alphabet, ensuring each symbol is equally likely. Bytes at or
// above this value are discarded and redrawn (rejection sampling).
// 256 - (256 % 62) = 256 - 8 = 248.
const rejectThreshold = 256 - (256 % len(base62))

// New generates a short random ID (8 chars, base62-encoded).
//
// Panic rationale: crypto/rand.Read failing indicates a fundamental OS-level
// problem (e.g., /dev/urandom unavailable). There is no reasonable recovery
// path — continuing with weak randomness would silently produce predictable
// IDs, which is worse than crashing. This matches the Go standard library
// convention (e.g., crypto/rand.Prime panics on read failure).
func New() string {
	out := make([]byte, 8)
	var buf [1]byte
	for i := range out {
		for {
			if _, err := rand.Read(buf[:]); err != nil {
				panic("crypto/rand failed: " + err.Error())
			}
			if int(buf[0]) < rejectThreshold {
				out[i] = base62[int(buf[0])%len(base62)]
				break
			}
		}
	}
	return string(out)
}
