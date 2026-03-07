package openxml

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
