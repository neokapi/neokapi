package transfer

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// previewExtra encodes a collection's `preview:` block the way the recipe
// loader leaves it: an unknown-to-the-framework key on the collection, decoded
// by the venue extension that registered it.
func previewExtra(t *testing.T, kind, url string) map[string]yaml.Node {
	t.Helper()
	var node yaml.Node
	require.NoError(t, node.Encode(map[string]string{"kind": kind, "url": url}))
	return map[string]yaml.Node{"preview": node}
}

// A push carries where each collection's strings can be read in place, so the
// server holds a fact only the repository knows — and holds it per collection,
// because this repository publishes one host per surface it ships.
func TestBuildPushContext_CarriesThePreviewHostPerCollection(t *testing.T) {
	app := &host.App{}
	root := t.TempDir()

	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
		Collections: []coreproj.Collection{
			{
				Name:    "bowrain-app",
				Content: []coreproj.ContentItem{{Path: "app/**/*.kbf.json"}},
				Extras:  previewExtra(t, "storybook", "https://neokapi.github.io/storybook/bowrain/"),
			},
			{
				Name:    "neokapi-desktop",
				Content: []coreproj.ContentItem{{Path: "desktop/**/*.kbf.json"}},
				Extras:  previewExtra(t, "storybook", "https://neokapi.github.io/storybook/kapi/"),
			},
			{
				Name:    "neokapi-docs",
				Content: []coreproj.ContentItem{{Path: "docs/**/*.md"}},
			},
		},
	})
	require.NoError(t, err)

	pushCtx, _, err := BuildPushContext(t.Context(), app, proj, false)
	require.NoError(t, err)
	require.NotNil(t, pushCtx)

	byName := entriesByName(pushCtx.Entries)
	require.NotNil(t, byName["bowrain-app"].GetPreview())
	assert.Equal(t, "storybook", byName["bowrain-app"].GetPreview().GetKind())
	assert.Equal(t, "https://neokapi.github.io/storybook/bowrain/",
		byName["bowrain-app"].GetPreview().GetUrl())

	assert.Equal(t, "https://neokapi.github.io/storybook/kapi/",
		byName["neokapi-desktop"].GetPreview().GetUrl(),
		"two collections in one push name two hosts")

	assert.Nil(t, byName["neokapi-docs"].GetPreview(),
		"a collection that declares none carries none")
}
