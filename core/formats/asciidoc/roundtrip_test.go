package asciidoc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSkeletonRoundTripByteExact asserts that reading then writing an AsciiDoc
// document with a wired skeleton store reproduces the source bytes exactly.
func TestSkeletonRoundTripByteExact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"heading", "== A Section\n"},
		{"doctitle+header", "= Title\n:author: A\n\nBody text.\n"},
		{"paragraph with inline", "A *bold* and _em_ and `code` line.\n"},
		{"links and attrs", "See https://x.io[here] and {name} and link:t.adoc[doc].\n"},
		{"sub/super", "H~2~O and x^2^.\n"},
		{"list", "* one\n* two\n** nested\n"},
		{"ordered list", ". first\n. second\n"},
		{"block title", ".My Title\nSome text.\n"},
		{"admonition", "NOTE: pay attention here.\n"},
		{"listing verbatim", "[source,go]\n----\nfmt.Println(\"x\")\n----\n"},
		{"comment", "// a comment\nReal text.\n"},
		{"attribute entry", ":toc: left\n\nText.\n"},
		{"table", "|===\n| A | B\n\n| c1 | c2\n|===\n"},
		{"no trailing newline", "Just one line."},
		{"crlf", "Line one.\r\nLine two.\r\n"},
		{"blank lines", "Para one.\n\n\nPara two.\n"},
		{"xref", "See <<sect-1,the section>> and <<sect-2>>.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := skelRoundtrip(t, tc.input, "")
			assert.Equal(t, tc.input, out, "skeleton round-trip must be byte-exact")
		})
	}
}

// TestOriginalContentRoundTripByteExact asserts the SetOriginalContent path
// (no externally-wired skeleton) is also byte-exact.
func TestOriginalContentRoundTripByteExact(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	out := origRoundtrip(t, input, "")
	assert.Equal(t, input, out)
}

// TestSampleFixtureRoundTrip round-trips the committed exemplar byte-exact via
// both paths.
func TestSampleFixtureRoundTrip(t *testing.T) {
	t.Parallel()
	input := mustRead(t, filepath.Join("testdata", "sample.adoc"))
	assert.Equal(t, input, skelRoundtrip(t, input, ""), "skeleton path")
	assert.Equal(t, input, origRoundtrip(t, input, ""), "original-content path")
}

// TestTranslationSplicesTargets asserts a translated target locale is spliced
// in while the surrounding structure stays byte-exact skeleton.
func TestTranslationSplicesTargets(t *testing.T) {
	t.Parallel()
	input := "== Hello\n\nWorld text.\n"

	parts := readParts(t, input)
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		switch b.SourceText() {
		case "Hello":
			b.SetTargetText(model.LocaleFrench, "Bonjour")
		case "World text.":
			b.SetTargetText(model.LocaleFrench, "Texte du monde.")
		}
	}

	out := writeOriginalParts(t, input, parts, model.LocaleFrench)
	assert.Equal(t, "== Bonjour\n\nTexte du monde.\n", out)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
