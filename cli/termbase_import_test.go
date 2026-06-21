package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTermbaseImport_Monolingual drives `kapi termbase import --monolingual`
// over a single-locale term[, definition] CSV and asserts the concepts land as
// one-term records (no translation pair) — the path that wires
// CSVImportOptions.Monolingual through from the CLI flag.
func TestTermbaseImport_Monolingual(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "vocab.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("term,definition\nBowrain,The localization platform\non-brand,Consistent with the brand voice\n"), 0o644))

	a := &App{TBBackend: termbase.NewInMemoryTermBase()}
	cmd := a.newTermbaseImportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{csvPath, "-s", "en", "--monolingual", "--header"})
	require.NoError(t, cmd.Execute())

	ctx := context.Background()
	total, err := a.TBBackend.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "both monolingual rows should import")

	// Each concept carries a single source-locale term — no translation pair.
	concepts, err := a.TBBackend.Concepts(ctx)
	require.NoError(t, err)
	require.Len(t, concepts, 2)
	for _, c := range concepts {
		require.Len(t, c.Terms, 1, "monolingual concept %s should carry one term", c.ID)
		assert.Equal(t, model.LocaleID("en"), c.Terms[0].Locale)
	}

	// The imported term is found by its source text and keeps its definition.
	matches, err := a.TBBackend.Lookup(ctx, "Bowrain", termbase.LookupOptions{SourceLocale: model.LocaleID("en")})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "The localization platform", matches[0].Concept.Definition)
	assert.Len(t, matches[0].Concept.Terms, 1)
}

// TestTermbaseImport_BilingualUnchanged confirms the default (no --monolingual)
// CSV path still imports source/target term pairs.
func TestTermbaseImport_BilingualUnchanged(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "glossary.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("dashboard,tableau de bord\nsettings,paramètres\n"), 0o644))

	a := &App{TBBackend: termbase.NewInMemoryTermBase()}
	cmd := a.newTermbaseImportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{csvPath, "-s", "en", "-t", "fr"})
	require.NoError(t, cmd.Execute())

	ctx := context.Background()
	matches, err := a.TBBackend.Lookup(ctx, "dashboard", termbase.LookupOptions{
		SourceLocale: model.LocaleID("en"),
		TargetLocale: model.LocaleID("fr"),
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	// Bilingual concept keeps both the source and the target term.
	assert.Len(t, matches[0].Concept.Terms, 2)
}
