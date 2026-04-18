package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureProjectArchive writes a tiny .klz at `path` with one block
// partially translated (fr only) and one block source-only, so the
// coverage reporter has material to diff against the declared
// target-languages in a .kapi project.
func fixtureProjectArchive(t *testing.T, path string) {
	t.Helper()
	doc := &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "test", Version: "0.0.0"},
		Project:       klf.ProjectInfo{ID: "demo", SourceLocale: "en"},
		Documents: []klf.Document{
			{
				ID:           "doc1",
				DocumentType: klf.DocumentTypeJSX,
				Path:         "src/App.tsx",
				Blocks: []klf.Block{
					{
						ID:           "b1",
						Hash:         "abc",
						Translatable: true,
						Type:         klf.BlockTypeJSXElement,
						Source:       []klf.Run{{Text: &klf.TextRun{Text: "Hello"}}},
						Targets: map[klf.LocaleID][]klf.Run{
							"fr": {{Text: &klf.TextRun{Text: "Bonjour"}}},
						},
					},
					{
						ID:           "b2",
						Hash:         "def",
						Translatable: true,
						Type:         klf.BlockTypeJSXElement,
						Source:       []klf.Run{{Text: &klf.TextRun{Text: "World"}}},
					},
				},
			},
		},
	}

	w := klz.NewWriter(klz.WriterOptions{
		Generator: klz.ManifestGenerator{ID: "test", Version: "0.0.0"},
		Project:   klz.ManifestProject{ID: "demo", SourceLocale: "en"},
		Created:   "2026-04-18T00:00:00Z",
	})
	require.NoError(t, w.AddDocument("documents/App.klf", doc, nil))

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	_, err = w.Write(f)
	require.NoError(t, err)
}

func writeProject(t *testing.T, dir string, yamlContent string) string {
	t.Helper()
	path := filepath.Join(dir, "project.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))
	return path
}

func TestCollectProjectStatus(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))

	proj := writeProject(t, dir, `
version: v1
name: Demo
defaults:
  source_language: en
  target_languages: [fr, de]
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
  - name: legacy
    items:
      - path: "legacy/**/*.json"
`)

	status, err := CollectProjectStatus(proj)
	require.NoError(t, err)
	require.Len(t, status.Collections, 2)

	ui := status.Collections[0]
	assert.Equal(t, "ui", ui.Name)
	assert.Equal(t, "i18n/ui.klz", ui.Archive)
	assert.True(t, ui.ArchiveExists)
	assert.Equal(t, 2, ui.BlockCount)
	assert.Equal(t, 1, ui.Coverage["fr"], "only one block has fr target")
	assert.Equal(t, 0, ui.Coverage["de"], "no block has de target")

	legacy := status.Collections[1]
	assert.Empty(t, legacy.Archive)
	assert.False(t, legacy.ArchiveExists)
}

func TestCollectProjectStatusMissingArchive(t *testing.T) {
	dir := t.TempDir()
	// Don't create the .klz.
	proj := writeProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/never-extracted.klz
    items:
      - path: "src/**/*.tsx"
`)

	status, err := CollectProjectStatus(proj)
	require.NoError(t, err)
	require.Len(t, status.Collections, 1)
	assert.Equal(t, "ui", status.Collections[0].Name)
	assert.Equal(t, "i18n/never-extracted.klz", status.Collections[0].Archive)
	assert.False(t, status.Collections[0].ArchiveExists)
	assert.Zero(t, status.Collections[0].BlockCount)
}

func TestRunStatusOutput(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))
	proj := writeProject(t, dir, `
version: v1
name: Demo
defaults:
  target_languages: [fr, de]
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	require.NoError(t, runStatus(&out, proj))
	s := out.String()
	assert.Contains(t, s, "Demo", "project name appears")
	assert.Contains(t, s, "i18n/ui.klz")
	assert.Contains(t, s, "2 blocks")
	assert.Contains(t, s, "fr:")
	assert.Contains(t, s, "1/2 translated")
	assert.Contains(t, s, "de:")
	assert.Contains(t, s, "not translated")
}

func TestPlanSyncIdentifiesMissingLocales(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))
	proj := writeProject(t, dir, `
version: v1
defaults:
  target_languages: [fr, de, ja]
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	_, plan, err := PlanSync(proj)
	require.NoError(t, err)
	require.Len(t, plan, 3, "fr partial + de missing + ja missing = 3 steps")

	locales := make([]string, 0, 3)
	for _, step := range plan {
		locales = append(locales, string(step.Locale))
		assert.Equal(t, "ui", step.Collection)
		assert.Equal(t, "i18n/ui.klz", step.Archive)
	}
	assert.ElementsMatch(t, []string{"fr", "de", "ja"}, locales)
}

func TestRunShowFindsBlock(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))
	proj := writeProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	require.NoError(t, runShow(&out, proj, "abc"))
	s := out.String()
	assert.Contains(t, s, "abc")
	assert.Contains(t, s, "Hello", "source text rendered")
	assert.Contains(t, s, "fr:", "locale-keyed target listed")
	assert.Contains(t, s, "Bonjour", "target text rendered")
}

func TestRunShowUnknownHash(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))
	proj := writeProject(t, dir, `
version: v1
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	err := runShow(&out, proj, "unknownhash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRunSyncDryRun(t *testing.T) {
	dir := t.TempDir()
	fixtureProjectArchive(t, filepath.Join(dir, "i18n", "ui.klz"))
	proj := writeProject(t, dir, `
version: v1
defaults:
  target_languages: [fr, de]
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
`)

	var out bytes.Buffer
	require.NoError(t, runSync(context.Background(), &out, proj, "ai-translate", true))
	s := out.String()
	assert.Contains(t, s, "--dry-run", "dry-run footer present")
	assert.Contains(t, s, "kapi ai-translate")
	assert.Contains(t, s, "--target-lang fr")
	assert.Contains(t, s, "--target-lang de")
	assert.True(t,
		strings.Count(s, "--target-lang") == 2,
		"exactly one line per missing locale")
}
