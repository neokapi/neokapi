package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStatsJSON(t *testing.T, name, content string, extra ...string) StatsOutput {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	app := newAppForTest(t)
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	cmd := app.NewStatsCmd()
	// --json is a persistent root flag in production; add it for the test.
	cmd.Flags().Bool("json", true, "")
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append(extra, path))
	require.NoError(t, cmd.Execute())

	var got StatsOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), "output must be JSON: %s", out.String())
	return got
}

func TestStats_Counts(t *testing.T) {
	got := runStatsJSON(t, "en.json", `{"greeting":"Hello there world","bye":"Goodbye"}`)

	require.Len(t, got.Files, 1)
	assert.Equal(t, 2, got.Total.Translatable, "two string values are two translatable blocks")
	assert.Equal(t, 4, got.Total.Words, "'Hello there world' (3) + 'Goodbye' (1)")
	// "Hello there world" = 17 runes, "Goodbye" = 7 → 24 characters.
	assert.Equal(t, 24, got.Total.Characters)
	assert.Positive(t, got.Total.Segments, "every block counts as at least one segment")
}

func TestStats_ByRole(t *testing.T) {
	got := runStatsJSON(t, "page.md", "# Title\n\nA paragraph here.\n")
	assert.Positive(t, got.Total.ByRole[string(model.RoleHeading)], "a markdown heading is counted by role: %+v", got.Total.ByRole)
	assert.Positive(t, got.Total.Blocks)
}

func TestStats_MultiFileTotal(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	app := newAppForTest(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	require.NoError(t, os.WriteFile(a, []byte(`{"x":"one two"}`), 0o644))
	require.NoError(t, os.WriteFile(b, []byte(`{"y":"three","z":"four five"}`), 0o644))

	cmd := app.NewStatsCmd()
	cmd.Flags().Bool("json", true, "")
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{a, b})
	require.NoError(t, cmd.Execute())

	var got StatsOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Len(t, got.Files, 2)
	assert.Equal(t, 3, got.Total.Translatable, "1 + 2 blocks across files")
	assert.Equal(t, 5, got.Total.Words, "'one two'(2) + 'three'(1) + 'four five'(2)")
}
