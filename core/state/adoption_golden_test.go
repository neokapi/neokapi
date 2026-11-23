package state_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/state"
)

// TestAdoptionIDIsStable pins the bytes of AdoptionID, the id the
// document_adoption table stores.
func TestAdoptionIDIsStable(t *testing.T) {
	a := state.Adoption{Key: "k1", Path: "docs/a.md", Digest: "sha256:00", Prev: "p0"}
	assert.Equal(t, "c5fb2cd28217e0b95416c678c91386ebe222f19e186fe08a93db922cb1427665", state.AdoptionID(a))
}
