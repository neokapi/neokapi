package host

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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

func TestReviewNeighbourhoodKeepsDocumentOrder(t *testing.T) {
	blocks := fiveBlockDoc()

	tests := []struct {
		name       string
		idx        int
		window     int
		wantKey    string
		wantBefore []string
		wantAfter  []string
		wantWindow int
	}{
		{
			name:       "middle of the document",
			idx:        2,
			window:     2,
			wantKey:    "three",
			wantBefore: []string{"one", "two"},
			wantAfter:  []string{"four", "five"},
			wantWindow: 2,
		},
		{
			name:       "first block has nothing before it",
			idx:        0,
			window:     2,
			wantKey:    "one",
			wantBefore: nil,
			wantAfter:  []string{"two", "three"},
			wantWindow: 2,
		},
		{
			name:       "last block has nothing after it",
			idx:        4,
			window:     2,
			wantKey:    "five",
			wantBefore: []string{"three", "four"},
			wantAfter:  nil,
			wantWindow: 2,
		},
		{
			name:       "second block sees one neighbour before",
			idx:        1,
			window:     2,
			wantKey:    "two",
			wantBefore: []string{"one"},
			wantAfter:  []string{"three", "four"},
			wantWindow: 2,
		},
		{
			name:       "a window of one narrows both sides",
			idx:        2,
			window:     1,
			wantKey:    "three",
			wantBefore: []string{"two"},
			wantAfter:  []string{"four"},
			wantWindow: 1,
		},
		{
			name:       "an unset window falls back to the default",
			idx:        2,
			window:     0,
			wantKey:    "three",
			wantBefore: []string{"one", "two"},
			wantAfter:  []string{"four", "five"},
			wantWindow: DefaultReviewWindow,
		},
		{
			name:       "a window wider than the document stops at its ends",
			idx:        2,
			window:     10,
			wantKey:    "three",
			wantBefore: []string{"one", "two"},
			wantAfter:  []string{"four", "five"},
			wantWindow: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reviewNeighbourhood(blocks, tc.idx, tc.window, "nb")
			assert.Equal(t, tc.wantKey, got.Key)
			assert.Equal(t, tc.wantWindow, got.Window)
			assert.Equal(t, tc.wantBefore, neighbourKeys(got.Before))
			assert.Equal(t, tc.wantAfter, neighbourKeys(got.After))
		})
	}
}

func TestReviewNeighbourCarriesRunsNotText(t *testing.T) {
	block := &model.Block{
		ID:           "credits",
		Name:         "billing.credits",
		Translatable: true,
		Source: []model.Run{
			model.TextR("Your credits reset on "),
			model.PhR(model.PlaceholderRun{ID: "1", Equiv: "date", Data: "{date}"}),
			model.TextR("."),
		},
	}
	block.SetTarget("nb", &model.Target{Runs: []model.Run{
		model.TextR("Kredittene dine nullstilles "),
		model.PhR(model.PlaceholderRun{ID: "1", Equiv: "date", Data: "{date}"}),
		model.TextR("."),
	}})

	n, ok := reviewNeighbour(block, "nb")
	require.True(t, ok)
	assert.Equal(t, "billing.credits", n.Key)
	require.Len(t, n.Source, 3, "the placeholder must survive into the neighbourhood")
	require.NotNil(t, n.Source[1].Ph)
	assert.Equal(t, "date", n.Source[1].Ph.Equiv)
	require.Len(t, n.Target, 3)
	require.NotNil(t, n.Target[1].Ph)
	assert.Empty(t, n.Status, "a target on no rung reports no status")

	block.Target("nb").Status = model.TargetStatusReviewed
	n, ok = reviewNeighbour(block, "nb")
	require.True(t, ok)
	assert.Equal(t, "reviewed", n.Status, "the neighbour's rung travels with it")
}

func TestReviewNeighbourSkipsUnreadableBlocks(t *testing.T) {
	tests := []struct {
		name  string
		block *model.Block
	}{
		{name: "nil block", block: nil},
		{name: "untranslatable block", block: &model.Block{ID: "x", Source: []model.Run{model.TextR("x")}}},
		{name: "block with no source runs", block: &model.Block{ID: "y", Translatable: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := reviewNeighbour(tc.block, "nb")
			assert.False(t, ok)
		})
	}
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

func TestReviewProvenanceGroupsTheDecision(t *testing.T) {
	block := docBlock("greeting", "Hello", "Hei")

	tests := []struct {
		name string
		unit *state.UnitState
		want ReviewProvenance
	}{
		{
			name: "no record leaves the group empty",
			unit: nil,
			want: ReviewProvenance{},
		},
		{
			name: "the decision in force travels with its identity",
			unit: &state.UnitState{
				Origin: model.Origin{Kind: "memory"},
				Status: model.TargetStatusReviewed,
				Decision: state.Decision{
					ReviewState: "approved",
					By:          "agent/claude-code",
					At:          "2026-02-01T09:00:00Z",
					Note:        "matches the approved wording",
				},
			},
			want: ReviewProvenance{
				Origin:      &model.Origin{Kind: "memory"},
				ReviewState: "approved",
				Status:      "reviewed",
				By:          "agent/claude-code",
				At:          "2026-02-01T09:00:00Z",
				Note:        "matches the approved wording",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reviewProvenance(block, "nb", tc.unit))
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
