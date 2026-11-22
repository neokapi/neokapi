//go:build unix

package connector

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
)

// linkTree makes a connector root holding one real catalog, and beside it an
// outside directory holding a file the connector must never read or write.
func linkTree(t *testing.T) (base, outside, secret string) {
	t.Helper()
	root := t.TempDir()
	base = filepath.Join(root, "checkout")
	outside = filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "locales"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "locales", "en.json"), []byte(`{"greeting":"Hello"}`), 0o644))
	secret = filepath.Join(outside, "secret.json")
	require.NoError(t, os.WriteFile(secret, []byte(`{"secret":"outside the root"}`), 0o644))
	return base, outside, secret
}

func isEscape(err error) bool {
	var esc *rootpath.EscapeError
	return errors.As(err, &esc)
}

func TestFetchRefusesALinkOutOfTheRoot(t *testing.T) {
	base, _, secret := linkTree(t)
	require.NoError(t, os.Symlink(secret, filepath.Join(base, "locales", "fr.json")))
	c := setupFileConnector(t, base)

	t.Run("discovery skips the link", func(t *testing.T) {
		items, err := c.List(t.Context())
		require.NoError(t, err)
		var paths []string
		for _, it := range items {
			paths = append(paths, filepath.ToSlash(it.Path))
		}
		assert.Equal(t, []string{"locales/en.json"}, paths)

		fetched, err := c.Fetch(t.Context(), platconn.FetchOptions{})
		require.NoError(t, err)
		for _, it := range fetched {
			for _, b := range it.Blocks {
				assert.NotContains(t, b.SourceText(), "outside the root")
			}
		}
	})

	t.Run("a fetch that names the link is refused", func(t *testing.T) {
		_, err := c.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{"locales/fr.json"}})
		require.Error(t, err)
		assert.True(t, isEscape(err), "want an escape error, got %v", err)
		assert.NotContains(t, err.Error(), "secret.json", "the error never names where the link points")
	})

	t.Run("a fetch through a directory link is refused", func(t *testing.T) {
		require.NoError(t, os.Symlink(filepath.Dir(secret), filepath.Join(base, "shared")))
		_, err := c.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{"shared/secret.json"}})
		require.Error(t, err)
		assert.True(t, isEscape(err), "want an escape error, got %v", err)
	})
}

func TestFetchRefusesWhatIsNotARegularFile(t *testing.T) {
	base, _, _ := linkTree(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(base, "locales", "pipe.json"), 0o600))
	c := setupFileConnector(t, base)

	items, err := c.List(t.Context())
	require.NoError(t, err)
	for _, it := range items {
		assert.NotEqual(t, "pipe.json", it.Name, "a FIFO is not listed")
	}
	_, err = c.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{"locales/pipe.json"}})
	require.ErrorIs(t, err, rootpath.ErrNotRegular)
}

func TestFetchFollowsALinkThatStaysInside(t *testing.T) {
	base, _, _ := linkTree(t)
	require.NoError(t, os.Symlink("en.json", filepath.Join(base, "locales", "en-GB.json")))
	c := setupFileConnector(t, base)
	items, err := c.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{"locales/en-GB.json"}})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotEmpty(t, items[0].Blocks)
}

func publishItem(path string) *platconn.ContentItem {
	return &platconn.ContentItem{
		ID: path, Name: path, Path: path, Format: "json",
		Blocks: []*model.Block{model.NewBlock("greeting", "Bonjour")},
	}
}

func TestPublishRefusesALinkOutOfTheRoot(t *testing.T) {
	t.Run("a target file that links out", func(t *testing.T) {
		base, _, secret := linkTree(t)
		require.NoError(t, os.Symlink(secret, filepath.Join(base, "locales", "fr.json")))
		c := setupFileConnector(t, base)
		err := c.Publish(t.Context(), []*platconn.ContentItem{publishItem("locales/fr.json")}, platconn.PublishOptions{})
		require.Error(t, err)
		assert.True(t, isEscape(err), "want an escape error, got %v", err)
		b, _ := os.ReadFile(secret)
		assert.JSONEq(t, `{"secret":"outside the root"}`, string(b), "the outside file is untouched")
	})

	t.Run("a new file beneath a directory that links out", func(t *testing.T) {
		base, outside, _ := linkTree(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(base, "values-fr")))
		c := setupFileConnector(t, base)
		err := c.Publish(t.Context(), []*platconn.ContentItem{publishItem("values-fr/sub/strings.json")}, platconn.PublishOptions{})
		require.Error(t, err)
		assert.True(t, isEscape(err), "want an escape error, got %v", err)
		assert.NoDirExists(t, filepath.Join(outside, "sub"), "no directory is created through the link")
	})

	t.Run("a link that stays inside the root is written through", func(t *testing.T) {
		base, _, _ := linkTree(t)
		require.NoError(t, os.MkdirAll(filepath.Join(base, "real-fr"), 0o755))
		require.NoError(t, os.Symlink("real-fr", filepath.Join(base, "fr")))
		c := setupFileConnector(t, base)
		require.NoError(t, c.Publish(t.Context(), []*platconn.ContentItem{publishItem("fr/app.json")}, platconn.PublishOptions{}))
		assert.FileExists(t, filepath.Join(base, "real-fr", "app.json"))
	})

	t.Run("a root reached through a link", func(t *testing.T) {
		base, _, _ := linkTree(t)
		via := filepath.Join(t.TempDir(), "via")
		require.NoError(t, os.Symlink(base, via))
		c := setupFileConnector(t, via)
		require.NoError(t, c.Publish(t.Context(), []*platconn.ContentItem{publishItem("locales/de.json")}, platconn.PublishOptions{}))
		assert.FileExists(t, filepath.Join(base, "locales", "de.json"))
	})
}
