package state_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
)

func readShardBytes(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	require.NoError(t, err)
	return b
}

func shardNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), state.CommittedExt) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func removeAll(path string) error { return os.RemoveAll(filepath.Dir(path)) }

// One line per unit: recording a decision is a one-line diff, not a rewrite of
// the whole project.
func TestCommitted_IsOneLinePerUnit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "units")
	units := []state.UnitState{unit("u1", "d-intro", "Alpha"), unit("u2", "d-intro", "Bravo")}
	require.NoError(t, state.WriteCommitted(dir, units))

	body := string(readShardBytes(t, dir, "d-intro.jsonl"))
	assert.Equal(t, 2, strings.Count(body, "\n"))
	assert.NotContains(t, body, "\n  ", "no indentation — a line is a record")
}

// Byte-stable output, so a shard's diff reflects a real change rather than map
// iteration order.
func TestCommitted_IsDeterministic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "units")
	units := []state.UnitState{unit("u2", "d-intro", "Bravo"), unit("u1", "d-intro", "Alpha")}

	require.NoError(t, state.WriteCommitted(dir, units))
	first := readShardBytes(t, dir, "d-intro.jsonl")

	// Same units, opposite order in.
	require.NoError(t, state.WriteCommitted(dir, []state.UnitState{units[1], units[0]}))
	assert.Equal(t, first, readShardBytes(t, dir, "d-intro.jsonl"))
}

func TestCommitted_RoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "units")
	in := []state.UnitState{unit("u1", "d-intro", "Alpha"), unit("u2", "d-guide", "Bravo")}
	require.NoError(t, state.WriteCommitted(dir, in))

	out, err := state.ReadCommitted(dir)
	require.NoError(t, err)
	require.Len(t, out, 2)

	// Shards are read in name order, so the result is grouped by document rather
	// than in the order units were handed in. Compare as a set.
	got := map[string]string{}
	for _, u := range out {
		got[u.Unit] = u.ContentHash
	}
	assert.Equal(t, in[0].ContentHash, got[in[0].Unit])
	assert.Equal(t, in[1].ContentHash, got[in[1].Unit])
}

// A project with no state yet is not an error.
func TestCommitted_MissingDirectoryIsEmpty(t *testing.T) {
	out, err := state.ReadCommitted(filepath.Join(t.TempDir(), "nope"))
	require.NoError(t, err)
	assert.Empty(t, out)
}

// State is authoritative: a line that cannot be parsed must stop the run, never
// be skipped, or a decision would vanish silently.
func TestCommitted_MalformedLineIsAnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "units")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "d-intro.jsonl"),
		[]byte("{\"unit\":\"u1\"}\nnot json\n"), 0o644))

	_, err := state.ReadCommitted(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
}
