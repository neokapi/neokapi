package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyTargetOverlay_CarriesStatus verifies the kpz/kbf workspace overlay
// carries lifecycle status through the extract → merge round-trip, and that
// older status-less overlays still hydrate cleanly.
func TestApplyTargetOverlay_CarriesStatus(t *testing.T) {
	b := model.NewBlock("tu1", "Hello")
	require.NoError(t, applyTargetOverlay(b, model.LocaleFrench, []byte(`{"text":"Bonjour","status":"established"}`)))
	tgt, ok := b.TargetEdition(model.LocaleFrench)
	require.True(t, ok)
	assert.Equal(t, "Bonjour", b.TargetText(model.LocaleFrench))
	assert.Equal(t, model.TargetStatusEstablished, model.TargetStatus(tgt.Status))

	// Backward-compatible: an overlay without a status leaves it unset.
	b2 := model.NewBlock("tu2", "World")
	require.NoError(t, applyTargetOverlay(b2, model.LocaleFrench, []byte(`{"text":"Monde"}`)))
	tgt2, ok := b2.TargetEdition(model.LocaleFrench)
	require.True(t, ok)
	assert.Equal(t, model.TargetStatusNew, model.TargetStatus(tgt2.Status))
}
