package applestrings_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCorpusByteFaithfulRoundTrip reads every real-world legacy Apple .strings
// and .stringsdict file vendored under testdata/corpus/ and asserts that an
// untouched read→write reproduces the original bytes exactly. These are genuine,
// permissively-licensed files (see testdata/corpus/SOURCES.md): a failure here
// is a real-world byte-fidelity bug in the reader/writer, not a fixture artefact.
//
// The file extension (.strings / .stringsdict) drives the reader's kind
// detection, so the corpus files keep their native extensions.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	var matches []string
	for _, pat := range []string{"*.strings", "*.stringsdict"} {
		m, err := filepath.Glob(filepath.Join("testdata", "corpus", pat))
		if err != nil {
			t.Fatalf("glob corpus %q: %v", pat, err)
		}
		matches = append(matches, m...)
	}
	if len(matches) == 0 {
		t.Skip("no corpus files vendored")
	}

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, original := readParts(t, path)
			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"corpus round-trip must reproduce original bytes for %s", filepath.Base(path))
		})
	}
}
