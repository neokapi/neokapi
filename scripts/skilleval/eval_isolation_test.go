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

func evalEnvValue(t *testing.T, env []string, name string) string {
	t.Helper()
	for _, pair := range env {
		if key, value, ok := strings.Cut(pair, "="); ok && key == name {
			return value
		}
	}
	return ""
}

func TestEvalEnvKeepsEveryRootInsideTheCell(t *testing.T) {
	paths := evalPaths(filepath.Join(t.TempDir(), "release-note-claude"))
	env := evalEnv(paths)

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
		assert.Equal(t, want, evalEnvValue(t, env, name), "%s names this cell's own directory", name)
	}
	assert.NotContains(t, evalEnvNames(env), "KAPI_NO_PROJECT",
		"the fixture's own recipe must be discoverable, which is part of what the drill measures")
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"} {
		assert.True(t, strings.HasPrefix(evalEnvValue(t, env, name), paths.State),
			"%s keeps the host out of the developer's own configuration", name)
	}
	assert.Equal(t, os.DevNull, evalEnvValue(t, env, "GIT_CONFIG_GLOBAL"))
}

func TestEvalEnvNamesCarryNoValues(t *testing.T) {
	names := evalEnvNames([]string{"KAPI_DATA_DIR=/somewhere", "PATH=/bin"})
	assert.Equal(t, []string{"KAPI_DATA_DIR", "PATH"}, names)
	assert.True(t, slices.IsSorted(names))
}

func TestEvalAncestorFindings(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "nested", evalRepoName)
	require.NoError(t, os.MkdirAll(repo, 0o700))

	findings, err := evalAncestorFindings(repo)
	require.NoError(t, err)
	assert.Empty(t, findings, "a fixture under a bare temporary directory has nothing above it")

	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte("version: v1\n"), 0o600))
	findings, err = evalAncestorFindings(repo)
	require.NoError(t, err)
	require.Len(t, findings, 1, "a recipe above the fixture is what the upward walk would bind to")
	assert.True(t, strings.HasSuffix(findings[0], filepath.Join("001", "kapi.yaml")), "reported %s", findings[0])
}

func TestEvalWitnessNoticesAWrite(t *testing.T) {
	dir := t.TempDir()
	before, err := evalWitness(dir)
	require.NoError(t, err)

	unchanged, err := evalWitness(dir)
	require.NoError(t, err)
	assert.Equal(t, before, unchanged)
	assert.False(t, evalWitnessChanged(before, unchanged))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "workspace.db"), []byte("x"), 0o600))
	after, err := evalWitness(dir)
	require.NoError(t, err)
	assert.True(t, evalWitnessChanged(before, after),
		"a write under the person's own data root has to be visible")
}

func TestEvalWitnessOfAnAbsentRoot(t *testing.T) {
	witness, err := evalWitness(filepath.Join(t.TempDir(), "never-created"))
	require.NoError(t, err)
	assert.Equal(t, "absent", witness)
	assert.False(t, evalWitnessChanged("absent", "absent"))
}

func TestEvalResolveOnPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o700))
	target := filepath.Join(dir, "kapi-build")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700))
	require.NoError(t, os.Symlink(target, filepath.Join(bin, "kapi")))

	resolved, err := evalResolveOnPath("kapi", bin)
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, expected, resolved,
		"the bare command name in .mcp.json resolves to the build first on the cell's PATH")
}

// A prompt that named kapi, context or checking would be asking rather than
// measuring.
func TestEvalPromptsAskForWritingOnly(t *testing.T) {
	forbidden := []string{"kapi", "context", "terms", "voice profile", "record ", "check "}
	for _, task := range evalTasks() {
		lower := strings.ToLower(task.Prompt)
		for _, word := range forbidden {
			assert.NotContains(t, lower, word, "task %s names %q in its prompt", task.ID, word)
		}
		assert.NotEmpty(t, task.Note, "task %s carries an evaluator note", task.ID)
	}
}
