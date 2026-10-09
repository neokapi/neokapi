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

// Codex hands a stdio MCP server a filtered environment. The roots the cell
// sets are the ones `kapi init` names under env_vars in the fixture, which the
// probe reads back from Codex, so every root the cell sets is one the probe
// requires.
func TestEvalCodexProbeRequiresEveryRootTheCellSets(t *testing.T) {
	paths := evalPaths(filepath.Join(t.TempDir(), "release-note-codex"))

	for _, pair := range evalEnv(paths) {
		key, _, ok := strings.Cut(pair, "=")
		if ok && (strings.HasPrefix(key, "KAPI_") || strings.HasPrefix(key, "XDG_")) {
			assert.Contains(t, evalCodexForwardedRoots, key, "the cell sets %s, which the probe does not require of the server entry", key)
		}
	}
	assert.NotContains(t, evalCodexForwardedRoots, "CODEX_HOME", "only the kapi roots are required of the server")
}

// A cells directory the person names is where the fixtures are generated, so a
// review that comes days after the batch still has them to read.
func TestEvalCellsDir(t *testing.T) {
	repo := t.TempDir()
	cells := t.TempDir()

	defaulted, err := evalCellsDir(EvalOptions{RepoRoot: repo})
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(os.TempDir()), filepath.Clean(defaulted),
		"an unnamed cells directory is the system temporary directory")

	named, err := evalCellsDir(EvalOptions{RepoRoot: repo, CellsDir: cells})
	require.NoError(t, err)
	assert.Equal(t, cells, named)

	for _, inside := range []string{repo, filepath.Join(repo, "harness", "out", "cells")} {
		_, err := evalCellsDir(EvalOptions{RepoRoot: repo, CellsDir: inside})
		require.ErrorContains(t, err, "sits inside this checkout",
			"a cell inside the tree would bind to neokapi's own project")
	}
}

// A recipe above a selected cells directory must block the batch, just as one
// above a temporary directory does, to prevent accidental project discovery.
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
