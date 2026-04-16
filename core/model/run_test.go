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

func TestRunRejectsBadShapes(t *testing.T) {
	var r Run
	require.Error(t, r.UnmarshalJSON([]byte(`{}`)))
	require.Error(t, r.UnmarshalJSON([]byte(`{"text":{"text":"x"},"ph":{"id":"1","type":"t","data":"d","equiv":"e"}}`)))
}

func TestFragmentToRuns_TextOnly(t *testing.T) {
	f := &Fragment{CodedText: "hello"}
	runs := FragmentToRuns(f)
	require.Len(t, runs, 1)
	assert.Equal(t, "hello", runs[0].Text.Text)
}

func TestFragmentToRuns_WithMarkers(t *testing.T) {
	// "Files <span>({count} matched)</span>" shape.
	f := &Fragment{
		CodedText: "Files \uE001(\uE003 matched)\uE002",
		Spans: []*Span{
			{SpanType: SpanOpening, Type: "jsx:element", SubType: "span", ID: "1", Data: "<span>", EquivText: "span"},
			{SpanType: SpanPlaceholder, Type: "jsx:var", ID: "2", Data: "{count}", EquivText: "count"},
			{SpanType: SpanClosing, Type: "jsx:element", SubType: "span", ID: "1", Data: "</span>"},
		},
	}
	runs := FragmentToRuns(f)
	// Expected: "Files " + pcOpen + "(" + ph + " matched)" + pcClose
	require.Len(t, runs, 6)
	assert.Equal(t, "Files ", runs[0].Text.Text)
	assert.NotNil(t, runs[1].PcOpen)
	assert.Equal(t, "1", runs[1].PcOpen.ID)
	assert.Equal(t, "(", runs[2].Text.Text)
	assert.NotNil(t, runs[3].Ph)
	assert.Equal(t, "count", runs[3].Ph.Equiv)
	assert.Equal(t, " matched)", runs[4].Text.Text)
	assert.NotNil(t, runs[5].PcClose)
}

func TestRunsToFragment_RoundTrip(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "Files "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "<span>", Equiv: "span"}},
		{Text: &TextRun{Text: "("}},
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " matched)"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "</span>"}},
	}
	frag := RunsToFragment(runs)
	require.NotNil(t, frag)
	require.Len(t, frag.Spans, 3)
	// Round trip back to Runs must recover the original shape.
	again := FragmentToRuns(frag)
	require.Len(t, again, len(runs))
	assert.Equal(t, "Files ", again[0].Text.Text)
	assert.NotNil(t, again[1].PcOpen)
	assert.NotNil(t, again[3].Ph)
	assert.Equal(t, "count", again[3].Ph.Equiv)
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

func TestAsCodedTextRoundTrip(t *testing.T) {
	// Build a simple source fragment, convert to runs, then back to
	// coded text + spans. The shape should match.
	original := &Fragment{
		CodedText: "hi \uE001world\uE002",
		Spans: []*Span{
			{SpanType: SpanOpening, Type: "fmt:bold", ID: "1", Data: "<b>"},
			{SpanType: SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>"},
		},
	}
	runs := FragmentToRuns(original)
	coded, spans := AsCodedText(runs)
	assert.Equal(t, original.CodedText, coded)
	require.Len(t, spans, 2)
}
