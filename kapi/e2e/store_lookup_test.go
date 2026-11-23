//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store lookups read in the project's source language when no -s is
// given, and a translation the content memory holds under two origins is one
// result that names both. The sample's source language is en-GB, so a lookup
// that defaulted to "en" would find nothing.
func TestStoreLookupsUseTheProjectSourceLanguage(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
		out, err := cmd.Output()
		require.NoError(t, err, "kapi %v failed:\n%s", args, out)
		return string(out)
	}
	run("store", "import", "--yes", "./context")

	var terms struct {
		Matches []struct {
			Term    string `json:"term"`
			Locale  string `json:"locale"`
			Targets []struct {
				Text string `json:"text"`
			} `json:"targets"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal([]byte(run("terms", "lookup", "alert", "-t", "de", "--json")), &terms))
	require.Len(t, terms.Matches, 1, "terms lookup finds the en-GB term without -s")
	assert.Equal(t, "en-GB", terms.Matches[0].Locale)
	require.NotEmpty(t, terms.Matches[0].Targets)
	assert.Equal(t, "Warnung", terms.Matches[0].Targets[0].Text)

	var none struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(run("terms", "lookup", "alert", "-s", "en", "-t", "de", "--json")), &none))
	assert.Zero(t, none.Total, "an explicit -s wins over the project's source language")

	// The first run absorbs the committed Norwegian file and materializes the
	// targets, which records "Berth plan" under both origins.
	run("up")
	var memory struct {
		Matches []struct {
			Source   string   `json:"source"`
			Target   string   `json:"target"`
			EntryID  string   `json:"entry_id"`
			EntryIDs []string `json:"entry_ids"`
			Origins  []string `json:"origins"`
		} `json:"matches"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(run("memory", "lookup", "Berth plan", "-t", "nb", "--json")), &memory))
	require.Len(t, memory.Matches, 1, "one translation is one result")
	m := memory.Matches[0]
	assert.Equal(t, "Berth plan", m.Source)
	assert.Equal(t, "Kaiplan", m.Target)
	assert.Equal(t, 1, memory.Total)
	assert.Contains(t, m.EntryIDs, m.EntryID)
	assert.Equal(t, []string{"merge", "record"}, m.Origins)
}

// A standalone store holds content of no particular project, so a lookup in
// one reads in the built-in default even inside a project whose source
// language is en-GB. Each store selector of each lookup command is driven:
// --file, --name and --local.
func TestStoreLookupsInAStandaloneStoreKeepTheDefault(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dataRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	configDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFixture(t, filepath.Join(findRoot(), "samples", "compass"), dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(kapiBin, append(args, "-p", ".")...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), isoEnv...),
			"KAPI_DATA_DIR="+dataRoot, "KAPI_CONFIG_DIR="+configDir)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "kapi %v failed:\n%s", args, out)
		return string(out)
	}
	total := func(out string) int {
		t.Helper()
		var res struct {
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &res), out)
		return res.Total
	}

	for _, sel := range [][]string{
		{"--file", filepath.Join(dir, "legacy.db")},
		{"--name", "legacy"},
		{"--local"},
	} {
		t.Run(sel[0], func(t *testing.T) {
			memArgs := append([]string{"memory", "import", filepath.Join(testdata, "project.tmx"), "-s", "en", "-t", "fr"}, sel...)
			run(memArgs...)
			lookup := append([]string{"memory", "lookup", "Settings", "-t", "fr", "--json"}, sel...)
			assert.Equal(t, 1, total(run(lookup...)), "memory lookup %s reads in en", sel[0])

			termArgs := append([]string{"terms", "import", filepath.Join(testdata, "terms.csv"),
				"--format", "csv", "-s", "en", "-t", "fr", "--header"}, sel...)
			run(termArgs...)
			lookup = append([]string{"terms", "lookup", "password", "-t", "fr", "--json"}, sel...)
			assert.Equal(t, 1, total(run(lookup...)), "terms lookup %s reads in en", sel[0])
		})
	}
}
