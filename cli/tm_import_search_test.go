package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// singleTMX is a minimal valid TMX 1.4 document with one en→fr TU whose source
// text is distinctive enough to search for.
const singleTMX = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="t" creationtoolversion="1" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu>
      <tuv xml:lang="en"><seg>Welcome to the dashboard</seg></tuv>
      <tuv xml:lang="fr"><seg>Bienvenue sur le tableau de bord</seg></tuv>
    </tu>
  </body>
</tmx>`

// runSingleTMXImport writes singleTMX to a temp file, then drives the real
// `kapi tm import <file>` RunE (with --file pointing at a fresh SQLite TM).
// It returns the resolved TM db path so callers can reopen and assert.
func runSingleTMXImport(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tmxPath := filepath.Join(dir, "corpus.tmx")
	require.NoError(t, os.WriteFile(tmxPath, []byte(singleTMX), 0o644))

	dbPath := filepath.Join(dir, "tm.db")

	a := &App{Quiet: true}
	cmd := a.newTMImportCmd()
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	require.NoError(t, cmd.Flags().Set("source-locale", "en"))
	require.NoError(t, cmd.Flags().Set("target-locale", "fr"))

	require.NoError(t, cmd.RunE(cmd, []string{tmxPath}))
	return dbPath
}

// TestTMImportSingleFile_RebuildsSearchIndex is the regression test for
// finding #38: a single-file `kapi tm import` must rebuild the FTS5 side-tables
// so the imported entry is visible to `kapi tm search` (SearchEntries), not
// just to exact lookup. Before the fix, the bulk add path left
// tm_variant_search / tm_variant_trigram empty and SearchEntries returned zero.
func TestTMImportSingleFile_RebuildsSearchIndex(t *testing.T) {
	dbPath := runSingleTMXImport(t)

	tm, err := sievepen.NewSQLiteTM(dbPath)
	require.NoError(t, err)
	defer tm.Close()

	require.Equal(t, 1, tm.Count(), "exactly one TU should be imported")

	// FTS5-backed search — the surface `kapi tm search` exercises.
	entries, total := tm.SearchEntries("dashboard", "en", "fr", 0, 25)
	require.Positive(t, total, "FTS search must find the imported entry after single-file import")
	require.NotEmpty(t, entries, "search must return the imported entry, not an empty FTS index")
	assert.Equal(t, "Welcome to the dashboard", entries[0].VariantText("en"))
	assert.Equal(t, "Bienvenue sur le tableau de bord", entries[0].VariantText("fr"))
}

// TestTMImportSingleFile_FuzzyLookup asserts the fuzzy (trigram) index is also
// rebuilt: a near-but-not-exact query still finds the imported entry via
// LookupText, which falls back to the fuzzy index for non-exact matches.
func TestTMImportSingleFile_FuzzyLookup(t *testing.T) {
	dbPath := runSingleTMXImport(t)

	tm, err := sievepen.NewSQLiteTM(dbPath)
	require.NoError(t, err)
	defer tm.Close()

	matches, err := tm.LookupText("Welcome to the dashboard!", "en", "fr", sievepen.LookupOptions{
		MinScore:   0.5,
		MaxResults: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches, "fuzzy lookup must find the near-match after single-file import rebuilt the trigram index")
	assert.Equal(t, "Bienvenue sur le tableau de bord", matches[0].Entry.VariantText("fr"))
}
