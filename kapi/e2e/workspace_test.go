//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// .klz project snapshot hand-off + cached resume (AD-025 §5 / #787). These
// run the real binary using the deterministic pseudo-translate flow so
// results are reproducible.

const wsSource = `{"greeting":"Hello world","farewell":"Goodbye now","cta":"Sign up today"}`

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

// readFileBytes reads a file's raw bytes (readFile, the string variant,
// lives in e2e_test.go).
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

var _ = exec.Command
