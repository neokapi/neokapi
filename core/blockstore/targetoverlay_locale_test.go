package blockstore

import (
	"context"
	"iter"
	"testing"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetOverlayKind_Canonical(t *testing.T) {
	assert.Equal(t, "targets/nb-NO", TargetOverlayKind("nb_NO"))
	assert.Equal(t, "targets/nb-NO", TargetOverlayKind("NB-no"))
	assert.Equal(t, "targets/nb-NO", TargetOverlayKind("nb-NO"))
	assert.Equal(t, "targets/", TargetOverlayKind(""))
}

func TestCanonicalOverlayKind(t *testing.T) {
	tests := map[string]string{
		"targets/nb_NO":             "targets/nb-NO",
		"targets/nb-NO":             "targets/nb-NO",
		"targets/fr_ca;tone=formal": "targets/fr-CA;tone=formal",
		"targets/":                  "targets/",
		"annotations/qa":            "annotations/qa",
		"skeletons/json":            "skeletons/json",
		"":                          "",
	}
	for in, want := range tests {
		assert.Equal(t, want, CanonicalOverlayKind(in), in)
	}
}

func TestTargetOverlayLocale(t *testing.T) {
	loc, ok := TargetOverlayLocale("targets/nb_NO")
	require.True(t, ok)
	assert.Equal(t, model.LocaleID("nb-NO"), loc)
	loc, ok = TargetOverlayLocale("targets/fr-CA;tone=formal")
	require.True(t, ok)
	assert.Equal(t, model.LocaleID("fr-CA"), loc)
	_, ok = TargetOverlayLocale("annotations/qa")
	assert.False(t, ok)
	_, ok = TargetOverlayLocale("targets/")
	assert.False(t, ok)
}

// An overlay written under one spelling of a locale is read and listed under
// every other, because the store canonicalizes the kind on both sides.
func TestMemoryStore_OverlayKindIsCanonical(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutOverlay(Overlay{Kind: "targets/nb_NO", BlockHash: "b1", Payload: []byte(`{"text":"Kai"}`)}))

	got, err := sess.GetOverlay("targets/nb-NO", "b1")
	require.NoError(t, err)
	assert.Equal(t, "targets/nb-NO", got.Kind, "the store holds the canonical kind")
	got, err = sess.GetOverlay(TargetOverlayKind("NB-no"), "b1")
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"text":"Kai"}`), got.Payload)

	n := 0
	for _, lerr := range sess.ListOverlays("targets/nb_no") {
		require.NoError(t, lerr)
		n++
	}
	assert.Equal(t, 1, n)

	// A second write in the canonical spelling replaces, rather than doubles.
	require.NoError(t, sess.PutOverlay(Overlay{Kind: "targets/nb-NO", BlockHash: "b1", Payload: []byte(`{"text":"Kaiplass"}`)}))
	all, ok := sess.(interface {
		AllOverlays() iter.Seq2[Overlay, error]
	})
	require.True(t, ok)
	n = 0
	for _, lerr := range all.AllOverlays() {
		require.NoError(t, lerr)
		n++
	}
	assert.Equal(t, 1, n)
}

func TestBlockTexts_TargetLocalesAreCanonical(t *testing.T) {
	b := &kbf.Block{
		ID:     "tu1",
		Source: []model.Run{{Text: &model.TextRun{Text: "Berth"}}},
		Targets: map[string][]model.Run{
			"nb_NO": {{Text: &model.TextRun{Text: "Kai"}}},
		},
	}
	texts := BlockTexts(b)
	require.Len(t, texts, 2)
	assert.Equal(t, SourceLocale, texts[0].Locale)
	assert.Equal(t, "nb-NO", texts[1].Locale)

	opts := TextSearchOptions{Locales: []string{"nb_NO", SourceLocale}}
	assert.Equal(t, []string{"nb-NO", ""}, opts.CanonicalLocales())
	assert.True(t, opts.wants("nb-NO"))
	assert.True(t, opts.wants(SourceLocale))
	assert.False(t, opts.wants("de"))
	assert.Nil(t, TextSearchOptions{}.CanonicalLocales(), "no filter stays no filter")
}

func TestScanText_LocaleFilterIsCanonical(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("app", &kbf.Block{
		ID:     "tu1",
		Hash:   "h1",
		Source: []model.Run{{Text: &model.TextRun{Text: "Berth"}}},
		Targets: map[string][]model.Run{
			"nb_NO": {{Text: &model.TextRun{Text: "Kai"}}},
		},
	}))
	require.NoError(t, sess.Commit())

	hits, err := SearchText(ctx, store, "kai", TextSearchOptions{Locales: []string{"NB-no"}})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "nb-NO", hits[0].Locale)
}
