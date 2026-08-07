package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDogfoodRecipeResolves loads this repository's own recipe and pins the
// paths its collections resolve to. The recipe is the worked example every doc
// page and scaffold is written against, and it is the one place where `base:`,
// qualified and bare channel references and a platform venue block all appear
// at once — a rename that leaves it loading but resolving different files
// would be invisible in every other test.
func TestDogfoodRecipeResolves(t *testing.T) {
	path := filepath.Join("..", "..", "kapi.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("not running inside the neokapi checkout")
	}

	// SkipRequiresCheck: the bowrain extension is registered by the plugin,
	// not by this module.
	p, err := LoadWithOptions(path, LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	byName := make(map[string]*Collection, len(p.Collections))
	for i := range p.Collections {
		byName[p.Collections[i].Name] = &p.Collections[i]
	}

	tests := []struct {
		collection string
		profile    string
		channel    string
		source     string
		wantPath   string
		wantTarget string
	}{
		{
			collection: "bowrain-docs",
			profile:    "bowrain",
			channel:    "docs",
			source:     "bowrain/web/docs/docs/guide/intro.mdx",
			wantPath:   "bowrain/web/docs/docs/**/*.mdx",
			wantTarget: "bowrain/web/docs/i18n/nb/docusaurus-plugin-content-docs/current/guide/intro.mdx",
		},
		{
			collection: "neokapi-docs",
			profile:    "neokapi",
			channel:    "docs",
			source:     "web/docs/reference/project-file.mdx",
			wantPath:   "web/docs/**/*.mdx",
			wantTarget: "web/i18n/nb/docusaurus-plugin-content-docs/current/reference/project-file.mdx",
		},
		{
			collection: "neokapi-demos",
			profile:    "neokapi",
			channel:    "demos",
			source:     "harness/demos/kapi-up-loop/demo.yaml",
			wantPath:   "harness/demos/*/demo.yaml",
			wantTarget: "harness/demos/kapi-up-loop/demo.nb.yaml",
		},
		{
			collection: "bowrain-app",
			profile:    "bowrain",
			channel:    "app",
			source:     "bowrain/packages/app/i18n/en/app.kbf.json",
			wantPath:   "bowrain/packages/app/i18n/**/*.kbf.json",
			wantTarget: "bowrain/packages/app/i18n-nb/app.kbf.json",
		},
		{
			collection: "bowrain-email-subjects",
			profile:    "bowrain",
			channel:    "email",
			source:     "bowrain/mailer/subjects/en.json",
			wantPath:   "bowrain/mailer/subjects/en.json",
			wantTarget: "bowrain/mailer/subjects/nb.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.collection, func(t *testing.T) {
			coll := byName[tt.collection]
			require.NotNil(t, coll)

			ref, err := p.ResolveChannel(coll.Channel)
			require.NoError(t, err)
			assert.Equal(t, tt.profile, ref.Profile)
			assert.Equal(t, tt.channel, ref.Channel)

			var matched *ContentItem
			for _, item := range coll.EffectiveItems() {
				if MatchGlob(item.Path, tt.source) {
					it := item
					matched = &it
					break
				}
			}
			require.NotNil(t, matched, "no item pattern matches %s", tt.source)
			assert.Equal(t, tt.wantPath, matched.Path)
			assert.Equal(t, tt.wantTarget,
				filepath.ToSlash(ResolveTargetPath(matched.Path, matched.Base, matched.Target, tt.source, "nb")))
		})
	}
}

// TestDogfoodRecipeVenue pins that the platform's block reaches the framework
// through the venue seam and not through a key the framework spells out.
func TestDogfoodRecipeVenue(t *testing.T) {
	path := filepath.Join("..", "..", "kapi.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("not running inside the neokapi checkout")
	}
	p, err := LoadWithOptions(path, LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)

	_, ok := p.Venue()
	assert.False(t, ok, "no venue extension is registered in this module, so the block carries no meaning here")
	assert.Contains(t, p.Extras, "bowrain", "the block still round-trips verbatim")
}
