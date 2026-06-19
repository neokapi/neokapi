package asciidoc_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReaderMetadataAndSignature asserts identity + detection metadata.
func TestReaderMetadataAndSignature(t *testing.T) {
	t.Parallel()
	r := asciidoc.NewReader()
	assert.Equal(t, "asciidoc", r.Name())
	assert.Equal(t, "AsciiDoc", r.DisplayName())
	sig := r.Signature()
	assert.Contains(t, sig.Extensions, ".adoc")
	assert.Contains(t, sig.Extensions, ".asciidoc")
	assert.Contains(t, sig.MIMETypes, "text/asciidoc")
	assert.NotContains(t, sig.MIMETypes, "text/plain", "must not claim plaintext MIME")
}

// TestReaderBlockExtraction asserts the exemplar fixture yields the expected
// translatable blocks plus the code listing surfaced as non-translatable content
// (visible for ingestion, skipped by MT) rather than buried in skeleton.
func TestReaderBlockExtraction(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	blocks := readBlocks(t, input)

	var translatable, content []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			translatable = append(translatable, b)
		} else {
			content = append(content, b)
		}
	}

	require.Len(t, translatable, 17, "exemplar must yield exactly 17 translatable blocks")

	texts := testutil.BlockTexts(translatable)
	for _, want := range []string{
		"Document Title", "First Section", "Subsection",
		"A list of items", "First item", "Second item", "Nested item",
		"This is an admonition paragraph.",
		"Name", "Role", "Alice", "Engineer", "Bob", "Designer",
	} {
		assert.Contains(t, texts, want)
	}

	// The listing body is contextual content: surfaced as a non-translatable
	// content block (role=code), NOT extracted as translatable prose and NOT
	// hidden in opaque skeleton.
	require.Len(t, content, 1, "the code listing must surface as one non-translatable content block")
	listing := content[0]
	assert.False(t, listing.Translatable, "code content must not be translatable")
	assert.Contains(t, listing.SourceText(), "not translated", "code content must be visible as content")
	assert.Equal(t, model.RoleCode, listing.SemanticRole(), "code content carries the code role")
	assert.True(t, listing.PreserveWhitespace, "code content preserves whitespace")

	// No translatable block may carry the code content.
	for _, b := range translatable {
		assert.NotContains(t, b.SourceText(), "not translated",
			"code content must never be extracted as translatable prose")
	}
}

// TestReaderLayerBookends asserts the part stream opens with LayerStart and ends
// with LayerEnd and that every group bracket is balanced.
func TestReaderLayerBookends(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	parts := readParts(t, input)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	depth := 0
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			depth++
		case model.PartGroupEnd:
			depth--
			assert.GreaterOrEqual(t, depth, 0, "group end without matching start")
		}
	}
	assert.Equal(t, 0, depth, "all groups must be balanced")
}

// TestReaderInlineVocabulary is the Vocabulary-axis (V) evidence: each inline
// construct maps to its canonical run type, and the runs are lossless (their
// concatenated literal content reproduces the source).
func TestReaderInlineVocabulary(t *testing.T) {
	t.Parallel()

	type want struct {
		typ  string
		data string
	}
	cases := []struct {
		name   string
		text   string
		expect want
	}{
		{"bold", "a *strong* b", want{"fmt:bold", "*"}},
		{"italic", "a _emphasis_ b", want{"fmt:italic", "_"}},
		{"mono", "a `mono` b", want{"fmt:code", "`"}},
		{"superscript", "x^2^ y", want{"fmt:superscript", "^"}},
		{"subscript", "H~2~O y", want{"fmt:subscript", "~"}},
		{"url-link", "see https://x.io[text] end", want{"link:hyperlink", "https://x.io["}},
		{"link-macro", "see link:t.adoc[text] end", want{"link:hyperlink", "link:t.adoc["}},
		{"xref-text", "see <<id,text>> end", want{"code:markup", "<<id,"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runs := firstBlockRuns(t, tc.text)
			// Find the typed paired-open run carrying the expected type.
			var found bool
			for _, r := range runs {
				if r.PcOpen != nil && r.PcOpen.Type == tc.expect.typ {
					assert.Equal(t, tc.expect.data, r.PcOpen.Data)
					found = true
				}
			}
			require.True(t, found, "expected a %s paired-open run in %q", tc.expect.typ, tc.text)
			assert.Equal(t, tc.text, model.RenderRunsWithData(runs),
				"runs must losslessly reproduce the source")
		})
	}
}

// TestReaderInlinePlaceholders covers the standalone-placeholder vocabulary:
// attribute references and text-less cross references.
func TestReaderInlinePlaceholders(t *testing.T) {
	t.Parallel()

	t.Run("attribute-ref", func(t *testing.T) {
		runs := firstBlockRuns(t, "hi {product} bye")
		var ok bool
		for _, r := range runs {
			if r.Ph != nil && r.Ph.Type == "code:variable" {
				assert.Equal(t, "{product}", r.Ph.Data)
				ok = true
			}
		}
		assert.True(t, ok, "attribute reference must be a code:variable placeholder")
		assert.Equal(t, "hi {product} bye", model.RenderRunsWithData(runs))
	})

	t.Run("xref-no-text", func(t *testing.T) {
		runs := firstBlockRuns(t, "see <<anchor>> now")
		var ok bool
		for _, r := range runs {
			if r.Ph != nil && r.Ph.Type == "code:markup" {
				assert.Equal(t, "<<anchor>>", r.Ph.Data)
				ok = true
			}
		}
		assert.True(t, ok, "text-less xref must be a code:markup placeholder")
		assert.Equal(t, "see <<anchor>> now", model.RenderRunsWithData(runs))
	})
}

// TestReaderConstrainedBoundaries asserts mid-word markers are NOT mistaken for
// formatting (AsciiDoc constrained rule): 2*3*4 stays plain text.
func TestReaderConstrainedBoundaries(t *testing.T) {
	t.Parallel()
	runs := firstBlockRuns(t, "2*3*4 stays plain")
	for _, r := range runs {
		assert.Nil(t, r.PcOpen, "mid-word * must not open a constrained pair")
	}
	assert.Equal(t, "2*3*4 stays plain", model.RenderRunsWithData(runs))
}

// firstBlockRuns reads input and returns the source runs of the first block.
func firstBlockRuns(t *testing.T, input string) []model.Run {
	t.Helper()
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)
	return blocks[0].Source
}

// TestReaderConfigExtractionToggles verifies the config knobs gate extraction.
func TestReaderConfigExtractionToggles(t *testing.T) {
	t.Parallel()
	input := ".Block Title\nA paragraph.\n\n|===\n| A | B\n|===\n"

	// Defaults: block title + table cells are translatable.
	def := readBlocks(t, input)
	assert.Contains(t, testutil.BlockTexts(def), "Block Title")
	assert.Contains(t, testutil.BlockTexts(def), "A")

	// With both toggles off, the block title and table cells stay skeleton.
	r := asciidoc.NewReader()
	cfg := r.Config()
	require.NoError(t, cfg.ApplyMap(map[string]any{"extractBlockTitles": false, "extractTableCells": false}))
	require.NoError(t, r.Open(t.Context(), testutil.RawDocFromString(input, model.LocaleEnglish)))
	defer r.Close()
	off := testutil.FilterBlocks(testutil.CollectParts(t, r.Read(t.Context())))
	assert.NotContains(t, testutil.BlockTexts(off), "Block Title")
	assert.NotContains(t, testutil.BlockTexts(off), "A")
}

// TestConfigRejectsUnknownKey asserts ApplyMap rejects unknown parameters.
func TestConfigRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	cfg := asciidoc.NewReader().Config()
	require.NotNil(t, cfg)
	err := cfg.ApplyMap(map[string]any{"nope": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

// TestReaderEmpty asserts an empty document yields just the layer bookends.
func TestReaderEmpty(t *testing.T) {
	t.Parallel()
	parts := readParts(t, "")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
	assert.Empty(t, testutil.FilterBlocks(parts))
}
