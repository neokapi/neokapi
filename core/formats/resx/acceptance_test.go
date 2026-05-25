//go:build acceptance

package resx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Consumer-toolchain acceptance tests for the RESX writer. Built only under the
// `acceptance` tag and gated on tool presence via exec.LookPath, so they are
// inert in the normal test pass.
//
// The strongest possible check for a .resx is that the REAL .NET resource
// compiler (resgen — Mono's System.Resources.Tools front-end) can compile it to
// a .resources binary. A .resx that resgen compiles cleanly (exit 0) is, by
// construction, consumable by .NET. We additionally assert well-formedness with
// xmllint. We feed resgen our TRANSLATED output (the writer's splice path), not
// just the verbatim input, so the test exercises the code that actually changes
// bytes. If resgen rejects kapi output, the writer has a real defect to fix.

// runTool runs argv under ctx and returns combined output + exit error.
func runTool(ctx context.Context, t *testing.T, name string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return out, err
}

// writeTranslatedFixture extracts a corpus file, pseudo-translates every string
// entry into `loc` (preserving placeholders), writes the result to a temp .resx,
// and returns its path.
func writeTranslatedFixture(t *testing.T, src string, loc model.LocaleID) string {
	t.Helper()
	parts, _ := readParts(t, src)
	require.NotEmpty(t, blocks(parts), "%s must extract string entries", src)
	pseudoTranslateRESX(t, parts, loc, "[x]")
	out := writeParts(t, parts, loc)

	dir := t.TempDir()
	dst := filepath.Join(dir, filepath.Base(src))
	require.NoError(t, os.WriteFile(dst, out, 0o644))
	return dst
}

// TestAcceptanceResgenCompiles is the key acceptance check: the real .NET
// resource compiler compiles our translated RESX output with exit 0.
func TestAcceptanceResgenCompiles(t *testing.T) {
	resgen, err := exec.LookPath("resgen")
	if err != nil {
		t.Skip("resgen not found on PATH — skipping the .NET resource-compiler acceptance check")
	}
	t.Logf("using resgen at %s", resgen)

	const loc = model.LocaleID("qps-ploc")
	for _, src := range corpusFiles(t) {
		t.Run(baseName(src), func(t *testing.T) {
			dst := writeTranslatedFixture(t, src, loc)
			resources := dst[:len(dst)-len(filepath.Ext(dst))] + ".resources"

			out, err := runTool(t.Context(), t, resgen, dst, resources)
			require.NoErrorf(t, err, "resgen must compile translated %s (output: %s)",
				baseName(src), string(out))
			// resgen reports success and writes the .resources binary.
			info, statErr := os.Stat(resources)
			require.NoError(t, statErr, "resgen must emit a .resources binary")
			assert.Positive(t, info.Size(), ".resources binary must be non-empty")
			t.Logf("resgen ACCEPTED translated %s -> %d byte .resources",
				baseName(src), info.Size())
		})
	}
}

// TestAcceptanceXmllintWellFormed asserts the translated RESX output is
// well-formed XML per libxml2 (an independent parser).
func TestAcceptanceXmllintWellFormed(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not found on PATH — skipping well-formedness acceptance check")
	}

	const loc = model.LocaleID("qps-ploc")
	for _, src := range corpusFiles(t) {
		t.Run(baseName(src), func(t *testing.T) {
			dst := writeTranslatedFixture(t, src, loc)
			out, err := runTool(t.Context(), t, xmllint, "--noout", dst)
			assert.NoErrorf(t, err, "xmllint must accept translated %s (output: %s)",
				baseName(src), string(out))
		})
	}
}
