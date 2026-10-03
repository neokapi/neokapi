package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
)

// A database built before notes became annotations on the block holds the
// block_notes table and the notes people wrote there. Migrating it moves each
// note onto its block's note overlay, where the notes route reads it, beside
// any note a change set already wrote there, and drops the table. A note whose
// block is gone goes with the table.
func TestMigrations_MoveBlockNotesOntoTheirBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	tableExists := func() bool {
		var exists bool
		require.NoError(t, s.db.QueryRowContext(ctx, `SELECT to_regclass('public.block_notes') IS NOT NULL`).Scan(&exists))
		return exists
	}
	require.False(t, tableExists(), "a database built from the current baseline has no notes table")

	entity := model.Overlay{Type: model.OverlayEntity, Spans: []model.Span{{
		ID:    "entity:0",
		Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4}),
		Value: &model.EntityAnnotation{Text: "Acme", Type: model.EntityOrganization},
	}}}
	landed := model.Span{
		ID:    "note-1",
		Range: model.BlockAnchor(),
		Props: map[string]string{"author_id": "u-ana", "author": "Ana", "created_at": "2026-09-30T08:00:00Z"},
		Value: &model.Notes{Items: []*model.NoteAnnotation{{Text: "Keep the brand name.", From: "Ana"}}},
	}
	plain := model.NewBlock("b-plain", "Acme widget")
	plain.Overlays = []model.Overlay{entity}
	annotated := model.NewBlock("b-annotated", "Sign in")
	annotated.Overlays = []model.Overlay{{Type: model.OverlayType("note"), Spans: []model.Span{landed}}}
	untouched := model.NewBlock("b-untouched", "Sign out")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{plain, annotated, untouched}))
	onFeature := model.NewBlock("b-plain", "Acme widget")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "feature", []*model.Block{onFeature}))

	// The table and the notes a database at version 37 carries.
	_, err := s.db.ExecContext(ctx, `CREATE TABLE block_notes (
		id         TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		block_id   TEXT NOT NULL,
		author     TEXT NOT NULL DEFAULT '',
		text       TEXT NOT NULL,
		stream     TEXT NOT NULL DEFAULT 'main',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`)
	require.NoError(t, err)
	for _, n := range []struct{ id, stream, block, author, text, at string }{
		{"n-late", "main", "b-plain", "", "Who approved this?", "2026-09-02T09:30:00Z"},
		{"n-early", "main", "b-plain", "Reviewer", "The source reads oddly here.", "2026-09-01T10:00:00Z"},
		{"n-joins", "main", "b-annotated", "Bo", "Checked against the style guide.", "2026-09-03T11:15:00Z"},
		{"n-feature", "feature", "b-plain", "Cy", "Different on this stream.", "2026-09-04T12:00:00Z"},
		{"n-orphan", "main", "b-deleted", "Dee", "Its block is gone.", "2026-09-05T13:00:00Z"},
	} {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO block_notes (id, project_id, block_id, author, text, stream, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			n.id, p.ID, n.block, n.author, n.text, n.stream, n.at)
		require.NoError(t, err)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM store_schema_migrations WHERE version >= 38`)
	require.NoError(t, err)

	require.NoError(t, storage.MigratePostgresNS(s.db, "store_schema_migrations", Migrations))
	assert.False(t, tableExists(), "migrating drops the retired notes table")

	note := func(id, author, text, at string) model.Span {
		item := &model.NoteAnnotation{Text: text, From: author}
		return model.Span{
			ID:    id,
			Range: model.BlockAnchor(),
			Props: map[string]string{"author": author, "created_at": at},
			Value: &model.Notes{Items: []*model.NoteAnnotation{item}},
		}
	}
	notesOn := func(stream, id string) *model.Overlay {
		sb, err := s.GetBlock(ctx, p.ID, stream, id)
		require.NoError(t, err)
		return sb.Block.OverlayOf(model.OverlayType("note"))
	}

	got, err := s.GetBlock(ctx, p.ID, "main", "b-plain")
	require.NoError(t, err)
	assert.Equal(t, entity, *got.Block.OverlayOf(model.OverlayEntity), "the block's other overlays stay as they were")
	require.NotNil(t, got.Block.OverlayOf(model.OverlayType("note")))
	assert.Equal(t, []model.Span{
		note("n-early", "Reviewer", "The source reads oddly here.", "2026-09-01T10:00:00Z"),
		note("n-late", "", "Who approved this?", "2026-09-02T09:30:00Z"),
	}, got.Block.OverlayOf(model.OverlayType("note")).Spans, "each note lands on its block, oldest first")

	joined, err := s.GetBlock(ctx, p.ID, "main", "b-annotated")
	require.NoError(t, err)
	require.Len(t, joined.Block.Overlays, 1, "the block keeps one note overlay")
	assert.Equal(t, []model.Span{landed, note("n-joins", "Bo", "Checked against the style guide.", "2026-09-03T11:15:00Z")},
		joined.Block.Overlays[0].Spans, "a moved note joins the notes a change set wrote")

	assert.Nil(t, notesOn("main", "b-untouched"), "a block nobody wrote a note on gains none")
	feature := notesOn("feature", "b-plain")
	require.NotNil(t, feature)
	assert.Equal(t, []model.Span{note("n-feature", "Cy", "Different on this stream.", "2026-09-04T12:00:00Z")},
		feature.Spans, "a stream's note stays on that stream's block")
}
