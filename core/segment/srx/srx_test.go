package srx_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	"github.com/neokapi/neokapi/core/segment/srx"
)

// textRuns is a convenience for building a single-text-run block.
func textRuns(s string) []model.Run {
	return []model.Run{{Text: &model.TextRun{Text: s}}}
}

// segText returns the concatenated text of a span's runs.
func segText(runs []model.Run, sp model.Span) string {
	return model.RunsText(sp.Range.ExtractRuns(runs))
}

// segTexts returns each span's extracted text.
func segTexts(runs []model.Run, spans []model.Span) []string {
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = segText(runs, sp)
	}
	return out
}

func newDefault(t *testing.T) segment.Segmenter {
	t.Helper()
	eng, err := srx.New(segment.Config{})
	require.NoError(t, err)
	return eng
}

func TestSegment_Default(t *testing.T) {
	tests := []struct {
		name   string
		locale model.LocaleID
		input  string
		want   []string
	}{
		// SRX places the boundary AFTER the sentence-final punctuation, so the
		// inter-segment whitespace leads the following segment. With the default
		// (no-trim) mask the leading space is part of the next segment; see
		// TestSegment_TrimWhitespace for the trimmed projection.
		{
			name:   "two plain sentences",
			locale: "en-US",
			input:  "Hello world. This is a test.",
			want:   []string{"Hello world.", " This is a test."},
		},
		{
			name:   "three sentences with ! and ?",
			locale: "en-US",
			input:  "Stop! Who goes there? It is I.",
			want:   []string{"Stop!", " Who goes there?", " It is I."},
		},
		{
			name:   "honorific abbreviation does not break",
			locale: "en-US",
			input:  "Dr. Smith left. He returned.",
			want:   []string{"Dr. Smith left.", " He returned."},
		},
		{
			name:   "Mr and Mrs abbreviations",
			locale: "en-US",
			input:  "Mr. and Mrs. Brown arrived. They sat down.",
			want:   []string{"Mr. and Mrs. Brown arrived.", " They sat down."},
		},
		{
			name:   "decimal number does not break",
			locale: "en-US",
			input:  "It cost 3.50 today. Yes.",
			want:   []string{"It cost 3.50 today.", " Yes."},
		},
		{
			name:   "single initials do not break",
			locale: "en-US",
			input:  "The author is J. K. Rowling. She is famous.",
			want:   []string{"The author is J. K. Rowling.", " She is famous."},
		},
		{
			name:   "e.g. does not break",
			locale: "en-US",
			input:  "Use a fruit, e.g. an apple, for this. Then stop.",
			want:   []string{"Use a fruit, e.g. an apple, for this.", " Then stop."},
		},
		{
			name:   "ellipsis trailing into lowercase does not break",
			locale: "en-US",
			input:  "Well... it depends. Maybe.",
			want:   []string{"Well... it depends.", " Maybe."},
		},
		{
			name:   "ellipsis followed by uppercase breaks",
			locale: "en-US",
			input:  "Wait... What happened?",
			want:   []string{"Wait...", " What happened?"},
		},
		{
			name:   "single sentence no trailing break",
			locale: "en-US",
			input:  "Just one sentence here.",
			want:   []string{"Just one sentence here."},
		},
		{
			name:   "no terminal punctuation is one segment",
			locale: "en-US",
			input:  "no punctuation at all",
			want:   []string{"no punctuation at all"},
		},
		{
			name:   "abbreviation at true sentence end still terminates",
			locale: "en-US",
			input:  "Smith lives on Main Ave. The house is blue.",
			want:   []string{"Smith lives on Main Ave. The house is blue."},
		},
	}

	eng := newDefault(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runs := textRuns(tc.input)
			spans, err := eng.Segment(context.Background(), runs, tc.locale)
			require.NoError(t, err)
			assert.Equal(t, tc.want, segTexts(runs, spans))
		})
	}
}

func TestSegment_NonASCII(t *testing.T) {
	tests := []struct {
		name   string
		locale model.LocaleID
		input  string
		want   []string
	}{
		{
			name:   "accented French",
			locale: "fr-FR",
			input:  "Café fermé. Déjà vu.",
			want:   []string{"Café fermé.", " Déjà vu."},
		},
		{
			name:   "German with umlauts",
			locale: "de-DE",
			input:  "Das ist schön. Über alles.",
			want:   []string{"Das ist schön.", " Über alles."},
		},
		{
			name:   "CJK ideographic full stops",
			locale: "ja-JP",
			input:  "これはテストです。次の文です。",
			want:   []string{"これはテストです。", "次の文です。"},
		},
		{
			// The astral emoji (one rune, two UTF-16 code units) must not shift
			// the boundary: regexp2 reports rune offsets, so the break lands at
			// the period regardless of supplementary-plane characters before it.
			name:   "astral emoji does not shift offsets",
			locale: "en-US",
			input:  "Look 😀 here. Now stop.",
			want:   []string{"Look 😀 here.", " Now stop."},
		},
	}

	eng := newDefault(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runs := textRuns(tc.input)
			spans, err := eng.Segment(context.Background(), runs, tc.locale)
			require.NoError(t, err)
			assert.Equal(t, tc.want, segTexts(runs, spans))
		})
	}
}

func TestSegment_Empty(t *testing.T) {
	eng := newDefault(t)

	spans, err := eng.Segment(context.Background(), nil, "en-US")
	require.NoError(t, err)
	assert.Nil(t, spans)

	spans, err = eng.Segment(context.Background(), textRuns(""), "en-US")
	require.NoError(t, err)
	assert.Nil(t, spans)
}

func TestSegment_WithCodes(t *testing.T) {
	eng := newDefault(t)

	// A placeholder run between two sentences must not derail boundary
	// detection: by default it contributes nothing to the masked text, so the
	// surrounding text is "First one.Second two." and breaks at the period.
	runs := []model.Run{
		{Text: &model.TextRun{Text: "First one. "}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "var", Equiv: "{x}"}},
		{Text: &model.TextRun{Text: "Second two."}},
	}
	spans, err := eng.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	got := segTexts(runs, spans)
	require.Len(t, got, 2)
	assert.Equal(t, "First one.", got[0])
	// The second segment includes the placeholder run and the trailing text.
	assert.Contains(t, got[1], "Second two.")
}

func TestSegment_TrimWhitespace(t *testing.T) {
	// With TrimLeadingWS set on the mask, the inter-segment space is moved out
	// of the following segment and becomes an implicit ignorable.
	eng, err := srx.New(segment.Config{Mask: segment.MaskOptions{TrimLeadingWS: true}})
	require.NoError(t, err)

	runs := textRuns("Hello world. This is a test.")
	spans, err := eng.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	assert.Equal(t, []string{"Hello world.", "This is a test."}, segTexts(runs, spans))
}

func TestSegment_TrimWhitespaceFromHeader(t *testing.T) {
	// A ruleset that asks to trim via Okapi's header extension
	// (<okpsrx:options trimLeadingWhitespaces="yes" .../>) trims its segments
	// even with no mask set on the Config — this is how okapi.srx behaves.
	const rules = `<?xml version="1.0"?>
<srx xmlns="http://www.lisa.org/srx20" xmlns:okpsrx="http://okapi.sf.net/srx-extensions" version="2.0">
  <header segmentsubflows="yes" cascade="yes">
    <okpsrx:options trimLeadingWhitespaces="yes" trimTrailingWhitespaces="yes"/>
  </header>
  <body>
    <languagerules>
      <languagerule languagerulename="default">
        <rule break="yes"><beforebreak>[.!?]</beforebreak><afterbreak>\s</afterbreak></rule>
      </languagerule>
    </languagerules>
    <maprules><languagemap languagepattern=".*" languagerulename="default"/></maprules>
  </body>
</srx>`
	eng, err := srx.New(segment.Config{SrxRules: rules})
	require.NoError(t, err)

	runs := textRuns("Hello world. This is a test.")
	spans, err := eng.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	// Without the header trim the second segment would lead with a space.
	assert.Equal(t, []string{"Hello world.", "This is a test."}, segTexts(runs, spans))
}

func TestSegment_LayerIsSentence(t *testing.T) {
	eng := newDefault(t)
	assert.Equal(t, segment.LayerSentence, eng.Layer())
}

func TestSegment_RegisteredAsDefault(t *testing.T) {
	assert.True(t, segment.HasEngine(segment.DefaultEngine))
	eng, err := segment.NewEngine(segment.DefaultEngine, segment.Config{})
	require.NoError(t, err)
	require.NotNil(t, eng)
	assert.Equal(t, segment.LayerSentence, eng.Layer())
}

func TestSegment_CustomInlineRules(t *testing.T) {
	// A minimal ruleset that breaks on every semicolon followed by a space.
	const rules = `<?xml version="1.0"?>
<srx version="2.0">
  <header segmentsubflows="yes" cascade="no"/>
  <body>
    <languagerules>
      <languagerule languagerulename="Semi">
        <rule break="yes">
          <beforebreak>;</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern=".*" languagerulename="Semi"/>
    </maprules>
  </body>
</srx>`

	eng, err := srx.New(segment.Config{SrxRules: rules})
	require.NoError(t, err)

	runs := textRuns("alpha; beta; gamma")
	spans, err := eng.Segment(context.Background(), runs, "xx")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha;", " beta;", " gamma"}, segTexts(runs, spans))
}

// TestSegment_FirstMatchWins verifies that a no-break exception placed before a
// break rule suppresses the split at the same position (the SRX cascade /
// decision-map semantics), and that swapping the order changes the outcome.
func TestSegment_FirstMatchWins(t *testing.T) {
	// Exception first: "no break before a space + lowercase after a period"
	// then "break on a period + space". For "ab. cd." the first period is
	// followed by " c" (lowercase) so the exception wins -> no break there.
	const exceptionFirst = `<?xml version="1.0"?>
<srx version="2.0">
  <header cascade="no"/>
  <body>
    <languagerules>
      <languagerule languagerulename="R">
        <rule break="no">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s\p{Ll}</afterbreak>
        </rule>
        <rule break="yes">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern=".*" languagerulename="R"/>
    </maprules>
  </body>
</srx>`

	eng, err := srx.New(segment.Config{SrxRules: exceptionFirst})
	require.NoError(t, err)
	runs := textRuns("ab. cd. Ef.")
	spans, err := eng.Segment(context.Background(), runs, "xx")
	require.NoError(t, err)
	// First period: followed by " c" -> exception -> no break.
	// Second period: followed by " E" -> not lowercase, exception misses,
	// break rule fires -> split.
	assert.Equal(t, []string{"ab. cd.", " Ef."}, segTexts(runs, spans))

	// Break first: the break rule claims the position before the exception can,
	// so both periods (followed by a space) break.
	const breakFirst = `<?xml version="1.0"?>
<srx version="2.0">
  <header cascade="no"/>
  <body>
    <languagerules>
      <languagerule languagerulename="R">
        <rule break="yes">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
        <rule break="no">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s\p{Ll}</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern=".*" languagerulename="R"/>
    </maprules>
  </body>
</srx>`

	eng2, err := srx.New(segment.Config{SrxRules: breakFirst})
	require.NoError(t, err)
	runs2 := textRuns("ab. cd. Ef.")
	spans2, err := eng2.Segment(context.Background(), runs2, "xx")
	require.NoError(t, err)
	assert.Equal(t, []string{"ab.", " cd.", " Ef."}, segTexts(runs2, spans2))
}

// TestSegment_Cascade verifies that with cascade="yes" the rules of every
// matching language map are accumulated in map order, while cascade="no" uses
// only the first matching map's rules.
func TestSegment_Cascade(t *testing.T) {
	// Two maps both match "en". The first (Colon) breaks on ": ", the second
	// (Period) breaks on ". ".
	const ruleset = `<?xml version="1.0"?>
<srx version="2.0">
  <header cascade="%s"/>
  <body>
    <languagerules>
      <languagerule languagerulename="Colon">
        <rule break="yes">
          <beforebreak>:</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
      <languagerule languagerulename="Period">
        <rule break="yes">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern="en.*" languagerulename="Colon"/>
      <languagemap languagepattern=".*" languagerulename="Period"/>
    </maprules>
  </body>
</srx>`

	input := "one: two. three"

	// cascade=yes -> both Colon and Period rules apply.
	engYes, err := srx.New(segment.Config{SrxRules: fmt.Sprintf(ruleset, "yes")})
	require.NoError(t, err)
	runs := textRuns(input)
	spansYes, err := engYes.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	assert.Equal(t, []string{"one:", " two.", " three"}, segTexts(runs, spansYes))

	// cascade=no -> only the first matching map (Colon) applies.
	engNo, err := srx.New(segment.Config{SrxRules: fmt.Sprintf(ruleset, "no")})
	require.NoError(t, err)
	runs2 := textRuns(input)
	spansNo, err := engNo.Segment(context.Background(), runs2, "en-US")
	require.NoError(t, err)
	assert.Equal(t, []string{"one:", " two. three"}, segTexts(runs2, spansNo))
}

// TestSegment_LanguageOverride checks Config.Language overrides the per-call
// locale for map selection.
func TestSegment_LanguageOverride(t *testing.T) {
	const ruleset = `<?xml version="1.0"?>
<srx version="2.0">
  <header cascade="no"/>
  <body>
    <languagerules>
      <languagerule languagerulename="Colon">
        <rule break="yes"><beforebreak>:</beforebreak><afterbreak>\s</afterbreak></rule>
      </languagerule>
      <languagerule languagerulename="Period">
        <rule break="yes"><beforebreak>\.</beforebreak><afterbreak>\s</afterbreak></rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern="fr.*" languagerulename="Colon"/>
      <languagemap languagepattern=".*" languagerulename="Period"/>
    </maprules>
  </body>
</srx>`

	// Config.Language forces fr -> Colon rules, regardless of the per-call loc.
	eng, err := srx.New(segment.Config{SrxRules: ruleset, Language: "fr-CA"})
	require.NoError(t, err)
	runs := textRuns("a: b. c")
	spans, err := eng.Segment(context.Background(), runs, "en-US")
	require.NoError(t, err)
	assert.Equal(t, []string{"a:", " b. c"}, segTexts(runs, spans))
}
