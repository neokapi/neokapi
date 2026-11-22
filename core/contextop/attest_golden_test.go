package contextop_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/contextop"
)

// TestItemRefKeyIsStable pins the bytes of ItemRef.Key, the item key the
// Bowrain context session tables store.
func TestItemRefKeyIsStable(t *testing.T) {
	r := contextop.ItemRef{Kind: "term", Profile: "docs", ID: "c1"}
	assert.Equal(t, "term\x00docs\x00c1", r.Key())
}
