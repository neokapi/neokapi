package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaManagerVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/version", r.URL.Path)
		_, _ = w.Write([]byte(`{"version":"0.9.6"}`))
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	v, err := m.Version(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.9.6", v)
	assert.True(t, m.Reachable(context.Background()))
}

func TestOllamaManagerUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	m := NewOllamaManager(url)
	_, err := m.Version(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is it running")
	assert.False(t, m.Reachable(context.Background()))
}

func TestOllamaManagerListAndHas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []OllamaModelInfo{
				{Name: "llama3.2:3b", Size: 2019393189},
				{Name: "qwen3:latest", Size: 1400000000},
			},
		})
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	models, err := m.List(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, "llama3.2:3b", models[0].Name)

	has, err := m.Has(context.Background(), "llama3.2:3b")
	require.NoError(t, err)
	assert.True(t, has)

	// Unqualified name matches the :latest tag.
	has, err = m.Has(context.Background(), "qwen3")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = m.Has(context.Background(), "mistral")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestOllamaManagerPullStreamsProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/pull", r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "llama3.2:3b", body["name"])
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"status": "pulling manifest"})
		_ = enc.Encode(map[string]any{"status": "downloading", "digest": "sha256:abc", "total": 100, "completed": 50})
		_ = enc.Encode(map[string]any{"status": "downloading", "digest": "sha256:abc", "total": 100, "completed": 100})
		_ = enc.Encode(map[string]any{"status": "success"})
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	var statuses []string
	var lastCompleted int64
	err := m.Pull(context.Background(), "llama3.2:3b", func(p PullProgress) {
		statuses = append(statuses, p.Status)
		if p.Completed > 0 {
			lastCompleted = p.Completed
		}
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pulling manifest", "downloading", "downloading", "success"}, statuses)
	assert.Equal(t, int64(100), lastCompleted)
}

func TestOllamaManagerPullReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		_ = enc.Encode(map[string]any{"status": "pulling manifest"})
		_ = enc.Encode(map[string]any{"error": "file does not exist"})
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	err := m.Pull(context.Background(), "nope:1b", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file does not exist")
}

func TestOllamaManagerEnsureModelSkipsWhenPresent(t *testing.T) {
	var pullCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []OllamaModelInfo{{Name: "llama3.2:3b"}},
			})
		case "/api/pull":
			pullCalled = true
		}
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	pulled, err := m.EnsureModel(context.Background(), "llama3.2:3b", nil)
	require.NoError(t, err)
	assert.False(t, pulled)
	assert.False(t, pullCalled)
}
