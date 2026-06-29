package convergence_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestBlockKey_NameThenID(t *testing.T) {
	assert.Equal(t, "greeting", convergence.BlockKey(&model.Block{Name: "greeting", ID: "x"}))
	assert.Equal(t, "x", convergence.BlockKey(&model.Block{ID: "x"}), "falls back to ID when unnamed")
}

func TestTargetState_PresenceBaselineAndCommitted(t *testing.T) {
	b := model.NewBlock("a", "Apple")
	b.Translatable = true
	assert.Equal(t, "", convergence.TargetState(b, "nb"), "no target → below every rung")
	b.SetTargetText("nb", "Eple")
	assert.Equal(t, string(model.TargetStatusTranslated), convergence.TargetState(b, "nb"),
		"a present target counts as translated (presence baseline)")
	b.StampTargetProvenance("nb", model.TargetStatusReviewed, model.Origin{})
	assert.Equal(t, string(model.TargetStatusReviewed), convergence.TargetState(b, "nb"),
		"a committed status is authoritative")
}

func TestSourceState_PresenceBaselineAndCommitted(t *testing.T) {
	b := model.NewBlock("a", "Apple")
	assert.Equal(t, string(model.SourceStatusAuthored), convergence.SourceState(b))
	b.SourceStatus = model.SourceStatusApproved
	assert.Equal(t, string(model.SourceStatusApproved), convergence.SourceState(b))
	assert.Equal(t, "", convergence.SourceState(model.NewBlock("e", "  ")), "empty source is below every rung")
}

func TestPreview_TrimsAndCollapses(t *testing.T) {
	assert.Equal(t, "a b c", convergence.Preview("  a   b\nc  "))
	long := convergence.Preview(string(make([]byte, 100)))
	assert.LessOrEqual(t, len([]rune(long)), 72)
}
