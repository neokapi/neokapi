package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	id := New()
	assert.Len(t, id, 8)

	// All characters should be base62.
	for _, c := range id {
		assert.Contains(t, base62, string(c))
	}
}

func TestNew_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 10000)
	for range 10000 {
		id := New()
		require.NotContains(t, seen, id, "duplicate ID generated")
		seen[id] = struct{}{}
	}
}
