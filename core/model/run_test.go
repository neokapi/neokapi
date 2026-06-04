package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMarshalUnmarshalRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  Run
	}{
		{"text", Run{Text: &TextRun{Text: "hello"}}},
		{"ph", Run{Ph: &PlaceholderRun{ID: "1", Type: "jsx:var", Data: "{x}", Equiv: "x"}}},
		{"pcOpen", Run{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "<span>", Equiv: "span"}}},
		{"pcClose", Run{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "</span>"}}},
		{"sub", Run{Sub: &SubRun{ID: "1", Ref: "block-2", Equiv: "sub"}}},
		{"plural", Run{Plural: &PluralRun{
			Pivot: "count",
			Forms: map[PluralForm][]Run{
				PluralOne:   {{Text: &TextRun{Text: "one"}}},
				PluralOther: {{Text: &TextRun{Text: "other"}}},
			},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.run)
			require.NoError(t, err)
			var got Run
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, tc.run, got)
		})
	}
}

// TestRunMarshalDoesNotHTMLEscape guards the KLF "no HTML escaping" wire
// contract: a run's data/text routinely holds markup like `<span>` or an `&&`
// expression, and the bytes Run.MarshalJSON emits must keep those literal so a
// non-escaping encoder (core/klf.Marshal) yields output matching the TypeScript
// mirror (@neokapi/kapi-format, which uses JSON.stringify) and the content hash
// stays implementation-independent.
//
// Note: the assertion calls MarshalJSON directly rather than the package-level
// json.Marshal, because json.Marshal re-escapes a Marshaler's output during
// compaction (escapeHTML defaults on) — the klf path deliberately encodes with
// SetEscapeHTML(false), which preserves these literal bytes.
func TestRunMarshalDoesNotHTMLEscape(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  Run
		want string
	}{
		{"text", Run{Text: &TextRun{Text: "a < b && c > d"}}, `{"text":"a < b && c > d"}`},
		{
			"pcOpen",
			Run{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: `<span className="muted">`, Equiv: "muted"}},
			`{"pcOpen":{"id":"1","type":"jsx:element","subType":"span","data":"<span className=\"muted\">","equiv":"muted"}}`,
		},
		{
			"ph-logical-and",
			Run{Ph: &PlaceholderRun{ID: "1", Type: "jsx:node", Data: "a && <b/>", Equiv: "x"}},
			`{"ph":{"id":"1","type":"jsx:node","data":"a && <b/>","equiv":"x"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.run.MarshalJSON()
			require.NoError(t, err)
			// Exact match proves the markup bytes are literal: had they been
			// HTML-escaped, the angle brackets and ampersands would appear as
			// their \u-escaped forms and differ from want.
			assert.Equal(t, tc.want, string(data))
		})
	}
}

func TestRunRejectsBadShapes(t *testing.T) {
	var r Run
	require.Error(t, r.UnmarshalJSON([]byte(`{}`)))
	require.Error(t, r.UnmarshalJSON([]byte(`{"text":"x","ph":{"id":"1","type":"t","data":"d","equiv":"e"}}`)))
}

func TestFlattenRuns(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "Files "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "<span>", Equiv: "span"}},
		{Text: &TextRun{Text: "("}},
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " matched)"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "</span>"}},
	}
	assert.Equal(t, "Files ({count} matched)", FlattenRuns(runs))
}

func TestFlattenRunsPluralFallbackToFirstWhenNoOther(t *testing.T) {
	runs := []Run{
		{Plural: &PluralRun{
			Pivot: "count",
			Forms: map[PluralForm][]Run{
				PluralOne: {{Text: &TextRun{Text: "one item"}}},
			},
		}},
	}
	assert.Equal(t, "one item", FlattenRuns(runs))
}

func TestBlockRunsRoundTrip(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "Files "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "<span>", Equiv: "span"}},
		{Text: &TextRun{Text: "("}},
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " matched)"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "</span>"}},
	}
	b := NewRunsBlock("block-1", runs)
	got := b.SourceRuns()
	assert.Equal(t, len(runs), len(got))
	assert.Equal(t, "Files ", got[0].Text.Text)

	// Round-trip through the canonical JSON wire form (no coded-text bridge).
	data, err := json.Marshal(b.SourceRuns())
	require.NoError(t, err)
	var back []Run
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, runs, back)
}

func TestBlockSetTargetRuns(t *testing.T) {
	b := NewBlock("b1", "hello")
	target := []Run{{Text: &TextRun{Text: "hallo"}}}
	b.SetTargetRuns("de", target)
	assert.True(t, b.HasTarget("de"))
	assert.Equal(t, "hallo", b.TargetText("de"))
	got := b.TargetRuns("de")
	require.Len(t, got, 1)
	assert.Equal(t, "hallo", got[0].Text.Text)
}
