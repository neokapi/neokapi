package terms

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
)

func TestIsShadowID(t *testing.T) {
	assert.True(t, IsShadowID(ShadowIDPrefix+":c:cs1:branch:old"))
	assert.False(t, IsShadowID("old"))
	assert.False(t, IsShadowID(""))
	// A prefix made of underscores is a LIKE wildcard, so a store that filtered
	// with LIKE would also drop this one. Nothing here may.
	assert.False(t, IsShadowID("XXpilotXX:c:cs1:branch:old"))
}

func TestNotShadowSQL_BindsThePrefixRatherThanInterpolatingIt(t *testing.T) {
	predicate, arg := NotShadowSQL("c.id", "?")
	assert.Equal(t, "substr(c.id, 1, 9) <> ?", predicate)
	assert.Equal(t, ShadowIDPrefix, arg)
	assert.NotContains(t, predicate, "LIKE", "underscores are LIKE wildcards; the prefix is all underscores")

	pg, _ := NotShadowSQL("id", "$2")
	assert.Equal(t, "substr(id, 1, 9) <> $2", pg)
}

// A shadow is written for one stream and must be invisible to every read that
// does not name one. The stream-blind reads are the ones the checks and the
// exports go through, so this is the contract that keeps a branch's proposal
// from governing the whole workspace.
func TestSQLiteStore_StreamBlindReadsExcludeShadows(t *testing.T) {
	ctx := t.Context()
	tb, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tb.Close() })

	live := Concept{ID: "live", Terms: []Term{{Text: "widget", Locale: "en-US", Status: model.TermAdmitted}}}
	require.NoError(t, tb.AddConcept(ctx, live))

	shadow := Concept{
		ID:    ShadowIDPrefix + ":c:cs1:branch:live",
		Terms: []Term{{Text: "widget", Locale: "en-US", Status: model.TermForbidden}},
	}
	require.NoError(t, tb.AddConceptWithStream(ctx, shadow, "branch"))
	require.NoError(t, tb.AddRelationWithStream(ctx, ConceptRelation{
		ID: ShadowIDPrefix + ":r:cs1:branch:r1", SourceID: shadow.ID, TargetID: shadow.ID,
		RelationType: graph.LabelUseInstead,
	}, "branch"))

	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.Equal(t, "live", concepts[0].ID)

	matches, err := tb.LookupAll(ctx, "a widget here", LookupOptions{SourceLocale: "en-US"})
	require.NoError(t, err)
	require.Len(t, matches, 1, "the shadow's duplicate designation must not surface")
	assert.Equal(t, model.TermAdmitted, matches[0].Term.Status,
		"the branch's forbidden status reached a lookup nobody bound it to")

	rels, err := tb.ListRelations(ctx, nil)
	require.NoError(t, err)
	for _, r := range rels {
		assert.False(t, strings.HasPrefix(r.ID, ShadowIDPrefix), "shadow relation %q escaped", r.ID)
	}

	// The shadow is still there for anyone who names its stream — the filter
	// hides it from stream-blind reads, it does not delete it.
	got, ok, err := tb.GetConcept(ctx, shadow.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, shadow.ID, got.ID)
}
