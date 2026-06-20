//go:build e2e

// Package e2e contains end-to-end tests for the kapi CLI.
// These tests build the kapi binary and exercise complete user stories
// against real files, verifying input/output of every command.
//
// Run with: go test -tags=e2e -count=1 -v ./kapi/e2e/
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
	// SQLite TM/termbase to open (otherwise: "no such function: fts5").
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

// kapiAllowFail runs kapi and returns combined output + error WITHOUT failing
// the test. Use for QA gates (qa, term-check) that exit non-zero when
// they find issues — a non-zero exit is a result to assert on, not a harness
// failure. Same isolation env as kapi().
func kapiAllowFail(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ─── User Story 1: Terminology QA ───────────────────────────────────────────
// Verifies the complete workflow from terminology-qa.md:
//   Import glossary → inspect stats → lookup terms → search →
//   run QA on translations → export glossary

func TestTermbaseImport(t *testing.T) {
	tb := tempDB(t, "tb")

	out := kapi(t, "termbase", "import", filepath.Join(testdata, "glossary.csv"),
		"--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "--header")
	assert.Contains(t, out, "Imported 7") // 7 concepts imported
}

func TestTermbaseStats(t *testing.T) {
	tb := importedTermbase(t)

	out := kapi(t, "termbase", "stats", "--file", tb)
	assert.Contains(t, out, "Concepts:  7")  // 7 concepts
	assert.Contains(t, out, "Terms:     14") // 14 terms (7 en + 7 fr)
	assert.Contains(t, out, "en")
	assert.Contains(t, out, "fr")
}

func TestTermbaseLookup(t *testing.T) {
	tb := importedTermbase(t)

	out := kapi(t, "termbase", "lookup", "password", "--file", tb, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "password")
	assert.Contains(t, out, "mot de passe")
}

func TestTermbaseLookupFuzzy(t *testing.T) {
	tb := importedTermbase(t)

	out := kapi(t, "termbase", "lookup", "passwords", "--file", tb, "-s", "en", "-t", "fr", "--fuzzy")
	assert.Contains(t, out, "password")
}

func TestTermbaseSearch(t *testing.T) {
	tb := importedTermbase(t)

	out := kapi(t, "termbase", "search", "encrypt", "--file", tb, "-s", "en")
	assert.Contains(t, out, "encryption")
	assert.Contains(t, out, "chiffrement")
}

func TestTermbaseExportCSV(t *testing.T) {
	tb := importedTermbase(t)

	outFile := filepath.Join(t.TempDir(), "export.csv")
	kapi(t, "termbase", "export", "--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "password")
	assert.Contains(t, content, "mot de passe")
}

func TestTermbaseExportJSON(t *testing.T) {
	tb := importedTermbase(t)

	outFile := filepath.Join(t.TempDir(), "export.json")
	kapi(t, "termbase", "export", "--file", tb, "--format", "json", "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "encryption")
	assert.Contains(t, content, "chiffrement")
}

// TestTermCheckWithTermbase exercises terminology QA on a pseudo-translated
// file. Steps: pseudo-translate → term-check with termbase.
// The pseudo-translated output will not use correct French terminology, so
// term-check flags violations and exits non-zero (a QA gate, not a failure).
func TestTermCheckWithTermbase(t *testing.T) {
	tb := importedTermbase(t)
	tmp := t.TempDir()

	// Step 1: pseudo-translate to create bilingual content
	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 2: term-check against the termbase — exercises flag parsing,
	// termbase loading and processing. It runs as an informational QA pass
	// (exit 0; no stdout), so a clean run is the assertion.
	kapi(t, "term-check", pseudoOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termbase", tb)
}

// TestQACheckWithoutTermbase verifies that qa works standalone for
// baseline rule-based checks and writes its annotated output file.
func TestQACheckWithoutTermbase(t *testing.T) {
	tmp := t.TempDir()

	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")

	qaOut := filepath.Join(tmp, "qa.json")
	// qa annotates rather than gates; tolerate a non-zero exit and
	// assert it produced the output file.
	_, _ = kapiAllowFail(t, "qa", pseudoOut,
		"-o", qaOut,
		"--source-lang", "en",
		"--target-lang", "fr")
	assert.FileExists(t, qaOut)
}

// ─── User Story 2: Pre-Translation with TM + Terminology ────────────────────
// Verifies the complete workflow from terminology-pretranslation.md:
//   Import TM → inspect stats → lookup entries → search →
//   TM leverage → pseudo-translate remaining → QA with termbase

func TestTMImport(t *testing.T) {
	tmFile := tempDB(t, "tm")

	out := kapi(t, "tm", "import", filepath.Join(testdata, "project.tmx"),
		"--file", tmFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Imported 2") // 2 entries imported
}

func TestTMStats(t *testing.T) {
	tmFile := importedTM(t)

	out := kapi(t, "tm", "stats", "--file", tmFile)
	assert.Contains(t, out, "Entries: 2") // 2 entries
	assert.Contains(t, out, "en")
	assert.Contains(t, out, "fr")
}

func TestTMLookup(t *testing.T) {
	tmFile := importedTM(t)

	out := kapi(t, "tm", "lookup", "Settings", "--file", tmFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Paramètres")
}

// TestTMSearch verifies `kapi tm search` resolves source terms against the
// imported TM. This previously returned "No entries found." (#701) because
// single-file `kapi tm import` did not rebuild the FTS5 search side-tables;
// that was fixed by #38 (import now calls rebuildTMSearchIndexes), so the
// search now finds the same entries `kapi tm lookup` resolves.
func TestTMSearch(t *testing.T) {
	tmFile := importedTM(t)

	out := kapi(t, "tm", "search", "Settings", "--file", tmFile, "-s", "en", "-t", "fr")
	assert.Contains(t, out, "Settings")
	assert.Contains(t, out, "Paramètres")
}

func TestTMExport(t *testing.T) {
	tmFile := importedTM(t)

	outFile := filepath.Join(t.TempDir(), "export.tmx")
	kapi(t, "tm", "export", "--file", tmFile, "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "Settings")
	assert.Contains(t, content, "Paramètres")
}

// TestTMLeverage exercises standalone `kapi tm-leverage` against an external TM
// via --tm. It accepts --tm (and a project .kapi/tm.db), and — since the #700
// fix wired SourceLocale from --source-lang — it fills targets from exact TM
// matches: "Settings" and "File upload" (both in project.tmx) are leveraged into
// their French equivalents in the output.
func TestTMLeverage(t *testing.T) {
	tmFile := importedTM(t)
	tmp := t.TempDir()

	out := filepath.Join(tmp, "leveraged.json")
	kapi(t, "tm-leverage", filepath.Join(testdata, "messages_en.json"),
		"--tm", tmFile,
		"--source-lang", "en",
		"--target-lang", "fr",
		"-o", out)
	assert.FileExists(t, out)

	// The exact TM matches must be leveraged into the French output (#700).
	content := readFile(t, out)
	assert.Contains(t, content, "Paramètres")
	assert.Contains(t, content, "Téléversement de fichier")
}

// ─── Full Pipeline: TM Leverage → Pseudo-Translate → QA + Termbase ──────────

// TestFullPipeline runs the supported standalone pipeline end-to-end:
// pseudo-translate → qa → term-check against the project glossary.
// (TM leverage is covered separately by TestTMLeverage.)
func TestFullPipeline(t *testing.T) {
	tb := importedTermbase(t)
	tmp := t.TempDir()

	// Step 1: Pseudo-translate the source.
	pseudoOut := filepath.Join(tmp, "step1_pseudo.json")
	kapi(t, "pseudo-translate", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 2: Rule-based QA — writes annotated output.
	qaOut := filepath.Join(tmp, "step2_qa.json")
	_, _ = kapiAllowFail(t, "qa", pseudoOut,
		"-o", qaOut,
		"--source-lang", "en",
		"--target-lang", "fr")
	assert.FileExists(t, qaOut)

	// Step 3: Terminology QA against the glossary (informational, exit 0, no stdout).
	kapi(t, "term-check", pseudoOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termbase", tb)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func tempDB(t *testing.T, prefix string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), prefix+".db")
}

func importedTermbase(t *testing.T) string {
	t.Helper()
	tb := tempDB(t, "tb")
	kapi(t, "termbase", "import", filepath.Join(testdata, "glossary.csv"),
		"--file", tb, "--format", "csv", "-s", "en", "-t", "fr", "--header")
	return tb
}

func importedTM(t *testing.T) string {
	t.Helper()
	tmFile := tempDB(t, "tm")
	kapi(t, "tm", "import", filepath.Join(testdata, "project.tmx"),
		"--file", tmFile, "-s", "en", "-t", "fr")
	return tmFile
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
