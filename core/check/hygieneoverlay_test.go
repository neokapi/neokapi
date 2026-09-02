package check_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// check.HygieneOverlay is the one implementation of the two shape rules that
// report a *range* — double spaces and doubled words. It replaces per-package
// copies in the desktop and lab annotators that scanned SourceText(), i.e. the
// flattening with every inline-code run dropped (#1441).
//
// Two questions have to be answered per rule, and they are separate: does the
// rule fire, and — when it does — does the span it reports still cover the
// offending text? The second is where the copies would have gone wrong even after
// a string swap: the rules read a flattening in which each code occupies three
// bytes, while overlays are anchored in one where it occupies none.
//
// Semantics across an inline code, in both rules: the code is a real boundary.
// A placeholder between two spaces separates them, so they are not consecutive;
// a placeholder between two instances of a word separates those, so the word is
// not doubled. Two *adjacent text runs* have nothing between them, so they
// flatten to one stretch of text and a defect spanning the join is real.

// wantSpan is one expected overlay span: its category, and the text the span
// covers once projected back onto the runs.
type wantSpan struct {
	category string
	covers   string
}

// overlaySpans returns the (category, covered text) pairs of the hygiene overlay
// for runs, in overlay order. The covered text is read through the span's run
// range — so a span whose range is off by a sentinel width reports the wrong
// text here, which is the point.
func overlaySpans(t *testing.T, runs []model.Run) []wantSpan {
	t.Helper()
	ov := check.HygieneOverlay(runs)
	if ov == nil {
		return nil
	}
	require.Equal(t, model.OverlayCheck, ov.Type)

	plain := model.RunsText(runs)
	out := make([]wantSpan, 0, len(ov.Spans))
	for _, s := range ov.Spans {
		bs, be := s.Range.ByteSpan(runs)
		require.LessOrEqual(t, be, len(plain), "span range runs past the anchored text")
		out = append(out, wantSpan{category: s.Props["category"], covers: plain[bs:be]})
	}
	return out
}

func TestHygieneOverlay_DoubleSpacesIsRunAware(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		want []wantSpan
	}{
		{
			// The false positive: each space separates a word from the
			// placeholder between them. Nothing is consecutive.
			name: "space, placeholder, space is not a double space",
			runs: []model.Run{hygTx("Hello "), hygJSXVar("1", "{name}", "name"), hygTx(" world")},
			want: nil,
		},
		{
			name: "leading placeholder then a separator space",
			runs: []model.Run{hygJSXVar("1", "{p.price}", "p.price"), hygTx(" each")},
			want: nil,
		},
		{
			name: "clean text",
			runs: []model.Run{hygTx("Hello world")},
			want: nil,
		},
		{
			// A wrapped list item's continuation indent: line structure, so
			// nothing to highlight (#1854).
			name: "a continuation indent is not a double space",
			runs: []model.Run{hygTx("one two\n  three four")},
			want: nil,
		},

		// Genuine defects, and the span must still land on them.
		{
			name: "two spaces inside one text run",
			runs: []model.Run{hygTx("two  spaces")},
			want: []wantSpan{{category: "double-spaces", covers: "  "}},
		},
		{
			// Adjacent text runs: nothing separates them, so the join is real
			// text and the two spaces are consecutive.
			name: "two adjacent text runs each ending/starting with a space",
			runs: []model.Run{hygTx("foo "), hygTx(" bar")},
			want: []wantSpan{{category: "double-spaces", covers: "  "}},
		},
		{
			// The offset-mapping case: the rule sees the run at hygiene offset
			// 7, the overlay must anchor it at RunsText offset 4.
			name: "double space after a leading placeholder",
			runs: []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" two  spaces")},
			want: []wantSpan{{category: "double-spaces", covers: "  "}},
		},
		{
			// The separator space before the placeholder is not part of the
			// defect; only the two spaces in the following run are.
			name: "separator space then a placeholder then a genuine double space",
			runs: []model.Run{hygTx("foo "), hygJSXVar("1", "{x}", "x"), hygTx("  bar")},
			want: []wantSpan{{category: "double-spaces", covers: "  "}},
		},
		{
			name: "a longer run of spaces is one finding",
			runs: []model.Run{hygTx("a    b")},
			want: []wantSpan{{category: "double-spaces", covers: "    "}},
		},
		{
			name: "two separate runs of spaces are two findings",
			runs: []model.Run{hygTx("a  b  c")},
			want: []wantSpan{
				{category: "double-spaces", covers: "  "},
				{category: "double-spaces", covers: "  "},
			},
		},
		{
			// Only the prose defect is highlighted; the indent beside it is not
			// part of the span.
			name: "a double space on a continuation line, past the indent",
			runs: []model.Run{hygTx("one two\n  three  four")},
			want: []wantSpan{{category: "double-spaces", covers: "  "}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, overlaySpans(t, tt.runs))
		})
	}
}

func TestHygieneOverlay_DoubledWordIsRunAware(t *testing.T) {
	tests := []struct {
		name string
		runs []model.Run
		want []wantSpan
	}{
		{
			// `the {name} the cat` repeats a word either side of a placeholder.
			// Odd phrasing, but not the redundant token this rule reports — the
			// placeholder stands between them.
			name: "a placeholder between two instances of a word",
			runs: []model.Run{hygTx("the "), hygJSXVar("1", "{name}", "name"), hygTx("the cat")},
			want: nil,
		},
		{
			name: "a placeholder between two instances of a word, spaced",
			runs: []model.Run{hygTx("the "), hygJSXVar("1", "{name}", "name"), hygTx(" the cat")},
			want: nil,
		},
		{
			name: "clean text",
			runs: []model.Run{hygTx("the cat sat")},
			want: nil,
		},
		{
			// A code is a token, not part of the word beside it: two adjacent
			// codes are two objects, not a repeated word.
			name: "two adjacent placeholders",
			runs: []model.Run{hygJSXVar("1", "{a}", "a"), hygJSXVar("2", "{b}", "b")},
			want: nil,
		},

		// Genuine defects, and the span must cover the *second* occurrence.
		{
			name: "doubled word inside one text run",
			runs: []model.Run{hygTx("the the cat")},
			want: []wantSpan{{category: "doubled-word", covers: "the"}},
		},
		{
			// Adjacent text runs flatten to one stretch of text: a real double.
			name: "doubled word spanning two adjacent text runs",
			runs: []model.Run{hygTx("the "), hygTx("the cat")},
			want: []wantSpan{{category: "doubled-word", covers: "the"}},
		},
		{
			name: "doubled word after a leading placeholder",
			runs: []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" the the end")},
			want: []wantSpan{{category: "doubled-word", covers: "the"}},
		},
		{
			// The code is glued to nothing: it does not absorb the first "the"
			// and so cannot hide the double. Rendered, this reads "Bobthe the
			// cat" — the second "the" is still redundant.
			name: "placeholder immediately followed by a doubled word",
			runs: []model.Run{hygJSXVar("1", "{name}", "name"), hygTx("the the cat")},
			want: []wantSpan{{category: "doubled-word", covers: "the"}},
		},
		{
			name: "comparison is case-insensitive",
			runs: []model.Run{hygTx("The the cat")},
			want: []wantSpan{{category: "doubled-word", covers: "the"}},
		},
		{
			name: "each doubled word is its own finding",
			runs: []model.Run{hygTx("the the cat sat sat")},
			want: []wantSpan{
				{category: "doubled-word", covers: "the"},
				{category: "doubled-word", covers: "sat"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, overlaySpans(t, tt.runs))
		})
	}
}

// TestHygieneOverlay_SpanProps pins the props a consumer reads (the lab preview
// keys off category and renders message), so the shared implementation is a
// drop-in for the per-package copies it replaced.
func TestHygieneOverlay_SpanProps(t *testing.T) {
	runs := []model.Run{hygJSXVar("1", "{x}", "x"), hygTx(" two  spaces and the the end")}
	ov := check.HygieneOverlay(runs)
	require.NotNil(t, ov)
	require.Len(t, ov.Spans, 2)

	assert.Equal(t, map[string]string{
		"category": "double-spaces",
		"severity": "minor",
		"message":  "Source contains double spaces",
	}, ov.Spans[0].Props)

	assert.Equal(t, map[string]string{
		"category": "doubled-word",
		"severity": "minor",
		"message":  `Doubled word: "the"`,
	}, ov.Spans[1].Props)
}

// TestHygieneOverlay_MessageNeverLeaksTheSentinel: the overlay message quotes the
// offending text, which comes from the flattening the rule judged. A doubled word
// adjacent to a placeholder must not carry U+FFFC into the user-visible string.
func TestHygieneOverlay_MessageNeverLeaksTheSentinel(t *testing.T) {
	runs := []model.Run{
		hygJSXVar("1", "{x}", "x"), hygTx(" the the  "), hygJSXVar("2", "{y}", "y"),
	}
	ov := check.HygieneOverlay(runs)
	require.NotNil(t, ov)
	for _, s := range ov.Spans {
		assert.NotContains(t, s.Props["message"], string(model.ObjectReplacement),
			"a sentinel reached a user-visible message")
	}
}

// TestHygieneOverlay_AgreesWithTheCLIVerdict: the overlay and the `hygiene.*`
// findings `kapi check` reports are the same rules, so a preview highlight and a
// CLI finding must never disagree about whether the content is clean.
func TestHygieneOverlay_AgreesWithTheCLIVerdict(t *testing.T) {
	cases := [][]model.Run{
		{hygTx("Hello "), hygJSXVar("1", "{name}", "name"), hygTx(" world")},
		{hygTx("the "), hygJSXVar("1", "{name}", "name"), hygTx(" the cat")},
		{hygTx("two  spaces")},
		{hygTx("the "), hygTx("the cat")},
		{hygTx("foo "), hygTx(" bar")},
		{hygJSXVar("1", "{x}", "x"), hygTx(" the the end")},
	}

	for i, runs := range cases {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			overlayCats := map[string]bool{}
			for _, s := range overlaySpans(t, runs) {
				overlayCats[s.category] = true
			}

			cliCats := map[string]bool{}
			for _, c := range hygieneCategories(t, runs) {
				if c == "double-spaces" || c == "doubled-word" {
					cliCats[c] = true
				}
			}
			assert.Equal(t, cliCats, overlayCats)
		})
	}
}
