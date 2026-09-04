//go:build e2e

// Package e2e contains end-to-end tests for the kapi CLI.
// These tests build the kapi binary and exercise complete user stories
// against real files, verifying input/output of every command.
//
// Run with: make test-e2e-kapi. The suite links go-sqlite3, so it needs
// -tags "fts5,e2e" — one comma-joined flag, since repeated -tags do not union.
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	kapiBin  string
	testdata string
	// isoEnv pins kapi at a throwaway config/data/cache home and disables
	// project discovery, so these tests never read the developer's
	// ~/.config/kapi, user-installed plugins, or any checked-in .kapi recipe
	// (e.g. a repo-root dogfood project the upward walk would otherwise find).
	isoEnv []string
)

func TestMain(m *testing.M) {
	// Build kapi binary.
	root := findRoot()
	kapiBin = filepath.Join(root, "bin", "kapi-e2e-test")
	// Build with the same tags as `make build` — fts5 is required for the
	// SQLite content memory/terms to open (otherwise: "no such function: fts5").
	cmd := exec.Command("go", "build", "-tags", "fts5", "-o", kapiBin, "./cmd/kapi")
	cmd.Dir = filepath.Join(root, "kapi")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build kapi: " + err.Error())
	}
	testdata = filepath.Join(root, "kapi", "e2e", "testdata")

	iso, err := os.MkdirTemp("", "kapi-e2e-iso-")
	if err != nil {
		panic("failed to create isolated kapi home: " + err.Error())
	}
	isoEnv = []string{
		"NO_COLOR=1",
		"KAPI_NO_PROJECT=1",
		// Zero telemetry in e2e runs: even a keyed release-like build must
		// not notice or emit (the isolation contract, epic 018 workstream G).
		"KAPI_TELEMETRY=0",
		"KAPI_CONFIG_DIR=" + filepath.Join(iso, "config"),
		"XDG_DATA_HOME=" + filepath.Join(iso, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(iso, "cache"),
		// KAPI_PLUGINS_DIR_ONLY=1 confines plugin discovery to
		// $KAPI_PLUGINS_DIR (empty here → none). XDG_DATA_HOME only
		// redirects the user plugin root; without this flag the absolute
		// system plugin roots (Homebrew, /usr/share) still leak in.
		"KAPI_PLUGINS_DIR_ONLY=1",
	}

	code := m.Run()
	_ = os.RemoveAll(iso)
	os.Exit(code)
}

func findRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("cannot find repo root (go.work)")
		}
		dir = parent
	}
}

// kapi runs a kapi command and returns stdout. Fails the test on non-zero exit.
func kapi(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kapi %s failed:\n%s", strings.Join(args, " "), string(out))
	return string(out)
}

// kapiIn runs a kapi command from a given working directory. Use it when the
// behaviour under test is relative to where the user is standing — a workspace
// records its merge layout relative to wherever it is merged, so exercising
// that needs a cwd rather than an absolute path baked into the package.
//
// The isolation env is the same as kapi(); running from a fresh temp dir is if
// anything stricter, since the upward project walk finds nothing there.
func kapiIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kapi %s (in %s) failed:\n%s", strings.Join(args, " "), dir, string(out))
	return string(out)
}

// kapiAllowFail runs kapi and returns combined output + error WITHOUT failing
// the test. Use for check gates (qa, term-check) that exit non-zero when
// they find issues — a non-zero exit is a result to assert on, not a harness
// failure. Same isolation env as kapi().
func kapiAllowFail(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ─── User Story 1: Terminology checks ───────────────────────────────────────
// Verifies the complete workflow from terminology-qa.md:
//   Import terms → inspect stats → lookup terms → search →
//   run the checks on translations → export terms

func TestTermsImport(t *testing.T) {
	tb := tempDB(t, "tb")

	out := kapi(t, "terms", "import", filepath.Join(testdata, "terms.csv"),
		"--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "--header")
	assert.Contains(t, out, "Imported 7") // 7 concepts imported
}

func TestTermsStats(t *testing.T) {
	tb := importedTerms(t)

	out := kapi(t, "terms", "stats", "--file", tb)
	assert.Contains(t, out, "Concepts:  7")  // 7 concepts
	assert.Contains(t, out, "Terms:     14") // 14 terms (7 en + 7 fr)
	assert.Contains(t, out, "en")
	assert.Contains(t, out, "fr")
}

func TestTermsLookup(t *testing.T) {
	tb := importedTerms(t)

	out := kapi(t, "terms", "lookup", "password", "--file", tb, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "password")
	assert.Contains(t, out, "mot de passe")
}

func TestTermsLookupFuzzy(t *testing.T) {
	tb := importedTerms(t)

	out := kapi(t, "terms", "lookup", "passwords", "--file", tb, "-s", "en", "-t", "fr", "--fuzzy")
	assert.Contains(t, out, "password")
}

func TestTermsSearch(t *testing.T) {
	tb := importedTerms(t)

	out := kapi(t, "terms", "search", "encrypt", "--file", tb, "-s", "en")
	assert.Contains(t, out, "encryption")
	assert.Contains(t, out, "chiffrement")
}

func TestTermsExportCSV(t *testing.T) {
	tb := importedTerms(t)

	outFile := filepath.Join(t.TempDir(), "export.csv")
	kapi(t, "terms", "export", "--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "password")
	assert.Contains(t, content, "mot de passe")
}

func TestTermsExportJSON(t *testing.T) {
	tb := importedTerms(t)

	outFile := filepath.Join(t.TempDir(), "export.json")
	kapi(t, "terms", "export", "--file", tb, "--format", "json", "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "encryption")
	assert.Contains(t, content, "chiffrement")
}

// TestTermCheckWithTerms exercises terminology checks on a pseudo-translated
// file. Steps: pseudo-translate → term-check with terms.
// The pseudo-translated output will not use correct French terminology, so
// term-check flags violations and exits non-zero (a check gate, not a failure).
func TestTermCheckWithTerms(t *testing.T) {
	tb := importedTerms(t)
	tmp := t.TempDir()

	// Step 1: pseudo-translate to create bilingual content
	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 2: term-check against the terms store — exercises flag parsing,
	// terms loading and processing. It runs as an informational check pass
	// (exit 0; no stdout), so a clean run is the assertion. term-check is not
	// in the curated TopLevelTools tier, so it executes via `kapi exec`.
	// The store selector is --termstore (#1505): --terms is a different flag,
	// the boolean gate on `exec dnt-check`.
	kapi(t, "exec", "term-check", pseudoOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termstore", tb)
}

// TestRuleCheckWithoutTerms verifies that qa works standalone for
// baseline rule-based checks and writes its annotated output file.
func TestRuleCheckWithoutTerms(t *testing.T) {
	tmp := t.TempDir()

	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")

	checkOutput := filepath.Join(tmp, "check.json")
	// The raw qa tool annotates rather than gates (the porcelain gate is
	// `kapi check`); tolerate a non-zero exit and assert it produced the
	// output file.
	_, _ = kapiAllowFail(t, "exec", "qa", pseudoOut,
		"-o", checkOutput,
		"--source-lang", "en",
		"--target-lang", "fr")
	assert.FileExists(t, checkOutput)
}

// ─── User Story 2: Pre-Translation with content memory + Terminology ────────────────────
// Verifies the complete workflow from terminology-pretranslation.md:
//   Import content memory → inspect stats → lookup entries → search →
//   content-memory leverage → pseudo-translate remaining → check with terms

func TestMemoryImport(t *testing.T) {
	memoryFile := tempDB(t, "memory")

	out := kapi(t, "memory", "import", filepath.Join(testdata, "project.tmx"),
		"--file", memoryFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Imported 2") // 2 entries imported
}

func TestMemoryStats(t *testing.T) {
	memoryFile := importedMemory(t)

	out := kapi(t, "memory", "stats", "--file", memoryFile)
	assert.Contains(t, out, "Entries: 2") // 2 entries
	assert.Contains(t, out, "en")
	assert.Contains(t, out, "fr")
}

func TestMemoryLookup(t *testing.T) {
	memoryFile := importedMemory(t)

	out := kapi(t, "memory", "lookup", "Settings", "--file", memoryFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Paramètres")
}

// TestMemorySearch verifies `kapi memory search` resolves source terms against the
// imported content memory. Single-file `kapi memory import` rebuilds the FTS5
// search side-tables (rebuildMemorySearchIndexes), so search finds the same
// entries `kapi memory lookup` resolves rather than reporting "No entries found."
// (#701).
func TestMemorySearch(t *testing.T) {
	memoryFile := importedMemory(t)

	out := kapi(t, "memory", "search", "Settings", "--file", memoryFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Settings")
	assert.Contains(t, out, "Paramètres")
}

func TestMemoryExport(t *testing.T) {
	memoryFile := importedMemory(t)

	outFile := filepath.Join(t.TempDir(), "export.tmx")
	kapi(t, "memory", "export", "--file", memoryFile, "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "Settings")
	assert.Contains(t, content, "Paramètres")
}

// TestMemoryLeverage exercises content-memory leverage via `kapi exec recycle` (the raw
// registry layer — the porcelain surface folded content memory reuse into `kapi translate`
// and retired the standalone top-level verb) against an external content memory via --memory. It accepts --memory (and, inside a project, the content memory in
// `.kapi/work/store.db`), and — since the
// #700 fix wired SourceLocale from --source-lang — it fills targets from
// exact content-memory matches: "Settings" and "File upload" (both in project.tmx) are
// leveraged into their French equivalents in the output.
func TestMemoryLeverage(t *testing.T) {
	memoryFile := importedMemory(t)
	tmp := t.TempDir()

	out := filepath.Join(tmp, "leveraged.json")
	kapi(t, "exec", "recycle", filepath.Join(testdata, "messages_en.json"),
		"--memory", memoryFile,
		"--source-lang", "en",
		"--target-lang", "fr",
		"-o", out)
	assert.FileExists(t, out)

	// The exact content-memory matches must be leveraged into the French output (#700).
	content := readFile(t, out)
	assert.Contains(t, content, "Paramètres")
	assert.Contains(t, content, "Téléversement de fichier")
}

// ─── Full Pipeline: content memory Leverage → Pseudo-Translate → Checks + Terms ──────

// TestFullPipeline runs the supported standalone pipeline end-to-end:
// pseudo-translate → qa → term-check against the project terms.
// (content-memory leverage is covered separately by TestMemoryLeverage.)
func TestFullPipeline(t *testing.T) {
	tb := importedTerms(t)
	tmp := t.TempDir()

	// Step 1: Pseudo-translate the source.
	pseudoOut := filepath.Join(tmp, "step1_pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 2: The rule-based checks — write annotated output.
	checkOutput := filepath.Join(tmp, "step2_check.json")
	_, _ = kapiAllowFail(t, "exec", "qa", pseudoOut,
		"-o", checkOutput,
		"--source-lang", "en",
		"--target-lang", "fr")
	assert.FileExists(t, checkOutput)

	// Step 3: Terminology checks against the terms (informational, exit 0, no
	// stdout). Executes via `kapi exec` (not a curated top-level verb).
	kapi(t, "exec", "term-check", pseudoOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termstore", tb)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func tempDB(t *testing.T, prefix string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), prefix+".db")
}

func importedTerms(t *testing.T) string {
	t.Helper()
	tb := tempDB(t, "tb")
	kapi(t, "terms", "import", filepath.Join(testdata, "terms.csv"),
		"--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "--header")
	return tb
}

func importedMemory(t *testing.T) string {
	t.Helper()
	memoryFile := tempDB(t, "memory")
	kapi(t, "memory", "import", filepath.Join(testdata, "project.tmx"),
		"--file", memoryFile, "-s", "en", "-t", "fr")
	return memoryFile
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
