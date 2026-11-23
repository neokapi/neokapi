package memory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// AddOrigin records an origin on the entry as the store holds it: the
// variants, the other origins and the other properties stay as they are, and
// an origin with the same source and key is replaced rather than repeated.
func TestSQLiteStore_AddOrigin(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })
	ctx := t.Context()

	require.NoError(t, tm.Add(ctx, memory.Entry{
		ID: "record:save",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Save"}}},
			"de": {{Text: &model.TextRun{Text: "Sichern"}}},
		},
		Origins:    []memory.Origin{{Source: "record", Key: "de.json"}},
		Properties: map[string]string{"kept": "yes"},
	}))

	merge := memory.Origin{Source: "merge", Key: "en.json", Reference: "store"}
	ok, err := tm.AddOrigin(ctx, "record:save", merge, map[string]string{"kapi-merge:block-content-hash": "h1"})
	require.NoError(t, err)
	require.True(t, ok)
	merge.Reference = "store-2"
	ok, err = tm.AddOrigin(ctx, "record:save", merge, nil)
	require.NoError(t, err)
	require.True(t, ok)

	got, held, err := tm.GetEntry(ctx, "record:save")
	require.NoError(t, err)
	require.True(t, held)
	assert.Equal(t, "Sichern", got.VariantText("de"))
	assert.Equal(t, map[string]string{"kept": "yes", "kapi-merge:block-content-hash": "h1"}, got.Properties)
	require.Len(t, got.Origins, 2)
	assert.Equal(t, "record", got.Origins[0].Source)
	assert.Equal(t, "merge", got.Origins[1].Source)
	assert.Equal(t, "store-2", got.Origins[1].Reference, "the same source and key is replaced")

	ok, err = tm.AddOrigin(ctx, "missing", merge, nil)
	require.NoError(t, err)
	assert.False(t, ok, "an entry the store does not hold gets nothing")
}
