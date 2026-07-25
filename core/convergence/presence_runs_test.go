package convergence_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
)

// Presence — "is there content here?" — is a shape question, and the derived
// convergence state is where every surface reads the answer. Under SourceText() /
// TargetText(), which drop inline code, a block whose only run is a placeholder
// flattened to "" and read as unauthored and untranslated: below every rung of
// both ladders, so it dragged its scope's coverage down and could never reach a
// shipping state. The unit is legitimate content — a bare `{price}` cell, an
// icon-only label — and no amount of translating it would have moved the number.

func phRun(equiv string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{" + equiv + "}", Equiv: equiv}}
}

func TestSourceState_PlaceholderOnlyIsAuthored(t *testing.T) {
	b := &model.Block{ID: "price", Translatable: true, Source: []model.Run{phRun("p.price")}}
	assert.Equal(t, string(model.SourceStatusAuthored), convergence.SourceState(b),
		"a placeholder-only source is authored content, not a hole in the source ladder")

	// The boundary the fix must not cross: genuinely empty content, and
	// whitespace-only content, are still below every rung.
	assert.Empty(t, convergence.SourceState(&model.Block{ID: "e", Translatable: true}))
	assert.Empty(t, convergence.SourceState(model.NewBlock("w", "  \t")))
}

func TestTargetState_PlaceholderOnlyIsTranslated(t *testing.T) {
	b := &model.Block{ID: "price", Translatable: true, Source: []model.Run{phRun("p.price")}}
	b.SetTargetRuns("nb", []model.Run{phRun("p.price")})
	assert.Equal(t, string(model.TargetStatusTranslated), convergence.TargetState(b, "nb"),
		"a placeholder-only target is produced, so it reaches the translated rung and can ship")

	// A committed status still wins over the presence baseline.
	b.StampTargetProvenance("nb", model.TargetStatusReviewed, model.Origin{})
	assert.Equal(t, string(model.TargetStatusReviewed), convergence.TargetState(b, "nb"))

	// Still below every rung: no target at all, and a whitespace-only target.
	assert.Empty(t, convergence.TargetState(b, "de"))
	ws := model.NewBlock("w", "Hello")
	ws.SetTargetText("nb", "   ")
	assert.Empty(t, convergence.TargetState(ws, "nb"))
}
