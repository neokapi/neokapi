package openxml

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi-filter: openxml

// skipStart reads and discards the opening XML start element.
func skipStart(t *testing.T, d *xml.Decoder) {
	t.Helper()
	_, err := d.Token()
	require.NoError(t, err)
}

func TestParseRunPropsEmpty(t *testing.T) {
	input := `<w:rPr></w:rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.True(t, props.isEmpty())
}

func TestParseRunPropsBold(t *testing.T) {
	input := `<rPr><b/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.True(t, props.bold)
	assert.False(t, props.italic)
}

func TestParseRunPropsBoldFalse(t *testing.T) {
	input := `<rPr><b val="0"/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.False(t, props.bold)
}

func TestParseRunPropsMultiple(t *testing.T) {
	input := `<rPr><b/><i/><u val="single"/><strike/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.True(t, props.bold)
	assert.True(t, props.italic)
	assert.Equal(t, "single", props.underline)
	assert.True(t, props.strike)
}

func TestParseRunPropsVertAlign(t *testing.T) {
	input := `<rPr><vertAlign val="superscript"/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.Equal(t, "superscript", props.vertAlign)
}

func TestParseRunPropsVanish(t *testing.T) {
	input := `<rPr><vanish/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.True(t, props.vanish)
}

func TestParseRunPropsAggressiveCleanup(t *testing.T) {
	// rsid and proofErr should be stripped in aggressive mode
	input := `<rPr><b/><rsidR val="001234"/><noProof/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true, nil)
	require.NoError(t, err)
	assert.True(t, props.bold)
	// rsid should not affect formatting comparison
}

func TestRunPropsEqual(t *testing.T) {
	a := runProps{bold: true, italic: true}
	b := runProps{bold: true, italic: true}
	assert.True(t, a.equal(b))

	c := runProps{bold: true}
	assert.False(t, a.equal(c))
}

func TestRunBuilderAddTextCoalesces(t *testing.T) {
	b := &runBuilder{}
	b.AddText("hello ")
	b.AddText("world")
	runs := b.Runs()
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "hello world", runs[0].Text.Text)
}

func TestRunBuilderBreakSplitsTextRun(t *testing.T) {
	// Phase 4: Break() preserves heterogeneous-rPr boundaries between
	// adjacent source runs whose toggle props match (so no PcOpen/
	// PcClose break is emitted) but whose non-toggle rPrChildren
	// differ. Mirrors upstream Okapi RunBuilder.java lines 73-188 +
	// RunMerger.canRunPropertiesBeMerged (RunMerger.java lines
	// 156-229) — heterogeneous RunProperties keep runs distinct on
	// the way to the writer. Per ECMA-376-1 §17.3.2.
	b := &runBuilder{}
	b.AddText("red ")
	b.Break()
	b.AddText("blue")
	runs := b.Runs()
	require.Len(t, runs, 2)
	require.NotNil(t, runs[0].Text)
	require.NotNil(t, runs[1].Text)
	assert.Equal(t, "red ", runs[0].Text.Text)
	assert.Equal(t, "blue", runs[1].Text.Text)
}

func TestRunBuilderBreakIsOneShot(t *testing.T) {
	// Calling Break() then AddText starts a new run; subsequent
	// AddText calls coalesce as usual until Break() is called again.
	b := &runBuilder{}
	b.AddText("a")
	b.Break()
	b.AddText("b")
	b.AddText("c")
	runs := b.Runs()
	require.Len(t, runs, 2)
	assert.Equal(t, "a", runs[0].Text.Text)
	assert.Equal(t, "bc", runs[1].Text.Text)
}

func TestRunBuilderBreakBeforeFirstAddIsHarmless(t *testing.T) {
	b := &runBuilder{}
	b.Break()
	b.AddText("hello")
	runs := b.Runs()
	require.Len(t, runs, 1)
	assert.Equal(t, "hello", runs[0].Text.Text)
}

func TestRunPropsOpeningClosingRuns(t *testing.T) {
	props := runProps{bold: true, italic: true}
	counter := 0

	b := &runBuilder{}
	props.appendOpeningRuns(b, &counter)
	opening := b.Runs()
	assert.Len(t, opening, 2)
	assert.NotNil(t, opening[0].PcOpen)
	assert.NotNil(t, opening[1].PcOpen)
	assert.Equal(t, TypeBold, opening[0].PcOpen.Type)
	assert.Equal(t, TypeItalic, opening[1].PcOpen.Type)

	cb := &runBuilder{}
	props.appendClosingRuns(cb, &counter)
	closing := cb.Runs()
	assert.Len(t, closing, 2)
	assert.NotNil(t, closing[0].PcClose)
	assert.NotNil(t, closing[1].PcClose)
	// Closing should be in reverse order
	assert.Equal(t, TypeItalic, closing[0].PcClose.Type)
	assert.Equal(t, TypeBold, closing[1].PcClose.Type)
}

func TestMergeRuns(t *testing.T) {
	tests := []struct {
		name     string
		runs     []textRun
		expected int
	}{
		{
			name:     "single run",
			runs:     []textRun{{text: "hello", props: runProps{}}},
			expected: 1,
		},
		{
			name: "same formatting merges",
			runs: []textRun{
				{text: "hello ", props: runProps{bold: true}},
				{text: "world", props: runProps{bold: true}},
			},
			expected: 1,
		},
		{
			name: "different formatting keeps separate",
			runs: []textRun{
				{text: "hello ", props: runProps{bold: true}},
				{text: "world", props: runProps{italic: true}},
			},
			expected: 2,
		},
		{
			name: "three runs, two merge",
			runs: []textRun{
				{text: "a", props: runProps{bold: true}},
				{text: "b", props: runProps{bold: true}},
				{text: "c", props: runProps{}},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged := mergeRuns(tt.runs)
			assert.Len(t, merged, tt.expected)
		})
	}
}

func TestMergeRunsPreservesText(t *testing.T) {
	runs := []textRun{
		{text: "hello ", props: runProps{bold: true}},
		{text: "world", props: runProps{bold: true}},
	}
	merged := mergeRuns(runs)
	require.Len(t, merged, 1)
	assert.Equal(t, "hello world", merged[0].text)
}

func TestIsEmptyRuns(t *testing.T) {
	assert.True(t, isEmptyRuns(nil))
	assert.True(t, isEmptyRuns([]textRun{{text: "  "}}))
	assert.False(t, isEmptyRuns([]textRun{{text: "hello"}}))
}

// TestSentinelClassifiers locks the rune-inspection behavior of the
// sentinel predicates, which were converted from []rune(s) to
// utf8.DecodeRuneInString for zero-allocation (umbrella #608, O2). The
// classification must stay byte-identical to the slice-based version.
func TestSentinelClassifiers(t *testing.T) {
	const (
		tab          = "\uE100"
		image        = "\uE101"
		opaqueIDA    = "\uE102:abc"
		paraOpaque   = "\uE105"
		bookmarkOpen = "\uE106"
		bookmarkEnd  = "\uE107"
		field        = "\uE108"
		fieldPayload = "\uE108:fldChar"
		commentOpen  = "\uE10B"
		commentEnd   = "\uE10C"
		topRange     = "\uE10F"
		// Multi-char sentinel without the required ':' separator.
		badMulti = "\uE102x"
		// Just below / above the reserved sentinel range.
		belowRange = "\uE0FF"
		aboveRange = "\uE110"
	)

	t.Run("isSentinel", func(t *testing.T) {
		assert.False(t, isSentinel(""))
		assert.False(t, isSentinel("plain text"))
		assert.False(t, isSentinel(belowRange))
		assert.False(t, isSentinel(aboveRange))
		// Single-rune sentinels.
		assert.True(t, isSentinel(tab))
		assert.True(t, isSentinel(image))
		assert.True(t, isSentinel(paraOpaque))
		assert.True(t, isSentinel(topRange))
		// Multi-rune sentinels require a ':' immediately after the marker.
		assert.True(t, isSentinel(opaqueIDA))
		assert.True(t, isSentinel(fieldPayload))
		assert.False(t, isSentinel(badMulti))
	})

	t.Run("isFieldSentinel", func(t *testing.T) {
		assert.False(t, isFieldSentinel(""))
		assert.False(t, isFieldSentinel("plain"))
		assert.False(t, isFieldSentinel(image))
		assert.True(t, isFieldSentinel(field))
		assert.True(t, isFieldSentinel(fieldPayload))
	})

	t.Run("isCommentRangeSentinel", func(t *testing.T) {
		assert.False(t, isCommentRangeSentinel(""))
		assert.False(t, isCommentRangeSentinel(field))
		assert.True(t, isCommentRangeSentinel(commentOpen))
		assert.True(t, isCommentRangeSentinel(commentEnd))
	})

	t.Run("isBookmarkSentinel", func(t *testing.T) {
		assert.False(t, isBookmarkSentinel(""))
		assert.False(t, isBookmarkSentinel(field))
		assert.True(t, isBookmarkSentinel(bookmarkOpen))
		assert.True(t, isBookmarkSentinel(bookmarkEnd))
	})

	t.Run("isDrawingSentinel", func(t *testing.T) {
		assert.False(t, isDrawingSentinel(""))
		assert.False(t, isDrawingSentinel(field))
		assert.True(t, isDrawingSentinel(image))
		assert.True(t, isDrawingSentinel(paraOpaque))
	})
}

// TestParseRunPropsFromRawCached verifies the per-part rPr parse cache
// (#608, O1) is behaviourally identical to the uncached path: a cache hit
// yields equal runProps, the returned rPrChildren is independent of the
// cached entry (so downstream in-place minification cannot corrupt later
// hits), and distinct style chains do not collide.
func TestParseRunPropsFromRawCached(t *testing.T) {
	newParser := func(strict bool) *wmlParser {
		counter := 0
		cfg := &Config{}
		cfg.Reset()
		return &wmlParser{blockCounter: &counter, cfg: cfg, strict: strict}
	}

	rpr := `<w:rPr><w:rStyle w:val="Emphasis"/><w:color w:val="FF0000"/></w:rPr>`

	t.Run("matches uncached path", func(t *testing.T) {
		p := newParser(false)
		uncached, err := parseRunPropsFromRaw(rpr, p.cfg.AggressiveCleanup, p.strict, nil)
		require.NoError(t, err)

		cached1, err := p.parseRunPropsFromRawCached(rpr, nil)
		require.NoError(t, err)
		assert.Equal(t, uncached, cached1, "first (miss) result must match uncached path")

		cached2, err := p.parseRunPropsFromRawCached(rpr, nil)
		require.NoError(t, err)
		assert.Equal(t, uncached, cached2, "second (hit) result must match uncached path")
	})

	t.Run("hit returns independent rPrChildren slice", func(t *testing.T) {
		p := newParser(false)
		first, err := p.parseRunPropsFromRawCached(rpr, nil)
		require.NoError(t, err)
		require.NotEmpty(t, first.rPrChildren)

		// Mutate the caller's slice in place, mirroring the
		// children[:0]-append minification in parseParagraph.
		first.rPrChildren = first.rPrChildren[:0]

		second, err := p.parseRunPropsFromRawCached(rpr, nil)
		require.NoError(t, err)
		assert.Len(t, second.rPrChildren, 2,
			"cache hit must not be corrupted by the previous caller's in-place mutation")
	})

	t.Run("strict flag changes parse (separate parser)", func(t *testing.T) {
		langRPr := `<w:rPr><w:lang w:eastAsia="ru-RU"/></w:rPr>`
		// Transitional namespace: <w:lang> is a skippable element.
		trans := newParser(false)
		tp, err := trans.parseRunPropsFromRawCached(langRPr, nil)
		require.NoError(t, err)
		// Strict namespace: <w:lang> is preserved.
		strict := newParser(true)
		sp, err := strict.parseRunPropsFromRawCached(langRPr, nil)
		require.NoError(t, err)
		assert.NotEqual(t, len(tp.rPrChildren), len(sp.rPrChildren),
			"strict vs transitional must parse <w:lang> differently")
	})

	t.Run("distinct style chains do not collide", func(t *testing.T) {
		p := newParser(false)
		chainA := map[string]bool{"color": true}
		chainB := map[string]bool{"sz": true}
		a, err := p.parseRunPropsFromRawCached(rpr, chainA)
		require.NoError(t, err)
		b, err := p.parseRunPropsFromRawCached(rpr, chainB)
		require.NoError(t, err)

		refA, err := parseRunPropsFromRaw(rpr, p.cfg.AggressiveCleanup, p.strict, chainA)
		require.NoError(t, err)
		refB, err := parseRunPropsFromRaw(rpr, p.cfg.AggressiveCleanup, p.strict, chainB)
		require.NoError(t, err)
		assert.Equal(t, refA, a, "chainA result must match uncached path for chainA")
		assert.Equal(t, refB, b, "chainB result must match uncached path for chainB")
	})

	t.Run("chainKeyFor is content-based", func(t *testing.T) {
		p := newParser(false)
		// Two distinct maps with identical content must share a key so
		// paragraphs with the same resolved chain reuse cache entries.
		m1 := map[string]bool{"color": true, "sz": true}
		m2 := map[string]bool{"sz": true, "color": true}
		assert.Equal(t, p.chainKeyFor(m1), p.chainKeyFor(m2))
		assert.Equal(t, "", p.chainKeyFor(nil))
		assert.NotEqual(t, p.chainKeyFor(m1), p.chainKeyFor(map[string]bool{"color": true}))
	})
}

func TestAllHidden(t *testing.T) {
	assert.True(t, allHidden([]textRun{
		{text: "hidden", props: runProps{vanish: true}},
	}, false))
	assert.False(t, allHidden([]textRun{
		{text: "visible", props: runProps{}},
	}, false))
	// Inherited vanish (e.g. from a synthesised paragraph style) marks
	// the paragraph as hidden even when the run's own rPr lacks <w:vanish/>.
	assert.True(t, allHidden([]textRun{
		{text: "visible", props: runProps{}},
	}, true))
}

// --- Complex field definitions tests ---

func TestComplexFieldCodeName(t *testing.T) {
	tests := []struct {
		instrText string
		expected  string
	}{
		{` HYPERLINK "http://example.com" \t "_blank" `, "HYPERLINK"},
		{` TOC \o "1-3" \h \z \u `, "TOC"},
		{` PAGEREF _Toc277618961 \h `, "PAGEREF"},
		{`REF`, "REF"},
		{"  DATE  ", "DATE"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, complexFieldCodeName(tt.instrText))
		})
	}
}

func TestComplexFieldExtraction(t *testing.T) {
	// A paragraph with a HYPERLINK complex field:
	//   <w:p>
	//     <w:r><w:t>Before </w:t></w:r>
	//     <w:r><w:fldChar w:fldCharType="begin"/></w:r>
	//     <w:r><w:instrText> HYPERLINK "http://example.com" </w:instrText></w:r>
	//     <w:r><w:fldChar w:fldCharType="separate"/></w:r>
	//     <w:r><w:t>Link Text</w:t></w:r>
	//     <w:r><w:fldChar w:fldCharType="end"/></w:r>
	//     <w:r><w:t> after</w:t></w:r>
	//   </w:p>
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:t>Before </w:t></w:r>
  <w:r><w:fldChar w:fldCharType="begin"/></w:r>
  <w:r><w:instrText xml:space="preserve"> HYPERLINK "http://example.com" </w:instrText></w:r>
  <w:r><w:fldChar w:fldCharType="separate"/></w:r>
  <w:r><w:t>Link Text</w:t></w:r>
  <w:r><w:fldChar w:fldCharType="end"/></w:r>
  <w:r><w:t> after</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	t.Run("extractable field includes display text", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = []string{"HYPERLINK"}

		blocks := parseDocXML(t, docXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		assert.Contains(t, text, "Before ")
		assert.Contains(t, text, "Link Text")
		assert.Contains(t, text, " after")
	})

	t.Run("non-extractable field skips display text", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		// No fields in extract list → all complex fields skipped
		cfg.ComplexFieldDefinitionsToExtract = nil

		blocks := parseDocXML(t, docXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		assert.Contains(t, text, "Before ")
		assert.NotContains(t, text, "Link Text")
		assert.Contains(t, text, " after")
	})

	t.Run("case insensitive field code match", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = []string{"hyperlink"} // lowercase

		blocks := parseDocXML(t, docXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		assert.Contains(t, text, "Link Text")
	})
}

// TestComplexFieldPreFldCharBodyText covers the 830-7.docx shape where a
// source `<w:r>` authors translatable body text BEFORE the
// `<w:fldChar w:fldCharType="begin"/>` that opens a complex field in the
// SAME run:
//
//	<w:r><w:rPr>…</w:rPr><w:t>pre-field text</w:t>
//	  <w:fldChar w:fldCharType="begin"/></w:r>
//
// Per ECMA-376-1 §17.3.2.1 (CT_R) every run child applies to the run, and
// upstream Okapi's RunParser processes the `<w:t>` as a RunText body chunk
// of the field-opening run before transitioning to parseComplexField
// (RunParser.java:259, 537). The text must be EXTRACTED (translatable),
// not suppressed by the field's begin→separate markup-only window — the
// regression that this guards against dropped it entirely.
//
// The negative case confirms text that genuinely sits INSIDE the
// begin→separate suppressed window (in its own interior run) is still
// dropped: only the body text authored before the begin in the
// field-OPENING run survives.
func TestComplexFieldPreFldCharBodyText(t *testing.T) {
	// document.xml fragment: a HYPERLINK field whose begin marker shares a
	// <w:r> with leading body text, plus an interior suppressed text run
	// (between begin and separate) that must NOT be extracted.
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">pre-field body </w:t><w:fldChar w:fldCharType="begin"/></w:r>
  <w:r><w:instrText xml:space="preserve"> SUPPRESSED INTERIOR </w:instrText></w:r>
  <w:r><w:fldChar w:fldCharType="separate"/></w:r>
  <w:r><w:t>display</w:t></w:r>
  <w:r><w:fldChar w:fldCharType="end"/></w:r>
  <w:r><w:t> trailing</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	t.Run("pre-fldChar body text in field-opening run is extracted", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = []string{"HYPERLINK"}

		blocks := parseDocXML(t, docXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		// The body text authored before the begin marker must survive.
		assert.Contains(t, text, "pre-field body ",
			"pre-fldChar body text must be extracted, not dropped by the field window")
		assert.Contains(t, text, " trailing", "post-field text must survive")
	})

	t.Run("interior suppressed text between begin and separate is dropped", func(t *testing.T) {
		// A field whose suppressed begin→separate window holds a stray
		// <w:t> in its own interior run. That run starts while the field
		// is already active, so the text is field-internal markup and must
		// not be extracted as translatable.
		interiorXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:fldChar w:fldCharType="begin"/></w:r>
  <w:r><w:t>interior suppressed</w:t></w:r>
  <w:r><w:instrText xml:space="preserve"> HYPERLINK "x" </w:instrText></w:r>
  <w:r><w:fldChar w:fldCharType="separate"/></w:r>
  <w:r><w:t>display</w:t></w:r>
  <w:r><w:fldChar w:fldCharType="end"/></w:r>
</w:p>
</w:body>
</w:document>`
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = []string{"HYPERLINK"}

		blocks := parseDocXML(t, interiorXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		assert.NotContains(t, text, "interior suppressed",
			"text inside the begin→separate window must NOT be extracted")
		assert.Contains(t, text, "display", "the field's display text is extractable")
	})
}

func TestComplexFieldNested(t *testing.T) {
	// A paragraph with a TOC field containing nested PAGEREF fields
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:fldChar w:fldCharType="begin"/></w:r>
  <w:r><w:instrText xml:space="preserve"> TOC \o "1-3" </w:instrText></w:r>
  <w:r><w:fldChar w:fldCharType="separate"/></w:r>
  <w:r><w:t>Chapter 1</w:t></w:r>
  <w:r><w:fldChar w:fldCharType="begin"/></w:r>
  <w:r><w:instrText xml:space="preserve"> PAGEREF _Toc1 \h </w:instrText></w:r>
  <w:r><w:fldChar w:fldCharType="separate"/></w:r>
  <w:r><w:t>1</w:t></w:r>
  <w:r><w:fldChar w:fldCharType="end"/></w:r>
  <w:r><w:fldChar w:fldCharType="end"/></w:r>
</w:p>
</w:body>
</w:document>`

	t.Run("non-extractable outer field skips everything", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = nil

		blocks := parseDocXML(t, docXML, cfg)
		// No translatable text → empty paragraph → no blocks
		assert.Empty(t, blocks)
	})

	t.Run("extractable outer field includes display text", func(t *testing.T) {
		cfg := &Config{}
		cfg.Reset()
		cfg.ComplexFieldDefinitionsToExtract = []string{"TOC"}

		blocks := parseDocXML(t, docXML, cfg)
		require.Len(t, blocks, 1)
		text := blocks[0].Source[0].Text()
		assert.Contains(t, text, "Chapter 1")
	})
}

// --- Style optimization tests ---

func TestStyleOptimization(t *testing.T) {
	// Document with bold text where bold is inherited from style
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:pPr><w:pStyle w:val="BoldStyle"/></w:pPr>
  <w:r><w:rPr><w:b/></w:rPr><w:t>Bold text</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	styles := &styleMap{styles: map[string]*styleEntry{
		"BoldStyle": {id: "BoldStyle", props: runProps{bold: true}},
	}}

	cfg := &Config{}
	cfg.Reset()
	cfg.OptimiseWordStyles = true

	blocks := parseDocXMLWithStyles(t, docXML, cfg, styles)
	require.Len(t, blocks, 1)
	// Bold should be subtracted (inherited from style) → no spans
	assert.False(t, blocks[0].Source[0].HasInlineCodes())
}

func TestStyleOptimizationWithInheritance(t *testing.T) {
	styles := &styleMap{styles: map[string]*styleEntry{
		"BaseStyle":  {id: "BaseStyle", props: runProps{bold: true}},
		"ChildStyle": {id: "ChildStyle", basedOn: "BaseStyle", props: runProps{italic: true}},
	}}

	resolved := styles.resolveProps("ChildStyle")
	assert.True(t, resolved.bold, "should inherit bold from parent")
	assert.True(t, resolved.italic, "should have own italic")
}

// --- Font mapping tests ---

func TestFontMappingMergesRuns(t *testing.T) {
	// Two runs with different fonts that map to the same group should merge
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>Hello </w:t></w:r>
  <w:r><w:rPr><w:rFonts w:ascii="Helvetica"/></w:rPr><w:t>World</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.FontMappings = map[string]string{
		"Arial":     "sans-serif",
		"Helvetica": "sans-serif",
	}

	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	// After font mapping, both runs have same fontName "sans-serif" → should merge
	text := blocks[0].Source[0].Text()
	assert.Equal(t, "Hello World", text)
	// Should have no spans since the merged font is "other" property, not a formatting span
	assert.False(t, blocks[0].Source[0].HasInlineCodes())
}

// --- Code finder tests ---

func TestCodeFinderBasic(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:r><w:t>Hello &lt;br&gt; World</w:t></w:r></w:p>
</w:body>
</w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.UseCodeFinder = true
	cfg.CodeFinderRules = []string{`<br>`}

	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	assert.True(t, blocks[0].Source[0].HasInlineCodes(), "should have code finder inline-code runs")
}

func TestCodeFinderDisabled(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:r><w:t>Hello &lt;br&gt; World</w:t></w:r></w:p>
</w:body>
</w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.UseCodeFinder = false

	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	assert.False(t, blocks[0].Source[0].HasInlineCodes(), "no spans when code finder disabled")
}

// --- Extract run fonts info tests ---

func TestExtractRunFontsInfo(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:rPr><w:rFonts w:ascii="Arial" w:cs="Arial" w:eastAsia="MS Mincho"/></w:rPr><w:t>Hello</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.ExtractRunFontsInfo = true

	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	ann, ok := blocks[0].Annotations["fonts"]
	require.True(t, ok, "should have fonts annotation")
	ga, ok := ann.(*model.GenericAnnotation)
	require.True(t, ok, "should be GenericAnnotation")
	names, ok := ga.Fields["names"]
	require.True(t, ok)
	namesStr, ok := names.(string)
	require.True(t, ok)
	assert.Contains(t, namesStr, "Arial")
	assert.Contains(t, namesStr, "MS Mincho")
}

func TestExtractRunFontsInfoDisabled(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p>
  <w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>Hello</w:t></w:r>
</w:p>
</w:body>
</w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.ExtractRunFontsInfo = false

	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	_, ok := blocks[0].Annotations["fonts"]
	assert.False(t, ok, "no fonts annotation when disabled")
}

// --- Collect fonts helper ---

func TestCollectFonts(t *testing.T) {
	runs := []textRun{
		{props: runProps{fontName: "Arial", fontNameCS: "Arial", fontNameEA: "MS Mincho"}},
		{props: runProps{fontName: "Arial"}}, // duplicate
		{props: runProps{fontName: "Times New Roman", fontNameCS: "Simplified Arabic"}},
	}
	result := collectFonts(runs)
	assert.Contains(t, result, "Arial")
	assert.Contains(t, result, "MS Mincho")
	assert.Contains(t, result, "Times New Roman")
	assert.Contains(t, result, "Simplified Arabic")
	// Arial should appear only once
	assert.Equal(t, 1, strings.Count(result, "Arial"))
}

// --- Helpers ---

// parseDocXML parses a WML document XML string and returns the emitted blocks.
func parseDocXML(t *testing.T, docXML string, cfg *Config) []*model.Block {
	t.Helper()
	return parseDocXMLWithStyles(t, docXML, cfg, nil)
}

func parseDocXMLWithStyles(t *testing.T, docXML string, cfg *Config, styles *styleMap) []*model.Block {
	t.Helper()
	blockCounter := 0

	var cf *codeFinder
	if cfg.UseCodeFinder && len(cfg.CodeFinderRules) > 0 {
		var err error
		cf, err = newCodeFinder(cfg.CodeFinderRules)
		require.NoError(t, err)
	}

	parser := &wmlParser{
		cfg:          cfg,
		blockCounter: &blockCounter,
		codeFinder:   cf,
		styles:       styles,
	}

	var blocks []*model.Block
	err := parser.parsePart([]byte(docXML), "word/document.xml",
		func(block *model.Block) { blocks = append(blocks, block) },
		func() {},
	)
	require.NoError(t, err)
	return blocks
}

// TestDrawingNameAttrRE verifies the regex extracts name="..." from
// docPr/cNvPr elements with various namespace prefixes — see
// drawingNameAttrRE for the matched element list.
func TestDrawingNameAttrRE(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantValue string
		wantOpen  string // expected first capture group prefix (sanity check)
	}{
		{
			name:      "wp:docPr with double quotes",
			input:     `<wp:docPr id="1" name="Bild 1"/>`,
			wantValue: "Bild 1",
		},
		{
			name:      "pic:cNvPr with double quotes",
			input:     `<pic:cNvPr id="0" descr="x" name="Picture 1"/>`,
			wantValue: "Picture 1",
		},
		{
			name:      "wps:cNvPr with single quotes",
			input:     `<wps:cNvPr id='2' name='Shape 1'/>`,
			wantValue: "Shape 1",
		},
		{
			name:      "no namespace prefix",
			input:     `<docPr id="1" name="No Prefix"/>`,
			wantValue: "No Prefix",
		},
		{
			name:      "open-close form",
			input:     `<wp:docPr id="1" name="Open Form"></wp:docPr>`,
			wantValue: "Open Form",
		},
		{
			name:      "name with empty value",
			input:     `<wp:docPr id="1" name=""/>`,
			wantValue: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := drawingNameAttrRE.FindStringSubmatch(tc.input)
			require.NotNil(t, m, "regex must match: %s", tc.input)
			assert.Equal(t, tc.wantValue, m[3], "captured value")
		})
	}
}

// TestDrawingNameAttrRE_NonMatches verifies the regex does NOT match
// unrelated elements that happen to have a name attribute.
func TestDrawingNameAttrRE_NonMatches(t *testing.T) {
	nonMatches := []string{
		// other elements with name attribute should not match
		`<w:bookmarkStart name="bookmark"/>`,
		`<wp:cNvGraphicFramePr name="x"/>`, // not docPr/cNvPr
		// docPr without name attribute
		`<wp:docPr id="1"/>`,
		// stray text
		`name="Picture 1"`,
	}
	for _, s := range nonMatches {
		t.Run(s, func(t *testing.T) {
			m := drawingNameAttrRE.FindStringSubmatch(s)
			assert.Nil(t, m, "regex must not match: %s", s)
		})
	}
}

// TestExtractDrawingTranslations_NameAttr verifies that a docPr
// name attribute inside a captured drawing payload is replaced
// with a property marker and a "property" Block emitted.
func TestExtractDrawingTranslations_NameAttr(t *testing.T) {
	counter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &wmlParser{blockCounter: &counter, cfg: cfg}
	var emitted []*model.Block
	emit := func(b *model.Block) { emitted = append(emitted, b) }
	in := `<w:drawing><wp:docPr id="1" name="Picture 1"/></w:drawing>`
	out := p.extractDrawingTranslations(in, "word/document.xml", emit)
	require.Len(t, emitted, 1)
	assert.Equal(t, "property", emitted[0].Type)
	assert.Equal(t, "drawing-name", emitted[0].Properties["element"])
	assert.Contains(t, out, drawingMarkerPropPrefix)
	assert.NotContains(t, out, `name="Picture 1"`)
}

// TestExtractDrawingTranslations_TextpathString verifies that a
// v:textpath string attribute inside a captured drawing payload is
// replaced with a property marker.
func TestExtractDrawingTranslations_TextpathString(t *testing.T) {
	counter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &wmlParser{blockCounter: &counter, cfg: cfg}
	var emitted []*model.Block
	emit := func(b *model.Block) { emitted = append(emitted, b) }
	in := `<w:pict><v:shape><v:textpath string="Word art is amazing!" trim="t"/></v:shape></w:pict>`
	out := p.extractDrawingTranslations(in, "word/document.xml", emit)
	require.Len(t, emitted, 1)
	assert.Equal(t, "property", emitted[0].Type)
	assert.Equal(t, "vml-textpath-string", emitted[0].Properties["element"])
	assert.Contains(t, out, drawingMarkerPropPrefix)
	assert.NotContains(t, out, `string="Word art is amazing!"`)
	// Other attributes preserved.
	assert.Contains(t, out, `trim="t"`)
}

// TestExtractDrawingTranslations_TxbxContent verifies that a
// <w:txbxContent><w:p>...</w:p></w:txbxContent> body produces a
// paragraph marker and a "paragraph" Block.
func TestExtractDrawingTranslations_TxbxContent(t *testing.T) {
	counter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &wmlParser{blockCounter: &counter, cfg: cfg}
	var emitted []*model.Block
	emit := func(b *model.Block) { emitted = append(emitted, b) }
	in := `<wps:txbx><w:txbxContent><w:p><w:r><w:t>This is a test sentence.</w:t></w:r></w:p></w:txbxContent></wps:txbx>`
	out := p.extractDrawingTranslations(in, "word/document.xml", emit)
	require.Len(t, emitted, 1)
	assert.Equal(t, "paragraph", emitted[0].Type)
	assert.Contains(t, out, drawingMarkerParaPrefix)
	assert.NotContains(t, out, "This is a test sentence.")
	// txbxContent and paragraph wrappers preserved.
	assert.Contains(t, out, "<w:txbxContent>")
	assert.Contains(t, out, "<w:p>")
}

// TestExtractDrawingTranslations_TxbxComplexFieldVerbatim verifies
// that a textbox paragraph carrying a complex field is preserved
// verbatim — extraction would lose the fldChar markers since
// parseRunWithFieldState's non-extractable-field path drops them.
func TestExtractDrawingTranslations_TxbxComplexFieldVerbatim(t *testing.T) {
	counter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &wmlParser{blockCounter: &counter, cfg: cfg}
	var emitted []*model.Block
	emit := func(b *model.Block) { emitted = append(emitted, b) }
	in := `<wps:txbx><w:txbxContent><w:p><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p></w:txbxContent></wps:txbx>`
	out := p.extractDrawingTranslations(in, "word/document.xml", emit)
	assert.Empty(t, emitted, "complex-field paragraph must NOT emit a translatable block")
	assert.NotContains(t, out, drawingMarkerParaPrefix)
	assert.Contains(t, out, `<w:fldChar w:fldCharType="begin">`)
	assert.Contains(t, out, "PAGE")
	assert.Contains(t, out, `<w:fldChar w:fldCharType="end">`)
}

// TestExtractDrawingTranslations_BareTInChoice verifies that a
// bare <w:t> element appearing as a direct child of <mc:Choice>
// (AltContentEscaping.docx pattern) emits a TEXT marker plus a
// translatable property block. Per ECMA-376 Part 3 §10 the
// consumer walks INTO mc:Choice transparently, so any inner
// <w:t> retains its CT_Text translatable semantics.
func TestExtractDrawingTranslations_BareTInChoice(t *testing.T) {
	counter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &wmlParser{blockCounter: &counter, cfg: cfg}
	var emitted []*model.Block
	emit := func(b *model.Block) { emitted = append(emitted, b) }
	in := `<mc:AlternateContent><mc:Choice Requires="wpg"><w:t xml:space="preserve"> &amp; &lt; &gt; &amp;amp; Grouping options</w:t></mc:Choice></mc:AlternateContent>`
	out := p.extractDrawingTranslations(in, "word/document.xml", emit)
	require.Len(t, emitted, 1)
	assert.Equal(t, "property", emitted[0].Type)
	assert.Equal(t, "alt-content-text", emitted[0].Properties["element"])
	srcRuns := emitted[0].Source[0].Runs
	require.Len(t, srcRuns, 1)
	require.NotNil(t, srcRuns[0].Text)
	// Decoder applies entity decoding once on read.
	assert.Equal(t, " & < > &amp; Grouping options", srcRuns[0].Text.Text)
	// The rewritten payload preserves the wrapper + Choice + start/
	// end <w:t> tag, with the character data replaced by a TEXT
	// marker.
	assert.Contains(t, out, drawingMarkerTextPrefix)
	assert.Contains(t, out, `<mc:AlternateContent>`)
	assert.Contains(t, out, `<mc:Choice Requires="wpg">`)
	assert.Contains(t, out, `<w:t xml:space="preserve">`)
	assert.NotContains(t, out, "Grouping options")
}

// TestDrawingMarkerRE verifies the marker regex captures kind+id.
func TestDrawingMarkerRE(t *testing.T) {
	cases := []struct {
		input    string
		wantKind string
		wantID   string
	}{
		{`<!--KAPI-PROP:tu1-->`, "PROP", "tu1"},
		{`<!--KAPI-PARA:tu42-->`, "PARA", "tu42"},
		{`<!--KAPI-TEXT:tu7-->`, "TEXT", "tu7"},
	}
	for _, tc := range cases {
		m := drawingMarkerRE.FindStringSubmatch(tc.input)
		require.Len(t, m, 3, "regex must match: %s", tc.input)
		assert.Equal(t, tc.wantKind, m[1])
		assert.Equal(t, tc.wantID, m[2])
	}
}

// TestParseParagraph_BookmarkPreserved verifies that a non-_GoBack
// bookmark inside a paragraph is captured as an inline placeholder
// run carrying the verbatim XML (start AND end), so the writer can
// reinsert the bookmark at its original position. ECMA-376 Part 1
// §17.13.6.1 / §17.13.6.2; mirrors upstream Okapi
// BlockSkippableElements default-fall-through behaviour for non-
// _GoBack bookmarks (BlockSkippableElements.java lines 116-121,
// BlockParser.java line 294 — the bookmark element is added as a
// Markup chunk on the Block).
func TestParseParagraph_BookmarkPreserved(t *testing.T) {
	docXML := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p>` +
		`<w:bookmarkStart w:id="1" w:name="Text1"/>` +
		`<w:r><w:t>hello</w:t></w:r>` +
		`<w:bookmarkEnd w:id="1"/>` +
		`</w:p>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	runs := blocks[0].Source[0].Runs
	// Expected: bookmarkStart placeholder, "hello" text, bookmarkEnd placeholder.
	require.Len(t, runs, 3, "expect bookmarkStart + text + bookmarkEnd runs")

	require.NotNil(t, runs[0].Ph)
	assert.Equal(t, TypeBookmark, runs[0].Ph.Type)
	assert.Equal(t, SubTypeBookmarkStart, runs[0].Ph.SubType)
	assert.Contains(t, runs[0].Ph.Data, `<w:bookmarkStart`)
	assert.Contains(t, runs[0].Ph.Data, `w:id="1"`)
	assert.Contains(t, runs[0].Ph.Data, `w:name="Text1"`)

	require.NotNil(t, runs[1].Text)
	assert.Equal(t, "hello", runs[1].Text.Text)

	require.NotNil(t, runs[2].Ph)
	assert.Equal(t, TypeBookmark, runs[2].Ph.Type)
	assert.Equal(t, SubTypeBookmarkEnd, runs[2].Ph.SubType)
	assert.Contains(t, runs[2].Ph.Data, `<w:bookmarkEnd`)
	assert.Contains(t, runs[2].Ph.Data, `w:id="1"`)
}

// TestParseParagraph_GoBackBookmarkSkipped verifies that the well-
// known `_GoBack` bookmark — Word's auto-generated last-edit-position
// marker — is silently dropped along with its matching end (by id),
// mirroring upstream Okapi
// SkippableElements.BookmarkCrossStructure.SKIPPABLE_BOOKMARK_NAME
// (SkippableElements.java line 304). The test also threads the
// state machine: a different-id bookmark before _GoBack should be
// preserved, and a different-id bookmark after _GoBack should also
// be preserved (because the skipped-id state is cleared once the
// matching end is consumed).
func TestParseParagraph_GoBackBookmarkSkipped(t *testing.T) {
	docXML := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p>` +
		`<w:bookmarkStart w:id="0" w:name="_GoBack"/>` +
		`<w:r><w:t>hello</w:t></w:r>` +
		`<w:bookmarkEnd w:id="0"/>` +
		`</w:p>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 1)
	runs := blocks[0].Source[0].Runs
	// Expected: just the text run, both _GoBack markers dropped.
	require.Len(t, runs, 1, "expect _GoBack start AND end to be skipped")
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "hello", runs[0].Text.Text)
}

// TestParseParagraph_BookmarkSpanningParagraphs verifies that a
// bookmark whose start and end live in different paragraphs is
// preserved as separate inline placeholder runs on each paragraph's
// block. This is the cross-structure case the upstream type name
// `BookmarkCrossStructure` is named for: per ECMA-376 §17.13.6 the
// `<w:bookmarkStart>` / `<w:bookmarkEnd>` pair can span runs,
// paragraphs, table rows, and even sections.
func TestParseParagraph_BookmarkSpanningParagraphs(t *testing.T) {
	docXML := `<?xml version="1.0"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p>` +
		`<w:bookmarkStart w:id="2" w:name="span"/>` +
		`<w:r><w:t>first</w:t></w:r>` +
		`</w:p>` +
		`<w:p>` +
		`<w:r><w:t>second</w:t></w:r>` +
		`<w:bookmarkEnd w:id="2"/>` +
		`</w:p>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)
	require.Len(t, blocks, 2)

	// Paragraph 1: bookmarkStart + "first".
	runs1 := blocks[0].Source[0].Runs
	require.Len(t, runs1, 2)
	require.NotNil(t, runs1[0].Ph)
	assert.Equal(t, SubTypeBookmarkStart, runs1[0].Ph.SubType)
	assert.Contains(t, runs1[0].Ph.Data, `w:name="span"`)
	require.NotNil(t, runs1[1].Text)
	assert.Equal(t, "first", runs1[1].Text.Text)

	// Paragraph 2: "second" + bookmarkEnd.
	runs2 := blocks[1].Source[0].Runs
	require.Len(t, runs2, 2)
	require.NotNil(t, runs2[0].Text)
	assert.Equal(t, "second", runs2[0].Text.Text)
	require.NotNil(t, runs2[1].Ph)
	assert.Equal(t, SubTypeBookmarkEnd, runs2[1].Ph.SubType)
	assert.Contains(t, runs2[1].Ph.Data, `w:id="2"`)
}

// TestRowDeletionAutoAccept verifies that <w:tr> rows carrying the
// row-deletion revision marker <w:trPr><w:del .../></w:trPr> are
// dropped entirely when AutomaticallyAcceptRevisions is true (default,
// matching Okapi's ConditionalParameters.java line 813). The row's
// cell contents must NOT emit blocks. Mirrors upstream
// StyledTextPart.process() lines 530-551 — drain the row markup and
// remove it from the queued table buffer.
//
// Per ECMA-376 Part 1 §17.13.5.13 (Deleted Table Row): the <w:del>
// child of <w:trPr> indicates that the entire table row was deleted
// in a tracked revision; an "accept" action removes the row from
// the document.
func TestRowDeletionAutoAccept(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:tbl>` +
		// Row 1: kept.
		`<w:tr>` +
		`<w:tc><w:p><w:r><w:t>kept</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		// Row 2: marked for deletion — must be dropped.
		`<w:tr>` +
		`<w:trPr><w:del w:id="1" w:author="A" w:date="2026-05-10T00:00:00Z"/></w:trPr>` +
		`<w:tc><w:p><w:r><w:t>deleted</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	// Only the kept row's text becomes a block.
	require.Len(t, blocks, 1, "deleted row's content must not produce a block")
	runs := blocks[0].Source[0].Runs
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "kept", runs[0].Text.Text)
}

// TestRowDeletionAttributeVariants verifies the row-deletion detector
// matches the marker regardless of attribute count, ordering, or
// element form (self-closing vs open/close).
func TestRowDeletionAttributeVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "self-closing del with attrs",
			raw:  `<w:trPr><w:del w:id="5" w:author="User" w:date="2021-07-21T18:29:00Z"/></w:trPr>`,
			want: true,
		},
		{
			name: "open/close del",
			raw:  `<w:trPr><w:del w:id="5" w:author="User" w:date="2021-07-21T18:29:00Z"></w:del></w:trPr>`,
			want: true,
		},
		{
			name: "del with no attrs",
			raw:  `<w:trPr><w:del/></w:trPr>`,
			want: true,
		},
		{
			name: "del among siblings",
			raw:  `<w:trPr><w:cantSplit/><w:del w:id="1"/></w:trPr>`,
			want: true,
		},
		{
			name: "ins (row insertion) — not a deletion",
			raw:  `<w:trPr><w:ins w:id="1" w:author="U" w:date="2021-07-21T18:29:00Z"/></w:trPr>`,
			want: false,
		},
		{
			name: "no revision marker",
			raw:  `<w:trPr><w:cantSplit/><w:trHeight w:val="240"/></w:trPr>`,
			want: false,
		},
		{
			name: "empty trPr",
			raw:  `<w:trPr></w:trPr>`,
			want: false,
		},
		{
			name: "del nested inside another element — not a top-level child",
			raw:  `<w:trPr><w:trPrChange><w:trPr><w:del/></w:trPr></w:trPrChange></w:trPr>`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := trPrHasRowDeletion(tc.raw)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRowDeletionDisabledWhenAcceptRevisionsFalse verifies that
// when AutomaticallyAcceptRevisions=false, deleted rows are NOT
// dropped (mirroring upstream Okapi behaviour where the absence of
// auto-accept causes the filter to throw or preserve the marker).
// In our native reader we simply preserve the row + skip the
// deletion logic; downstream the user sees the row.
func TestRowDeletionDisabledWhenAcceptRevisionsFalse(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:tbl>` +
		`<w:tr>` +
		`<w:trPr><w:del w:id="1" w:author="A" w:date="2026-05-10T00:00:00Z"/></w:trPr>` +
		`<w:tc><w:p><w:r><w:t>deleted</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	cfg.AutomaticallyAcceptRevisions = false
	blocks := parseDocXML(t, docXML, cfg)

	// With auto-accept disabled, the row is kept and its text
	// extracted as a normal block.
	require.Len(t, blocks, 1)
	runs := blocks[0].Source[0].Runs
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "deleted", runs[0].Text.Text)
}

// TestRowInsertionMarkerKeepsRow verifies that a row with a
// <w:trPr><w:ins .../></w:trPr> marker (row insertion, ECMA-376 Part 1
// §17.13.5.16) is KEPT — the inserted content is the post-revision
// state we want. The <w:ins> marker itself is stripped at write time
// by wmlRevisionParagraphMarkRE.
func TestRowInsertionMarkerKeepsRow(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:tbl>` +
		`<w:tr>` +
		`<w:trPr><w:ins w:id="1" w:author="A" w:date="2026-05-10T00:00:00Z"/></w:trPr>` +
		`<w:tc><w:p><w:r><w:t>inserted</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	require.Len(t, blocks, 1, "row insertion must keep the row")
	runs := blocks[0].Source[0].Runs
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "inserted", runs[0].Text.Text)
}

// TestNestedTableRowDeletion verifies that row-deletion handling
// works correctly inside a nested table (table cell containing
// another table). Mirrors fixtures
// 848-nested-tables-with-revisions.docx where deleted rows live
// inside a nested <w:tbl> within an outer cell.
func TestNestedTableRowDeletion(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:tbl>` +
		`<w:tr>` +
		`<w:tc>` +
		// Nested table.
		`<w:tbl>` +
		`<w:tr>` +
		`<w:tc><w:p><w:r><w:t>nested-kept</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		`<w:tr>` +
		`<w:trPr><w:del w:id="1" w:author="A" w:date="2026-05-10T00:00:00Z"/></w:trPr>` +
		`<w:tc><w:p><w:r><w:t>nested-deleted</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	require.Len(t, blocks, 1, "nested-deleted row's content must not emit a block")
	runs := blocks[0].Source[0].Runs
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "nested-kept", runs[0].Text.Text)
}

// TestMoveFromRowAutoAccept verifies that <w:tr> rows whose content
// carries a <w:moveFrom> revision-tracking wrapper (ECMA-376 Part 1
// §17.13.5.17 Move From Run Content) are dropped entirely when
// AutomaticallyAcceptRevisions is true. Mirrors upstream Okapi
// MoveFromRevisionCrossStructure (lines 371-450 of SkippableElements.java)
// + StyledTextPart row removal at lines 299-305 of StyledTextPart.java.
func TestMoveFromRowAutoAccept(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:tbl>` +
		// Row 1: kept, plain content.
		`<w:tr>` +
		`<w:tc><w:p><w:r><w:t>kept</w:t></w:r></w:p></w:tc>` +
		`</w:tr>` +
		// Row 2: every cell's paragraph contents are wrapped in <w:moveFrom>.
		`<w:tr>` +
		`<w:tc><w:p>` +
		`<w:moveFromRangeStart w:id="0" w:author="U" w:date="2026-05-10T00:00:00Z" w:name="m1"/>` +
		`<w:moveFrom w:id="1" w:author="U" w:date="2026-05-10T00:00:00Z">` +
		`<w:r><w:t>moved-from</w:t></w:r>` +
		`</w:moveFrom>` +
		`</w:p></w:tc>` +
		`<w:tc><w:p>` +
		`<w:moveFrom w:id="2" w:author="U" w:date="2026-05-10T00:00:00Z">` +
		`<w:r><w:t>also-moved</w:t></w:r>` +
		`</w:moveFrom>` +
		`<w:moveFromRangeEnd w:id="0"/>` +
		`</w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	// Only the kept row's text becomes a block — moveFrom-row's
	// translatable content is dropped.
	require.Len(t, blocks, 1, "moveFrom row's content must not produce a block")
	runs := blocks[0].Source[0].Runs
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "kept", runs[0].Text.Text)
}

// TestMoveFromRowEmptyTableDropped verifies that when every row of a
// table is a moveFrom row (so all rows are dropped by
// dropMoveFromTableRows), the now-empty <w:tbl> is also removed by the
// dropEmptyTables follow-up pass. Mirrors upstream
// StyledTextPart.process lines 410-424 (the TableEnd branch removes
// the entire table when no translatable block reached the writer).
func TestMoveFromRowEmptyTableDropped(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:p><w:r><w:t>before</w:t></w:r></w:p>` +
		`<w:tbl>` +
		`<w:tblGrid><w:gridCol w:w="1000"/></w:tblGrid>` +
		`<w:tr>` +
		`<w:tc><w:p>` +
		`<w:moveFrom w:id="1" w:author="U" w:date="2026-05-10T00:00:00Z">` +
		`<w:r><w:t>only-moved</w:t></w:r>` +
		`</w:moveFrom>` +
		`</w:p></w:tc>` +
		`</w:tr>` +
		`</w:tbl>` +
		`<w:p><w:r><w:t>after</w:t></w:r></w:p>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	require.Len(t, blocks, 2, "empty-after-moveFrom table must not emit blocks")
	assert.Equal(t, "before", blocks[0].Source[0].Runs[0].Text.Text)
	assert.Equal(t, "after", blocks[1].Source[0].Runs[0].Text.Text)
}

// TestMoveFromRowDetectorAttributeForms verifies the row-body detector
// matches the moveFrom wrapper across attribute and self-closing
// variants while NOT matching the cross-structure range markers
// <w:moveFromRangeStart> / <w:moveFromRangeEnd> which carry
// different element local names.
func TestMoveFromRowDetectorAttributeForms(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "moveFrom with attrs",
			body: `<w:tc><w:p><w:moveFrom w:id="1" w:author="U" w:date="2026-05-10T00:00:00Z"><w:r><w:t>x</w:t></w:r></w:moveFrom></w:p></w:tc>`,
			want: true,
		},
		{
			name: "moveFrom no attrs (open form)",
			body: `<w:tc><w:p><w:moveFrom><w:r><w:t>x</w:t></w:r></w:moveFrom></w:p></w:tc>`,
			want: true,
		},
		{
			name: "only moveFromRangeStart — not the wrapper",
			body: `<w:tc><w:p><w:moveFromRangeStart w:id="0" w:name="m"/></w:p></w:tc>`,
			want: false,
		},
		{
			name: "only moveFromRangeEnd — not the wrapper",
			body: `<w:tc><w:p><w:moveFromRangeEnd w:id="0"/></w:p></w:tc>`,
			want: false,
		},
		{
			name: "no moveFrom at all",
			body: `<w:tc><w:p><w:r><w:t>plain</w:t></w:r></w:p></w:tc>`,
			want: false,
		},
		{
			name: "moveFromRange* alongside moveFrom — the wrapper still triggers",
			body: `<w:tc><w:p><w:moveFromRangeStart w:id="0" w:name="m"/><w:moveFrom w:id="1"><w:r><w:t>x</w:t></w:r></w:moveFrom></w:p></w:tc>`,
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rowBodyHasMoveFromContent([]byte(tc.body))
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestInsContentRunExtraction verifies that <w:ins> revision-content
// wrappers (ECMA-376 Part 1 §17.13.5.16) have their inner <w:r> runs
// extracted as translatable text on the SAME paragraph block as
// adjacent non-wrapped <w:r> siblings. Mirrors 859.docx where a single
// paragraph contains `<w:r>Saving as OOXML Strict in MS Office 2013.
// </w:r><w:ins><w:r> New text for tracking changes.</w:r></w:ins>` —
// both runs must reach the translatable block so pseudo-translation
// (or any Block tool) sees the inserted content.
//
// Upstream Okapi BlockParser.java treats <w:ins> as a transparent
// RUN_CONTAINER per RunContainer.java lines 29-43 and
// SkippableElements.RevisionInline (lines 209-212 of
// SkippableElements.java) which returns early without skipping for
// INSERTED_CONTENT/MOVED_CONTENT_TO under the auto-accept-revisions
// default (ConditionalParameters.java line 819).
func TestInsContentRunExtraction(t *testing.T) {
	// Use the EXACT shape from 859.docx — both <w:r> elements carry an
	// <w:rPr><w:lang w:val="en-US"/></w:rPr>, the <w:ins>-wrapped run
	// also has w:rsidR="00C97B0B". This matters because if mergeRuns
	// cannot collapse them (different rPr), the two runs survive as
	// distinct entries on the block and the writer's per-run handling
	// must keep both texts intact.
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body>` +
		`<w:p w:rsidR="007F21CC" w:rsidRDefault="007F21CC">` +
		`<w:pPr><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>` +
		`<w:r><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>Saving as OOXML Strict in MS Office 2013.</w:t></w:r>` +
		`<w:ins w:id="0" w:author="U" w:date="2019-08-29T21:16:00Z">` +
		`<w:r w:rsidR="00C97B0B"><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t xml:space="preserve"> New text for tracking changes.</w:t></w:r>` +
		`</w:ins>` +
		`</w:p>` +
		`</w:body></w:document>`

	cfg := &Config{}
	cfg.Reset()
	blocks := parseDocXML(t, docXML, cfg)

	require.Len(t, blocks, 1, "single paragraph must emit one block")
	require.Len(t, blocks[0].Source, 1)
	runs := blocks[0].Source[0].Runs
	// Collect all TextRun strings in source order.
	var texts []string
	for _, r := range runs {
		if r.Text != nil {
			texts = append(texts, r.Text.Text)
		}
	}
	// Adjacent same-props runs may merge via mergeRuns; what matters is
	// both texts reach the block in source order. Join all extracted
	// texts and assert the concatenation matches the source.
	assert.Equal(t,
		"Saving as OOXML Strict in MS Office 2013. New text for tracking changes.",
		strings.Join(texts, ""),
		"both the plain <w:r> and the <w:ins>-wrapped <w:r> must extract as translatable text on the block",
	)
	assert.True(t, blocks[0].Translatable, "block carrying <w:ins> content must remain translatable so pseudo-translation reaches the inserted run")
}

// TestRead859InsParagraph reads the real 859.docx fixture and verifies
// the inserted run text reaches a translatable block. The fixture has a
// single body paragraph: `<w:r>Saving as OOXML Strict in MS Office 2013.
// </w:r><w:ins><w:r> New text for tracking changes.</w:r></w:ins>`. Both
// texts must surface on the translatable block so downstream tools
// (pseudo-translation, MT, TM) operate on the inserted content too.
func TestRead859InsParagraph(t *testing.T) {
	parts := readDocx(t, "testdata/test_859.docx")
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Search for the paragraph containing the inserted text.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Saving as OOXML Strict") {
			found = true
			assert.Equal(t,
				"Saving as OOXML Strict in MS Office 2013. New text for tracking changes.",
				b.SourceText(),
				"body block must include the <w:ins>-wrapped inserted run text",
			)
			break
		}
	}
	assert.True(t, found,
		"expected a body block with the inserted-content paragraph — strict OOXML <w:p> must reach the translatable-block pipeline so <w:ins> children are pseudo-translated",
	)
}
