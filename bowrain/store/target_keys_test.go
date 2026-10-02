package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// A target filed under a key that is not canonical is stored under its
// canonical key, and one filed under the zero key, which names the edition the
// block was read in, is not stored as a translation. Neither fails the write.
func TestStoreBlocks_TargetKeysAsFiled(t *testing.T) {
	text := func(s string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: s}}} }
	tests := []struct {
		name string
		key  model.VariantKey
		want []model.EditionKey
	}{
		{"a key that is not canonical", model.VariantKey{Locale: "fr_FR"}, []model.EditionKey{{}, {Locale: "fr-FR"}}},
		{"the zero key", model.VariantKey{}, []model.EditionKey{{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			b := model.NewBlock("b1", "Hello")
			b.Targets = map[model.VariantKey]*model.Target{
				tc.key: {Runs: text("Bonjour"), Status: model.TargetStatusTranslated},
			}
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
