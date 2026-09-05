package projectdb

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNonCanonicalLocales(t *testing.T) {
	ctx := context.Background()
	layout := project.LayoutAt(t.TempDir())
	require.NoError(t, project.EnsureLayout(layout))
	db, err := Open(ctx, layout)
	require.NoError(t, err)
	defer db.Close()

	// Rows written through the stores are canonical whatever they were handed,
	// so a fresh store audits clean.
	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:    "c1",
		Terms: []terms.Term{{Text: "berth", Locale: "en_US", Status: model.TermPreferred}},
	}))
	sess, err := db.BlocksAutocommit().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb_NO", BlockHash: "b1", Payload: []byte(`{}`)}))
	require.NoError(t, sess.Close())

	drift, err := db.NonCanonicalLocales(ctx)
	require.NoError(t, err)
	assert.Empty(t, drift)

	// Rows a store wrote before it normalized locales are what the audit is for.
	raw := db.Raw()
	for _, row := range [][2]string{{"e1", "en_US"}, {"e1", "nb_NO"}, {"e2", "nb_NO"}} {
		_, err = raw.ExecContext(ctx, `INSERT OR IGNORE INTO tm_entries (id, project_id, stream, hint_src_lang, properties, note, has_codes, point, unit, created_at, updated_at)
			VALUES (?, '', '', 'en', '', '', 0, '', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, row[0])
		require.NoError(t, err)
		_, err = raw.ExecContext(ctx, `INSERT INTO tm_variants (entry_id, locale, coded, plain, struct_key, general_key) VALUES (?, ?, 'x', 'x', 'x', 'x')`, row[0], row[1])
		require.NoError(t, err)
	}
	_, err = raw.ExecContext(ctx, `INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags)
		VALUES ('c1', 'kai', 'kai', 'NB-no', 'preferred', '', '', '', 0, NULL, NULL, '{}')`)
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, `INSERT INTO overlays (kind, block_hash, payload, updated_at) VALUES ('targets/nb_NO', 'b2', '{}', 1)`)
	require.NoError(t, err)

	drift, err = db.NonCanonicalLocales(ctx)
	require.NoError(t, err)
	assert.Equal(t, []LocaleDrift{
		{Subsystem: "block cache", Locale: "targets/nb_NO", Canonical: "targets/nb-NO", Rows: 1},
		{Subsystem: "content memory", Locale: "en_US", Canonical: "en-US", Rows: 1},
		{Subsystem: "content memory", Locale: "nb_NO", Canonical: "nb-NO", Rows: 2},
		{Subsystem: "terms", Locale: "NB-no", Canonical: "nb-NO", Rows: 1},
	}, drift)
	assert.Equal(t, `terms: 1 row(s) under "NB-no" (lookups ask for "nb-NO")`, drift[3].String())
}
