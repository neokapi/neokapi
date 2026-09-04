package review

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
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
		b.SetTarget("nb", &model.Target{Runs: []model.Run{model.TextR(target)}, Status: model.TargetStatusTranslated})
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

func TestNeighbourhoodOfKeepsDocumentOrder(t *testing.T) {
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
		{name: "middle of the document", idx: 2, window: 2, wantKey: "three", wantBefore: []string{"one", "two"}, wantAfter: []string{"four", "five"}, wantWindow: 2},
		{name: "first block has nothing before it", idx: 0, window: 2, wantKey: "one", wantAfter: []string{"two", "three"}, wantWindow: 2},
		{name: "last block has nothing after it", idx: 4, window: 2, wantKey: "five", wantBefore: []string{"three", "four"}, wantWindow: 2},
		{name: "second block sees one neighbour before", idx: 1, window: 2, wantKey: "two", wantBefore: []string{"one"}, wantAfter: []string{"three", "four"}, wantWindow: 2},
		{name: "a window of one narrows both sides", idx: 2, window: 1, wantKey: "three", wantBefore: []string{"two"}, wantAfter: []string{"four"}, wantWindow: 1},
		{name: "an unset window falls back to the default", idx: 2, window: 0, wantKey: "three", wantBefore: []string{"one", "two"}, wantAfter: []string{"four", "five"}, wantWindow: DefaultWindow},
		{name: "a window wider than the document stops at its ends", idx: 2, window: 10, wantKey: "three", wantBefore: []string{"one", "two"}, wantAfter: []string{"four", "five"}, wantWindow: 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NeighbourhoodOf(blocks, tc.idx, tc.window, "nb")
			assert.Equal(t, tc.wantKey, got.Key)
			assert.Equal(t, tc.wantWindow, got.Window)
			assert.Equal(t, tc.wantBefore, neighbourKeys(got.Before))
			assert.Equal(t, tc.wantAfter, neighbourKeys(got.After))
		})
	}
}

func TestNeighbourOfCarriesRunsAndTheRung(t *testing.T) {
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

	n, ok := NeighbourOf(block, "nb")
	require.True(t, ok)
	assert.Equal(t, "billing.credits", n.Key)
	require.Len(t, n.Source, 3, "the placeholder must survive into the neighbourhood")
	require.NotNil(t, n.Source[1].Ph)
	assert.Equal(t, "date", n.Source[1].Ph.Equiv)
	require.Len(t, n.Target, 3)
	require.NotNil(t, n.Target[1].Ph)
	assert.Empty(t, n.Status, "a target on no rung reports no status")

	block.Target("nb").Status = model.TargetStatusReviewed
	n, ok = NeighbourOf(block, "nb")
	require.True(t, ok)
	assert.Equal(t, "reviewed", n.Status, "the neighbour's rung travels with it")

	untranslated, ok := NeighbourOf(docBlock("bare", "Sign in", ""), "nb")
	require.True(t, ok)
	assert.Nil(t, untranslated.Target, "nothing on the target side, rather than an empty list")
}

func TestNeighbourOfSkipsUnreadableBlocks(t *testing.T) {
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
			_, ok := NeighbourOf(tc.block, "nb")
			assert.False(t, ok)
		})
	}
}

func TestPromptKeyPrefersTheReadersName(t *testing.T) {
	assert.Equal(t, "app.title", PromptKey(&model.Block{ID: "b1", Name: " app.title "}))
	assert.Equal(t, "u-1", PromptKey(&model.Block{ID: "b1", Unit: "u-1"}), "the stable unit key when nothing is named")
	assert.Equal(t, "b1", PromptKey(&model.Block{ID: "b1"}), "the reader's id as the last resort")
	assert.Empty(t, PromptKey(nil))
}

func TestProvenanceOfGroupsTheDecision(t *testing.T) {
	block := docBlock("greeting", "Hello", "Hei")

	tests := []struct {
		name string
		unit *state.UnitState
		want Provenance
	}{
		{name: "no record leaves the group empty", unit: nil, want: Provenance{}},
		{
			name: "the decision in force travels with its identity and its rung",
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
			want: Provenance{
				Origin:      &model.Origin{Kind: "memory"},
				ReviewState: "approved",
				Status:      "reviewed",
				By:          "agent/claude-code",
				At:          "2026-02-01T09:00:00Z",
				Note:        "matches the approved wording",
			},
		},
		{
			name: "source wording reports its authoring rung",
			unit: &state.UnitState{SourceStatus: model.SourceStatusApproved, Decision: state.Decision{ReviewState: "approved"}},
			want: Provenance{ReviewState: "approved", Status: string(model.SourceStatusApproved)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ProvenanceOf(block, "nb", tc.unit))
		})
	}

	t.Run("the format's own provenance wins over the record's", func(t *testing.T) {
		stamped := docBlock("greeting", "Hello", "Hei")
		stamped.Target("nb").Origin = model.Origin{Kind: "ai", Engine: "claude"}
		got := ProvenanceOf(stamped, "nb", &state.UnitState{Origin: model.Origin{Kind: "memory"}})
		require.NotNil(t, got.Origin)
		assert.Equal(t, "ai", got.Origin.Kind)
	})
}

// neighbourKeys names a neighbour list for a table comparison.
func neighbourKeys(ns []Neighbour) []string {
	if len(ns) == 0 {
		return nil
	}
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.Key)
	}
	return out
}
