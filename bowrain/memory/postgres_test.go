//go:build integration

package memory_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"

	pgmemory "github.com/neokapi/neokapi/bowrain/memory"
	storage "github.com/neokapi/neokapi/bowrain/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestPostgresMemory(t *testing.T) *pgmemory.PostgresStore {
	t.Helper()
	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	wsID := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
	tm, err := pgmemory.NewPostgresStoreFromDB(db, wsID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Exec("DELETE FROM tm_entries WHERE workspace_id = $1", wsID)
		db.Exec("DELETE FROM tm_import_sessions WHERE workspace_id = $1", wsID)
		db.Close()
	})
	return tm
}

func trilingual(id, en, fr, de string) memory.Entry {
	return memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: en}}},
			"fr": {{Text: &model.TextRun{Text: fr}}},
			"de": {{Text: &model.TextRun{Text: de}}},
		},
		HintSrcLang: "en",
	}
}

func TestPostgresMemory_MultilingualAddAndLookup(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.Add(t.Context(), trilingual("e1", "Hello", "Bonjour", "Hallo")))
	count, err := tm.Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	matches, err := tm.LookupText(t.Context(), "Hello", "en", "fr", memory.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.VariantText("fr"))
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, memory.MatchExact, matches[0].MatchType)
}

func TestPostgresMemory_LookupCrossDirection(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.Add(t.Context(), trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText(t.Context(), "Enregistrer", "fr", "de", memory.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Speichern", matches[0].Entry.VariantText("de"))
}

func TestPostgresMemory_SearchRequireLocale(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
			"fr": {{Text: &model.TextRun{Text: "bonjour"}}},
		},
	}))
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
		},
	}))
	entries, total, err := tm.SearchEntries(t.Context(), memory.SearchParams{Query: "hello", AnyLocale: "en", RequireLocale: "fr", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestPostgresMemory_FacetLocales(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.Add(t.Context(), trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "d"}}},
			"fr": {{Text: &model.TextRun{Text: "e"}}},
		},
	}))
	f, err := tm.FacetStats(t.Context())
	require.NoError(t, err)
	counts := map[string]int{}
	for _, lf := range f.Locales {
		counts[lf.Locale] = lf.Count
	}
	assert.Equal(t, 2, counts["en"])
	assert.Equal(t, 2, counts["fr"])
	assert.Equal(t, 1, counts["de"])
}

func TestPostgresMemory_ImportSessionCRUD(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.CreateImportSession(t.Context(), memory.ImportSession{
		ID: "s1", FileKey: "a.tmx", FileHash: "deadbeef",
	}))
	s, ok, err := tm.GetImportSession(t.Context(), "s1")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "a.tmx", s.FileKey)

	require.NoError(t, tm.UpdateImportSessionCount(t.Context(), "s1", 42))
	s, _, _ = tm.GetImportSession(t.Context(), "s1")
	assert.Equal(t, 42, s.EntryCount)

	hit, ok, err := tm.FindImportSessionByHash(t.Context(), "deadbeef")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "s1", hit.ID)

	require.NoError(t, tm.DeleteImportSession(t.Context(), "s1"))
	_, ok, _ = tm.GetImportSession(t.Context(), "s1")
	assert.False(t, ok)
}

func TestPostgresMemory_DeleteSessionKeepsOrigins(t *testing.T) {
	tm := openTestPostgresMemory(t)
	require.NoError(t, tm.CreateImportSession(t.Context(), memory.ImportSession{ID: "s1", FileKey: "a.tmx"}))
	e := trilingual("e1", "a", "b", "c")
	e.Origins = []memory.Origin{{Source: "import", SessionID: "s1"}}
	require.NoError(t, tm.Add(t.Context(), e))
	require.NoError(t, tm.DeleteImportSession(t.Context(), "s1"))
	got, ok, err := tm.GetEntry(t.Context(), "e1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, got.Origins, 1)
	assert.Equal(t, "", got.Origins[0].SessionID)
}

func TestPostgresMemory_EntityRoundtrip(t *testing.T) {
	tm := openTestPostgresMemory(t)
	e := trilingual("e1", "John works here", "Jean travaille ici", "Johann arbeitet hier")
	e.Entities = []memory.EntityMapping{
		{
			PlaceholderID: "e1",
			Type:          "entity:person",
			Values: map[model.LocaleID]memory.EntityValue{
				"en": {Text: "John", Start: 0, End: 4},
				"fr": {Text: "Jean", Start: 0, End: 4},
				"de": {Text: "Johann", Start: 0, End: 6},
			},
		},
	}
	require.NoError(t, tm.Add(t.Context(), e))
	got, ok, err := tm.GetEntry(t.Context(), "e1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "Jean", got.Entities[0].Values["fr"].Text)
}

// A generalized match adapts the stored entry's entity to the one the lookup
// block locates. The Postgres store reads the block's entities through
// memory.ExtractEntityAnnotations as the framework stores do, from the entity
// overlay where every producer records them.
func TestPostgresMemory_LookupAdaptsTheEntityTheBlockLocates(t *testing.T) {
	tm := openTestPostgresMemory(t)
	org := string(model.EntityOrganization)
	runs := func(before, entity, after string) []model.Run {
		return []model.Run{
			{Text: &model.TextRun{Text: before}},
			{Ph: &model.PlaceholderRun{ID: "e1", Type: org, Data: entity}},
			{Text: &model.TextRun{Text: after}},
		}
	}
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "acme",
		Variants: map[model.LocaleID][]model.Run{
			"en": runs("Contact ", "Acme", " for support"),
			"fr": runs("Contactez ", "Acme", " pour le support"),
		},
		HintSrcLang: "en",
		Entities: []memory.EntityMapping{{
			PlaceholderID: "e1",
			Type:          model.EntityOrganization,
			Values: map[model.LocaleID]memory.EntityValue{
				"en": {Text: "Acme"},
				"fr": {Text: "Acme"},
			},
		}},
	}))

	b := &model.Block{ID: "lookup", Translatable: true}
	b.SetSourceRuns(runs("Contact ", "Globex", " for support"))
	b.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    "entity:0",
		Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2}),
		Value: &model.EntityAnnotation{Text: "Globex", Type: model.EntityOrganization},
	})
	matches, err := tm.Lookup(t.Context(), b, "en", "fr", memory.LookupOptions{MinScore: 0.5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, memory.MatchGeneralizedExact, matches[0].MatchType)
	require.Len(t, matches[0].EntityAdaptations, 1, "the stored entity is adapted to the one the block carries")
	assert.Equal(t, "Acme", matches[0].EntityAdaptations[0].StoredValue)
	assert.Equal(t, "Globex", matches[0].EntityAdaptations[0].CurrentValue)
}

// Leverage adapts the entity at its placeholder, and the town named after the
// company keeps its name.
func TestPostgresMemory_LeverageAdaptsOnlyTheEntity(t *testing.T) {
	tm := openTestPostgresMemory(t)
	org := func(id, text string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: string(model.EntityOrganization), Data: text}}
	}
	txt := func(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "acme",
		Variants: map[model.LocaleID][]model.Run{
			"en": {txt("Contact "), org("e1", "Acme"), txt(" in Acmeville.")},
			"fr": {txt("Contactez "), org("e1", "Acme"), txt(" à Acmeville.")},
		},
		HintSrcLang: "en",
		Entities: []memory.EntityMapping{{PlaceholderID: "e1", Type: model.EntityOrganization,
			Values: map[model.LocaleID]memory.EntityValue{"en": {Text: "Acme"}, "fr": {Text: "Acme"}}}},
	}))
	b := &model.Block{ID: "lookup", Translatable: true}
	b.SetSourceRuns([]model.Run{txt("Contact "), org("e1", "Globex"), txt(" in Acmeville.")})
	b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "entity:0",
		Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2}),
		Value: &model.EntityAnnotation{Text: "Globex", Type: model.EntityOrganization}})
	got, ok := leverage.NewProvider(tm).Lookup(t.Context(), corememory.Request{Block: b, Source: "en", Target: "fr", MinScore: 50})
	require.True(t, ok)
	want := []model.Run{txt("Contactez "), org("e1", "Globex"), txt(" à Acmeville.")}
	assert.Equal(t, string(model.CanonicalRunsJSON(want)), string(model.CanonicalRunsJSON(got.TargetRuns)))
}

// Stored entities pair with the block's in the order they sit in the stored
// source; the store reads them back by id, where e10 sorts before e2. A
// segment's lookup pairs with that segment's entities.
func TestPostgresMemory_EntitiesPairInTextOrderAndBySegment(t *testing.T) {
	tm := openTestPostgresMemory(t)
	org := func(id, text string) model.Run {
		return model.Run{Ph: &model.PlaceholderRun{ID: id, Type: string(model.EntityOrganization), Data: text}}
	}
	txt := func(s string) model.Run { return model.Run{Text: &model.TextRun{Text: s}} }
	mapping := func(id, v string) memory.EntityMapping {
		return memory.EntityMapping{PlaceholderID: id, Type: model.EntityOrganization,
			Values: map[model.LocaleID]memory.EntityValue{"en": {Text: v}, "fr": {Text: v}}}
	}
	locate := func(b *model.Block) {
		for i, r := range b.Source {
			if r.Ph != nil {
				b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "entity:" + r.Ph.ID,
					Range: model.SpanAnchor(model.RunPos{Run: i}, model.RunPos{Run: i + 1}),
					Value: &model.EntityAnnotation{Text: r.Ph.Data, Type: model.EntityOrganization}})
			}
		}
	}
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "pair",
		Variants: map[model.LocaleID][]model.Run{
			"en": {txt("Ask "), org("e2", "Acme"), txt(" or "), org("e10", "Initech"), txt(".")},
			"fr": {txt("Demandez à "), org("e2", "Acme"), txt(" ou "), org("e10", "Initech"), txt(".")},
		},
		HintSrcLang: "en",
		Entities:    []memory.EntityMapping{mapping("e2", "Acme"), mapping("e10", "Initech")},
	}))
	b := &model.Block{ID: "lookup", Translatable: true}
	b.SetSourceRuns([]model.Run{txt("Ask "), org("e1", "Globex"), txt(" or "), org("e2", "Hooli"), txt(".")})
	locate(b)
	matches, err := tm.Lookup(t.Context(), b, "en", "fr", memory.LookupOptions{MinScore: 0.5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	got := map[string]string{}
	for _, a := range matches[0].EntityAdaptations {
		got[a.StoredValue] = a.CurrentValue
	}
	assert.Equal(t, map[string]string{"Acme": "Globex", "Initech": "Hooli"}, got)

	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "ask",
		Variants: map[model.LocaleID][]model.Run{
			"en": {txt("Tell "), org("e1", "Initech"), txt(" now.")},
			"fr": {txt("Dites à "), org("e1", "Initech"), txt(" maintenant.")},
		},
		HintSrcLang: "en",
		Entities:    []memory.EntityMapping{mapping("e1", "Initech")},
	}))
	seg := &model.Block{ID: "two", Translatable: true}
	seg.SetSourceRuns([]model.Run{txt("Contact "), org("e1", "Acme"), txt(" today. "), txt("Tell "), org("e2", "Globex"), txt(" now.")})
	locate(seg)
	seg.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 3})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 3}, model.RunPos{Run: 6})},
	})
	matches, err = tm.LookupSegment(t.Context(), seg, 1, "en", "fr", memory.LookupOptions{MinScore: 0.5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	require.Len(t, matches[0].EntityAdaptations, 1)
	assert.Equal(t, "Globex", matches[0].EntityAdaptations[0].CurrentValue)
}
