package tool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Target reads the target TargetRuns reads, whatever key it is filed under: no
// language (the KBF reader files a bundle's "" target there), a malformed
// locale x/text reads in two steps, and the source language.
func TestView_TargetReadsTheTargetTargetRunsReads(t *testing.T) {
	twoStep := model.LocaleID("AA-u-00-00-u-00-00")
	once := model.NormalizeLocale(twoStep)
	locales := []model.LocaleID{"", "en-US", "fr", twoStep, once, model.NormalizeLocale(once)}
	origin := model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-filed"}

	for _, filed := range locales {
		b := model.NewBlock("b1", "Hello")
		b.SourceLocale = "en-US"
		b.SetTargetRuns(filed, []model.Run{model.TextR("filed")})
		b.StampTargetProvenance(filed, model.TargetStatusDraft, origin)
		v := tool.NewBlockView(b)

		for _, loc := range locales {
			got := v.Target(loc)
			if b.TargetRuns(loc) == nil {
				assert.Nil(t, got, "filed under %q, read under %q", filed, loc)
				continue
			}
			require.NotNil(t, got, "filed under %q, read under %q", filed, loc)
			assert.Equal(t, "filed", model.RunsText(got.Runs), "filed under %q, read under %q", filed, loc)
			assert.Equal(t, string(model.TargetStatusDraft), string(got.Status), "filed under %q, read under %q", filed, loc)
			assert.Equal(t, origin, got.Origin, "filed under %q, read under %q", filed, loc)
		}
	}
}
