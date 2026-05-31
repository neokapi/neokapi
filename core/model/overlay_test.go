package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- small run constructors for readable test tables ---

func tx(s string) Run { return Run{Text: &TextRun{Text: s}} }

func ph(id string) Run { return Run{Ph: &PlaceholderRun{ID: id, Type: "var", Equiv: id}} }

func plural(other ...Run) Run {
	return Run{Plural: &PluralRun{Pivot: "n", Forms: map[PluralForm][]Run{PluralOther: other}}}
}

func pluralForms(forms map[PluralForm][]Run) Run {
	return Run{Plural: &PluralRun{Pivot: "n", Forms: forms}}
}

func sel(other ...Run) Run {
	return Run{Select: &SelectRun{Pivot: "g", Cases: map[string][]Run{"other": other}}}
}

func selCases(cases map[string][]Run) Run {
	return Run{Select: &SelectRun{Pivot: "g", Cases: cases}}
}

// TestRunFlatLenMatchesRunsText is the core invariant behind #15: the rune
// width runFlatLen attributes to a run sequence must equal the rune length of
// RunsText over the same runs, including across plural/select branches.
func TestRunFlatLenMatchesRunsText(t *testing.T) {
	cases := []struct {
		name string
		runs []Run
	}{
		{"empty", nil},
		{"text only", []Run{tx("Hello world")}},
		{"multi text", []Run{tx("Hello "), tx("world")}},
		{"with placeholder", []Run{tx("Click "), ph("1"), tx(" here")}},
		{"multibyte", []Run{tx("héllo "), tx("wörld")}},
		{"emoji", []Run{tx("a😀b")}},
		{"plural other", []Run{tx("You have "), plural(tx("items"))}},
		{"plural no other first", []Run{pluralForms(map[PluralForm][]Run{
			PluralOne: {tx("one item")},
		})}},
		{"select other", []Run{sel(tx("they"))}},
		{"select no other first", []Run{selCases(map[string][]Run{
			"male": {tx("he")},
		})}},
		{"nested plural with placeholder", []Run{plural(tx("got "), ph("x"), tx(" things"))}},
		{"plural then text", []Run{plural(tx("N items")), tx(" total")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := len([]rune(RunsText(tc.runs)))
			assert.Equal(t, want, runsFlatLen(tc.runs))
		})
	}
}

func TestRunPosition(t *testing.T) {
	runs := []Run{tx("Hello "), ph("1"), tx("world")} // RunsText = "Hello world" (11 runes)
	require.Equal(t, "Hello world", RunsText(runs))

	cases := []struct {
		name             string
		off              int
		wantRun, wantOff int
	}{
		{"start", 0, 0, 0},
		{"mid first run", 3, 0, 3},
		{"end of first run attaches to following run", 6, 1, 0},
		{"into second text run", 8, 2, 2},
		{"end", 11, 3, 0},
		{"beyond end", 50, 3, 0},
		{"negative", -5, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, o := runPosition(runs, tc.off)
			assert.Equal(t, tc.wantRun, r, "run")
			assert.Equal(t, tc.wantOff, o, "offset")
		})
	}
}

func TestRunPositionPluralSelect(t *testing.T) {
	// "Sees " + plural(other="cats") + " now" => "Sees cats now" (13 runes)
	runs := []Run{tx("Sees "), plural(tx("cats")), tx(" now")}
	require.Equal(t, "Sees cats now", RunsText(runs))

	cases := []struct {
		name             string
		off              int
		wantRun, wantOff int
	}{
		{"before plural", 5, 1, 0},
		{"inside plural attributes to plural run", 7, 1, 2},
		{"end of plural jumps to next", 9, 2, 0},
		{"inside trailing text", 11, 2, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, o := runPosition(runs, tc.off)
			assert.Equal(t, tc.wantRun, r, "run")
			assert.Equal(t, tc.wantOff, o, "offset")
		})
	}
}

func TestRunRangeForAndTextSpanRoundTrip(t *testing.T) {
	runs := []Run{tx("Hello "), ph("1"), tx("world")} // "Hello world"
	cases := []struct {
		name             string
		start, end       int
		wantStart, wantE int
	}{
		{"full", 0, 11, 0, 11},
		{"first word", 0, 5, 0, 5},
		{"second word", 6, 11, 6, 11},
		{"across code", 4, 8, 4, 8},
		{"empty at start", 0, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := RunRangeFor(runs, tc.start, tc.end)
			s, e := rr.TextSpan(runs)
			assert.Equal(t, tc.wantStart, s, "TextSpan start")
			assert.Equal(t, tc.wantE, e, "TextSpan end")
		})
	}
}

func TestTextSpanWithPluralSelect(t *testing.T) {
	runs := []Run{tx("Sees "), plural(tx("cats")), tx(" now")} // "Sees cats now"
	// Range covering "cats now": from the plural run start to end.
	rr := RunRangeFor(runs, 5, 13)
	s, e := rr.TextSpan(runs)
	assert.Equal(t, 5, s)
	assert.Equal(t, 13, e)

	bs, be := rr.ByteSpan(runs)
	assert.Equal(t, 5, bs)
	assert.Equal(t, 13, be)
}

func TestByteSpanMultibyte(t *testing.T) {
	// "héllo wörld": é and ö are 2 bytes each in UTF-8.
	runs := []Run{tx("héllo "), tx("wörld")}
	text := RunsText(runs)
	require.Equal(t, "héllo wörld", text)

	// Rune span [6,11) = "wörld"; bytes: "héllo " = 7 bytes (h é(2) l l o space).
	rr := RunRangeFor(runs, 6, 11)
	bs, be := rr.ByteSpan(runs)
	assert.Equal(t, 7, bs)
	assert.Equal(t, len(text), be) // 13 bytes total ("wörld" trailing)
	assert.Equal(t, "wörld", text[bs:be])
}

func TestRuneToByteOffset(t *testing.T) {
	s := "héllo" // h(1) é(2) l(1) l(1) o(1) = 6 bytes, 5 runes
	cases := []struct {
		runeOff  int
		wantByte int
	}{
		{0, 0},
		{1, 1},
		{2, 3}, // after é
		{5, 6},
		{99, 6}, // clamped to len
		{-1, 0},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantByte, runeToByteOffset(s, tc.runeOff), "runeOff=%d", tc.runeOff)
	}
}

func TestRunRangeForBytes(t *testing.T) {
	runs := []Run{tx("héllo wörld")}
	text := RunsText(runs)

	// Byte span for "wörld" starts at byte 7.
	rr := RunRangeForBytes(runs, 7, len(text))
	got := rr.ExtractRuns(runs)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Text)
	assert.Equal(t, "wörld", got[0].Text.Text)

	// Out-of-range bytes clamp.
	rr2 := RunRangeForBytes(runs, -5, 9999)
	s, e := rr2.TextSpan(runs)
	assert.Equal(t, 0, s)
	assert.Equal(t, len([]rune(text)), e)
}

// TestRunRangeForBytesPluralRegression is the direct #15 reproduction: a byte
// span reported against RunsText over a plural-bearing block must yield a
// non-degenerate range that ExtractRuns can resolve to real content.
func TestRunRangeForBytesPluralRegression(t *testing.T) {
	// "You have " + plural(other="3 messages") => RunsText "You have 3 messages"
	runs := []Run{tx("You have "), plural(tx("3 messages"))}
	text := RunsText(runs)
	require.Equal(t, "You have 3 messages", text)

	// A detector reports the byte span of "messages" inside RunsText.
	start := indexOf(text, "messages")
	require.GreaterOrEqual(t, start, 0)
	rr := RunRangeForBytes(runs, start, start+len("messages"))

	// Before the fix this produced a degenerate empty range / "".
	assert.False(t, rr.IsZero(), "range must not be degenerate")
	got := rr.ExtractRuns(runs)
	require.NotEmpty(t, got, "ExtractRuns must return content")
	// The plural run is atomic for extraction: the whole plural is returned.
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Plural)
}

func TestExtractRuns(t *testing.T) {
	runs := []Run{tx("Hello "), ph("1"), tx("world")}
	cases := []struct {
		name  string
		rr    RunRange
		wantT []string // expected text-run contents (nil entries ignored)
	}{
		{
			name:  "full range keeps code",
			rr:    RunRange{StartRun: 0, StartOffset: 0, EndRun: 3, EndOffset: 0},
			wantT: []string{"Hello ", "world"},
		},
		{
			name:  "split first run",
			rr:    RunRange{StartRun: 0, StartOffset: 0, EndRun: 0, EndOffset: 5},
			wantT: []string{"Hello"},
		},
		{
			name:  "second word only",
			rr:    RunRange{StartRun: 2, StartOffset: 0, EndRun: 2, EndOffset: 5},
			wantT: []string{"world"},
		},
		{
			name:  "code excluded at exclusive end",
			rr:    RunRange{StartRun: 0, StartOffset: 0, EndRun: 1, EndOffset: 0},
			wantT: []string{"Hello "},
		},
		{
			name:  "invalid range returns nil",
			rr:    RunRange{StartRun: 5, StartOffset: 0, EndRun: 2, EndOffset: 0},
			wantT: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.rr.ExtractRuns(runs)
			var texts []string
			for _, r := range got {
				if r.Text != nil {
					texts = append(texts, r.Text.Text)
				}
			}
			assert.Equal(t, tc.wantT, texts)
		})
	}
}

func TestExtractRunsPluralAtomic(t *testing.T) {
	runs := []Run{tx("a "), plural(tx("b")), tx(" c")}
	// Range from the plural to end: plural included whole.
	rr := RunRange{StartRun: 1, StartOffset: 0, EndRun: 3, EndOffset: 0}
	got := rr.ExtractRuns(runs)
	require.Len(t, got, 2)
	assert.NotNil(t, got[0].Plural)
	require.NotNil(t, got[1].Text)
	assert.Equal(t, " c", got[1].Text.Text)
}

func TestRunRangeIsZero(t *testing.T) {
	assert.True(t, RunRange{}.IsZero())
	assert.False(t, RunRange{EndRun: 1}.IsZero())
}

func TestSegmentationOverlays(t *testing.T) {
	b := &Block{Source: []Run{tx("One. "), tx("Two.")}}

	// No segmentation yet.
	assert.Nil(t, b.SourceSegmentation())
	assert.Nil(t, b.SegmentationFor(nil))
	assert.False(t, b.HasSourceOverlays())
	assert.Equal(t, 1, b.SourceSegmentCount())

	spans := []Span{
		{ID: "s1", Range: RunRange{StartRun: 0, StartOffset: 0, EndRun: 1, EndOffset: 0}},
		{ID: "s2", Range: RunRange{StartRun: 1, StartOffset: 0, EndRun: 2, EndOffset: 0}},
	}
	b.SetSegmentation(nil, spans)

	require.NotNil(t, b.SourceSegmentation())
	require.NotNil(t, b.SegmentationFor(nil))
	assert.True(t, b.HasSourceOverlays())
	assert.Equal(t, 2, b.SourceSegmentCount())
	assert.Equal(t, []string{""}, b.SegmentationLayers(nil))

	// First segment runs.
	seg0 := b.SourceSegmentRuns(0)
	require.Len(t, seg0, 1)
	require.NotNil(t, seg0[0].Text)
	assert.Equal(t, "One. ", seg0[0].Text.Text)

	seg1 := b.SourceSegmentRuns(1)
	require.Len(t, seg1, 1)
	assert.Equal(t, "Two.", seg1[0].Text.Text)

	// Out-of-range index.
	assert.Nil(t, b.SourceSegmentRuns(5))

	// Named layer coexists with primary.
	b.SetSegmentationLayer(nil, "clause", spans[:1])
	assert.ElementsMatch(t, []string{"", "clause"}, b.SegmentationLayers(nil))
	require.NotNil(t, b.SegmentationLayerFor(nil, "clause"))
	assert.Equal(t, spans[:1], b.SegmentationLayerFor(nil, "clause").Spans)

	// Removing the primary layer leaves the named one.
	b.SetSegmentation(nil, nil)
	assert.Nil(t, b.SourceSegmentation())
	assert.NotNil(t, b.SegmentationLayerFor(nil, "clause"))
}

func TestSourceSegmentRunsNoOverlay(t *testing.T) {
	b := &Block{Source: []Run{tx("Whole")}}
	assert.Equal(t, b.Source, b.SourceSegmentRuns(0))
	assert.Nil(t, b.SourceSegmentRuns(1))

	empty := &Block{}
	assert.Equal(t, 0, empty.SourceSegmentCount())
}

func TestSpanIgnorable(t *testing.T) {
	assert.False(t, Span{}.Ignorable())
	assert.True(t, Span{Props: map[string]string{SpanPropIgnorable: "true"}}.Ignorable())
	assert.False(t, Span{Props: map[string]string{SpanPropIgnorable: "false"}}.Ignorable())
}

func TestOverlayOnSource(t *testing.T) {
	var nilOverlay *Overlay
	assert.True(t, nilOverlay.OnSource())
	assert.True(t, (&Overlay{}).OnSource())
	vk := VariantKey{}
	assert.False(t, (&Overlay{Variant: &vk}).OnSource())
}

func TestSameVariantViaSegmentation(t *testing.T) {
	vk := VariantKey{}
	b := &Block{Source: []Run{tx("x")}}
	b.SetSegmentation(&vk, []Span{{Range: RunRange{EndRun: 1}}})

	// Target-side segmentation must not be returned for the source side.
	assert.Nil(t, b.SegmentationFor(nil))
	assert.NotNil(t, b.SegmentationFor(&vk))
	assert.False(t, b.HasSourceOverlays())
}

// indexOf returns the byte index of sub in s, or -1.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
