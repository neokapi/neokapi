package connector

import (
	"os"
	"path/filepath"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escapingPaths are the item paths a delivery must refuse to resolve. They are
// content — an item's stored Name or Path — not configuration the host chose.
var escapingPaths = []struct {
	name string
	path string
}{
	{"parent traversal", "../escaped.json"},
	{"deep traversal", "../../../../tmp/escaped.json"},
	{"traversal below a real directory", "locales/../../escaped.json"},
	{"absolute path", "/tmp/escaped.json"},
	{"absolute path with traversal", "/../../tmp/escaped.json"},
	{"traversal with a trailing segment", "..//../escaped.json"},
}

// TestPublishStaysInsideTheConnectorRoot pins that delivery writes only within
// the checkout the connector was pointed at. The target path comes from stored
// content, and MkdirAll would otherwise create whatever directories were needed
// to reach a location outside it.
func TestPublishStaysInsideTheConnectorRoot(t *testing.T) {
	for _, tc := range escapingPaths {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "checkout")
			require.NoError(t, os.MkdirAll(base, 0o755))

			c := setupFileConnector(t, base)

			b := model.NewBlock("greeting", "Hello")
			item := &platconn.ContentItem{
				ID:     tc.path,
				Name:   tc.path,
				Path:   tc.path,
				Format: "json",
				Blocks: []*model.Block{b},
			}

			err := c.Publish(t.Context(), []*platconn.ContentItem{item}, platconn.PublishOptions{})
			require.Error(t, err, "delivery outside the connector root must fail")

			// Nothing was created anywhere above the root.
			assertNoFilesOutside(t, root, base)
		})
	}
}

// TestPublishStillWritesNestedPaths guards the fix against over-tightening:
// a nested, relative target is the ordinary case and must keep working,
// including creating the directories it needs.
func TestPublishStillWritesNestedPaths(t *testing.T) {
	base := t.TempDir()
	c := setupFileConnector(t, base)

	b := model.NewBlock("greeting", "Hello")
	item := &platconn.ContentItem{
		ID:     "locales/fr/messages.json",
		Name:   "locales/en/messages.json",
		Path:   "locales/fr/messages.json",
		Format: "json",
		Blocks: []*model.Block{b},
	}

	require.NoError(t, c.Publish(t.Context(), []*platconn.ContentItem{item}, platconn.PublishOptions{}))
	assert.FileExists(t, filepath.Join(base, "locales", "fr", "messages.json"))
}

// TestFetchStaysInsideTheConnectorRoot covers the read direction, where the
// same join happens against caller-supplied paths.
func TestFetchStaysInsideTheConnectorRoot(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "checkout")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.html"),
		[]byte("<html><body><p>secret</p></body></html>"), 0o600))

	c := setupFileConnector(t, base)

	_, err := c.Fetch(t.Context(), platconn.FetchOptions{Paths: []string{"../secret.html"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the connector root")
}

// TestResolveSourcePathStaysInsideTheConnectorRoot covers the faithful-delivery
// re-parse, which reads a source document chosen by item metadata. An absolute
// source_path used to be honoured outright, so metadata could name any readable
// file on the host and have its content parsed into the delivery.
func TestResolveSourcePathStaysInsideTheConnectorRoot(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "checkout")
	require.NoError(t, os.MkdirAll(base, 0o755))

	outside := filepath.Join(root, "outside.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"k":"v"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, "en.json"), []byte(`{"k":"v"}`), 0o600))

	c := setupFileConnector(t, base)

	tests := []struct {
		name string
		item *platconn.ContentItem
		want string
	}{
		{
			name: "absolute source_path is refused",
			item: &platconn.ContentItem{Metadata: map[string]string{"source_path": outside}},
			want: "",
		},
		{
			name: "relative traversal in source_path is refused",
			item: &platconn.ContentItem{Metadata: map[string]string{"source_path": "../outside.json"}},
			want: "",
		},
		{
			name: "traversal in the item name is refused",
			item: &platconn.ContentItem{Name: "../outside.json"},
			want: "",
		},
		{
			name: "a contained source_path still resolves",
			item: &platconn.ContentItem{Metadata: map[string]string{"source_path": "en.json"}},
			want: filepath.Join(base, "en.json"),
		},
		{
			name: "the item name is still the fallback",
			item: &platconn.ContentItem{Name: "en.json"},
			want: filepath.Join(base, "en.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.resolveSourcePath(tt.item))
		})
	}
}

// assertNoFilesOutside fails if anything exists under root other than within
// the allowed subtree.
func assertNoFilesOutside(t *testing.T, root, allowed string) {
	t.Helper()
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(allowed, p)
		require.NoError(t, relErr)
		assert.NotContains(t, rel, "..", "file %q was written outside the connector root", p)
		return nil
	})
	require.NoError(t, err)
}
