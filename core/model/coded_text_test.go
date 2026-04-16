package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bridge-shape tests live alongside coded_text.go because they exercise
// the MarshalRuns / UnmarshalRuns helpers, which are the single API
// outside the bridge file itself for round-tripping Run sequences
// through the legacy PUA-marker representation.

func TestUnmarshalRuns_TextOnly(t *testing.T) {
	runs := UnmarshalRuns("hello", nil)
	require.Len(t, runs, 1)
	assert.Equal(t, "hello", runs[0].Text.Text)
}

func TestUnmarshalRuns_WithMarkers(t *testing.T) {
	// "Files <span>({count} matched)</span>" shape.
	coded := "Files \uE001(\uE003 matched)\uE002"
	spans := []*Span{
		{SpanType: SpanOpening, Type: "jsx:element", SubType: "span", ID: "1", Data: "<span>", EquivText: "span"},
		{SpanType: SpanPlaceholder, Type: "jsx:var", ID: "2", Data: "{count}", EquivText: "count"},
		{SpanType: SpanClosing, Type: "jsx:element", SubType: "span", ID: "1", Data: "</span>"},
	}
	runs := UnmarshalRuns(coded, spans)
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

func TestMarshalUnmarshalRuns_RoundTrip(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "Files "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "<span>", Equiv: "span"}},
		{Text: &TextRun{Text: "("}},
		{Ph: &PlaceholderRun{ID: "2", Type: "jsx:var", Data: "{count}", Equiv: "count"}},
		{Text: &TextRun{Text: " matched)"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "jsx:element", SubType: "span", Data: "</span>"}},
	}
	coded, spans := MarshalRuns(runs)
	require.Len(t, spans, 3)
	again := UnmarshalRuns(coded, spans)
	require.Len(t, again, len(runs))
	assert.Equal(t, "Files ", again[0].Text.Text)
	assert.NotNil(t, again[1].PcOpen)
	assert.NotNil(t, again[3].Ph)
	assert.Equal(t, "count", again[3].Ph.Equiv)
}

func TestMarshalRuns_PUAMarkerPositions(t *testing.T) {
	runs := []Run{
		{Text: &TextRun{Text: "hi "}},
		{PcOpen: &PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		{Text: &TextRun{Text: "world"}},
		{PcClose: &PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}
	coded, spans := MarshalRuns(runs)
	assert.Equal(t, "hi \uE001world\uE002", coded)
	require.Len(t, spans, 2)
	assert.Equal(t, SpanOpening, spans[0].SpanType)
	assert.Equal(t, SpanClosing, spans[1].SpanType)
}
