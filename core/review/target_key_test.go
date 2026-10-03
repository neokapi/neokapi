package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The neighbourhood, the governing fingerprint and the provenance read the
// target TargetRuns reads, whatever key it is filed under: no language (the
// KBF reader files a bundle's "" target there, and a review in a project with
// no source language reads under the empty locale), a malformed locale x/text
// reads in two steps, and the source language.
func TestReview_ReadsTheTargetTargetRunsReads(t *testing.T) {
	twoStep := model.LocaleID("AA-u-00-00-u-00-00")
	once := model.NormalizeLocale(twoStep)
	locales := []model.LocaleID{"", "en-US", "fr", twoStep, once, model.NormalizeLocale(once)}
	origin := model.Origin{Kind: model.OriginAI, ContextFingerprint: "fp-filed"}

	for _, filed := range locales {
		b := model.NewBlock("b1", "Hello")
		b.SourceLocale = "en-US"
		b.SetTargetRuns(filed, []model.Run{model.TextR("filed")})
		b.StampTargetProvenance(filed, model.TargetStatusDraft, origin)

		for _, loc := range locales {
			held := b.TargetRuns(loc) != nil
			n, ok := NeighbourOf(b, loc)
			require.True(t, ok)
			gov := GoverningFingerprint(b, loc, "recorded")
			prov := ProvenanceOf(b, loc, nil)
			if !held {
				assert.Nil(t, n.Target, "filed under %q, read under %q", filed, loc)
				assert.Empty(t, n.Status, "filed under %q, read under %q", filed, loc)
				assert.Equal(t, "recorded", gov, "filed under %q, read under %q", filed, loc)
				assert.Nil(t, prov.Origin, "filed under %q, read under %q", filed, loc)
				continue
			}
			assert.Equal(t, "filed", model.RunsText(n.Target), "filed under %q, read under %q", filed, loc)
			assert.Equal(t, string(model.TargetStatusDraft), n.Status, "filed under %q, read under %q", filed, loc)
			assert.Equal(t, "fp-filed", gov, "filed under %q, read under %q", filed, loc)
			require.NotNil(t, prov.Origin, "filed under %q, read under %q", filed, loc)
			assert.Equal(t, origin, *prov.Origin, "filed under %q, read under %q", filed, loc)
		}
	}
}

// A review under the empty locale shows a neighbour's target filed under no
// language beside its source.
func TestNeighbourhoodOf_TargetFiledUnderNoLanguage(t *testing.T) {
	first := model.NewBlock("b1", "Sign in")
	first.SetTargetRuns("", []model.Run{model.TextR("EMPTYLOC")})
	blocks := []*model.Block{first, model.NewBlock("b2", "Sign out")}

	nh := NeighbourhoodOf(blocks, 1, 2, "")
	require.Len(t, nh.Before, 1)
	assert.Equal(t, "Sign in", model.RunsText(nh.Before[0].Source))
	assert.Equal(t, "EMPTYLOC", model.RunsText(nh.Before[0].Target))
}
