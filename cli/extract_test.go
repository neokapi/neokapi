package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/klz"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/neokapi/neokapi/core/formats" // register JSON etc.
)

// newExtractApp builds a fresh App with registries populated, as the
// extract command depends on the format registry.
func newExtractApp(t *testing.T) *App {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	return a
}

// writeJSONSource writes a single-key JSON source file and returns both
// absolute and relative paths.
func writeJSONSource(t *testing.T, projectDir, rel string, content string) string {
	t.Helper()
	abs := filepath.Join(projectDir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	return abs
}

// extractProjectFixture builds a minimal `.kapi` project at `dir` with one
// JSON content pattern and two target locales. Returns the absolute path
// to the recipe file.
func extractProjectFixture(t *testing.T, dir string, targetLanguages []model.LocaleID) string {
	t.Helper()
	recipe := filepath.Join(dir, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "ExtractTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: targetLanguages,
			TM: project.TMDefaults{
				FuzzyThreshold: 75,
			},
		},
		Content: []project.ContentCollection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	return recipe
}

// runExtractCmd invokes `kapi extract` with the given flags on a fresh
// cobra command. Stdout is captured and returned.
func runExtractCmd(t *testing.T, recipe string, flags ...string) (string, error) {
	t.Helper()
	a := newExtractApp(t)
	cmd := a.NewExtractCmd(ExtractCmdOptions{})
	// No PersistentPreRun/PostRun is applied by the test harness — we
	// short-circuit by invoking runExtract directly via Execute.
	args := append([]string{"--project", recipe}, flags...)
	cmd.SetArgs(args)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestExtract_MultiTargetWritesOneOutputPerPair(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR", "de-DE"})
	writeJSONSource(t, real, "src/locales/en/messages.json",
		`{"greeting": "Hello, world.", "farewell": "Goodbye."}`)

	out, err := runExtractCmd(t, recipe)
	require.NoError(t, err, "stdout/stderr: %s", out)

	outDir := filepath.Join(real, "out")
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// Two target locales → two bilingual files (one source).
	require.Len(t, names, 2, "expected 2 output files, got %v", names)
	assert.Contains(t, strings.Join(names, " "), "en-US-to-fr-FR.xliff")
	assert.Contains(t, strings.Join(names, " "), "en-US-to-de-DE.xliff")

	// Manifest is written under .kapi/cache/extractions/<batch-id>/.
	extractionsRoot := filepath.Join(real, project.StateDirName, project.CacheDirName, project.ExtractionsDirName)
	batches, err := os.ReadDir(extractionsRoot)
	require.NoError(t, err)
	require.Len(t, batches, 1)
	batchID := batches[0].Name()
	manifest, err := project.LoadExtractionManifest(project.Layout{
		Root: real, RecipePath: recipe, StateDir: filepath.Join(real, project.StateDirName),
	}, batchID)
	require.NoError(t, err)
	assert.Equal(t, "kapi-extraction", manifest.Kind)
	assert.Equal(t, batchID, manifest.BatchID)
	assert.Len(t, manifest.Pairs, 2)
	// Every pair has a source → output entry for the one source file.
	for _, pair := range manifest.Pairs {
		require.Len(t, pair.Files, 1)
		assert.Equal(t, "src/locales/en/messages.json", pair.Files[0].Source)
		assert.NotEmpty(t, pair.Files[0].SourceHash)
	}
}

// TestExtractMergeKlzInterchangeRoundTrip exercises the bilingual interchange
// profile (AD-025 §7): `kapi extract --format klz` writes a
// kind=kapi-interchange package with TM-pre-filled target overlays + skeleton,
// and `kapi merge <file.klz>` hydrates those targets and writes translated
// output.
func TestExtractMergeKlzInterchangeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	writeJSONSource(t, real, "src/locales/en/messages.json",
		`{"greeting": "Hello, world."}`)

	// TM with an exact match for the source text so extract pre-fills a target.
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(t.Context(), sievepen.TMEntry{
		ID:          "tm-greeting",
		HintSrcLang: "en-US",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "Hello, world."}}},
			"fr-FR": {{Text: &model.TextRun{Text: "Bonjour le monde."}}},
		},
	}))

	// Extract --format klz.
	ea := newExtractApp(t)
	ea.TMBackend = tm
	ecmd := ea.NewExtractCmd(ExtractCmdOptions{})
	var eout bytes.Buffer
	ecmd.SetOut(&eout)
	ecmd.SetErr(&eout)
	ecmd.SetArgs([]string{"--project", recipe, "--format", "klz"})
	require.NoError(t, ecmd.Execute(), "extract output: %s", eout.String())

	klzPath := filepath.Join(real, "out", "src-locales-en-messages.en-US-to-fr-FR.klz")
	data, err := os.ReadFile(klzPath)
	require.NoError(t, err, "extract did not write the interchange .klz")
	pkg, err := klz.Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, klz.KindInterchange, pkg.Kind)
	require.NotNil(t, pkg.InterchangeTask)
	assert.Equal(t, "fr-FR", pkg.InterchangeTask.TargetLocale)
	require.NotEmpty(t, pkg.Skeletons, "interchange package must carry a skeleton")
	require.NotEmpty(t, pkg.Overlays, "interchange package must carry TM-prefilled target overlays")

	// Merge the interchange .klz back.
	ma := newExtractApp(t)
	ma.TMBackend = tm
	mcmd := ma.NewMergeCmd(MergeCmdOptions{})
	var mout bytes.Buffer
	mcmd.SetOut(&mout)
	mcmd.SetErr(&mout)
	mcmd.SetArgs([]string{"--project", recipe, klzPath})
	require.NoError(t, mcmd.Execute(), "merge output: %s", mout.String())
	assert.Contains(t, mout.String(), "applied=1")

	// The merged target file should exist and carry the translated text.
	mergedPath := filepath.Join(real, "src/locales/en", "fr-FR", "messages.json")
	merged, err := os.ReadFile(mergedPath)
	require.NoError(t, err, "merge did not write the translated output")
	assert.Contains(t, string(merged), "Bonjour le monde.")
}

func TestExtract_TargetLangFlagSubsetsRecipe(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR", "de-DE", "es-ES"})
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k": "Hello"}`)

	out, err := runExtractCmd(t, recipe, "--target-lang", "fr-FR,de-DE")
	require.NoError(t, err, "stdout: %s", out)

	outDir := filepath.Join(real, "out")
	entries, err := os.ReadDir(outDir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	require.Len(t, names, 2, "expected 2 output files, got %v", names)
	for _, n := range names {
		assert.NotContains(t, n, "es-ES")
	}
}

func TestExtract_StampsBatchIDAndSourceHashInXLIFFFile(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	sourcePath := writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	// Find the output xliff file.
	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	outBytes, err := os.ReadFile(filepath.Join(real, "out", entries[0].Name()))
	require.NoError(t, err)
	s := string(outBytes)

	assert.Contains(t, s, `category="kapi"`)
	assert.Contains(t, s, `id="batch-id"`)
	assert.Contains(t, s, `id="source-file"`)
	assert.Contains(t, s, `id="source-hash"`)

	hashExpected, err := project.HashFile(sourcePath)
	require.NoError(t, err)
	assert.Contains(t, s, hashExpected)
}

func TestExtract_TMExactPrefillFillsTarget(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	// Seed the project TM with an exact match.
	tmPath := filepath.Join(real, project.StateDirName, "tm.db")
	tm, err := sievepen.NewSQLiteTM(tmPath)
	require.NoError(t, err)
	require.NoError(t, tm.Add(t.Context(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr-FR": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
	}))
	require.NoError(t, tm.Close())

	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	outBytes, err := os.ReadFile(filepath.Join(real, "out", entries[0].Name()))
	require.NoError(t, err)
	// Target should be pre-filled with the exact match.
	assert.Contains(t, string(outBytes), "Bonjour")
}

func TestExtract_NoTMSkipsPrefill(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	writeJSONSource(t, real, "src/locales/en/app.json", `{"k":"Hello"}`)

	// Seed TM with a would-be match.
	tmPath := filepath.Join(real, project.StateDirName, "tm.db")
	tm, err := sievepen.NewSQLiteTM(tmPath)
	require.NoError(t, err)
	require.NoError(t, tm.Add(t.Context(), sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr-FR": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
	}))
	require.NoError(t, tm.Close())

	_, err = runExtractCmd(t, recipe, "--no-tm")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	outBytes, err := os.ReadFile(filepath.Join(real, "out", entries[0].Name()))
	require.NoError(t, err)
	assert.NotContains(t, string(outBytes), "Bonjour")
}

func TestExtract_NoSourceFilesIsAClearError(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"fr-FR"})
	// Intentionally no source files written.

	_, err = runExtractCmd(t, recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no source files matched")
}

func TestExtract_OnlyFiltersByCollectionName(t *testing.T) {
	// Build a recipe with two named collections; --only picks one.
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Two",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
		},
		Content: []project.ContentCollection{
			{
				Name: "web",
				Items: []project.ContentItem{
					{Path: "web/*.json", Format: &project.FormatSpec{Name: "json"}},
				},
			},
			{
				Name: "mobile",
				Items: []project.ContentItem{
					{Path: "mobile/*.json", Format: &project.FormatSpec{Name: "json"}},
				},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	writeJSONSource(t, real, "web/a.json", `{"k":"A"}`)
	writeJSONSource(t, real, "mobile/b.json", `{"k":"B"}`)

	_, err = runExtractCmd(t, recipe, "--only", "mobile")
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), "mobile")
	assert.NotContains(t, entries[0].Name(), "web")
}

// shim to keep imports stable
var (
	_ = cobra.Command{}
	_ = registry.FormatID("")
	_ = xliff2.FileNote{}
)

// Segmentation overlay — AD-017 / issue #417.

func TestExtract_SegmentationSplitsSourceIntoMultipleSegments(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "SegmentationOn",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
			Segmentation:    project.SegmentationDefaults{Source: true},
		},
		Content: []project.ContentCollection{
			{
				Path:   "src/locales/en/*.json",
				Format: &project.FormatSpec{Name: "json"},
			},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, project.StateDirName), 0o755))
	writeJSONSource(t, real, "src/locales/en/app.json",
		`{"k": "This is a sentence. Here is another one."}`)

	_, err = runExtractCmd(t, recipe)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(real, "out"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	out, err := os.ReadFile(filepath.Join(real, "out", entries[0].Name()))
	require.NoError(t, err)

	// With segmentation on, the unit should carry multiple <segment>
	// children instead of a single one.
	content := string(out)
	count := strings.Count(content, "<segment ")
	assert.GreaterOrEqual(t, count, 2, "expected multiple segments when segmentation.source=true, got %d in:\n%s", count, content)
}

func TestExtract_IncrementalReuse(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := extractProjectFixture(t, real, []model.LocaleID{"nb"})

	srcDir := filepath.Join(real, "src", "locales", "en")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.json"), []byte(`{"x":"Hello"}`), 0o644))
	outDir := filepath.Join(real, "out")

	// First extract: nothing to reuse.
	out1, err := runExtractCmd(t, recipe, "--out-dir", outDir)
	require.NoError(t, err, out1)
	assert.NotContains(t, out1, "Reused")

	// Second extract, source unchanged: reuse it (no re-parse).
	out2, err := runExtractCmd(t, recipe, "--out-dir", outDir)
	require.NoError(t, err, out2)
	assert.Contains(t, out2, "Reused 1", "an unchanged source is reused")

	// Change the source: it must re-extract (not reuse a stale result).
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.json"), []byte(`{"x":"Hello","y":"World"}`), 0o644))
	out3, err := runExtractCmd(t, recipe, "--out-dir", outDir)
	require.NoError(t, err, out3)
	assert.NotContains(t, out3, "Reused", "a changed source re-extracts")

	// Unchanged again: reuse the new (3-key) result — content-addressed, so it
	// must not pick the stale 1-key batch from the same test second.
	out4, err := runExtractCmd(t, recipe, "--out-dir", outDir)
	require.NoError(t, err, out4)
	assert.Contains(t, out4, "Reused 1")

	// --force bypasses reuse.
	out5, err := runExtractCmd(t, recipe, "--out-dir", outDir, "--force")
	require.NoError(t, err, out5)
	assert.NotContains(t, out5, "Reused")
}
