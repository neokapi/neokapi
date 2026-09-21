package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coldStartEnvValue(t *testing.T, env []string, name string) string {
	t.Helper()
	for _, pair := range env {
		if key, value, ok := strings.Cut(pair, "="); ok && key == name {
			return value
		}
	}
	return ""
}

func TestColdStartEnvKeepsEveryRootInsideTheCell(t *testing.T) {
	paths := coldStartPaths(filepath.Join(t.TempDir(), "release-note-claude"))
	env := coldStartEnv(paths)

	for name, want := range map[string]string{
		"KAPI_DATA_DIR":         paths.Data,
		"KAPI_CONFIG_DIR":       paths.Config,
		"XDG_CACHE_HOME":        paths.Cache,
		"XDG_DATA_HOME":         filepath.Join(paths.Root, "xdg-data"),
		"KAPI_PLUGINS_DIR":      paths.Plugins,
		"KAPI_PLUGINS_DIR_ONLY": "1",
		"HOME":                  paths.Home,
		"PATH":                  paths.Bin,
	} {
		assert.Equal(t, want, coldStartEnvValue(t, env, name), "%s names this cell's own directory", name)
	}
	assert.NotContains(t, coldStartEnvNames(env), "KAPI_NO_PROJECT",
		"the fixture's own recipe must be discoverable, which is part of what the drill measures")
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"} {
		assert.True(t, strings.HasPrefix(coldStartEnvValue(t, env, name), paths.State),
			"%s keeps the host out of the developer's own configuration", name)
	}
	assert.Equal(t, os.DevNull, coldStartEnvValue(t, env, "GIT_CONFIG_GLOBAL"))
}

func TestColdStartEnvNamesCarryNoValues(t *testing.T) {
	names := coldStartEnvNames([]string{"KAPI_DATA_DIR=/somewhere", "PATH=/bin"})
	assert.Equal(t, []string{"KAPI_DATA_DIR", "PATH"}, names)
	assert.True(t, slices.IsSorted(names))
}

func TestColdStartAncestorFindings(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "nested", coldStartRepoName)
	require.NoError(t, os.MkdirAll(repo, 0o700))

	findings, err := coldStartAncestorFindings(repo)
	require.NoError(t, err)
	assert.Empty(t, findings, "a fixture under a bare temporary directory has nothing above it")

	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("version: v1\n"), 0o600))
	findings, err = coldStartAncestorFindings(repo)
	require.NoError(t, err)
	require.Len(t, findings, 1, "a recipe above the fixture is what the upward walk would bind to")
	assert.True(t, strings.HasSuffix(findings[0], filepath.Join("001", "kapi.yaml")), "reported %s", findings[0])
}

func TestColdStartWitnessNoticesAWrite(t *testing.T) {
	dir := t.TempDir()
	before, err := coldStartWitness(dir)
	require.NoError(t, err)

	unchanged, err := coldStartWitness(dir)
	require.NoError(t, err)
	assert.Equal(t, before, unchanged)
	assert.False(t, coldStartWitnessChanged(before, unchanged))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace.db"), []byte("x"), 0o600))
	after, err := coldStartWitness(dir)
	require.NoError(t, err)
	assert.True(t, coldStartWitnessChanged(before, after),
		"a write under the person's own data root has to be visible")
}

func TestColdStartWitnessOfAnAbsentRoot(t *testing.T) {
	witness, err := coldStartWitness(filepath.Join(t.TempDir(), "never-created"))
	require.NoError(t, err)
	assert.Equal(t, "absent", witness)
	assert.False(t, coldStartWitnessChanged("absent", "absent"))
}

func TestColdStartRecipeMapping(t *testing.T) {
	scaffolded := "version: v1\nname: fernwell-ledger\ndefaults:\n  source_language: en\n  voice:\n    pack: professional-b2b\n\ncollections: []\n\nflows:\n  check:\n    steps:\n      - tool: voice-vocab-check\n"

	mapped, err := coldStartRecipeMapping([]byte(scaffolded))
	require.NoError(t, err)
	body := string(mapped)
	assert.NotContains(t, body, "professional-b2b", "the drill starts from a context that holds nothing")
	assert.NotContains(t, body, "collections: []")
	assert.Contains(t, body, `- path: "docs/**/*.md"`)
	assert.Contains(t, body, "voice-vocab-check", "the scaffolded check flow stays as it was")

	_, err = coldStartRecipeMapping([]byte("version: v1\ncollections: []\n"))
	require.ErrorContains(t, err, "starter voice pack")

	_, err = coldStartRecipeMapping([]byte("version: v1\n  voice:\n    pack: professional-b2b\n"))
	require.ErrorContains(t, err, "collections: []")
}

func TestColdStartResolveOnPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	target := filepath.Join(dir, "kapi-build")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700))
	require.NoError(t, os.Symlink(target, filepath.Join(bin, "kapi")))

	resolved, err := coldStartResolveOnPath("kapi", bin)
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, expected, resolved,
		"the bare command name in .mcp.json resolves to the build first on the cell's PATH")
}

func TestMaterializeColdStartRepo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), coldStartRepoName)
	require.NoError(t, materializeColdStartRepo(dir))

	for _, name := range []string{"README.md", "docs/getting-started.md", "docs/billing.md", "docs/troubleshooting.md", "ui/strings.json", "emails/welcome.md"} {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
		require.NoError(t, err, "the fixture ships %s", name)
	}
	for _, name := range []string{"kapi.yaml", ".mcp.json", ".kapi"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.True(t, os.IsNotExist(err), "the fixture carries no kapi wiring of its own: %s", name)
	}
}

// The latent conventions are the drill's subject, so the fixture keeps them.
// A fixture that says "log in" somewhere has no consistent verb to notice, and
// the correction the one cued task carries would contradict its own repository.
func TestColdStartFixtureKeepsItsConventions(t *testing.T) {
	files, err := coldStartRepoFiles()
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for name, body := range files {
		text := string(body)
		assert.NotContains(t, strings.ToLower(text), "log in", "%s keeps the one verb", name)
		assert.NotContains(t, text, "FernWell", "%s keeps the product name's casing", name)
		assert.NotContains(t, text, "QuickCast", "%s keeps the feature name's spelling", name)
		assert.NotContains(t, strings.ToLower(text), "kapi", "%s mentions no kapi surface", name)
	}
	assert.Contains(t, string(files["README.md"]), "Fernwell Ledger")
	assert.Contains(t, string(files["docs/getting-started.md"]), "sign in")
}

// A prompt that named kapi, context or checking would be asking rather than
// measuring.
func TestColdStartPromptsAskForWritingOnly(t *testing.T) {
	forbidden := []string{"kapi", "context", "terms", "voice profile", "record ", "check "}
	for _, task := range coldStartTasks() {
		lower := strings.ToLower(task.Prompt)
		for _, word := range forbidden {
			assert.NotContains(t, lower, word, "task %s names %q in its prompt", task.ID, word)
		}
		assert.NotEmpty(t, task.Note, "task %s carries an evaluator note", task.ID)
	}
}
