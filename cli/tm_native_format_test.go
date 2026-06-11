package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/klftb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTMFileFormat(t *testing.T) {
	assert.Equal(t, "klftm", resolveTMFileFormat("auto", "seeds/builtins-nb.klftm"))
	assert.Equal(t, "tmx", resolveTMFileFormat("auto", "corpus.tmx"))
	assert.Equal(t, "tmx", resolveTMFileFormat("auto", "corpus.tmx.gz"))
	assert.Equal(t, "tmx", resolveTMFileFormat("", ""))
	assert.Equal(t, "klftm", resolveTMFileFormat("klftm", "anything.xml"))
	assert.Equal(t, "tmx", resolveTMFileFormat("TMX", "x.klftm"))
}

// runTMExport drives the real `kapi tm export` RunE against the given db.
func runTMExport(t *testing.T, dbPath, outPath, format string) {
	t.Helper()
	a := &App{Quiet: true}
	cmd := a.newTMExportCmd()
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	require.NoError(t, cmd.Flags().Set("output", outPath))
	if format != "" {
		require.NoError(t, cmd.Flags().Set("format", format))
	}
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.RunE(cmd, nil))
}

// runTMImport drives the real `kapi tm import` RunE against the given db.
func runTMImport(t *testing.T, dbPath, inPath string) {
	t.Helper()
	a := &App{Quiet: true}
	cmd := a.newTMImportCmd()
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.RunE(cmd, []string{inPath}))
}

// TestTMKLFTMRoundTrip proves the native-seed contract: TMX import → klftm
// export → import into a FRESH db → klftm export is byte-identical, because
// klftm preserves entry identity (unlike TMX import, which mints new ids and
// sessions on every run).
func TestTMKLFTMRoundTrip(t *testing.T) {
	dbPath := runSingleTMXImport(t)
	dir := filepath.Dir(dbPath)

	seed := filepath.Join(dir, "seed.klftm")
	runTMExport(t, dbPath, seed, "") // auto: .klftm extension selects klftm

	data, err := os.ReadFile(seed)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind"`, "klftm envelope expected")
	require.Contains(t, string(data), "tableau de bord")

	// Fresh db seeded from the klftm must round-trip byte-identically.
	db2 := filepath.Join(dir, "tm2.db")
	runTMImport(t, db2, seed)
	seed2 := filepath.Join(dir, "seed2.klftm")
	runTMExport(t, db2, seed2, "klftm")

	data2, err := os.ReadFile(seed2)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(data2), "klftm reseed must be byte-identical")

	// And the reseeded TM must serve lookups exactly like the original.
	tm, err := sievepen.NewSQLiteTM(db2)
	require.NoError(t, err)
	defer tm.Close()
	entries, err := tm.Entries(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Welcome to the dashboard", entries[0].VariantText("en"))
}

// TestTermbaseKLFTBRoundTrip mirrors the contract for the termbase: CSV
// import → klftb export → import into a fresh db → identical concepts.
func TestTermbaseKLFTBRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "terms.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte(
		"source_term,target_term,domain,definition,status\n"+
			"flow,flyt,localization,A composed chain of tools.,preferred\n"), 0o644))

	dbPath := filepath.Join(dir, "tb.db")
	a := &App{Quiet: true}
	imp := a.newTermbaseImportCmd()
	AddResourceFlags(imp)
	require.NoError(t, imp.Flags().Set("file", dbPath))
	require.NoError(t, imp.Flags().Set("source-locale", "en"))
	require.NoError(t, imp.Flags().Set("target-locale", "nb"))
	require.NoError(t, imp.Flags().Set("header", "true"))
	imp.SetContext(ctx)
	require.NoError(t, imp.RunE(imp, []string{csvPath}))

	// Export native (.klftb auto-detected from the -o extension).
	exp := a.newTermbaseExportCmd()
	AddResourceFlags(exp)
	require.NoError(t, exp.Flags().Set("file", dbPath))
	seed := filepath.Join(dir, "terms.klftb")
	require.NoError(t, exp.Flags().Set("output", seed))
	exp.SetContext(ctx)
	require.NoError(t, exp.RunE(exp, nil))

	data, err := os.ReadFile(seed)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), "flyt"))
	parsed, err := klftb.Unmarshal(data)
	require.NoError(t, err)
	require.Len(t, parsed.Concepts, 1)

	// Fresh termbase seeded from the klftb (auto-detected on import).
	db2 := filepath.Join(dir, "tb2.db")
	imp2 := a.newTermbaseImportCmd()
	AddResourceFlags(imp2)
	require.NoError(t, imp2.Flags().Set("file", db2))
	imp2.SetContext(ctx)
	require.NoError(t, imp2.RunE(imp2, []string{seed}))

	tb2, err := termbase.NewSQLiteTermBase(db2)
	require.NoError(t, err)
	defer tb2.Close()
	concepts, err := tb2.Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.Equal(t, parsed.Concepts[0].ID, concepts[0].ID, "concept identity preserved")
	nbTerm := concepts[0].PreferredTerm("nb")
	require.NotNil(t, nbTerm)
	assert.Equal(t, "flyt", nbTerm.Text)
}
