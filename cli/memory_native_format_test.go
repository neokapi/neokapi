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

// TestResolveMemoryImportFormat pins the identify-or-fail contract: `auto`
// names the format only when the extension actually says what the file is.
// Guessing is what makes an unreadable file report "Imported 0 entries".
func TestResolveMemoryImportFormat(t *testing.T) {
	for _, tt := range []struct {
		flag, path, want string
	}{
		{"auto", "seeds/builtins-nb.memory.json", "bundle"},
		{"auto", "memory.json", "bundle"}, // the conventional bare name
		{"auto", ".kapi/memory.json", "bundle"},
		{"auto", "corpus.tmx", "tmx"},
		{"auto", "corpus.tmx.gz", "tmx"},
		{"auto", "CORPUS.TMX", "tmx"},
		// An explicit --format always wins, extension notwithstanding.
		{"bundle", "anything.xml", "bundle"},
		{"TMX", "x.memory.json", "tmx"},
	} {
		got, err := ResolveMemoryImportFormat(tt.flag, tt.path)
		require.NoError(t, err, "%s + %s", tt.flag, tt.path)
		assert.Equal(t, tt.want, got)
	}

	// An extension that says nothing is an error, not a guess. A plain .json
	// is one: the compound suffix is what identifies a bundle.
	for _, path := range []string{"messages.json", "notes.txt", "archive.zip"} {
		_, err := ResolveMemoryImportFormat("auto", path)
		require.Error(t, err, "%s must not be guessed at", path)
		assert.Contains(t, err.Error(), filepath.Base(path), "the error must name the file")
		assert.Contains(t, err.Error(), ".memory.json", "the error must name what is supported")
		assert.Contains(t, err.Error(), "--format", "the error must name the way out")
	}
}

// TestResolveMemoryExportFormat pins the other direction: an output path
// expresses intent rather than identity, so `auto` may fall back to the
// interchange default — stdout has no extension at all.
func TestResolveMemoryExportFormat(t *testing.T) {
	assert.Equal(t, "bundle", ResolveMemoryExportFormat("auto", "seeds/builtins-nb.memory.json"))
	assert.Equal(t, "bundle", ResolveMemoryExportFormat("auto", "memory.json"))
	assert.Equal(t, "tmx", ResolveMemoryExportFormat("auto", "corpus.tmx"))
	assert.Equal(t, "tmx", ResolveMemoryExportFormat("", ""), "stdout takes the default")
	assert.Equal(t, "tmx", ResolveMemoryExportFormat("auto", "messages.json"))
	assert.Equal(t, "bundle", ResolveMemoryExportFormat("bundle", "anything.xml"))
	assert.Equal(t, "tmx", ResolveMemoryExportFormat("TMX", "x.memory.json"))
}

// TestResolveTermsImportFormat is the terms half of the identify-or-fail
// contract. Guessing bites harder here than on the memory side: the CSV reader
// skips rows it cannot use rather than failing, so an unidentified file imports
// as zero concepts and exits 0.
func TestResolveTermsImportFormat(t *testing.T) {
	for _, tt := range []struct {
		flag, path, want string
	}{
		{"auto", "seeds/product.terms.json", "bundle"},
		{"auto", "terms.json", "bundle"}, // the conventional bare name
		{"auto", ".kapi/terms.json", "bundle"},
		{"auto", "product.csv", "csv"},
		{"auto", "product.tsv", "tsv"},
		{"auto", "vocab.json", "json"},
		{"auto", "terms.tbx", "tbx"},
		{"auto", "TERMS.TBX", "tbx"},
		// An explicit --format always wins, even over a native bundle path.
		{"tbx", "seeds/product.terms.json", "tbx"},
		{"CSV", "out.terms.json", "csv"},
	} {
		got, err := ResolveTermsImportFormat(tt.flag, tt.path)
		require.NoError(t, err, "%s + %s", tt.flag, tt.path)
		assert.Equal(t, tt.want, got)
	}

	for _, path := range []string{"notes.txt", "archive.zip", "noext"} {
		_, err := ResolveTermsImportFormat("auto", path)
		require.Error(t, err, "%s must not be guessed at", path)
		assert.Contains(t, err.Error(), filepath.Base(path), "the error must name the file")
		assert.Contains(t, err.Error(), ".terms.json", "the error must name what is supported")
		assert.Contains(t, err.Error(), "--format", "the error must name the way out")
	}
}

// TestResolveTermsExportFormat pins the export direction: the caller's
// --format is the default, and a native bundle path overrides it so
// `kapi terms export -o seeds/terms.json` writes the lossless bundle rather
// than the lossy JSON interchange doc.
func TestResolveTermsExportFormat(t *testing.T) {
	assert.Equal(t, "bundle", ResolveTermsExportFormat("json", "seeds/product.terms.json", false))
	assert.Equal(t, "bundle", ResolveTermsExportFormat("json", "terms.json", false), "the conventional bare name is a bundle")
	assert.Equal(t, "bundle", ResolveTermsExportFormat("json", ".kapi/terms.json", false))
	assert.Equal(t, "csv", ResolveTermsExportFormat("csv", "product.csv", false))
	assert.Equal(t, "json", ResolveTermsExportFormat("json", "", false), "stdout takes the default")
	// A plain .json path is not a bundle: the compound suffix is what
	// identifies one, and `--format json` is a different, lossy serialization.
	assert.Equal(t, "json", ResolveTermsExportFormat("json", "vocab.json", false))
	// Explicit --format always wins, even over a native bundle path.
	assert.Equal(t, "tbx", ResolveTermsExportFormat("tbx", "seeds/product.terms.json", true))
	assert.Equal(t, "csv", ResolveTermsExportFormat("CSV", "out.terms.json", true))
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

// TestUnidentifiedImportFailsRatherThanReportingSuccess is the honest-failure
// contract at the command surface, and it matters more than it looks.
//
// Neither importer fails loudly on a file it cannot identify if it is allowed
// to guess. Guessing TMX finds no translation units and reports "Imported 0
// entries" — indistinguishable from an empty content memory. Guessing CSV is
// worse: the CSV reader skips every row it cannot turn into a concept, so a zip
// archive reports "Imported 0 concepts" and exits 0. Both are success-shaped
// output for a file that was never read.
func TestUnidentifiedImportFailsRatherThanReportingSuccess(t *testing.T) {
	dir := t.TempDir()
	a := &App{Quiet: true}

	for _, tt := range []struct {
		name, file, body string
		newCmd           func() *cobra.Command
	}{
		{"memory: json that is not a bundle", "seed.dat", `{"schemaVersion":"1.0","entries":[]}`,
			func() *cobra.Command { return newMemoryImportCmd(a) }},
		{"memory: binary", "seed.zip", "PK\x03\x04rubbish",
			func() *cobra.Command { return newMemoryImportCmd(a) }},
		{"memory: plain json", "messages.json", `{"hello":"world"}`,
			func() *cobra.Command { return newMemoryImportCmd(a) }},
		// Quote-free, comma-free prose is *valid* CSV — one column per row —
		// so nothing downstream can catch it. Only refusing to guess does.
		{"terms: prose", "product.dat", "hello world\nsecond line\n",
			func() *cobra.Command { return newTermsImportCmd(a) }},
		{"terms: binary", "product.zip", "PK\x03\x04rubbish",
			func() *cobra.Command { return newTermsImportCmd(a) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o644))

			cmd := tt.newCmd()
			AddResourceFlags(cmd)
			require.NoError(t, cmd.Flags().Set("file", filepath.Join(dir, tt.file+".db")))
			cmd.SetContext(t.Context())

			err := cmd.RunE(cmd, []string{path})
			require.Error(t, err, "an unidentified file must fail, not import zero rows")
			assert.Contains(t, err.Error(), tt.file, "the error must name the file")
			assert.Contains(t, err.Error(), "--format", "the error must name the way out")
		})
	}
}

// TestIdentifiableImportIsNotRejected pins the other half: every extension the
// commands do understand must still import with no --format at all.
func TestIdentifiableImportIsNotRejected(t *testing.T) {
	for _, path := range []string{
		"seeds/cli-nb.memory.json", "memory.json", ".kapi/memory.json", "corpus.tmx", "corpus.tmx.gz",
	} {
		_, err := ResolveMemoryImportFormat("auto", path)
		require.NoError(t, err, "%s names a format the memory importer reads", path)
	}
	for _, path := range []string{
		"seeds/product.terms.json", "terms.json", ".kapi/terms.json",
		"product.csv", "product.tsv", "vocab.json", "terms.tbx",
	} {
		_, err := ResolveTermsImportFormat("auto", path)
		require.NoError(t, err, "%s names a format the terms importer reads", path)
	}
}
