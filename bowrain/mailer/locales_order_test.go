package mailer_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/mailer"
)

// Locales answers in sorted order, the same on every call.
func TestLocalesAreSorted(t *testing.T) {
	m, err := mailer.New(&recordingSender{})
	require.NoError(t, err)
	first := m.Locales()
	assert.True(t, slices.IsSorted(first), "locales are sorted: %v", first)
	for range 20 {
		assert.Equal(t, first, m.Locales())
	}
}
