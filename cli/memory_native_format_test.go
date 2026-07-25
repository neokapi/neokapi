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
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMemoryFileFormat(t *testing.T) {
	assert.Equal(t, "bundle", ResolveMemoryFileFormat("auto", "seeds/builtins-nb.memory.json"))
	assert.Equal(t, "bundle", ResolveMemoryFileFormat("auto", "memory.json"), "the conventional bare name is a bundle")
	assert.Equal(t, "bundle", ResolveMemoryFileFormat("auto", ".kapi/memory.json"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("auto", "corpus.tmx"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("auto", "corpus.tmx.gz"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("", ""))
	// A plain .json file is not a bundle: the compound suffix is what identifies one.
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("auto", "messages.json"))
	assert.Equal(t, "bundle", ResolveMemoryFileFormat("bundle", "anything.xml"))
	assert.Equal(t, "tmx", ResolveMemoryFileFormat("TMX", "x.memory.json"))
}

func TestResolveTermsFileFormat(t *testing.T) {
	// Not explicit: a native bundle path wins over the caller's default.
	assert.Equal(t, "bundle", ResolveTermsFileFormat("csv", "seeds/glossary.terms.json", false))
	assert.Equal(t, "bundle", ResolveTermsFileFormat("csv", "terms.json", false), "the conventional bare name is a bundle")
	assert.Equal(t, "bundle", ResolveTermsFileFormat("csv", ".kapi/terms.json", false))
	assert.Equal(t, "csv", ResolveTermsFileFormat("csv", "glossary.csv", false))
	assert.Equal(t, "json", ResolveTermsFileFormat("json", "", false))
	// A plain .json file keeps the caller's default: `kapi terms export
	// --format json` is a different, lossy serialization and must not be
	// captured by the native bundle tier.
	assert.Equal(t, "csv", ResolveTermsFileFormat("csv", "vocab.json", false))
	// Explicit --format always wins, even over a native bundle path.
	assert.Equal(t, "tbx", ResolveTermsFileFormat("tbx", "seeds/glossary.terms.json", true))
	assert.Equal(t, "csv", ResolveTermsFileFormat("CSV", "out.terms.json", true))
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

	seed := filepath.Join(dir, "seed.memory.json")
	runMemoryExport(t, dbPath, seed, "") // auto: .memory.json extension selects kmb

	data, err := os.ReadFile(seed)
	require.NoError(t, err)
	require.Contains(t, string(data), `"kind"`, "kmb envelope expected")
	require.Contains(t, string(data), "tableau de bord")

	// Fresh db seeded from the kmb must round-trip byte-identically.
	db2 := filepath.Join(dir, "tm2.db")
	runMemoryImport(t, db2, seed)
	seed2 := filepath.Join(dir, "seed2.memory.json")
	runMemoryExport(t, db2, seed2, "bundle")

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

	// Export native (.terms.json auto-detected from the -o extension).
	exp := newTermsExportCmd(a)
	AddResourceFlags(exp)
	require.NoError(t, exp.Flags().Set("file", dbPath))
	seed := filepath.Join(dir, "terms.json")
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

// TestRetiredBundlePathIsRejectedWithGuidance is the self-explaining-failure
// contract at the command surface.
//
// It matters more than it looks. Neither importer *fails* on a retired
// extension without the guard: `.kmb` is no longer registered, so the memory
// importer's TMX fallback claims the file and reports "Imported 0 entries" — a
// silent no-op indistinguishable from an empty content memory — while the terms
// importer's CSV fallback reports a column-parse error from inside a CSV reader.
// Both are worse than an error.
func TestRetiredBundlePathIsRejectedWithGuidance(t *testing.T) {
	dir := t.TempDir()
	a := &App{Quiet: true}

	tests := []struct {
		name     string
		file     string
		body     string
		newCmd   func() *cobra.Command
		wantsAll []string
	}{
		{
			name:     "memory import .kmb",
			file:     "seed.kmb",
			body:     `{"schemaVersion":"1.0","kind":"kapi-memory","entries":[]}`,
			newCmd:   func() *cobra.Command { return newMemoryImportCmd(a) },
			wantsAll: []string{".kmb", ".memory.json", "kapi memory export"},
		},
		{
			name:     "memory import withdrawn .kmz",
			file:     "seed.kmz",
			body:     "PK",
			newCmd:   func() *cobra.Command { return newMemoryImportCmd(a) },
			wantsAll: []string{".kmz", "withdrawn", ".memory.json"},
		},
		{
			name:     "terms import .ktb",
			file:     "glossary.ktb",
			body:     `{"schemaVersion":"1.0","kind":"kapi-terms","concepts":[]}`,
			newCmd:   func() *cobra.Command { return newTermsImportCmd(a) },
			wantsAll: []string{".ktb", ".terms.json", "kapi terms export"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))

			cmd := tt.newCmd()
			AddResourceFlags(cmd)
			require.NoError(t, cmd.Flags().Set("file", filepath.Join(dir, tt.file+".db")))
			cmd.SetContext(t.Context())

			err := cmd.RunE(cmd, []string{path})
			require.Error(t, err, "a retired extension must be rejected, not silently mis-parsed")
			for _, want := range tt.wantsAll {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// TestLiveBundlePathIsNotRejected pins the other half: the guard must reject
// only retired spellings, never the conventions that replaced them.
func TestLiveBundlePathIsNotRejected(t *testing.T) {
	for _, path := range []string{
		"seeds/cli-nb.memory.json", "memory.json", ".kapi/terms.json",
		"seeds/glossary.terms.json", "corpus.tmx", "glossary.csv", "app.kbf.json",
	} {
		assert.NoError(t, CheckRetiredBundlePath(path), "%s is a live path", path)
	}
}
