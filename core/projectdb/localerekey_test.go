package projectdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/terms"
)

// openRekeyStore opens a project store and seeds it with rows written the way a
// store that predates canonical locales wrote them.
func openRekeyStore(t *testing.T) (*DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	layout := project.LayoutAt(t.TempDir())
	require.NoError(t, project.EnsureLayout(layout))
	db, err := Open(ctx, layout)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

// putEntry writes one content-memory entry through the raw pool, as a store
// that keyed its rows by whatever spelling it was handed would hold it.
func putEntry(t *testing.T, ctx context.Context, raw *storage.DB, id string) {
	t.Helper()
	_, err := raw.ExecContext(ctx, `
INSERT OR IGNORE INTO tm_entries (id, project_id, stream, hint_src_lang, properties, note, has_codes, point, unit, created_at, updated_at)
VALUES (?, '', '', 'en', '', '', 0, '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, id)
	require.NoError(t, err)
}

// putVariant writes one variant row under a locale spelling of the caller's
// choosing, which the store's own writers would have normalized.
func putVariant(t *testing.T, ctx context.Context, raw *storage.DB, id, locale, text string) {
	t.Helper()
	_, err := raw.ExecContext(ctx, `
INSERT INTO tm_variants (entry_id, locale, coded, plain, struct_key, general_key)
VALUES (?, ?, ?, ?, ?, ?)`, id, locale, text, text, text, text)
	require.NoError(t, err)
}

// variantText reads one variant's wording back by the exact spelling it is
// keyed under.
func variantText(t *testing.T, ctx context.Context, raw *storage.DB, id, locale string) (string, bool) {
	t.Helper()
	var plain string
	err := raw.QueryRowContext(ctx,
		`SELECT plain FROM tm_variants WHERE entry_id = ? AND locale = ?`, id, locale).Scan(&plain)
	if err != nil {
		return "", false
	}
	return plain, true
}

// TestRekeyContextLocales_MovesFoldsAndKeeps covers the three cases a move
// meets: a free canonical spelling, one already holding the same thing, and one
// holding something else.
func TestRekeyContextLocales_MovesFoldsAndKeeps(t *testing.T) {
	db, ctx := openRekeyStore(t)
	raw := db.Raw()

	for _, id := range []string{"free", "same", "other"} {
		putEntry(t, ctx, raw, id)
	}
	putVariant(t, ctx, raw, "free", "nb_NO", "Hei der")
	putVariant(t, ctx, raw, "same", "nb_NO", "Hei der")
	putVariant(t, ctx, raw, "same", "nb-NO", "Hei der")
	putVariant(t, ctx, raw, "other", "nb_NO", "Hei der")
	putVariant(t, ctx, raw, "other", "nb-NO", "Hallo der")

	done, err := db.RekeyContextLocales(ctx)
	require.NoError(t, err)
	require.Len(t, done, 1)
	assert.Equal(t, LocaleRekey{
		Subsystem: "content memory", Locale: "nb_NO", Canonical: "nb-NO",
		Moved: 1, Merged: 1, Conflicted: 1,
	}, done[0])
	assert.True(t, done[0].Rekeyed())

	moved, ok := variantText(t, ctx, raw, "free", "nb-NO")
	assert.True(t, ok, "a row whose canonical spelling was free is keyed by it")
	assert.Equal(t, "Hei der", moved)
	_, ok = variantText(t, ctx, raw, "free", "nb_NO")
	assert.False(t, ok, "and is no longer under the spelling nothing asks for")

	_, ok = variantText(t, ctx, raw, "same", "nb_NO")
	assert.False(t, ok, "a row the canonical spelling already said is folded into it")
	kept, ok := variantText(t, ctx, raw, "same", "nb-NO")
	require.True(t, ok)
	assert.Equal(t, "Hei der", kept)

	left, ok := variantText(t, ctx, raw, "other", "nb_NO")
	assert.True(t, ok, "a row the canonical spelling answers differently is kept")
	assert.Equal(t, "Hei der", left)
	answer, ok := variantText(t, ctx, raw, "other", "nb-NO")
	require.True(t, ok, "and so is the answer every lookup already reads")
	assert.Equal(t, "Hallo der", answer)

	// The audit still reports what was not moved, and nothing else.
	drift, err := db.NonCanonicalLocales(ctx)
	require.NoError(t, err)
	require.Len(t, drift, 1)
	assert.Equal(t, LocaleDrift{
		Subsystem: "content memory", Pool: PoolContext,
		Locale: "nb_NO", Canonical: "nb-NO", Rows: 1,
	}, drift[0])
}

// TestRekeyContextLocales_MovesTermsAndEntityValues: every table a subsystem
// keys by a locale travels, and a term row that is in the store twice under two
// spellings becomes one.
func TestRekeyContextLocales_MovesTermsAndEntityValues(t *testing.T) {
	db, ctx := openRekeyStore(t)
	raw := db.Raw()

	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c1",
		Terms: []terms.Term{{Text: "kai", Locale: "nb-NO", Status: model.TermPreferred}},
	}))
	// The same term as the store already holds, and one it does not, both under
	// a spelling no lookup asks for. Copied from the row the store wrote, so
	// the duplicate differs from it in nothing but the spelling.
	for _, text := range []string{"kai", "brygge"} {
		_, err := raw.ExecContext(ctx, `
INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags, forms)
SELECT concept_id, ?, ?, 'NB-no', status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags, forms
  FROM tb_terms WHERE concept_id = 'c1' AND locale = 'nb-NO'`, text, text)
		require.NoError(t, err)
	}

	putEntry(t, ctx, raw, "e1")
	putVariant(t, ctx, raw, "e1", "NB-no", "Hei")
	_, err := raw.ExecContext(ctx,
		`INSERT INTO tm_entry_entities (entry_id, placeholder_id, entity_type) VALUES ('e1', 'p1', 'name')`)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, `
INSERT INTO tm_entry_entity_values (entry_id, placeholder_id, locale, text_value, start_pos, end_pos)
VALUES ('e1', 'p1', 'NB-no', 'Kari', 0, 4)`)
	require.NoError(t, err)

	done, err := db.RekeyContextLocales(ctx)
	require.NoError(t, err)
	require.Len(t, done, 2)

	byName := map[string]LocaleRekey{}
	for _, r := range done {
		byName[r.Subsystem] = r
	}
	assert.Equal(t, LocaleRekey{
		Subsystem: "content memory", Locale: "NB-no", Canonical: "nb-NO",
		Moved: 2, Merged: 0, Conflicted: 0,
	}, byName["content memory"], "the variant and the entity value travel together")
	assert.Equal(t, LocaleRekey{
		Subsystem: "terms", Locale: "NB-no", Canonical: "nb-NO",
		Moved: 1, Merged: 1, Conflicted: 0,
	}, byName["terms"])

	// The concept reads back with both terms and no duplicate.
	concept, ok, err := db.Terms().GetConcept(ctx, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	var texts []string
	for _, term := range concept.Terms {
		assert.Equal(t, model.LocaleID("nb-NO"), term.Locale)
		texts = append(texts, term.Text)
	}
	assert.ElementsMatch(t, []string{"kai", "brygge"}, texts)

	var locale string
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT locale FROM tm_entry_entity_values WHERE entry_id = 'e1' AND placeholder_id = 'p1'`).Scan(&locale))
	assert.Equal(t, "nb-NO", locale)

	drift, err := db.NonCanonicalLocales(ctx)
	require.NoError(t, err)
	assert.Empty(t, drift)
}

// TestRekeyContextLocales_LeavesTheProjectionAlone: the block cache's overlays
// are a reading of the working tree, so they are rebuilt rather than moved, and
// the audit keeps reporting them until the projection is deleted.
func TestRekeyContextLocales_LeavesTheProjectionAlone(t *testing.T) {
	db, ctx := openRekeyStore(t)

	sess, err := db.BlocksAutocommit().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb-NO", BlockHash: "b1", Payload: []byte(`{}`)}))
	require.NoError(t, sess.Close())
	_, err = db.Raw().ExecContext(ctx,
		`INSERT INTO overlays (kind, block_hash, payload, updated_at) VALUES ('targets/nb_NO', 'b2', '{}', 1)`)
	require.NoError(t, err)

	done, err := db.RekeyContextLocales(ctx)
	require.NoError(t, err)
	assert.Empty(t, done, "nothing in the context store drifted")

	drift, err := db.NonCanonicalLocales(ctx)
	require.NoError(t, err)
	require.Len(t, drift, 1)
	assert.Equal(t, PoolProjection, drift[0].Pool)
}

// TestRekeyContextLocales_IsANoOpOnACanonicalStore: a store every lookup can
// read writes nothing.
func TestRekeyContextLocales_IsANoOpOnACanonicalStore(t *testing.T) {
	db, ctx := openRekeyStore(t)
	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c1",
		Terms: []terms.Term{{Text: "berth", Locale: "en_US", Status: model.TermPreferred}},
	}))

	done, err := db.RekeyContextLocales(ctx)
	require.NoError(t, err)
	assert.Empty(t, done)
}
