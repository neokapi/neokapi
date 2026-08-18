package project

import (
	"os"
	"path/filepath"
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This repository is the worked example of why the preview host is declared per
// collection: it publishes two of them, one for the desktop app's components
// and one for the web app's, and its own recipe has to be able to say so.
//
// Loaded here rather than in core/project because the key is a venue extension:
// this is the module that registers it, so this is the module where the recipe
// actually validates.
func TestDogfoodRecipePreviewHosts(t *testing.T) {
	path := filepath.Join("..", "..", "..", "kapi.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("not running inside the neokapi checkout")
	}

	p, err := coreproj.Load(path)
	require.NoError(t, err)

	byName := make(map[string]*coreproj.Collection, len(p.Collections))
	for i := range p.Collections {
		byName[p.Collections[i].Name] = &p.Collections[i]
	}

	for _, tt := range []struct {
		collection string
		wantURL    string
	}{
		{"neokapi-desktop", "https://neokapi.github.io/storybook/kapi/"},
		{"bowrain-app", "https://neokapi.github.io/storybook/bowrain/"},
		{"bowrain-ctrl", "https://neokapi.github.io/storybook/bowrain/"},
		{"bowrain-pulse", "https://neokapi.github.io/storybook/bowrain/"},
	} {
		t.Run(tt.collection, func(t *testing.T) {
			coll := byName[tt.collection]
			require.NotNil(t, coll, "collection %s is gone from the recipe", tt.collection)

			preview := CollectionPreview(coll)
			require.NotNil(t, preview, "%s declares no preview host", tt.collection)
			assert.Equal(t, schema.PreviewKindStorybook, preview.Kind)
			assert.Equal(t, tt.wantURL, preview.URL)
			require.NoError(t, preview.Validate())
		})
	}

	// The two hosts are genuinely different, which is the claim the per-
	// collection shape rests on. A project-level setting could name one.
	assert.NotEqual(t,
		CollectionPreview(byName["neokapi-desktop"]).URL,
		CollectionPreview(byName["bowrain-app"]).URL)

	// A collection with no components behind it declares none, and the reviewer
	// offers document reading for it.
	assert.Nil(t, CollectionPreview(byName["neokapi-docs"]))
}
