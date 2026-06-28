package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyTargetOverlay_CarriesStatus verifies the klz/klf workspace overlay
// carries lifecycle status through the extract → merge round-trip, and that
// older status-less overlays still hydrate cleanly.
func TestApplyTargetOverlay_CarriesStatus(t *testing.T) {
	b := model.NewBlock("tu1", "Hello")
	applyTargetOverlay(b, model.LocaleFrench, []byte(`{"text":"Bonjour","status":"reviewed"}`))
	tgt := b.Target(model.LocaleFrench)
	require.NotNil(t, tgt)
	assert.Equal(t, "Bonjour", b.TargetText(model.LocaleFrench))
	assert.Equal(t, model.TargetStatusReviewed, tgt.Status)

	// Backward-compatible: an overlay without a status leaves it unset.
	b2 := model.NewBlock("tu2", "World")
	applyTargetOverlay(b2, model.LocaleFrench, []byte(`{"text":"Monde"}`))
	require.NotNil(t, b2.Target(model.LocaleFrench))
	assert.Equal(t, model.TargetStatusNew, b2.Target(model.LocaleFrench).Status)
}
