package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDocumentPreview_HTML(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Preview HTML", "en", []string{"fr"})
	require.NoError(t, err)

	htmlFile := filepath.Join("testdata", "page.html")
	info, err = app.AddItems(info.ID, []string{htmlFile})
	require.NoError(t, err)
	require.Len(t, info.Items, 1)

	html, err := app.RenderDocumentPreview(info.ID, "page.html", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Contains(t, html, "kat-block")
	assert.Contains(t, html, "<script>")
}

func TestRenderDocumentPreview_Plaintext(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Preview Plain", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{txtFile})
	require.NoError(t, err)
	require.Len(t, info.Items, 1)

	html, err := app.RenderDocumentPreview(info.ID, "hello.txt", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Contains(t, html, "kat-block")
}

func TestRenderDocumentPreview_Cached(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Preview Cached", "en", []string{"fr"})
	require.NoError(t, err)

	htmlFile := filepath.Join("testdata", "page.html")
	info, err = app.AddItems(info.ID, []string{htmlFile})
	require.NoError(t, err)

	// First call generates preview
	html1, err := app.RenderDocumentPreview(info.ID, "page.html", "fr")
	require.NoError(t, err)

	// Second call should return the same result
	html2, err := app.RenderDocumentPreview(info.ID, "page.html", "fr")
	require.NoError(t, err)

	assert.Equal(t, html1, html2)
}

func TestRenderDocumentPreview_NotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Preview Not Found", "en", []string{"fr"})
	require.NoError(t, err)

	_, err = app.RenderDocumentPreview(info.ID, "nonexistent.txt", "fr")
	assert.Error(t, err)
}

func TestRenderBlockHTML_Source(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Block HTML", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{txtFile})
	require.NoError(t, err)

	// Get blocks to find a block ID
	blocks, err := app.GetItemBlocks(info.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	blockID := blocks[0].ID

	// No target locale -> returns source
	html, err := app.RenderBlockHTML(info.ID, "hello.txt", blockID, "")
	require.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Equal(t, blocks[0].FlattenSource(), html)
}

func TestRenderBlockHTML_WithTarget(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Block HTML Target", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{txtFile})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(info.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)

	blockID := blocks[0].ID

	// Set a target translation
	err = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    info.ID,
		ItemName:     "hello.txt",
		BlockID:      blockID,
		TargetLocale: "fr",
		Text:         "Bonjour le monde",
	})
	require.NoError(t, err)

	// With target locale -> returns target
	html, err := app.RenderBlockHTML(info.ID, "hello.txt", blockID, "fr")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", html)
}

func TestRenderBlockHTML_NotFoundBlock(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Block Not Found", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	_, err = app.AddItems(info.ID, []string{txtFile})
	require.NoError(t, err)

	_, err = app.RenderBlockHTML(info.ID, "hello.txt", "nonexistent-block", "fr")
	assert.Error(t, err)
}

func TestRenderBlockHTML_NotFoundItem(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Item Not Found", "en", []string{"fr"})
	require.NoError(t, err)

	_, err = app.RenderBlockHTML(info.ID, "nonexistent.txt", "block1", "fr")
	assert.Error(t, err)
}

func TestRenderDocumentPreview_OnTheFly(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("On-The-Fly", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{txtFile})
	require.NoError(t, err)

	// RenderDocumentPreview generates on-the-fly from source bytes
	html, err := app.RenderDocumentPreview(info.ID, "hello.txt", "fr")
	require.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Contains(t, html, "kat-block")

	// Second call also regenerates (no in-memory cache now)
	html2, err := app.RenderDocumentPreview(info.ID, "hello.txt", "fr")
	require.NoError(t, err)
	assert.Equal(t, html, html2)
}
