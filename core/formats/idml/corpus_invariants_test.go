package idml

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipEntryNames returns the names of every entry in a ZIP archive.
func zipEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

// TestCorpusPseudoTranslationApplied is the translate-side invariant: across
// the real corpus, applying a pseudo translation to every extracted Block and
// writing the package back must (a) preserve the block sequence and count on
// re-read, and (b) carry the translated text — proving the writer splices
// targets into the right <Content> slots and the source text is gone.
//
// This exercises the production path (translate → merge → re-extract) on real,
// structurally varied stories, not just the untouched round-trip.
func TestCorpusPseudoTranslationApplied(t *testing.T) {
	const fr = model.LocaleID("fr-FR")
	for _, path := range corpusFiles(t) {
		path := path
		base := filepath.Base(path)
		if corpusZeroTranslatable[base] {
			continue // nothing to translate
		}
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			skel, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer skel.Close()

			parts := corpusReadParts(t, data, skel)
			src := testutil.BlockTexts(testutil.FilterBlocks(parts))
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

			out := corpusWrite(t, data, parts, skel, fr)
			got := corpusReadTexts(t, out, nil)

			// The translated package must re-read to the SAME sequence of
			// blocks, each carrying the pseudo-translated text.
			require.Len(t, got, len(want),
				"translated package %s must re-read to the same block count", base)
			assert.Equal(t, want, got,
				"each block's pseudo-translation must be spliced into the right <Content> slot for %s", base)

			// Spot-check that the marker actually made it into the bytes (not
			// merely reconstructed by the model) for at least one story.
			joined := strings.Join(got, "")
			assert.Contains(t, joined, "«",
				"pseudo-translation markers must be present in re-read content for %s", base)
		})
	}
}
