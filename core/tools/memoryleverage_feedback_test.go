package tools_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// TestMemoryLeverageLeavesARejectedUnitForTheDrafter: a unit a reviewer
// rejected a draft of is not filled from content memory, which could only hand
// back wording the reviewer has already seen. A unit nobody rejected is filled
// as before.
func TestMemoryLeverageLeavesARejectedUnitForTheDrafter(t *testing.T) {
	t.Parallel()
	provider := &mockBlockMemoryProvider{
		match: corememory.Match{TargetRuns: iconTargetRuns(), Score: 100, Exact: true},
		found: true,
	}
	cfg := &tools.MemoryLeverageConfig{
		TargetLocale: model.LocaleFrench, SourceLocale: model.LocaleEnglish, FuzzyThreshold: 70, Memory: provider,
		ReviewFeedback: tool.ReviewFeedback{"tu1": {Rejections: []tool.ReviewRejection{{Note: "Too formal."}}}},
	}
	result := processPart(t, tools.NewMemoryLeverageTool(cfg), &model.Part{Type: model.PartBlock, Resource: iconBlock("tu1")})
	_, ok := result.Resource.(*model.Block).Edition(model.Variant(model.LocaleFrench))
	assert.False(t, ok, "the rejected unit is left for the drafter")

	other := processPart(t, tools.NewMemoryLeverageTool(cfg), &model.Part{Type: model.PartBlock, Resource: iconBlock("tu2")})
	_, ok = other.Resource.(*model.Block).Edition(model.Variant(model.LocaleFrench))
	assert.True(t, ok, "a unit nobody rejected is filled as before")
}
