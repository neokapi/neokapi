package sqlitestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// A block's editions round-trip through the store: the edition it was read in
// through the block row, every other edition through the translations table.
// A translations row filed under no language reaches the edition the block was
// read in, which only the block row may write, so reading it back leaves that
// edition alone.
func TestStoreBlocks_EditionsRoundTrip(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := model.NewBlock("b1", "Hello")
	b.SetEdition(model.Variant(model.LocaleFrench), model.Edition{
		Runs: text("Bonjour"), Status: model.Status(model.TargetStatusTranslated),
		Origin: model.Origin{Kind: model.OriginHuman}, Score: 0.9,
	})
	b.SetEdition(model.EditionKey{Locale: model.LocaleFrench, Channel: "short"}, model.Edition{Runs: text("Salut")})
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	require.NoError(t, bstore.UpsertBlockTarget(ctx, s.DB(), "sqlite", p.ID, "main", "b1",
		model.EditionKey{}, model.Edition{Runs: text("Filed under no language")}, nil, time.Now().UTC()))

	got, err := s.GetBlock(ctx, p.ID, "", "b1")
	require.NoError(t, err)

	src, ok := got.Block.Edition(model.EditionKey{})
	require.True(t, ok)
	assert.Equal(t, "Hello", model.RunsText(src.Runs), "the block row holds the edition the block was read in")
	assert.Equal(t, []model.EditionKey{
		{},
		model.Variant(model.LocaleFrench),
		{Locale: model.LocaleFrench, Channel: "short"},
	}, got.Block.Editions())

	fr, ok := got.Block.Edition(model.Variant(model.LocaleFrench))
	require.True(t, ok)
	assert.Equal(t, "Bonjour", model.RunsText(fr.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), fr.Status)
	assert.Equal(t, model.OriginHuman, fr.Origin.Kind)
	assert.InDelta(t, 0.9, fr.Score, 1e-9)

	short, ok := got.Block.Edition(model.EditionKey{Locale: model.LocaleFrench, Channel: "short"})
	require.True(t, ok)
	assert.Equal(t, "Salut", model.RunsText(short.Runs))
}

// A target given to SetTargetEdition under another spelling of its locale is
// filed under the canonical spelling, and the store writes one row for it and
// reads it back there. One filed under the zero key, which names the edition
// the block was read in, is not stored as a translation. Neither fails the
// write.
func TestStoreBlocks_TargetKeysAsFiled(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	tests := []struct {
		name string
		key  model.VariantKey
		want []model.EditionKey
		rows int
	}{
		{"a locale spelled another way", model.VariantKey{Locale: "fr_FR"}, []model.EditionKey{{}, {Locale: "fr-FR"}}, 1},
		{"the zero key", model.VariantKey{}, []model.EditionKey{{}}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			b := model.NewBlock("b1", "Hello")
			// SetTargetEdition files the target under the canonical spelling of
			// its key, and files one under the zero key as a target, where
			// SetEdition would write the edition the block was read in.
			b.SetTargetEdition(tc.key, model.Edition{Runs: text("Bonjour"), Status: model.Status(model.TargetStatusTranslated)})
			require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

			var rows int
			require.NoError(t, s.DB().QueryRowContext(ctx,
				`SELECT COUNT(*) FROM translations WHERE project_id = ? AND block_id = ?`, p.ID, "b1").Scan(&rows))
			assert.Equal(t, tc.rows, rows)

			got, err := s.GetBlock(ctx, p.ID, "", "b1")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Block.Editions())
			src, _ := got.Block.Edition(model.EditionKey{})
			assert.Equal(t, "Hello", model.RunsText(src.Runs))
			if tc.rows > 0 {
				fr, ok := got.Block.Edition(model.EditionKey{Locale: "fr-FR"})
				require.True(t, ok)
				assert.Equal(t, "Bonjour", model.RunsText(fr.Runs))
				assert.Equal(t, model.Status(model.TargetStatusTranslated), fr.Status)
			}
		})
	}
}

// A translations row filed under another spelling of its locale, as a writer
// that did not canonicalize the key left it, hydrates under the canonical key,
// the key every reader looks the edition up by.
func TestGetBlock_ReadsARowUnderItsCanonicalKey(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))
	require.NoError(t, bstore.UpsertBlockTarget(ctx, s.DB(), "sqlite", p.ID, "main", "b1",
		model.VariantKey{Locale: "fr_FR"},
		model.Edition{Runs: text("Bonjour"), Status: model.Status(model.TargetStatusTranslated)}, nil, time.Now().UTC()))
	var locale string
	require.NoError(t, s.DB().QueryRowContext(ctx,
		`SELECT locale FROM translations WHERE project_id = ? AND block_id = ?`, p.ID, "b1").Scan(&locale))
	require.Equal(t, "fr_FR", locale, "the row is filed under the spelling it was given")

	got, err := s.GetBlock(ctx, p.ID, "", "b1")
	require.NoError(t, err)
	assert.Equal(t, []model.EditionKey{{}, {Locale: "fr-FR"}}, got.Block.Editions())
	fr, ok := got.Block.Edition(model.EditionKey{Locale: "fr-FR"})
	require.True(t, ok)
	assert.Equal(t, "Bonjour", model.RunsText(fr.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), fr.Status)
}
