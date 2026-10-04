package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A target given to SetTargetEdition under another spelling of its locale is
// filed under the canonical spelling, and the store writes it and reads it back
// there. One filed under the zero key, which names the edition the block was
// read in, is not stored as a translation. Neither fails the write.
func TestStoreBlocks_TargetKeysAsFiled(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	tests := []struct {
		name string
		key  model.VariantKey
		want []model.EditionKey
	}{
		{"a locale spelled another way", model.VariantKey{Locale: "fr_FR"}, []model.EditionKey{{}, {Locale: "fr-FR"}}},
		{"the zero key", model.VariantKey{}, []model.EditionKey{{}}},
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

			got, err := s.GetBlock(ctx, p.ID, "", "b1")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Block.Editions())
			src, _ := got.Block.Edition(model.EditionKey{})
			assert.Equal(t, "Hello", model.RunsText(src.Runs))
			if len(tc.want) > 1 {
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
	require.NoError(t, UpsertBlockTarget(ctx, s.SQLDB(), "pg", p.ID, "main", "b1",
		model.VariantKey{Locale: "fr_FR"},
		model.Edition{Runs: text("Bonjour"), Status: model.Status(model.TargetStatusTranslated)}, nil, time.Now().UTC()))
	var locale string
	require.NoError(t, s.SQLDB().QueryRowContext(ctx,
		`SELECT locale FROM translations WHERE project_id = $1 AND block_id = $2`, p.ID, "b1").Scan(&locale))
	require.Equal(t, "fr_FR", locale, "the row is filed under the spelling it was given")

	got, err := s.GetBlock(ctx, p.ID, "", "b1")
	require.NoError(t, err)
	assert.Equal(t, []model.EditionKey{{}, {Locale: "fr-FR"}}, got.Block.Editions())
	fr, ok := got.Block.Edition(model.EditionKey{Locale: "fr-FR"})
	require.True(t, ok)
	assert.Equal(t, "Bonjour", model.RunsText(fr.Runs))
	assert.Equal(t, model.Status(model.TargetStatusTranslated), fr.Status)
}
