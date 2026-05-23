package mif_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpusInvariantPseudoTranslationApplied is the translate-side invariant
// on a clean (non-marker-split) real-world corpus file: applying a pseudo
// translation to every extracted paragraph Block and writing the document back
// must produce output that re-reads to the SAME block sequence, each carrying
// the translated text — proving the writer splices targets into the right
// <String> slots and the source text is gone.
//
// okapi-TestFootnote.mif is chosen because it is a large (~200 KB), structurally
// rich FrameMaker document (body pages, footnotes, catalogs) that round-trips
// cleanly, so it exercises the translate→merge→re-extract path at realistic
// scale.
func TestCorpusInvariantPseudoTranslationApplied(t *testing.T) {
	const fr = model.LocaleID("fr-FR")
	const fixture = "testdata/corpus/okapi-TestFootnote.mif"

	data, err := os.ReadFile(fixture)
	require.NoError(t, err)
	input := string(data)

	skel, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skel.Close()

	parts, src := corpusExtract(t, input, skel)
	require.NotEmpty(t, src)

	// Pseudo-translate every block: wrap its source text in « ».
	want := make([]string, 0, len(src))
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		tgt := "«" + b.SourceText() + "»"
		b.SetTargetText(fr, tgt)
		want = append(want, tgt)
	}
	require.NotEmpty(t, want)

	var buf bytes.Buffer
	w := mif.NewWriter()
	w.SetSkeletonStore(skel)
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(fr)
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	w.Close()

	_, got := corpusExtract(t, buf.String(), nil)
	require.Len(t, got, len(want),
		"translated document must re-read to the same block count")
	assert.Equal(t, want, got,
		"each block's pseudo-translation must be spliced into the right <String> slot")
	assert.Contains(t, strings.Join(got, ""), "«",
		"pseudo-translation markers must be present in re-read content")
}

// TestCorpusInvariantNonTranslatableCatalogsPreserved verifies that across the
// clean corpus the reader never emits catalog/structural tag NAMES (FontCatalog,
// ColorCatalog, PgfCatalog tags, MIFFile version) as translatable Block text —
// only paragraph string content is translatable. A regression that leaked
// skeleton tag bytes into a Block (the exact failure mode the marker-split gap
// produces) would trip this on a clean file.
func TestCorpusInvariantNonTranslatableCatalogsPreserved(t *testing.T) {
	for _, path := range corpusFiles(t) {
		path := path
		base := pathBase(path)
		if _, gated := corpusMarkerSplitGap[base]; gated {
			continue
		}
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			_, texts := corpusExtract(t, string(data), nil)
			for _, txt := range texts {
				// No extracted Block may BE a bare catalog/structural tag token
				// or contain raw MIF statement syntax.
				assert.NotContains(t, txt, "<MIFFile",
					"MIF file header leaked into a translatable block in %s: %q", base, txt)
				assert.False(t, strings.HasPrefix(txt, "FontCatalog") || strings.HasPrefix(txt, "ColorCatalog"),
					"catalog tag name leaked as translatable text in %s: %q", base, txt)
			}
		})
	}
}

func pathBase(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
