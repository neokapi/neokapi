package schemaversion_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/schemaversion"
	"github.com/stretchr/testify/assert"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		name      string
		v         string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"typical", "1.0", 1, 0, true},
		{"multi-digit both sides", "12.34", 12, 34, true},
		{"minor zero", "1.99", 1, 99, true},

		{"empty string", "", 0, 0, false},
		{"no dot", "x", 0, 0, false},
		{"no dot, digits only", "10", 0, 0, false},

		// The four pre-consolidation implementations disagreed here: one
		// accepted a leading dot as major=0. Rejecting it is the point of
		// having one implementation instead of four.
		{"leading dot", ".5", 0, 0, false},
		{"trailing dot", "5.", 0, 0, false},
		{"only a dot", ".", 0, 0, false},

		// Three of the four implementations never validated anything past
		// the first dot, so a malformed minor silently parsed as whatever
		// major preceded it. The shared parser validates both sides.
		{"non-digit minor", "1.abc", 0, 0, false},
		{"non-digit major", "abc.1", 0, 0, false},
		{"trailing garbage", "1.0.1", 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			major, minor, ok := schemaversion.Split(c.v)
			assert.Equal(t, c.wantOK, ok, "ok")
			if c.wantOK {
				assert.Equal(t, c.wantMajor, major, "major")
				assert.Equal(t, c.wantMinor, minor, "minor")
			}
		})
	}
}

func TestMajor(t *testing.T) {
	major, ok := schemaversion.Major("2.7")
	assert.True(t, ok)
	assert.Equal(t, 2, major)

	_, ok = schemaversion.Major("bad")
	assert.False(t, ok)
}
