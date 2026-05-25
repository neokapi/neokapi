package androidxml_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpusByteFaithfulRoundTrip reads every genuine Android strings.xml
// fixture vendored under testdata/corpus/ (provenance + licenses in
// testdata/corpus/SOURCES.md) and asserts that reading then writing it with no
// translation applied reproduces the original bytes EXACTLY.
//
// These are real Android resource files (AOSP Calendar, K-9 Mail): <plurals>,
// <string-array>, translatable="false", CDATA, <xliff:g> spans, %1$s/%d printf
// args, Android backslash escapes, the product= qualifier, and hundreds of XML
// comments. A byte-faithful round-trip over them is strong evidence the lossless
// tokenizer + splice-only writer faithfully model the format. The product=
// qualifier and the bare-'>' element content surfaced real bugs (see SOURCES.md)
// that were fixed rather than skipped.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.xml"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "corpus must contain at least one real strings.xml file")

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parts, original := readParts(t, f)

			// Every corpus file must yield translatable blocks — otherwise the
			// round-trip would be trivially faithful for the wrong reason.
			require.NotEmpty(t, blocks(parts),
				"%s must contain translatable entries", name)

			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"byte-faithful round-trip must reproduce %s exactly", name)
		})
	}
}
