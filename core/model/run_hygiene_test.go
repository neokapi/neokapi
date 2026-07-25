package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The hygiene flattening exists so a shape rule can see that a placeholder
// occupies a position. A rule that reports a *range* additionally has to project
// its offsets back into RunsText coordinates, where a placeholder occupies none:
// the sentinel's three bytes are in the string it judged and absent from the
// space every overlay is anchored in. These tests pin that projection, because a
// silent off-by-three-per-placeholder highlight is exactly the failure the view
// exists to prevent.

func hvTx(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }

func hvPh(id string) model.Run {
	return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: "jsx:var", Data: "{x}", Equiv: "x"}}
}

func TestHygieneViewTextMatchesRunsHygieneText(t *testing.T) {
	sentinel := string(model.ObjectReplacement)

	tests := []struct {
		name string
		runs []model.Run
		want string
	}{
		{name: "no runs", runs: nil, want: ""},
		{name: "text only", runs: []model.Run{hvTx("Hello world")}, want: "Hello world"},
		{
			name: "placeholder then text",
			runs: []model.Run{hvPh("1"), hvTx(" each")},
			want: sentinel + " each",
		},
		{
			name: "placeholder between text",
			runs: []model.Run{hvTx("Hello "), hvPh("1"), hvTx(" world")},
			want: "Hello " + sentinel + " world",
		},
		{
			name: "plural takes the other branch",
			runs: []model.Run{{Plural: &model.PluralRun{Forms: map[model.PluralForm][]model.Run{
				model.PluralOne:   {hvTx("1 item")},
				model.PluralOther: {hvPh("1"), hvTx(" items")},
			}}}},
			want: sentinel + " items",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := model.NewHygieneView(tt.runs)
			assert.Equal(t, tt.want, v.Text())
			assert.Equal(t, model.RunsHygieneText(tt.runs), v.Text(),
				"the view is the single walk behind RunsHygieneText; they cannot differ")
		})
	}
}

// TestHygieneViewRealOffset walks every byte offset of the flattening and checks
// it against the offset of the same *text* in RunsText, so the mapping is
// verified as a whole rather than at a few hand-picked points.
func TestHygieneViewRealOffset(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		// want maps a byte offset in the hygiene text to the expected byte
		// offset in RunsText.
		want map[int]int
	}{
		{
			name: "no inline code: the two spaces coincide",
			runs: []model.Run{hvTx("Hello world")},
			want: map[int]int{0: 0, 5: 5, 11: 11},
		},
		{
			name: "leading placeholder shifts everything after it by its width",
			runs: []model.Run{hvPh("1"), hvTx(" each")},
			// hygiene "￼ each" (3 + 5 bytes) → RunsText " each"
			want: map[int]int{0: 0, 1: 0, 2: 0, 3: 0, 4: 1, 8: 5},
		},
		{
			name: "two placeholders subtract twice",
			runs: []model.Run{hvPh("1"), hvTx(" of "), hvPh("2"), hvTx(" total")},
			// hygiene "￼ of ￼ total" → RunsText " of  total"
			want: map[int]int{0: 0, 3: 0, 7: 4, 10: 4, 13: 7, 16: 10},
		},
		{
			name: "offsets outside the text clamp to its ends",
			runs: []model.Run{hvTx("ab")},
			want: map[int]int{-5: 0, 99: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := model.NewHygieneView(tt.runs)
			for off, want := range tt.want {
				assert.Equal(t, want, v.RealOffset(off), "RealOffset(%d)", off)
			}
		})
	}
}

// TestHygieneViewRangeAnchorsTheOffendingText is the property that matters to a
// caller: take a span of the hygiene text, project it, and the run range must
// still cover that same text. It fails for any mapping that is off by the
// sentinel width.
func TestHygieneViewRangeAnchorsTheOffendingText(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		// find is the substring of the hygiene text to project.
		find string
	}{
		{
			name: "double space in the only text run",
			runs: []model.Run{hvTx("two  spaces")},
			find: "  ",
		},
		{
			name: "double space after a leading placeholder",
			runs: []model.Run{hvPh("1"), hvTx(" two  spaces")},
			find: "  ",
		},
		{
			name: "doubled word after a leading placeholder",
			runs: []model.Run{hvPh("1"), hvTx(" the the end")},
			find: "the end",
		},
		{
			name: "text between two placeholders",
			runs: []model.Run{hvPh("1"), hvTx(" the the "), hvPh("2")},
			find: "the the",
		},
		{
			name: "non-ASCII before the span",
			runs: []model.Run{hvTx("naïve  café")},
			find: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := model.NewHygieneView(tt.runs)
			start := indexOf(t, v.Text(), tt.find)
			r := v.Range(start, start+len(tt.find))

			// Project the range back to bytes in RunsText and read the text it
			// covers: it must be the span we asked for.
			bs, be := r.ByteSpan(tt.runs)
			plain := model.RunsText(tt.runs)
			require.LessOrEqual(t, be, len(plain))
			assert.Equal(t, tt.find, plain[bs:be])
		})
	}
}

// RunsHaveContent is the one presence predicate every gate that asks "is there
// content here?" calls. It has to say yes to a block whose only run is an inline
// code: under RunsText that block flattened to "" and read as unauthored,
// untranslated, unproduced and uncheckable everywhere at once.
func TestRunsHaveContent(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		want bool
	}{
		{name: "no runs", runs: nil, want: false},
		{name: "empty text run", runs: []model.Run{hvTx("")}, want: false},
		{name: "whitespace only", runs: []model.Run{hvTx("  \t\n")}, want: false},
		{name: "text", runs: []model.Run{hvTx("Hello")}, want: true},
		{
			name: "placeholder only — the case RunsText answered wrongly",
			runs: []model.Run{hvPh("1")},
			want: true,
		},
		{
			name: "placeholder with whitespace around it is still content",
			runs: []model.Run{hvTx(" "), hvPh("1"), hvTx(" ")},
			want: true,
		},
		{
			name: "a paired code alone is content",
			runs: []model.Run{
				{PcOpen: &model.PcOpenRun{ID: "1", Type: "html:br"}},
				{PcClose: &model.PcCloseRun{ID: "1"}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, model.RunsHaveContent(tt.runs))
		})
	}
}

func indexOf(t *testing.T, s, sub string) int {
	t.Helper()
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	require.Failf(t, "substring not found", "%q not in %q", sub, s)
	return -1
}
