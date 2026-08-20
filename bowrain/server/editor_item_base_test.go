package server

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
)

func items(names ...string) []store.ItemTranslationStats {
	out := make([]store.ItemTranslationStats, 0, len(names))
	for _, n := range names {
		out = append(out, store.ItemTranslationStats{ItemName: n})
	}
	return out
}

func TestCommonItemBase(t *testing.T) {
	tests := []struct {
		name  string
		items []store.ItemTranslationStats
		want  string
	}{
		{"no items share nothing", nil, ""},
		{
			"a collection's declared base is what every item repeats",
			items("bowrain/packages/app/src/i18n.ts", "bowrain/packages/app/src/queries.ts"),
			"bowrain/packages/app/src/",
		},
		{
			"the shared part stops where the paths diverge",
			items("web/docs/intro.md", "web/blog/first.md"),
			"web/",
		},
		{
			"whole segments only — docs/api and docs/apps share docs/, not docs/ap",
			items("docs/api/index.md", "docs/apps/index.md"),
			"docs/",
		},
		{
			"an item at the root leaves nothing shared",
			items("README.md", "docs/intro.md"),
			"",
		},
		{
			"every item at the root shares nothing",
			items("a.json", "b.json"),
			"",
		},
		{
			"one item contributes its own directory, so the row reads as the file",
			items("bowrain/web/docs/docs/intro.md"),
			"bowrain/web/docs/docs/",
		},
		{
			"a shallower item caps the shared prefix",
			items("a/b/c/one.md", "a/two.md"),
			"a/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, commonItemBase(tt.items))
		})
	}
}

// The base names what the rows SHOW. A generated catalog reads as the file it
// was extracted from, so a base computed over item names would prefix none of
// the paths beside it and every row would read whole.
func TestCommonItemBaseTrimsWhatIsShown(t *testing.T) {
	catalog := func(name, source string) store.ItemTranslationStats {
		return store.ItemTranslationStats{ItemName: name, SourcePath: source}
	}

	t.Run("trims the source paths, not the catalog names", func(t *testing.T) {
		got := commonItemBase([]store.ItemTranslationStats{
			catalog(
				"bowrain/packages/app/i18n/bowrain/packages/ui/src/ReachPanel.kbf.json",
				"bowrain/packages/ui/src/ReachPanel.tsx",
			),
			catalog(
				"bowrain/packages/app/i18n/bowrain/packages/ui/src/TrialPanel.kbf.json",
				"bowrain/packages/ui/src/TrialPanel.tsx",
			),
		})
		assert.Equal(t, "bowrain/packages/ui/src/", got)
	})

	t.Run("an item that is its own source contributes its own name", func(t *testing.T) {
		got := commonItemBase([]store.ItemTranslationStats{
			catalog("web/i18n/docs/intro.kbf.json", "web/docs/intro.tsx"),
			{ItemName: "web/docs/releasing.md"},
		})
		assert.Equal(t, "web/docs/", got)
	})
}
