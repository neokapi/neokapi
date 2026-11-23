package ref

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIdentityKeepsItsBytes pins that fields holding neither NUL nor U+0001
// hash the bytes they always have: each field followed by NUL.
func TestIdentityKeepsItsBytes(t *testing.T) {
	sum := sha256.Sum256([]byte("approved\x00noun\x00\x00a note\x00"))
	assert.Equal(t, hex.EncodeToString(sum[:]), Identity("approved", "noun", "", "a note"))
}

// TestIdentityKeepsNULFieldsApart pins that fields splitting differently
// around a NUL, as a note imported from JSON can hold one, have two
// identities. Before, they had one, so a changed note could leave the terms
// component where it was.
func TestIdentityKeepsNULFieldsApart(t *testing.T) {
	assert.Equal(t, "noun\x00m"+"\x00"+"note"+"\x00", "noun"+"\x00"+"m\x00note"+"\x00", "the plain join is alike")
	assert.NotEqual(t, Identity("noun\x00m", "note"), Identity("noun", "m\x00note"))
}
