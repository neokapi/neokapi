package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context import|snapshot|export|restore` — the portability half of the
// context surface, CLI leg. The properties themselves are held in host; what is
// asserted here is that each verb reaches its host function, resolves the
// project the way every other project verb does, and renders through the
// result's own FormatText so --json and the text form say the same thing.

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
  voice:
    profile_file: .kapi/voice.yaml
  terms_source: .kapi/terms.json
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

// TestContextPortability_TheFourVerbsRunOnAProject walks the whole cycle the
// way a person does: read the committed files, write them back, pack the lot,
// and unpack it somewhere else.
func TestContextPortability_TheFourVerbsRunOnAProject(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	a := &App{}

	out := runContext(t, a, "import", "-p", recipe, "--json")
	var imported struct {
		Concepts      int `json:"concepts"`
		VoiceProfiles int `json:"voiceProfiles"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &imported))
	assert.Equal(t, 1, imported.Concepts)
	assert.Equal(t, 1, imported.VoiceProfiles)

	snapshot := filepath.Join(t.TempDir(), "context")
	out = runContext(t, a, "snapshot", "-p", recipe, "--out", snapshot, "--json")
	var written struct {
		Concepts      int      `json:"concepts"`
		VoiceProfiles int      `json:"voiceProfiles"`
		Written       []string `json:"written"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &written))
	assert.Equal(t, 1, written.Concepts)
	assert.Equal(t, 1, written.VoiceProfiles)
	assert.Contains(t, written.Written, "terms.json")
	assert.FileExists(t, filepath.Join(snapshot, "voice.yaml"))

	bundle := filepath.Join(t.TempDir(), "context.kpz")
	out = runContext(t, a, "export", "-p", recipe, "-o", bundle, "--json")
	var exported struct {
		RootHash string `json:"rootHash"`
		Bytes    int    `json:"bytes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &exported))
	assert.NotEmpty(t, exported.RootHash)
	assert.Positive(t, exported.Bytes)

	// Restoring into the project it came from is refused until the caller says
	// what should happen to what is already there.
	_, err := runContextE(t, a, "restore", bundle, "-p", recipe)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge")

	out = runContext(t, a, "restore", bundle, "-p", recipe, "--merge", "--json")
	var restored struct {
		Concepts int `json:"concepts"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &restored))
	assert.Equal(t, 1, restored.Concepts)
}

// TestContextPortability_RefusesTwoAnswersToOneQuestion: --merge and --replace
// ask for different things, so passing both is a refusal rather than a
// precedence rule nobody could guess.
func TestContextPortability_RefusesTwoAnswersToOneQuestion(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")

	_, err := runContextE(t, &App{}, "restore", "unused.kpz", "-p", recipe, "--merge", "--replace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge")
	assert.Contains(t, err.Error(), "--replace")
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

	// A second read has nothing to say, and says so rather than reporting work
	// it did not do.
	again := runContext(t, a, "import", "-p", recipe)
	assert.Contains(t, again, "Nothing to read")
}
