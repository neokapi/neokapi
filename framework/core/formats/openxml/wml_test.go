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
	props, err := parseRunProps(d, true)
	require.NoError(t, err)
	assert.True(t, props.isEmpty())
}

func TestParseRunPropsBold(t *testing.T) {
	input := `<rPr><b/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true)
	require.NoError(t, err)
	assert.True(t, props.bold)
	assert.False(t, props.italic)
}

func TestParseRunPropsBoldFalse(t *testing.T) {
	input := `<rPr><b val="0"/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true)
	require.NoError(t, err)
	assert.False(t, props.bold)
}

func TestParseRunPropsMultiple(t *testing.T) {
	input := `<rPr><b/><i/><u val="single"/><strike/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true)
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
	props, err := parseRunProps(d, true)
	require.NoError(t, err)
	assert.Equal(t, "superscript", props.vertAlign)
}

func TestParseRunPropsVanish(t *testing.T) {
	input := `<rPr><vanish/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true)
	require.NoError(t, err)
	assert.True(t, props.vanish)
}

func TestParseRunPropsAggressiveCleanup(t *testing.T) {
	// rsid and proofErr should be stripped in aggressive mode
	input := `<rPr><b/><rsidR val="001234"/><noProof/></rPr>`
	d := xml.NewDecoder(bytes.NewReader([]byte(input)))
	skipStart(t, d)
	props, err := parseRunProps(d, true)
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

func TestRunPropsOpeningClosingSpans(t *testing.T) {
	props := runProps{bold: true, italic: true}
	counter := 0

	opening := props.openingSpans(&counter)
	assert.Len(t, opening, 2)
	assert.Equal(t, TypeBold, opening[0].Type)
	assert.Equal(t, TypeItalic, opening[1].Type)

	closing := props.closingSpans(&counter)
	assert.Len(t, closing, 2)
	// Closing should be in reverse order
	assert.Equal(t, TypeItalic, closing[0].Type)
	assert.Equal(t, TypeBold, closing[1].Type)
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

func TestAllHidden(t *testing.T) {
	assert.True(t, allHidden([]textRun{
		{text: "hidden", props: runProps{vanish: true}},
	}))
	assert.False(t, allHidden([]textRun{
		{text: "visible", props: runProps{}},
	}))
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
		text := blocks[0].Source[0].Content.Text()
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
		text := blocks[0].Source[0].Content.Text()
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
		text := blocks[0].Source[0].Content.Text()
		assert.Contains(t, text, "Link Text")
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
		text := blocks[0].Source[0].Content.Text()
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
	assert.False(t, blocks[0].Source[0].Content.HasSpans())
}

func TestStyleOptimizationWithInheritance(t *testing.T) {
	styles := &styleMap{styles: map[string]*styleEntry{
		"BaseStyle": {id: "BaseStyle", props: runProps{bold: true}},
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
	text := blocks[0].Source[0].Content.Text()
	assert.Equal(t, "Hello World", text)
	// Should have no spans since the merged font is "other" property, not a formatting span
	assert.False(t, blocks[0].Source[0].Content.HasSpans())
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
	frag := blocks[0].Source[0].Content
	assert.True(t, frag.HasSpans(), "should have code finder spans")
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
	assert.False(t, blocks[0].Source[0].Content.HasSpans(), "no spans when code finder disabled")
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
		{props: runProps{fontName: "Arial"}},                                    // duplicate
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
