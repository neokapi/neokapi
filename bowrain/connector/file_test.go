package connector

import (
	"os"
	"path/filepath"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFileConnector(t *testing.T, dir string) *FileConnector {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	c, err := NewFileConnector(reg, map[string]string{
		"id":   "test-file",
		"path": dir,
	})
	require.NoError(t, err)
	return c
}

func TestFileConnectorIdentity(t *testing.T) {
	c := setupFileConnector(t, ".")
	assert.Equal(t, "test-file", c.ID())
	assert.Equal(t, platconn.CategoryFile, c.Category())
	assert.NoError(t, c.Close())
}

func TestFileConnectorList(t *testing.T) {
	// Create a temp directory with an HTML file.
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test.html"),
		[]byte("<html><body><p>Hello</p></body></html>"), 0644)
	require.NoError(t, err)

	c := setupFileConnector(t, dir)
	items, err := c.List(t.Context())
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "test.html", items[0].Name)
	assert.Equal(t, "html", items[0].Format)
}

func TestFileConnectorFetch(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test.html"),
		[]byte("<html><head><title>Test</title></head><body><p>Hello world</p></body></html>"), 0644)
	require.NoError(t, err)

	c := setupFileConnector(t, dir)
	items, err := c.Fetch(t.Context(), platconn.FetchOptions{
		Paths: []string{"test.html"},
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "test.html", items[0].Path)
	assert.NotEmpty(t, items[0].Blocks, "should have extracted blocks")
}

func TestFileConnectorFetchAll(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "a.html"),
		[]byte("<html><body><p>File A</p></body></html>"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "b.html"),
		[]byte("<html><body><p>File B</p></body></html>"), 0644)
	require.NoError(t, err)

	c := setupFileConnector(t, dir)
	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestFileConnectorStatus(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "test.html"),
		[]byte("<html><body><p>Hello</p></body></html>"), 0644)
	require.NoError(t, err)

	c := setupFileConnector(t, dir)
	status, err := c.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "test-file", status.ConnectorID)
	assert.Equal(t, 1, status.ItemCount)
}

func TestFileConnectorConfigure(t *testing.T) {
	c := setupFileConnector(t, ".")
	err := c.Configure(map[string]string{"path": "/tmp"})
	require.NoError(t, err)
	assert.Equal(t, "/tmp", c.basePath)
}
