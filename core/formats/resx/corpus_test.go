package resx_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpusByteFaithfulRoundTrip reads every genuine .resx/.resw fixture
// vendored under testdata/corpus/ (provenance + licenses in
// testdata/corpus/SOURCES.md) and asserts that reading then writing it with no
// translation applied reproduces the original bytes EXACTLY.
//
// These are real .NET resource files (roslyn, StyleCopAnalyzers): UTF-8 BOM,
// embedded xsd:schema, full <resheader> boilerplate, <comment> notes, {0}
// composite-format placeholders, typed/binary <data>, and the compact
// <resheader>text form. A byte-faithful round-trip over them is strong evidence
// the lossless tokenizer + splice-only writer faithfully model the format. If a
// real file fails this, the reader/writer has a real escaping/whitespace/entity
// bug that must be fixed (not skipped) — see the report in SOURCES.md.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	t.Parallel()

	var files []string
	for _, pat := range []string{"*.resx", "*.resw"} {
		matches, err := filepath.Glob(filepath.Join("testdata", "corpus", pat))
		require.NoError(t, err)
		files = append(files, matches...)
	}
	require.NotEmpty(t, files, "corpus must contain at least one real .resx/.resw file")

	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parts, original := readParts(t, f)

			// Every corpus file must yield at least one translatable string block —
			// otherwise the round-trip would be trivially faithful for the wrong
			// reason (nothing extracted).
			require.NotEmpty(t, blocks(parts),
				"%s must contain translatable string <data> entries", name)

			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"byte-faithful round-trip must reproduce %s exactly", name)
		})
	}
}
