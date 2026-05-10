package openxml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindParagraphs_simple(t *testing.T) {
	src := []byte(`<w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p></w:body>`)
	got := findParagraphs(src)
	assert.Len(t, got, 1)
	assert.Equal(t, "<w:p><w:r><w:t>hello</w:t></w:r></w:p>", string(src[got[0].start:got[0].end]))
}

func TestFindRuns_basic(t *testing.T) {
	src := []byte(`<w:p><w:r><w:t>a</w:t></w:r><w:r><w:t>b</w:t></w:r></w:p>`)
	got := findRuns(src)
	assert.Len(t, got, 2)
}

func TestParseRunPropElements_basic(t *testing.T) {
	src := []byte(`<w:rPr><w:rFonts w:ascii="Arial"/><w:b/></w:rPr>`)
	got := parseRunPropElements(src)
	assert.Len(t, got, 2)
	assert.Equal(t, "rFonts", got[0].name)
	assert.Equal(t, "b", got[1].name)
}

func TestOptimizeWMLPart_MultipleRunsCommonProps(t *testing.T) {
	// Two runs with the same rFonts — common prop should be extracted
	// into a synthesised style. Mirrors the 1437-color-exclusion fixture
	// shape (multi-run paragraphs where Okapi factors out a common rPr
	// shape into a paragraph style).
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>a</w:t></w:r><w:r><w:rPr><w:rFonts w:ascii="Arial"/></w:rPr><w:t>b</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.Contains(t, string(got), "NF974E24F-Normal1")
	assert.Len(t, ids, 1)
}

func TestOptimizeWMLPart_SingleRun_Bypassed(t *testing.T) {
	// 1-run paragraphs are bypassed by the post-write threshold (the
	// native reader/writer pipeline aggressively merges source runs and
	// the optimisation premise — common props across runs — no longer
	// holds). Verifies no synthesised style is created.
	src := []byte(`<w:body><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>a</w:t></w:r></w:p></w:body>`)
	existing := map[string]bool{}
	counters := map[string]int{}
	syn := map[string]synthesisedStyle{}
	var ids []string
	got := optimizeWMLPart(src, existing, counters, syn, &ids)
	assert.NotContains(t, string(got), "NF974E24F")
	assert.Len(t, ids, 0)
}
