//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/klz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// .klz project snapshot hand-off + cached resume (AD-025 §5 / #787). These
// run the real binary using the deterministic pseudo-translate flow so
// results are reproducible.

const wsSource = `{"greeting":"Hello world","farewell":"Goodbye now","cta":"Sign up today"}`

// writeWS writes content to dir/name and returns the path.
func writeWS(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// newProject writes a minimal .kapi recipe + source file into a fresh dir
// and returns (recipePath, sourcePath).
func newProject(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(wsSource), 0o644))
	recipe := filepath.Join(dir, "demo.kapi")
	require.NoError(t, os.WriteFile(recipe, []byte(
		"version: \"v1\"\nname: demo\ndefaults:\n  source_locale: en\n  target_locales: [fr-FR]\nflows:\n  pseudo:\n    steps:\n      - tool: pseudo-translate\n"), 0o644))
	return recipe, src
}

// TestPackUnpackRoundTrip verifies a project's working state packs to a .klz
// and rehydrates into a fresh project's .kapi/ state dir.
func TestPackUnpackRoundTrip(t *testing.T) {
	recipe, src := newProject(t)
	dir := filepath.Dir(recipe)

	// Run the flow so the project block store caches overlays.
	kapi(t, "run", "pseudo", "-p", recipe, "-i", src, "-o", filepath.Join(dir, "out.json"), "--target-lang", "fr-FR")
	assert.FileExists(t, filepath.Join(dir, ".kapi", "cache", "blocks.db"))

	snap := filepath.Join(dir, "snap.klz")
	kapi(t, "pack", "-p", recipe, "-o", snap)
	info, err := os.Stat(snap)
	require.NoError(t, err)
	assert.Positive(t, info.Size())

	// Unpack into a fresh project root.
	recipe2, _ := newProject(t)
	dir2 := filepath.Dir(recipe2)
	out := kapi(t, "unpack", snap, "-p", recipe2)
	assert.Contains(t, out, "Unpacked")
	assert.FileExists(t, filepath.Join(dir2, ".kapi", "cache", "blocks.db"))
}

// TestCachedResumeSkipsWork verifies the invisible resume story: a second
// run against the same project reuses the cached overlays (byte-identical
// output, no recompute) — the persistent block store is the workspace.
func TestCachedResumeSkipsWork(t *testing.T) {
	recipe, src := newProject(t)
	dir := filepath.Dir(recipe)

	out1 := filepath.Join(dir, "out1.json")
	kapi(t, "run", "pseudo", "-p", recipe, "-i", src, "-o", out1, "--target-lang", "fr-FR")

	// Second run hits the warm cache; output must be identical.
	out2 := filepath.Join(dir, "out2.json")
	kapi(t, "run", "pseudo", "-p", recipe, "-i", src, "-o", out2, "--target-lang", "fr-FR")

	assert.Equal(t, readFile(t, out1), readFile(t, out2),
		"a cached re-run must produce identical output")
}

// TestPackProvenanceLog verifies the opt-in tamper-evident provenance log:
// pack --log stamps a hash-chained line, and the package round-trips it.
func TestPackProvenanceLog(t *testing.T) {
	recipe, src := newProject(t)
	dir := filepath.Dir(recipe)
	kapi(t, "run", "pseudo", "-p", recipe, "-i", src, "-o", filepath.Join(dir, "out.json"), "--target-lang", "fr-FR")

	snap := filepath.Join(dir, "snap.klz")
	kapi(t, "pack", "-p", recipe, "-o", snap, "--log")
	// pack --log a second time chains another provenance line.
	kapi(t, "pack", "-p", recipe, "-o", snap, "--log")

	// Unpack verifies the chain (warns, never fails) and succeeds.
	recipe2, _ := newProject(t)
	out := kapi(t, "unpack", snap, "-p", recipe2)
	assert.Contains(t, out, "Unpacked")
}

// TestPackDeterministicWithoutLog verifies that, without --log, two packs of
// the same project state are byte-identical (content-deterministic).
func TestPackDeterministicWithoutLog(t *testing.T) {
	recipe, src := newProject(t)
	dir := filepath.Dir(recipe)
	kapi(t, "run", "pseudo", "-p", recipe, "-i", src, "-o", filepath.Join(dir, "out.json"), "--target-lang", "fr-FR")

	a := filepath.Join(dir, "a.klz")
	b := filepath.Join(dir, "b.klz")
	kapi(t, "pack", "-p", recipe, "-o", a)
	kapi(t, "pack", "-p", recipe, "-o", b)
	assert.Equal(t, readFileBytes(t, a), readFileBytes(t, b),
		"packs of the same state must be byte-identical without --log")
}

// .klz workspace: extract → transform-in-place → merge (AD-025 §5). The
// deterministic pseudo-translate tool keeps results reproducible.

// TestKlzExtractTransformMerge verifies the full ad-hoc workspace flow with
// no project, and that merged output equals a one-shot run per document.
func TestKlzExtractTransformMerge(t *testing.T) {
	dir := t.TempDir()
	a := writeWS(t, dir, "a.json", `{"greeting":"Hello world"}`)
	b := writeWS(t, dir, "b.json", `{"x":"Another string"}`)

	aExp := filepath.Join(dir, "a.exp.json")
	bExp := filepath.Join(dir, "b.exp.json")
	kapi(t, "pseudo-translate", a, "-o", aExp, "--target-lang", "qps")
	kapi(t, "pseudo-translate", b, "-o", bExp, "--target-lang", "qps")

	work := filepath.Join(dir, "work.klz")
	out := kapi(t, "extract", a, b, "-o", work, "--target-lang", "qps")
	assert.Contains(t, out, "Extracted 2")

	kapi(t, "pseudo-translate", work) // transform in place (qps from recipe)

	kapi(t, "merge", work, "-o", filepath.Join(dir, "l10n"))
	assert.Equal(t, readFile(t, aExp), readFile(t, filepath.Join(dir, "l10n", "a.json")))
	assert.Equal(t, readFile(t, bExp), readFile(t, filepath.Join(dir, "l10n", "b.json")),
		"each document's targets must stay isolated through the workspace")
}

// TestKlzMultiLocaleAccumulates verifies transforms accumulate locales and
// merge emits one file per source × locale under <out>/<lang>/.
func TestKlzMultiLocaleAccumulates(t *testing.T) {
	dir := t.TempDir()
	src := writeWS(t, dir, "app.json", `{"greeting":"Hello world"}`)
	work := filepath.Join(dir, "work.klz")
	kapi(t, "extract", src, "-o", work, "--target-lang", "qps")

	kapi(t, "pseudo-translate", work, "--target-lang", "fr")
	kapi(t, "pseudo-translate", work, "--target-lang", "qps")

	kapi(t, "merge", work, "-o", filepath.Join(dir, "l10n"))
	assert.FileExists(t, filepath.Join(dir, "l10n", "fr", "app.json"))
	assert.FileExists(t, filepath.Join(dir, "l10n", "qps", "app.json"))
}

// TestKlzRecipeRemembersOutput verifies the recipe (locales + out layout)
// travels with the .klz, so `merge` needs no flags.
func TestKlzRecipeRemembersOutput(t *testing.T) {
	dir := t.TempDir()
	src := writeWS(t, dir, "app.json", `{"greeting":"Hello world"}`)
	work := filepath.Join(dir, "work.klz")
	kapi(t, "extract", src, "-o", work, "--target-lang", "fr,qps",
		"--out", filepath.Join(dir, "l10n", "{lang}", "{name}.{ext}"))

	kapi(t, "pseudo-translate", work, "--target-lang", "fr")
	kapi(t, "pseudo-translate", work, "--target-lang", "qps")

	kapi(t, "merge", work) // no -o: uses the recipe's locales + out layout
	assert.FileExists(t, filepath.Join(dir, "l10n", "fr", "app.json"))
	assert.FileExists(t, filepath.Join(dir, "l10n", "qps", "app.json"))
}

// TestKlzTransformReusesCache pins the caching guarantee: the overlays in a
// .klz ARE the cache, so a second transform hydrates them instead of
// recomputing. We tamper a cached target overlay to a sentinel; if the
// second transform recomputed it would overwrite the sentinel — it must not.
func TestKlzTransformReusesCache(t *testing.T) {
	dir := t.TempDir()
	src := writeWS(t, dir, "app.json", `{"a":"Hello world"}`)
	work := filepath.Join(dir, "work.klz")
	kapi(t, "extract", src, "-o", work, "--target-lang", "qps")
	kapi(t, "pseudo-translate", work) // first transform: computes + caches

	// Tamper the cached target overlay to a sentinel value.
	pkg, err := klz.Unmarshal(readFileBytes(t, work))
	require.NoError(t, err)
	tampered := false
	for i := range pkg.Overlays {
		if strings.HasPrefix(pkg.Overlays[i].Kind, "targets/") {
			pkg.Overlays[i].Payload = json.RawMessage(`{"target":"SENTINEL-CACHED"}`)
			tampered = true
		}
	}
	require.True(t, tampered, "first transform must have written a target overlay")
	out, err := pkg.Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(work, out, 0o644))

	// Second transform must reuse the cached overlay, not recompute it.
	kapi(t, "pseudo-translate", work)
	kapi(t, "merge", work, "-o", filepath.Join(dir, "out"))
	assert.Contains(t, readFile(t, filepath.Join(dir, "out", "app.json")), "SENTINEL-CACHED",
		"a second transform must hydrate the cached overlay, not recompute it")
}

// TestKlzTransformGuards verifies the model's guards: creating a .klz needs
// extract; emitting needs merge.
func TestKlzTransformGuards(t *testing.T) {
	dir := t.TempDir()
	src := writeWS(t, dir, "app.json", wsSource)

	// A tool can't write a .klz directly — that's extract's job.
	_, err := kapiAllowFail(t, "pseudo-translate", src, "-o", filepath.Join(dir, "x.klz"), "--target-lang", "qps")
	require.Error(t, err)

	// A tool on a .klz with -o is rejected (transform is in place; use merge).
	work := filepath.Join(dir, "work.klz")
	kapi(t, "extract", src, "-o", work, "--target-lang", "qps")
	_, err = kapiAllowFail(t, "pseudo-translate", work, "-o", filepath.Join(dir, "out.json"))
	require.Error(t, err)
}

// readFileBytes reads a file's raw bytes (readFile, the string variant,
// lives in e2e_test.go).
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

var _ = exec.Command
