package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMemoryFileFormat(t *testing.T) {
	assert.Equal(t, "kmb", ResolveMemoryFileFormat("auto", "seeds/builtins-nb.kmb"))
	assert.Equal(t, "kmz", ResolveMemoryFileFormat("auto", "seeds/builtins-nb.kmz"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("auto", "corpus.tmx"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("auto", "corpus.tmx.gz"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("", ""))
	assert.Equal(t, "kmb", ResolveMemoryFileFormat("kmb", "anything.xml"))
	assert.Equal(t, "kmz", ResolveMemoryFileFormat("kmz", "anything.xml"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("TMX", "x.kmb"))
}

func TestResolveTermsFileFormat(t *testing.T) {
	// Not explicit: a native extension wins over the caller's default.
	assert.Equal(t, "ktb", ResolveTermsFileFormat("csv", "seeds/termbase.ktb", false))
	assert.Equal(t, "ktz", ResolveTermsFileFormat("csv", "seeds/terms.ktz", false))
	assert.Equal(t, "csv", ResolveTermsFileFormat("csv", "glossary.csv", false))
	assert.Equal(t, "json", ResolveTermsFileFormat("json", "", false))
	// Explicit --format always wins, even over a native extension.
	assert.Equal(t, "tbx", ResolveTermsFileFormat("tbx", "seeds/termbase.ktb", true))
	assert.Equal(t, "ktz", ResolveTermsFileFormat("KTZ", "out.bin", true))
}

// runMemoryExport drives the real `kapi memory export` RunE against the given db.
func runMemoryExport(t *testing.T, dbPath, outPath, format string) {
	t.Helper()
	a := &App{Quiet: true}
	cmd := newMemoryExportCmd(a)
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	require.NoError(t, cmd.Flags().Set("output", outPath))
	if format != "" {
		require.NoError(t, cmd.Flags().Set("format", format))
	}
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.RunE(cmd, nil))
}

// runMemoryImport drives the real `kapi memory import` RunE against the given db.
func runMemoryImport(t *testing.T, dbPath, inPath string) {
	t.Helper()
	a := &App{Quiet: true}
	cmd := newMemoryImportCmd(a)
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.RunE(cmd, []string{inPath}))
}

// TestMemoryKMBRoundTrip proves the native-seed contract: TMX import → kmb
// export → import into a FRESH db → kmb export is byte-identical, because
// kmb preserves entry identity (unlike TMX import, which mints new ids and
// sessions on every run).
func TestMemoryKMBRoundTrip(t *testing.T) {
	dbPath := runSingleTMXImport(t)
	dir := filepath.Dir(dbPath)

	seed := filepath.Join(dir, "seed.kmb")
	runMemoryExport(t, dbPath, seed, "") // auto: .kmb extension selects kmb

	data, err := os.ReadFile(seed)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind"`, "kmb envelope expected")
	require.Contains(t, string(data), "tableau de bord")

	// Fresh db seeded from the kmb must round-trip byte-identically.
	db2 := filepath.Join(dir, "tm2.db")
	runMemoryImport(t, db2, seed)
	seed2 := filepath.Join(dir, "seed2.kmb")
	runMemoryExport(t, db2, seed2, "kmb")

	data2, err := os.ReadFile(seed2)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(data2), "kmb reseed must be byte-identical")

	// And the reseeded content memory must serve lookups exactly like the original.
	tm, err := memory.NewSQLiteStore(db2)
	require.NoError(t, err)
	defer tm.Close()
	entries, err := tm.Entries(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Welcome to the dashboard", entries[0].VariantText("en"))
}

// TestMemoryKMZRoundTrip proves the archive tier carries the same content as the
// bundle tier: a .kmz export reseeds a fresh content memory, and re-exporting that content memory as a
// .kmb yields the same bytes the bundle export produced.
func TestMemoryKMZRoundTrip(t *testing.T) {
	dbPath := runSingleTMXImport(t)
	dir := filepath.Dir(dbPath)

	bundle := filepath.Join(dir, "seed.kmb")
	runMemoryExport(t, dbPath, bundle, "")
	archive := filepath.Join(dir, "seed.kmz")
	runMemoryExport(t, dbPath, archive, "") // auto: .kmz extension selects kmz

	bundleData, err := os.ReadFile(bundle)
	require.NoError(t, err)
	archiveData, err := os.ReadFile(archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("PK"), archiveData[:2], "a .kmz is a zip container")
	assert.Less(t, len(archiveData), len(bundleData), "the archive compresses the bundle")

	// Reseed a fresh db from the archive (auto-detected on import).
	db2 := filepath.Join(dir, "tm2.db")
	runMemoryImport(t, db2, archive)
	reexport := filepath.Join(dir, "seed2.kmb")
	runMemoryExport(t, db2, reexport, "kmb")

	reexportData, err := os.ReadFile(reexport)
	require.NoError(t, err)
	assert.Equal(t, string(bundleData), string(reexportData),
		"a .kmz reseed must reproduce the same content memory state as the .kmb it wraps")
}

// TestTermsKTZRoundTrip mirrors TestMemoryKMZRoundTrip for the terms store.
func TestTermsKTZRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "terms.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte(
		"source_term,target_term,domain,definition,status\n"+
			"flow,flyt,content,A composed chain of tools.,preferred\n"), 0o644))

	a := &App{Quiet: true}
	dbPath := filepath.Join(dir, "tb.db")
	imp := newTermsImportCmd(a)
	AddResourceFlags(imp)
	require.NoError(t, imp.Flags().Set("file", dbPath))
	require.NoError(t, imp.Flags().Set("source-locale", "en"))
	require.NoError(t, imp.Flags().Set("target-locale", "nb"))
	require.NoError(t, imp.Flags().Set("header", "true"))
	imp.SetContext(ctx)
	require.NoError(t, imp.RunE(imp, []string{csvPath}))

	// Export the archive tier (.ktz auto-detected from the -o extension).
	exp := newTermsExportCmd(a)
	AddResourceFlags(exp)
	require.NoError(t, exp.Flags().Set("file", dbPath))
	archive := filepath.Join(dir, "terms.ktz")
	require.NoError(t, exp.Flags().Set("output", archive))
	exp.SetContext(ctx)
	require.NoError(t, exp.RunE(exp, nil))

	archiveData, err := os.ReadFile(archive)
	require.NoError(t, err)
	assert.Equal(t, []byte("PK"), archiveData[:2], "a .ktz is a zip container")
	parsed, err := ktb.UnmarshalArchive(archiveData)
	require.NoError(t, err)
	require.Len(t, parsed.Concepts, 1)

	// Fresh terms seeded from the archive (auto-detected on import).
	db2 := filepath.Join(dir, "tb2.db")
	imp2 := newTermsImportCmd(a)
	AddResourceFlags(imp2)
	require.NoError(t, imp2.Flags().Set("file", db2))
	imp2.SetContext(ctx)
	require.NoError(t, imp2.RunE(imp2, []string{archive}))

	tb2, err := terms.NewSQLiteStore(db2)
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

// TestTermsKTBRoundTrip mirrors the contract for the terms store: CSV
// import → ktb export → import into a fresh db → identical concepts.
func TestTermsKTBRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	csvPath := filepath.Join(dir, "terms.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte(
		"source_term,target_term,domain,definition,status\n"+
			"flow,flyt,localization,A composed chain of tools.,preferred\n"), 0o644))

	dbPath := filepath.Join(dir, "tb.db")
	a := &App{Quiet: true}
	imp := newTermsImportCmd(a)
	AddResourceFlags(imp)
	require.NoError(t, imp.Flags().Set("file", dbPath))
	require.NoError(t, imp.Flags().Set("source-locale", "en"))
	require.NoError(t, imp.Flags().Set("target-locale", "nb"))
	require.NoError(t, imp.Flags().Set("header", "true"))
	imp.SetContext(ctx)
	require.NoError(t, imp.RunE(imp, []string{csvPath}))

	// Export native (.ktb auto-detected from the -o extension).
	exp := newTermsExportCmd(a)
	AddResourceFlags(exp)
	require.NoError(t, exp.Flags().Set("file", dbPath))
	seed := filepath.Join(dir, "termbase.ktb")
	require.NoError(t, exp.Flags().Set("output", seed))
	exp.SetContext(ctx)
	require.NoError(t, exp.RunE(exp, nil))

	data, err := os.ReadFile(seed)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), "flyt"))
	parsed, err := ktb.Unmarshal(data)
	require.NoError(t, err)
	require.Len(t, parsed.Concepts, 1)

	// Fresh terms seeded from the ktb (auto-detected on import).
	db2 := filepath.Join(dir, "tb2.db")
	imp2 := newTermsImportCmd(a)
	AddResourceFlags(imp2)
	require.NoError(t, imp2.Flags().Set("file", db2))
	imp2.SetContext(ctx)
	require.NoError(t, imp2.RunE(imp2, []string{seed}))

	tb2, err := terms.NewSQLiteStore(db2)
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
