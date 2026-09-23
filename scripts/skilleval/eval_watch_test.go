package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The watcher keeps the first version of each file an agent saved, whatever it
// did to the file afterwards, and never mistakes wiring for work.
func TestEvalWatcherKeepsTheFirstSavedVersion(t *testing.T) {
	repo := t.TempDir()
	write := func(name, body string) {
		path := filepath.Join(repo, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("docs/a.md", "# A\n")
	write("docs/gone.md", "# Gone\n")
	write(".kapi/work/store.db", "state")
	watcher, err := newEvalWatcher(repo)
	require.NoError(t, err)

	write("docs/new.md", "first draft!\n")
	write(".kapi/work/store.db", "changed state")
	watcher.poll()
	write("docs/new.md", "final draft.\n")
	write("docs/a.md", "# A\n\nAdded.\n")
	require.NoError(t, os.Remove(filepath.Join(repo, "docs", "gone.md")))
	watcher.poll()
	write("docs/a.md", "# A\n\nAdded, then edited.\n")

	names, baseline, first, final, err := watcher.versions()
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/a.md", "docs/gone.md", "docs/new.md"}, names, "state under .kapi is never a version")
	assert.Equal(t, "first draft!\n", string(first["docs/new.md"]))
	assert.Equal(t, "final draft.\n", string(final["docs/new.md"]))
	assert.Equal(t, "# A\n\nAdded.\n", string(first["docs/a.md"]))
	assert.Equal(t, "# A\n\nAdded, then edited.\n", string(final["docs/a.md"]))
	assert.Nil(t, first["docs/gone.md"], "a removed file's first version is its absence")
	assert.Equal(t, "Added.\nfirst draft!", evalAddedAcross(names, baseline, first))
	assert.Equal(t, "final draft.", evalAddedText(string(baseline["docs/new.md"]), string(final["docs/new.md"])))
}

func TestEvalToolCoverage(t *testing.T) {
	missing, unclassified := evalToolCoverage([]string{"context_observe", "context_search", "future_tool"})
	assert.Equal(t, []string{evalKindCheck}, missing, "no tool the reader counts as a check")
	assert.Equal(t, []string{"future_tool"}, unclassified)

	missing, unclassified = evalToolCoverage([]string{"context_read", "check_file", "context_withdraw"})
	assert.Empty(t, missing)
	assert.Empty(t, unclassified)
}
