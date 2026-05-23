//go:build acceptance

package androidxml_test

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

// Consumer-toolchain acceptance tests for the Android string-resources writer.
// Built only under the `acceptance` tag and gated on tool presence via
// exec.LookPath, so they are inert in the normal test pass.
//
// Android resources have no single canonical XSD, and the authoritative consumer
// (aapt2) requires a full build context. So the portable, always-meaningful
// acceptance check here is libxml2 well-formedness (xmllint --noout) of the
// TRANSLATED output, complemented by an independent structural re-parse. When
// aapt2 IS present we additionally compile the translated resources with it; it
// is absent in this environment, so that sub-test skips with a reason.
//
// We feed the tools our translated output (the writer's splice path), not just
// the verbatim input, so the test exercises the bytes the writer actually
// produces. If a tool rejects kapi output, the writer has a real defect to fix.

func runTool(ctx context.Context, t *testing.T, name string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return out, err
}

// writeTranslatedFixture extracts a corpus file, pseudo-translates every entry
// into `loc` (preserving inline codes), writes the result to a temp strings.xml,
// and returns its path.
func writeTranslatedFixture(t *testing.T, src string, loc model.LocaleID) string {
	t.Helper()
	parts, _ := readParts(t, src)
	require.NotEmpty(t, blocks(parts), "%s must extract entries", src)
	pseudoTranslateAndroid(t, parts, loc, "«X»")
	out := writeParts(t, parts, loc)

	dir := t.TempDir()
	dst := filepath.Join(dir, filepath.Base(src))
	require.NoError(t, os.WriteFile(dst, out, 0o644))
	return dst
}

func corpusFiles(t *testing.T) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.xml"))
	require.NoError(t, err)
	return m
}

// TestAcceptanceXmllintWellFormed asserts the translated output is well-formed
// XML per libxml2 (an independent parser, distinct from the format's tokenizer).
func TestAcceptanceXmllintWellFormed(t *testing.T) {
	xmllint, err := exec.LookPath("xmllint")
	if err != nil {
		t.Skip("xmllint not found on PATH — skipping well-formedness acceptance check")
	}
	t.Logf("using xmllint at %s", xmllint)

	const loc = model.LocaleID("qps-ploc")
	for _, src := range corpusFiles(t) {
		t.Run(filepath.Base(src), func(t *testing.T) {
			dst := writeTranslatedFixture(t, src, loc)
			out, err := runTool(t.Context(), t, xmllint, "--noout", dst)
			assert.NoErrorf(t, err, "xmllint must accept translated %s (output: %s)",
				filepath.Base(src), string(out))
		})
	}
}

// TestAcceptanceStructuralReparse asserts the translated output re-parses
// through the reader and yields a translatable block set identical to the
// source's — an independent structural acceptance check that does not depend on
// any external tool.
func TestAcceptanceStructuralReparse(t *testing.T) {
	const loc = model.LocaleID("qps-ploc")
	for _, src := range corpusFiles(t) {
		t.Run(filepath.Base(src), func(t *testing.T) {
			parts, _ := readParts(t, src)
			srcNames := map[string]bool{}
			for _, b := range blocks(parts) {
				srcNames[b.Name] = true
			}
			dst := writeTranslatedFixture(t, src, loc)
			data, err := os.ReadFile(dst)
			require.NoError(t, err)
			rt := readBytes(t, filepath.Base(src), data)
			rtNames := map[string]bool{}
			for _, b := range blocks(rt) {
				rtNames[b.Name] = true
			}
			assert.Equal(t, srcNames, rtNames,
				"translated output must re-parse with the same translatable set")
		})
	}
}

// TestAcceptanceAapt2 compiles the translated resources with the real Android
// asset packaging tool when present. aapt2 is not installed in this environment,
// so it skips with a reason; the gate keeps the check available wherever aapt2
// exists.
func TestAcceptanceAapt2(t *testing.T) {
	aapt2, err := exec.LookPath("aapt2")
	if err != nil {
		t.Skip("aapt2 not found on PATH — skipping the Android asset-packaging acceptance check")
	}
	t.Logf("using aapt2 at %s", aapt2)

	const loc = model.LocaleID("qps-ploc")
	for _, src := range corpusFiles(t) {
		t.Run(filepath.Base(src), func(t *testing.T) {
			// aapt2 compile expects res/values/<file>.xml layout.
			dir := t.TempDir()
			valuesDir := filepath.Join(dir, "res", "values")
			require.NoError(t, os.MkdirAll(valuesDir, 0o755))

			parts, _ := readParts(t, src)
			pseudoTranslateAndroid(t, parts, loc, "«X»")
			out := writeParts(t, parts, loc)
			resFile := filepath.Join(valuesDir, "strings.xml")
			require.NoError(t, os.WriteFile(resFile, out, 0o644))

			outDir := t.TempDir()
			flat := filepath.Join(outDir, "compiled.flat")
			res, cerr := runTool(t.Context(), t, aapt2, "compile", resFile, "-o", flat)
			assert.NoErrorf(t, cerr, "aapt2 must compile translated %s (output: %s)",
				filepath.Base(src), string(res))
		})
	}
}
