package convergence_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
)

// TargetState reads the target TargetRuns reads, whatever key it is filed
// under: no language (the KBF reader files a bundle's "" target there), a
// locale that normalizes in two steps, and the source language.
func TestTargetState_ReadsTheTargetTargetRunsReads(t *testing.T) {
	twoStep := model.LocaleID("AA-u-00-00-u-00-00")
	once := model.NormalizeLocale(twoStep)
	locales := []model.LocaleID{"", "en-US", "fr", twoStep, once, model.NormalizeLocale(once)}

	for _, filed := range locales {
		b := model.NewBlock("b1", "Hello")
		b.SourceLocale = "en-US"
		b.SetTargetRuns(filed, []model.Run{model.TextR("filed")})
		b.StampTargetProvenance(filed, model.TargetStatusDraft, model.Origin{Kind: model.OriginAI})

		for _, loc := range locales {
			want := ""
			if model.RunsHaveContent(b.TargetRuns(loc)) {
				want = string(model.TargetStatusDraft)
			}
			assert.Equal(t, want, convergence.TargetState(b, string(loc)), "filed under %q, read under %q", filed, loc)
		}
	}

	b := model.NewBlock("b1", "Hello")
	b.SourceLocale = "en-US"
	b.SetTargetRuns("", []model.Run{model.TextR("zero")})
	b.StampTargetProvenance("", model.TargetStatusTranslated, model.Origin{})
	assert.Equal(t, string(model.TargetStatusTranslated), convergence.TargetState(b, ""), "the target filed under no language")
	assert.Empty(t, convergence.TargetState(b, "en-US"), "the source language with no target in it")
}
