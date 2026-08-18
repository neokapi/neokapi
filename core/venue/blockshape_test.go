package venue

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

// What a producer declares about itself: the union of the property keys its
// readers emit, sorted and deduplicated so the same corpus always declares the
// same thing whatever order the blocks arrive in.
func TestBlockPropertyKeys_IsTheSortedUnion(t *testing.T) {
	keys := BlockPropertyKeys([]*model.Block{
		{Properties: map[string]string{"line": "4", "hash": "k1_abc"}},
		{Properties: map[string]string{"line": "9", "component": "Header"}},
		nil,
		{},
	})
	assert.Equal(t, []string{"component", "hash", "line"}, keys)
}

// Keys, never values — the declaration says what this reader records, not what
// any particular block currently holds.
func TestBlockPropertyKeys_ValuesAreNotDeclared(t *testing.T) {
	before := BlockPropertyKeys([]*model.Block{{Properties: map[string]string{"line": "4"}}})
	after := BlockPropertyKeys([]*model.Block{{Properties: map[string]string{"line": "5"}}})
	assert.Equal(t, before, after)
}

// A producer whose blocks carry no properties declares nothing, and the far
// side reads that as knowing nothing rather than as claiming emptiness — which
// is what keeps an older kapi from deleting a field it has never heard of.
func TestBlockPropertyKeys_NoPropertiesDeclaresNothing(t *testing.T) {
	assert.Empty(t, BlockPropertyKeys([]*model.Block{{ID: "b1"}, {ID: "b2"}}))
	assert.Empty(t, BlockPropertyKeys(nil))
}
