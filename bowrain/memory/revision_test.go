package memory

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/memory"
)

func pair(id, source, target string, origins ...fw.Origin) fw.Entry {
	return fw.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
		HintSrcLang: "en",
		Origins:     origins,
	}
}

func revisionOf(t *testing.T, tm *PostgresStore, id string) string {
	t.Helper()
	revs, err := tm.Revisions(t.Context(), []string{id})
	require.NoError(t, err)
	return revs[id]
}

func TestUpdateIsBasedOnARevision(t *testing.T) {
	ctx := t.Context()
	tm, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-rev")
	require.NoError(t, err)
	approval := fw.Origin{Source: "approval", Key: "u1"}
	require.NoError(t, tm.Add(ctx, pair("e1", "Hello", "Hei", approval)))
	base := revisionOf(t, tm, "e1")
	require.NotEmpty(t, base)

	rev, err := tm.Update(ctx, pair("e1", "Hello", "Hallo"), base, fw.Origin{Key: "alice", AddedBy: "alice"})
	require.NoError(t, err)
	assert.NotEqual(t, base, rev)
	got, _, err := tm.GetEntry(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Hallo", got.VariantText("nb"))
	sources := map[string]string{}
	for _, o := range got.Origins {
		sources[o.Source] = o.Key
	}
	assert.Equal(t, map[string]string{"approval": "u1", OriginPerson: "alice"}, sources,
		"an edit keeps every origin and records the person who made it")

	_, err = tm.Update(ctx, pair("e1", "Hello", "Heisann"), base, fw.Origin{Key: "bob"})
	require.ErrorIs(t, err, storage.ErrStale, "an edit based on an older revision is refused")
	got, _, err = tm.GetEntry(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Hallo", got.VariantText("nb"), "a refused edit writes nothing")

	_, err = tm.Update(ctx, pair("missing", "Hello", "Hei"), base, fw.Origin{Key: "bob"})
	require.ErrorIs(t, err, ErrEntryNotFound, "an edit never creates an entry")
	_, ok, err := tm.GetEntry(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, ok)
}

// Two people who opened the entry at the same revision save at once: exactly
// one edit lands.
func TestUpdateOnTheSameRevisionLandsOnce(t *testing.T) {
	ctx := t.Context()
	tm, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-race")
	require.NoError(t, err)
	require.NoError(t, tm.Add(ctx, pair("e1", "Hello", "Hei")))
	base := revisionOf(t, tm, "e1")

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = tm.Update(ctx, pair("e1", "Hello", "Hallo"), base, fw.Origin{Key: "p"})
		})
	}
	wg.Wait()
	landed, stale := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			landed++
		case errors.Is(err, storage.ErrStale):
			stale++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, landed)
	assert.Equal(t, len(errs)-1, stale)
}

func TestAddIfAbsentNeverReplaces(t *testing.T) {
	ctx := t.Context()
	tm, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-absent")
	require.NoError(t, err)

	created, err := tm.AddIfAbsent(ctx, pair("e1", "Hello", "Hei"))
	require.NoError(t, err)
	assert.True(t, created)
	created, err = tm.AddIfAbsent(ctx, pair("e1", "Hello", "Hallo"))
	require.NoError(t, err)
	assert.False(t, created)
	got, _, err := tm.GetEntry(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Hei", got.VariantText("nb"))
}

func TestAddOriginLeavesTheTextAndRevision(t *testing.T) {
	ctx := t.Context()
	tm, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-origin")
	require.NoError(t, err)
	require.NoError(t, tm.Add(ctx, pair("e1", "Hello", "Hei", fw.Origin{Source: "record", Key: "f.json"})))
	before := revisionOf(t, tm, "e1")

	ok, err := tm.AddOrigin(ctx, "e1", fw.Origin{Source: "approval", Key: "u1"}, map[string]string{"k": "v"})
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = tm.AddOrigin(ctx, "e1", fw.Origin{Source: "approval", Key: "u1", AddedBy: "again"}, nil)
	require.NoError(t, err)
	require.True(t, ok)

	got, _, err := tm.GetEntry(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Hei", got.VariantText("nb"))
	assert.Len(t, got.Origins, 2, "an origin with the same source and key is replaced")
	assert.Equal(t, "v", got.Properties["k"])
	assert.Equal(t, before, revisionOf(t, tm, "e1"), "adding an origin does not refuse a person's edit in flight")

	ok, err = tm.AddOrigin(ctx, "missing", fw.Origin{Source: "approval", Key: "u1"}, nil)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRemoveOriginRemovesTheEntryOnlyWhenNoOriginIsLeft(t *testing.T) {
	ctx := t.Context()
	tm, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-remove")
	require.NoError(t, err)
	approval := fw.Origin{Source: "approval", Key: "u1"}
	require.NoError(t, tm.Add(ctx, pair("held", "Hello", "Hei", approval, fw.Origin{Source: "record", Key: "f.json"})))
	require.NoError(t, tm.Add(ctx, pair("alone", "Bye", "Ha det", approval)))

	removed, err := tm.RemoveOrigin(ctx, "held", "approval", "u1")
	require.NoError(t, err)
	assert.False(t, removed, "another origin still holds the entry")
	got, ok, err := tm.GetEntry(ctx, "held")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Len(t, got.Origins, 1)

	removed, err = tm.RemoveOrigin(ctx, "alone", "approval", "u1")
	require.NoError(t, err)
	assert.True(t, removed)
	_, ok, err = tm.GetEntry(ctx, "alone")
	require.NoError(t, err)
	assert.False(t, ok)

	removed, err = tm.RemoveOrigin(ctx, "missing", "approval", "u1")
	require.NoError(t, err)
	assert.False(t, removed)
}
