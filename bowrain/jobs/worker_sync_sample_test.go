package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSampleItemNames(t *testing.T) {
	all := []string{"a.md", "b.md", "c.md", "d.md", "e.md"}

	// Caps the sample at n.
	assert.Equal(t, []string{"a.md", "b.md", "c.md"}, sampleItemNames(all, 3))

	// Returns the whole slice when it already fits.
	assert.Equal(t, []string{"a.md", "b.md"}, sampleItemNames([]string{"a.md", "b.md"}, 3))

	// Empty in, empty out.
	assert.Empty(t, sampleItemNames(nil, 3))
}
