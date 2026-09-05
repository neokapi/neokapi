package host

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docBlock builds one translatable block with a name, a source and an optional
// target for nb.
func docBlock(name, source, target string) *model.Block {
	b := &model.Block{
		ID:           name,
		Name:         name,
		Translatable: true,
		Source:       []model.Run{model.TextR(source)},
	}
	if target != "" {
		b.SetTarget("nb", &model.Target{Runs: []model.Run{model.TextR(target)}})
	}
	return b
}

// fiveBlockDoc is a document long enough for the window to sit inside it and at
// both ends.
func fiveBlockDoc() []*model.Block {
	return []*model.Block{
		docBlock("one", "First paragraph.", "Første avsnitt."),
		docBlock("two", "Second paragraph.", "Andre avsnitt."),
		docBlock("three", "Third paragraph.", "Tredje avsnitt."),
		docBlock("four", "Fourth paragraph.", "Fjerde avsnitt."),
		docBlock("five", "Fifth paragraph.", "Femte avsnitt."),
	}
}

// TestAssembleReviewContextComposesTheSharedLayers holds the assembler to the
// functions in core/review the platform composes too: over one unit, the
// neighbourhood, the history and the provenance it serves are what those
// functions answer for the same blocks, the same memory and the same record.
func TestAssembleReviewContextComposesTheSharedLayers(t *testing.T) {
	ctx := t.Context()
	blocks := fiveBlockDoc()
	blocks[2].Unit = "doc.three"
	blocks[2].Target("nb").Origin = model.Origin{Kind: "ai", Engine: "claude", ContextFingerprint: "fp-now"}
	tm := memory.NewInMemoryStore()
	at := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, tm.Add(ctx, chainAnswer("v2", "doc.three", "Third paragraph.", "Tredje avsnitt.", "fp-now", at)))
	unit := &state.UnitState{
		Status:   model.TargetStatusReviewed,
		Decision: state.Decision{ReviewState: "approved", By: "owner", At: "2026-02-02T09:00:00Z"},
	}

	got := (&App{}).AssembleReviewContext(ctx, ReviewContextRequest{
		Locale: "nb", SourceLang: "en", Blocks: blocks, Key: "doc.three", Memory: tm, Unit: unit,
	})
	require.NotNil(t, got)

	assert.Equal(t, review.NeighbourhoodOf(blocks, 2, DefaultReviewWindow, "nb"), got.Neighbourhood)
	assert.Equal(t, []string{"one", "two"}, neighbourKeys(got.Neighbourhood.Before))
	assert.Equal(t, []string{"four", "five"}, neighbourKeys(got.Neighbourhood.After))

	wantPrior := review.PriorVersionOf(ctx, tm, blocks[2], "en", "nb", review.GoverningFingerprint(blocks[2], "nb", unit.Origin))
	require.NotNil(t, wantPrior)
	assert.Equal(t, wantPrior, got.History.Prior)
	assert.True(t, got.History.Prior.Governed)
	require.NotNil(t, got.History.Match)
	assert.Equal(t, "Tredje avsnitt.", got.History.Match.Target)

	assert.Equal(t, review.ProvenanceOf(blocks[2], "nb", unit), got.Provenance)
	assert.Equal(t, "approved", got.Provenance.ReviewState)
	assert.Equal(t, "reviewed", got.Provenance.Status)
	require.NotNil(t, got.Provenance.Origin)
	assert.Equal(t, "ai", got.Provenance.Origin.Kind, "the format's own provenance wins")
	assert.Equal(t, "nb", got.Point.Language)
}

// neighbourKeys names a neighbour list for a table comparison.
func neighbourKeys(ns []ReviewNeighbour) []string {
	if len(ns) == 0 {
		return nil
	}
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.Key)
	}
	return out
}

// TestReviewHistoryThreadsTheGoverningFingerprint: the version chain is read
// through core/review, and Governed is judged against the fingerprint the
// current target was produced under, read from the state record's origin when
// the format keeps no provenance of its own.
func TestReviewHistoryThreadsTheGoverningFingerprint(t *testing.T) {
	ctx := t.Context()
	tm := memory.NewInMemoryStore()
	at := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, tm.Add(ctx, chainAnswer("v2", "settings.save", "Save the file", "Lagre filen", "fp-now", at)))
	block := &model.Block{
		ID: "b1", Name: "settings.save", Unit: "settings.save", Translatable: true,
		Source: []model.Run{model.TextR("Save this file")},
	}
	a := &App{}

	tests := []struct {
		name         string
		unit         *state.UnitState
		wantGoverned bool
	}{
		{name: "governed by the context in force", unit: &state.UnitState{Origin: model.Origin{ContextFingerprint: "fp-now"}}, wantGoverned: true},
		{name: "ungoverned once the rules moved", unit: &state.UnitState{Origin: model.Origin{ContextFingerprint: "fp-moved"}}},
		{name: "with no state record the chain still answers", unit: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := a.reviewHistory(ctx, ReviewContextRequest{Memory: tm, SourceLang: "en", Unit: tc.unit}, block, "nb")
			require.NotNil(t, h.Prior)
			assert.Equal(t, "Lagre filen", h.Prior.Target)
			assert.Equal(t, tc.wantGoverned, h.Prior.Governed)
			require.NotNil(t, h.Match, "the same entry is the corpus's best match for the source")
			assert.Equal(t, "Lagre filen", h.Match.Target)
			assert.NotEmpty(t, h.Match.Kind, "the match says how it matched")
		})
	}
}

func TestDoNotTranslateFromRules(t *testing.T) {
	tests := []struct {
		name  string
		rules []coreprofile.TermRule
		want  []string
	}{
		{name: "no rules", rules: nil, want: nil},
		{
			name: "a marked rule and a self-replacing rule both count",
			rules: []coreprofile.TermRule{
				{Term: "Bowrain", DoNotTranslate: true},
				{Term: "Kapi", Replacement: "Kapi"},
				{Term: "utilize", Replacement: "use"},
			},
			want: []string{"Bowrain", "Kapi"},
		},
		{
			name: "a term named twice is listed once",
			rules: []coreprofile.TermRule{
				{Term: "Bowrain", DoNotTranslate: true},
				{Term: "Bowrain", Replacement: "Bowrain"},
			},
			want: []string{"Bowrain"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, doNotTranslateFromRules(tc.rules))
		})
	}
}

// chainAnswer builds one approved answer in a block's version chain.
func chainAnswer(id, unit, source, target, fingerprint string, at time.Time) memory.Entry {
	return memory.Entry{
		ID:          id,
		Unit:        unit,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(source)},
			"nb": {model.TextR(target)},
		},
		Origins:   []memory.Origin{{Source: "tool", AddedAt: at, ContextFingerprint: fingerprint}},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

// TestReviewHistoryReportsAnUnseededStore: on a fresh clone the committed
// content memory has not been compiled into the store yet, so the store
// answers empty. A reviewer must be told that the memory is unread rather than
// shown "no close match" for wording the project has already approved.
func TestReviewHistoryReportsAnUnseededStore(t *testing.T) {
	ctx := t.Context()
	a, root, recipe := newSeedProject(t, false)
	writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
	block := docBlock("greeting", "Hello", "")

	before := a.reviewHistory(ctx, ReviewContextRequest{Root: root, Memory: a.ReviewMemory(ctx, root)}, block, "nb")
	assert.True(t, before.Unseeded, "a never-compiled store is unread, not empty")
	assert.Nil(t, before.Match, "the store holds nothing until the sources are compiled")

	_, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	after := a.reviewHistory(ctx, ReviewContextRequest{Root: root, Memory: a.ReviewMemory(ctx, root)}, block, "nb")
	assert.False(t, after.Unseeded, "once compiled, an empty answer means absent")
	require.NotNil(t, after.Match, "the committed wording is what the store now answers with")
	assert.Equal(t, "Hei", after.Match.Target)
}
