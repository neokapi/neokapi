package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context import|export` — the portability half of the context surface,
// CLI leg. The properties themselves are held in host; what is asserted here is
// that each verb reaches its host function, resolves the project the way every
// other project verb does, and renders through the result's own FormatText so
// --json and the text form say the same thing.

// writePortableCLIProject builds a small governed project and returns its root.
func writePortableCLIProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
name: portable-cli
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`)
	write(".kapi/voice.yaml", `name: Portable CLI Voice
version: 1
tone:
  formality: neutral
`)
	write(".kapi/terms.json", `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-widget",
      "terms": [
        {"text": "widget", "locale": "en", "status": "approved"}
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
`)
	write("locales/en/app.json", "{\n  \"greeting\": \"Hello there\"\n}\n")
	write("locales/nb/app.json", "{\n  \"greeting\": \"Hei der\"\n}\n")
	return root
}

// TestContextPortability_ExportAndImportRunOnAProject walks the cycle the way a
// person does: read the committed files, write the context to one file, and
// read that file into another checkout of the project.
func TestContextPortability_ExportAndImportRunOnAProject(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)

	out := runContext(t, a, "import", "-p", recipe, "--json")
	var imported struct {
		Concepts      int `json:"concepts"`
		VoiceProfiles int `json:"voiceProfiles"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &imported))
	assert.Equal(t, 1, imported.Concepts)
	assert.Equal(t, 1, imported.VoiceProfiles)

	file := filepath.Join(t.TempDir(), "context.kpz")
	out = runContext(t, a, "export", "-p", recipe, "-o", file, "--json")
	var exported struct {
		Operations int `json:"operations"`
		Bytes      int `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &exported))
	assert.Positive(t, exported.Operations)
	assert.Positive(t, exported.Bytes)

	_, err := runContextE(t, a, "export", "-p", recipe)
	require.Error(t, err, "export names the file it writes")

	clone := writePortableCLIProject(t)
	b := &App{}
	b.InitRegistries()
	t.Cleanup(b.Shutdown)
	got := runContext(t, b, "import", file, "-p", filepath.Join(clone, "kapi.yaml"))
	assert.Contains(t, got, "Merged ")
	db, err := b.ProjectDB(t.Context(), clone)
	require.NoError(t, err)
	concepts, err := db.Terms().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, concepts, "the other checkout holds the terms the file carried")
}

// TestContextPortability_TextFormIsTheResultsOwnRender: the text a reader sees
// is the result type's FormatText, which is what keeps --json and the text form
// describing one thing.
func TestContextPortability_TextFormIsTheResultsOwnRender(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}

	got := runContext(t, a, "import", "-p", recipe)
	assert.Contains(t, got, "1 concept")
	assert.Contains(t, got, "1 voice profile")

	// The same run's --json carries the numbers the text spells out, which is
	// what keeps a reader and a program describing one thing. It reads with
	// --force, because this checkout has already read these bytes.
	out := runContext(t, a, "import", "-p", recipe, "--force", "--json")
	var imported struct {
		Concepts      int `json:"concepts"`
		VoiceProfiles int `json:"voiceProfiles"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &imported))
	assert.Equal(t, 1, imported.Concepts)
	assert.Equal(t, 1, imported.VoiceProfiles)

	// Every importer upserts by the identity its file carries, so reading the
	// layout a third time leaves the store holding one copy of each.
	runContext(t, a, "import", "-p", recipe)
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	concepts, err := db.Terms().Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, concepts)
}

// TestContextLocales_ReportsAndFiles: the verb the drift warning names reports
// the spellings the store's rows carry, and --fix files the authored ones the
// way lookups ask.
func TestContextLocales_ReportsAndFiles(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)

	runContext(t, a, "import", "-p", recipe, "--json")
	assert.Contains(t, runContext(t, a, "locales", "-p", recipe),
		"Every row is keyed by the locale its lookups ask for.")

	// A term row as a store that predates canonical locales held it.
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	_, err = db.Raw().ExecContext(t.Context(), `
INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags, forms)
VALUES ('c-widget', 'dings', 'dings', 'NB-no', 'approved', '', '', '', 0, NULL, NULL, '[]', '[]')`)
	require.NoError(t, err)

	out := runContext(t, a, "locales", "-p", recipe, "--json")
	var found struct {
		Drift []struct {
			Subsystem string `json:"subsystem"`
			Pool      string `json:"pool"`
			Locale    string `json:"locale"`
			Canonical string `json:"canonical"`
			Rows      int    `json:"rows"`
		} `json:"drift"`
		Rekeyed []struct{} `json:"rekeyed"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &found))
	require.Len(t, found.Drift, 1)
	assert.Equal(t, "terms", found.Drift[0].Subsystem)
	assert.Equal(t, "context", found.Drift[0].Pool)
	assert.Equal(t, "nb-NO", found.Drift[0].Canonical)
	assert.Empty(t, found.Rekeyed, "a report writes nothing on its own")

	fixed := runContext(t, a, "locales", "-p", recipe, "--fix")
	assert.Contains(t, fixed, `terms: 1 row(s) moved from "NB-no" to "nb-NO"`)
	assert.Contains(t, fixed, "Every row is keyed by the locale its lookups ask for.")
}
