package openxml

// okapi-filter: openxml

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// pptxSourceFormExclusions names the decks whose untranslated round trip is
// deliberately not byte-identical, and why. It is empty: every pptx fixture
// upstream ships goes back byte for byte.
//
// The xlsx list next door (xlsxSourceFormExclusions) has one entry, for a <t>
// the writer repairs rather than replays. DrawingML needs no such repair:
// ECMA-376 Part 1 §21.1.2.3.11 types <a:t> as xsd:string with no trimming rule,
// so an <a:t> declaring no xml:space loses nothing. See
// dml_text_spelling_test.go.
var pptxSourceFormExclusions = map[string]string{}

// TestPPTXSourceForm_CorpusIsByteIdentical is the corpus-wide statement: every
// pptx fixture upstream ships goes back byte for byte on an untranslated round
// trip.
func TestPPTXSourceForm_CorpusIsByteIdentical(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(testdataDir(t), "*.pptx"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if why, skip := pptxSourceFormExclusions[filepath.Base(path)]; skip {
				t.Skip(why)
			}
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			out := skeletonRoundtripBytes(t, original, filepath.Base(path))
			assertSamePackageBytes(t, original, out)
		})
	}
}
