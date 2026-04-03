//go:build integration

package sievepen_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"

	pgtm "github.com/neokapi/neokapi/bowrain/sievepen"
	storage "github.com/neokapi/neokapi/bowrain/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgresTM(t *testing.T) *pgtm.PostgresTM {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	// Use a unique workspace per test to isolate.
	wsID := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
	tm, err := pgtm.NewPostgresTMFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Clean up entries for this workspace.
		db.Exec("DELETE FROM tm_entries WHERE workspace_id = $1", wsID)
		db.Close()
	})
	return tm
}

func TestPostgresTM_IntegrationAddAndLookup(t *testing.T) {
	tm := openTestPostgresTM(t)

	entry := sievepen.TMEntry{
		ID:           "entry-1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Properties:   map[string]string{"domain": "general"},
	}

	err := tm.Add(entry)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestPostgresTM_IntegrationExactMatch(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Save"), Target: model.NewFragment("Sauvegarder"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Cancel"), Target: model.NewFragment("Annuler"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("Save", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Sauvegarder", matches[0].Entry.TargetText())
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestPostgresTM_IntegrationFuzzyMatch(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("The file was saved successfully"),
		Target:       model.NewFragment("Le fichier a ete sauvegarde avec succes"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("The file was saved", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 0.5, MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, sievepen.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

func TestPostgresTM_IntegrationDelete(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	assert.Equal(t, 2, tm.Count())

	err := tm.Delete("e1")
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = tm.LookupText("Goodbye", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.TargetText())

	err = tm.Delete("non-existent")
	assert.Error(t, err)
}

func TestPostgresTM_IntegrationEmptyIDError(t *testing.T) {
	tm := openTestPostgresTM(t)

	err := tm.Add(sievepen.TMEntry{
		Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestPostgresTM_IntegrationUpdateExisting(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	// Update with same ID.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Salut"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	assert.Equal(t, 1, tm.Count())
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Salut", matches[0].Entry.TargetText())
}

func TestPostgresTM_IntegrationLocaleFiltering(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleGerman,
	}))

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())

	matches, err = tm.LookupText("Hello", model.LocaleEnglish, model.LocaleGerman, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Hallo", matches[0].Entry.TargetText())
}

func TestPostgresTM_IntegrationEntries(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
		Properties: map[string]string{"domain": "general"},
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entries := tm.Entries()
	assert.Len(t, entries, 2)
}

func TestPostgresTM_IntegrationSearchEntries(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello World"), Target: model.NewFragment("Bonjour le monde"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e3", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleGerman,
	}))

	// No filter returns all entries
	entries, total := tm.SearchEntries("", "", "", 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 3)

	// Search by query (case-insensitive, matches source)
	entries, total = tm.SearchEntries("hello", "", "", 0, 100)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)

	// Search by query matches target
	entries, total = tm.SearchEntries("revoir", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Equal(t, "e2", entries[0].ID)

	// Filter by target locale
	entries, total = tm.SearchEntries("", "", "de", 0, 100)
	assert.Equal(t, 1, total)
	assert.Equal(t, "e3", entries[0].ID)

	// Pagination
	entries, total = tm.SearchEntries("", "", "", 0, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 2)

	entries, total = tm.SearchEntries("", "", "", 2, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 1)
}

func TestPostgresTM_IntegrationGetEntry(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entry, ok := tm.GetEntry("e1")
	assert.True(t, ok)
	assert.Equal(t, "Hello", entry.SourceText())
	assert.Equal(t, "Bonjour", entry.TargetText())

	_, ok = tm.GetEntry("nonexistent")
	assert.False(t, ok)
}

func TestPostgresTM_IntegrationInterfaceCompliance(t *testing.T) {
	tm := openTestPostgresTM(t)

	var _ sievepen.TranslationMemory = tm
	var _ sievepen.EntryProvider = tm
	var _ pgtm.TMStore = tm
}

func TestPostgresTM_IntegrationAddWithStream(t *testing.T) {
	tm := openTestPostgresTM(t)

	mainEntry := sievepen.TMEntry{
		ID:           "main-1",
		Source:       model.NewFragment("Hello world"),
		Target:       model.NewFragment("Hallo Welt"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.AddWithStream(mainEntry, "main"))

	featureEntry := sievepen.TMEntry{
		ID:           "feat-1",
		Source:       model.NewFragment("Hello world"),
		Target:       model.NewFragment("Hallo Welt (Rebrand)"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.AddWithStream(featureEntry, "feature/rebrand"))

	workspaceEntry := sievepen.TMEntry{
		ID:           "ws-1",
		Source:       model.NewFragment("Goodbye"),
		Target:       model.NewFragment("Auf Wiedersehen"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.Add(workspaceEntry))

	entries, total := tm.SearchEntriesForStream("", "en-US", "de-DE",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 3)
	assert.Equal(t, "feat-1", entries[0].ID)

	entries, total = tm.SearchEntriesForStream("", "en-US", "de-DE", "", nil, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "ws-1", entries[0].ID)

	entries, total = tm.SearchEntriesForStream("goodbye", "en-US", "de-DE",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "ws-1", entries[0].ID)
}

func TestPostgresTM_IntegrationBlockLookup(t *testing.T) {
	tm := openTestPostgresTM(t)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Click the Save button"),
		Target:       model.NewFragment("Cliquez sur le bouton Enregistrer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	block := model.NewBlock("tu1", "Click the Save button")
	matches, err := tm.Lookup(block, model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, "Cliquez sur le bouton Enregistrer", matches[0].Entry.TargetText())
}

func TestPostgresTM_IntegrationFragmentRoundtrip(t *testing.T) {
	tm := openTestPostgresTM(t)

	frag := model.NewFragment("Click ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, ID: "1", Type: "bold"})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, ID: "1", Type: "bold"})
	frag.AppendText(" to continue")

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       frag,
		Target:       model.NewFragment("Cliquez ici pour continuer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	entry, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.Equal(t, "Click here to continue", entry.SourceText())
	assert.True(t, entry.Source.HasSpans())
	assert.Len(t, entry.Source.Spans, 2)
}

func TestPostgresTM_IntegrationTimestampPreservation(t *testing.T) {
	tm := openTestPostgresTM(t)

	now := time.Now().Truncate(time.Millisecond)
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
		CreatedAt: now, UpdatedAt: now,
	}))

	entry, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.WithinDuration(t, now, entry.CreatedAt, time.Second)
	assert.WithinDuration(t, now, entry.UpdatedAt, time.Second)
}
