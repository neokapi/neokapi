package idml

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpusFiles globs every real-world .idml package vendored under
// testdata/corpus/. Provenance (upstream repo, license, pinned commit) for
// each file is recorded in testdata/corpus/SOURCES.md.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.idml"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected real-world .idml packages under testdata/corpus/")
	return matches
}

// corpusZeroTranslatable lists corpus files that are valid IDML packages with
// NO translatable surface (layout-only templates). They are kept in the corpus
// to exercise the story-less round-trip path; the semantic round-trip below
// asserts they preserve their (empty) translatable surface rather than
// requiring it to be non-empty. See SOURCES.md.
var corpusZeroTranslatable = map[string]bool{
	"simpleidml-magazineA-template.idml": true,
}

// corpusReadTexts reads an IDML package and returns its translatable Block
// source texts in document order. The optional skeleton store, when supplied,
// records the package skeleton so the writer can reconstruct it.
func corpusReadTexts(t *testing.T, data []byte, skel *format.SkeletonStore) []string {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()
	if skel != nil {
		reader.SetSkeletonStore(skel)
	}
	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return testutil.BlockTexts(testutil.FilterBlocks(parts))
}

// corpusReadParts is corpusReadTexts but returns the full Part stream (so the
// caller can translate blocks before writing them back).
func corpusReadParts(t *testing.T, data []byte, skel *format.SkeletonStore) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()
	reader.SetSkeletonStore(skel)
	require.NoError(t, reader.Open(ctx, &model.RawDocument{
		URI:          "test.idml",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.adobe.indesign-idml-package",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// corpusWrite reconstructs an IDML package from the part stream and skeleton,
// rendering targets for locale (empty locale renders the source).
func corpusWrite(t *testing.T, data []byte, parts []*model.Part, skel *format.SkeletonStore, locale model.LocaleID) []byte {
	t.Helper()
	ctx := t.Context()
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetSkeletonStore(skel)
	writer.SetOriginalContent(data)
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(locale)
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	return buf.Bytes()
}

// TestCorpusSemanticRoundTrip is the PRIMARY corpus acceptance bar for IDML.
// IDML is a ZIP container whose story XML is whitespace- and entity-fragile, so
// a byte-exact read→write contract is NOT faithful (and not what Okapi
// promises). The faithful contract is SEMANTIC: an untouched read→write→re-read
// must preserve the translatable surface — every <Content> Block, in order,
// with identical source text.
//
// These are genuine, permissively-licensed packages (see SOURCES.md), so a
// failure here is a real-world reader/writer fidelity bug, not a fixture
// artefact. The corpus spans hello-world stories, footnotes, tables, hyperlink
// + table content, XML-structured stories, multi-page magazine layouts,
// business-card placeholder templates, and a layout-only template with no copy.
func TestCorpusSemanticRoundTrip(t *testing.T) {
	for _, path := range corpusFiles(t) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			skel, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer skel.Close()

			parts := corpusReadParts(t, data, skel)
			t1 := testutil.BlockTexts(testutil.FilterBlocks(parts))
			if corpusZeroTranslatable[filepath.Base(path)] {
				// A valid layout-only template: it has no story copy, so it
				// extracts nothing translatable. It must still round-trip
				// (reader does not error on a story-less package; writer copies
				// it through). See the story-less robustness fix in reader.go.
				assert.Empty(t, t1,
					"layout-only template %s is expected to carry no translatable copy", filepath.Base(path))
			} else {
				require.NotEmpty(t, t1,
					"corpus package %s must extract translatable content", filepath.Base(path))
			}

			// Write back with NO translation, then re-read.
			out := corpusWrite(t, data, parts, skel, model.LocaleEnglish)
			require.NotEmpty(t, out, "writer must produce a package for %s", filepath.Base(path))
			t2 := corpusReadTexts(t, out, nil)

			assert.Equal(t, t1, t2,
				"semantic round-trip (read→write→re-read) must preserve the translatable surface for %s", filepath.Base(path))
		})
	}
}

// TestCorpusOutputIsValidIDML verifies that the reconstructed package is a
// well-formed IDML ZIP whose entry set matches the source (no entry dropped or
// renamed, no spurious entry added). This guards the writer's package-assembly
// path on real, structurally varied packages.
func TestCorpusOutputIsValidIDML(t *testing.T) {
	for _, path := range corpusFiles(t) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			skel, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer skel.Close()

			parts := corpusReadParts(t, data, skel)
			out := corpusWrite(t, data, parts, skel, model.LocaleEnglish)

			origNames := zipEntryNames(t, data)
			outNames := zipEntryNames(t, out)
			assert.ElementsMatch(t, origNames, outNames,
				"reconstructed package must contain exactly the source's ZIP entries for %s", filepath.Base(path))
		})
	}
}
