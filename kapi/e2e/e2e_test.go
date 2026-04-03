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
)

func TestMain(m *testing.M) {
	// Build kapi binary.
	root := findRoot()
	kapiBin = filepath.Join(root, "bin", "kapi-e2e-test")
	cmd := exec.Command("go", "build", "-o", kapiBin, "./cmd/kapi")
	cmd.Dir = filepath.Join(root, "framework", "kapi")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build kapi: " + err.Error())
	}
	testdata = filepath.Join(root, "framework", "kapi", "e2e", "testdata")
	os.Exit(m.Run())
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
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "kapi %s failed:\n%s", strings.Join(args, " "), string(out))
	return string(out)
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

// TestQACheckWithTermbase exercises QA-with-termbase on a pseudo-translated
// file. Steps: pseudo-translate → qa-check with termbase.
// The pseudo-translated output will not use correct French terminology,
// so the term-enforce tool should annotate violations.
func TestQACheckWithTermbase(t *testing.T) {
	tb := importedTermbase(t)
	tmp := t.TempDir()

	// Step 1: pseudo-translate to create bilingual content
	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "flow", "run", "pseudo-translate",
		"-i", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 2: QA check with termbase
	qaOut := filepath.Join(tmp, "qa.json")
	kapi(t, "flow", "run", "qa-check",
		"-i", pseudoOut,
		"-o", qaOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termbase", tb)
	assert.FileExists(t, qaOut)
}

// TestQACheckWithoutTermbase verifies that qa-check works standalone
// (without --termbase) for baseline rule-based checks.
func TestQACheckWithoutTermbase(t *testing.T) {
	tmp := t.TempDir()

	pseudoOut := filepath.Join(tmp, "pseudo.json")
	kapi(t, "flow", "run", "pseudo-translate",
		"-i", filepath.Join(testdata, "messages_en.json"),
		"-o", pseudoOut,
		"--target-lang", "fr")

	qaOut := filepath.Join(tmp, "qa.json")
	kapi(t, "flow", "run", "qa-check",
		"-i", pseudoOut,
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

func TestTMSearch(t *testing.T) {
	tmFile := importedTM(t)

	out := kapi(t, "tm", "search", "file", "--file", tmFile, "-s", "en")
	assert.Contains(t, out, "upload")
}

func TestTMExport(t *testing.T) {
	tmFile := importedTM(t)

	outFile := filepath.Join(t.TempDir(), "export.tmx")
	kapi(t, "tm", "export", "--file", tmFile, "-s", "en", "-t", "fr", "-o", outFile)

	content := readFile(t, outFile)
	assert.Contains(t, content, "Settings")
	assert.Contains(t, content, "Paramètres")
}

func TestTMLeverage(t *testing.T) {
	tmFile := importedTM(t)
	tmp := t.TempDir()

	outFile := filepath.Join(tmp, "leveraged.json")
	kapi(t, "flow", "run", "tm-leverage",
		"-i", filepath.Join(testdata, "messages_en.json"),
		"-o", outFile,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--tm", tmFile)

	assert.FileExists(t, outFile)
	// The TM has exact matches for "Settings" and "File upload".
	content := readFile(t, outFile)
	assert.Contains(t, content, "Paramètres")
}

// ─── Full Pipeline: TM Leverage → Pseudo-Translate → QA + Termbase ──────────

func TestFullPipeline(t *testing.T) {
	tb := importedTermbase(t)
	tmFile := importedTM(t)
	tmp := t.TempDir()

	// Step 1: TM leverage — reuse previous translations
	tmOut := filepath.Join(tmp, "step1_tm.json")
	kapi(t, "flow", "run", "tm-leverage",
		"-i", filepath.Join(testdata, "messages_en.json"),
		"-o", tmOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--tm", tmFile)
	assert.FileExists(t, tmOut)

	// Step 2: Pseudo-translate remaining segments
	pseudoOut := filepath.Join(tmp, "step2_pseudo.json")
	kapi(t, "flow", "run", "pseudo-translate",
		"-i", tmOut,
		"-o", pseudoOut,
		"--target-lang", "fr")
	assert.FileExists(t, pseudoOut)

	// Step 3: QA check with termbase — verify terminology
	qaOut := filepath.Join(tmp, "step3_qa.json")
	kapi(t, "flow", "run", "qa-check",
		"-i", pseudoOut,
		"-o", qaOut,
		"--source-lang", "en",
		"--target-lang", "fr",
		"--termbase", tb)
	assert.FileExists(t, qaOut)
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
