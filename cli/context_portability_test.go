package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// TestContextPortability_WorkspaceCarriesEveryProject: --workspace moves the
// context of every project worked on here rather than the one in scope, and
// --dry-run reports what that file would hold without writing it.
func TestContextPortability_WorkspaceCarriesEveryProject(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	a.SetWorkspaceRoot(t.TempDir())
	t.Cleanup(a.Shutdown)

	// Two projects, each opened once so the workspace registers it.
	recipes := make([]string, 0, 2)
	for _, name := range []string{"alpha-cli", "beta-cli"} {
		root := writePortableCLIProject(t)
		recipe := filepath.Join(root, "kapi.yaml")
		body, err := os.ReadFile(recipe)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(recipe,
			[]byte(strings.Replace(string(body), "name: portable-cli", "name: "+name, 1)), 0o644))
		runContext(t, a, "import", "-p", recipe, "--json")
		recipes = append(recipes, recipe)
	}

	out := runContext(t, a, "export", "--workspace", "--dry-run", "--json")
	var listed struct {
		Path     string `json:"path"`
		Projects []struct {
			Key      string `json:"key"`
			Concepts int    `json:"concepts"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &listed))
	assert.Empty(t, listed.Path, "a listing writes nothing")
	require.Len(t, listed.Projects, 2)
	assert.Equal(t, "alpha-cli", listed.Projects[0].Key)
	assert.Equal(t, 1, listed.Projects[0].Concepts)

	bundle := filepath.Join(t.TempDir(), "workspace.kpz")
	out = runContext(t, a, "export", "--workspace", "-o", bundle, "--json")
	var exported struct {
		RootHash string `json:"rootHash"`
		Projects []struct {
			Key string `json:"key"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &exported))
	assert.NotEmpty(t, exported.RootHash)
	assert.Len(t, exported.Projects, 2)

	// The project verb and the workspace verb read different archives, and each
	// says which one it wanted.
	_, err := runContextE(t, a, "restore", bundle, "-p", recipes[0], "--merge")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kapi-workspace")

	// A workspace already holding projects is left alone until the caller says
	// what should happen to it.
	_, err = runContextE(t, a, "restore", "--workspace", bundle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--merge")

	out = runContext(t, a, "restore", "--workspace", bundle, "--merge", "--json")
	var restored struct {
		Projects []struct {
			Key      string `json:"key"`
			Concepts int    `json:"concepts"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &restored))
	require.Len(t, restored.Projects, 2)
	assert.Equal(t, 1, restored.Projects[0].Concepts)
}

// TestContextPortability_DryRunBelongsToTheWorkspaceExport: a flag that only
// means something with --workspace says so rather than being ignored.
func TestContextPortability_DryRunBelongsToTheWorkspaceExport(t *testing.T) {
	root := writePortableCLIProject(t)
	recipe := filepath.Join(root, "kapi.yaml")

	_, err := runContextE(t, &App{}, "export", "-p", recipe, "-o", "unused.kpz", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--workspace")
	assert.NoFileExists(t, "unused.kpz")
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
