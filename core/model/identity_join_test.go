package model

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComputeContextHashKeepsItsBytes pins that a context whose name, type and
// properties hold neither NUL nor U+0001 hashes the bytes it always has: each
// part followed by NUL.
func TestComputeContextHashKeepsItsBytes(t *testing.T) {
	props := map[string]string{"b": "two", "a": "one", "@advisory": "skipped"}
	sum := sha256.Sum256([]byte("para1\x00text\x00a\x00one\x00b\x00two\x00"))
	assert.Equal(t, hex.EncodeToString(sum[:]), ComputeContextHash("para1", "text", props))

	record := sha256.Sum256([]byte("c1\x00x1\x00r1"))
	assert.Equal(t, hex.EncodeToString(record[:]), ComputeRecordHash("c1", "x1", "r1"))
}

// TestComputeContextHashKeepsNULPartsApart pins that two contexts whose parts
// split differently around a NUL, as a JSON key or value can hold one, hash
// apart. Before, they hashed alike, so one block could match another's record.
func TestComputeContextHashKeepsNULPartsApart(t *testing.T) {
	// The bytes the plain join hashed, alike for both.
	assert.Equal(t, "a\x00b"+"\x00"+"c"+"\x00", "a"+"\x00"+"b\x00c"+"\x00")
	assert.NotEqual(t,
		ComputeContextHash("a\x00b", "c", nil),
		ComputeContextHash("a", "b\x00c", nil))
	assert.NotEqual(t,
		ComputeContextHash("n", "t", map[string]string{"k": "v\x00k2\x00v2"}),
		ComputeContextHash("n", "t", map[string]string{"k": "v", "k2": "v2"}))
}
