package cli

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestUnitState(t *testing.T) {
	// No target → untranslated (below every rung).
	b := model.NewBlock("tu1", "Hello")
	assert.Empty(t, unitState(b, "nb"))

	// Present, non-empty, no committed status → presence baseline = translated.
	b.SetTargetText(model.LocaleID("nb"), "Hei")
	assert.Equal(t, string(model.TargetStatusTranslated), unitState(b, "nb"))

	// A committed status is authoritative (a producer stamped it).
	b.StampTargetProvenance(model.LocaleID("nb"), model.TargetStatusReviewed, model.Origin{Kind: model.OriginHuman})
	assert.Equal(t, string(model.TargetStatusReviewed), unitState(b, "nb"))

	// Empty target text → untranslated even though a target record exists.
	b2 := model.NewBlock("tu2", "World")
	b2.SetTargetText(model.LocaleID("nb"), "   ")
	assert.Empty(t, unitState(b2, "nb"))
}
