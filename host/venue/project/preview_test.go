package project

import (
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func collectionFromYAML(t *testing.T, doc string) *coreproj.Collection {
	t.Helper()
	var coll coreproj.Collection
	require.NoError(t, yaml.Unmarshal([]byte(doc), &coll))
	return &coll
}

// The recipe declares the host on the collection, beside the channel that
// places it — the same grain, because a repository publishes one host per
// surface it ships.
func TestCollectionPreview_ReadsTheDeclaredHost(t *testing.T) {
	coll := collectionFromYAML(t, `
name: bowrain-app
channel: bowrain/app
preview:
  kind: storybook
  url: https://neokapi.github.io/storybook/bowrain/
`)

	preview := CollectionPreview(coll)
	require.NotNil(t, preview)
	assert.Equal(t, schema.PreviewKindStorybook, preview.Kind)
	assert.Equal(t, "https://neokapi.github.io/storybook/bowrain/", preview.URL)
}

// Two collections in one repository can name two hosts, which is the case a
// project-level setting could not express: the desktop app's components and the
// web app's are published separately.
func TestCollectionPreview_TwoCollectionsNameTwoHosts(t *testing.T) {
	desktop := CollectionPreview(collectionFromYAML(t, `
name: neokapi-desktop
preview: {kind: storybook, url: https://neokapi.github.io/storybook/kapi/}
`))
	web := CollectionPreview(collectionFromYAML(t, `
name: bowrain-app
preview: {kind: storybook, url: https://neokapi.github.io/storybook/bowrain/}
`))

	require.NotNil(t, desktop)
	require.NotNil(t, web)
	assert.NotEqual(t, desktop.URL, web.URL)
}

// A collection that declares none has none — the reviewer offers document
// reading and nothing else.
func TestCollectionPreview_UndeclaredIsNil(t *testing.T) {
	assert.Nil(t, CollectionPreview(collectionFromYAML(t, `name: neokapi-docs`)))
	assert.Nil(t, CollectionPreview(nil))
}

// Half a declaration reads as none here. The loader already refused it through
// the registered extension, so reaching this with one means the caller is
// holding a recipe nobody validated — and a preview is not what to fail them on.
func TestCollectionPreview_HalfADeclarationIsNone(t *testing.T) {
	assert.Nil(t, CollectionPreview(collectionFromYAML(t, `
name: bowrain-app
preview: {kind: storybook}
`)))
	assert.Nil(t, CollectionPreview(collectionFromYAML(t, `
name: bowrain-app
preview: {url: https://example.dev/sb/}
`)))
}
