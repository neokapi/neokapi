package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWordPressServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	posts := []wpPost{
		{
			ID:   1,
			Slug: "hello-world",
			Title:   wpContent{Rendered: "Hello World"},
			Content: wpContent{Rendered: "<p>Welcome to WordPress.</p>"},
			Excerpt: wpContent{Rendered: "A welcome post"},
			Modified: "2024-01-15T10:30:00",
		},
		{
			ID:   2,
			Slug: "sample-page",
			Title:   wpContent{Rendered: "Sample Page"},
			Content: wpContent{Rendered: "<p>This is a sample page.</p>"},
			Modified: "2024-01-16T08:00:00",
		},
	}

	mux.HandleFunc("/wp-json/wp/v2/posts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(posts)
	})

	mux.HandleFunc("/wp-json/wp/v2/posts/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(posts[0])
	})

	return httptest.NewServer(mux)
}

func TestWordPressPull(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, CategoryCMS, c.Category())

	items, err := c.Pull(context.Background(), PullOptions{})
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "Hello World", items[0].Name)
	assert.GreaterOrEqual(t, len(items[0].Blocks), 2) // title + content
	assert.Equal(t, "posts/hello-world", items[0].Path)
}

func TestWordPressPush(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": srv.URL,
	})
	require.NoError(t, err)

	items, err := c.Pull(context.Background(), PullOptions{})
	require.NoError(t, err)

	err = c.Push(context.Background(), items[:1], PushOptions{})
	require.NoError(t, err)
}

func TestWordPressSync(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": srv.URL,
	})
	require.NoError(t, err)

	status, err := c.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, status.ItemCount)
}
