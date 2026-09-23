package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Codex half of a cell is the shipped wiring plus one act: the fixture is
// marked as a trusted project, which is what a person does at the trust prompt.
// The server entry itself stays the one `kapi init` wrote into the repository.
func TestEvalCodexConfigTrustsTheFixtureAndNamesNoServer(t *testing.T) {
	paths := evalPaths(filepath.Join(t.TempDir(), "release-note-codex"))
	prepared := EvalPrepared{
		Session: EvalSession{Host: PairedAgentSpec{Host: "codex", Model: "gpt-5.6-terra", Effort: "medium"}},
		Paths:   paths,
		Env:     evalEnv(paths),
	}

	config := evalCodexConfig(prepared)

	assert.NotContains(t, config, "[mcp_servers",
		"the kapi server is the one kapi init wrote into the repository's own .codex/config.toml")
	assert.Contains(t, config, "[projects."+strconv.Quote(paths.Repo)+"]\ntrust_level = \"trusted\"\n")
	assert.Contains(t, config, `model_reasoning_effort = "medium"`)
	assert.Contains(t, config, `"KAPI_DATA_DIR" = `+strconv.Quote(paths.Data),
		"the shell tool a session runs commands with carries the cell's own roots")
}

// Codex hands a stdio MCP server a filtered environment, so what the cell's
// kapi server sees is what the launch names.
func TestEvalCodexForwardsEveryKapiRoot(t *testing.T) {
	paths := evalPaths(filepath.Join(t.TempDir(), "release-note-codex"))

	override := evalCodexForwardedEnv(evalEnv(paths))

	assert.True(t, strings.HasPrefix(override, "mcp_servers.kapi.env_vars=["), "rendered %s", override)
	for _, name := range []string{"KAPI_DATA_DIR", "KAPI_CONFIG_DIR", "KAPI_PLUGINS_DIR", "KAPI_PLUGINS_DIR_ONLY", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		assert.Contains(t, override, strconv.Quote(name))
	}
	assert.NotContains(t, override, "CODEX_HOME", "only the kapi roots travel to the server")
	assert.NotContains(t, override, "GIT_AUTHOR_NAME")
}

// A cells directory the person names is where the fixtures are generated, so a
// review that comes days after the batch still has them to read.
func TestEvalCellsDir(t *testing.T) {
	repo := t.TempDir()
	cells := t.TempDir()

	defaulted, err := evalCellsDir(EvalOptions{RepoRoot: repo})
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(os.TempDir()), filepath.Clean(defaulted),
		"an unnamed cells directory stays where the drill has always put it")

	named, err := evalCellsDir(EvalOptions{RepoRoot: repo, CellsDir: cells})
	require.NoError(t, err)
	assert.Equal(t, cells, named)

	for _, inside := range []string{repo, filepath.Join(repo, "harness", "out", "cells")} {
		_, err := evalCellsDir(EvalOptions{RepoRoot: repo, CellsDir: inside})
		require.ErrorContains(t, err, "sits inside this checkout",
			"a cell inside the tree would bind to neokapi's own project")
	}
}

// The ancestor check is what makes a chosen cells directory safe, so a recipe
// above it blocks the batch the same way one above a temporary directory does.
func TestEvalCellsDirIsCoveredByTheAncestorCheck(t *testing.T) {
	cells := t.TempDir()
	repo := filepath.Join(cells, "release-note-codex", evalRepoName)
	require.NoError(t, os.MkdirAll(repo, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cells, "CLAUDE.md"), []byte("# guidance\n"), 0o600))

	findings, err := evalAncestorFindings(repo)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.True(t, strings.HasSuffix(findings[0], "CLAUDE.md"), "reported %s", findings[0])
}
