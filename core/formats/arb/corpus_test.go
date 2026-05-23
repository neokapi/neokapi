package arb_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCorpusByteFaithfulRoundTrip reads every real-world ARB file vendored under
// testdata/corpus/ and asserts that an untouched read→write reproduces the
// original bytes exactly. These are genuine, permissively-licensed files (see
// testdata/corpus/SOURCES.md), so a failure here is a real-world byte-fidelity
// bug in the reader/writer, not a fixture artefact.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.arb"))
	if err != nil {
		t.Fatalf("glob corpus: %v", err)
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
