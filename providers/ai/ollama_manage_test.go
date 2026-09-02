package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/httputil"
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
	assert.Contains(t, err.Error(), "Is it running")
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
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
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

// A pull streams for as long as the download takes — minutes on any real model.
// Client.Timeout bounds the whole exchange including the body, so the management
// client's cannot apply here; the pull runs on a client with no timeout of its
// own and is bounded by the caller's context.
func TestOllamaManagerPullIsNotBoundedByTheManagementTimeout(t *testing.T) {
	assert.Equal(t, httputil.DefaultTimeout, NewOllamaManager("").client.Timeout,
		"the management endpoints keep a short timeout")
	assert.Zero(t, NewOllamaManager("").pullClient.Timeout,
		"a multi-gigabyte download has no wall-clock cap of its own")

	// A server that streams frames further apart than the management timeout
	// would allow. The pull completes only because it does not use that client.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		flusher, _ := w.(http.Flusher)
		for _, status := range []string{"pulling manifest", "downloading", "success"} {
			_ = enc.Encode(map[string]any{"status": status})
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-time.After(20 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer srv.Close()

	m := NewOllamaManager(srv.URL)
	m.client.Timeout = 10 * time.Millisecond

	var statuses []string
	err := m.Pull(context.Background(), "llama3.2:3b", func(p PullProgress) {
		statuses = append(statuses, p.Status)
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pulling manifest", "downloading", "success"}, statuses)
}

// No timeout does not mean no bound: the caller's context still stops a pull.
func TestOllamaManagerPullHonoursContextCancellation(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "downloading"})
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	m := NewOllamaManager(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := m.Pull(ctx, "llama3.2:3b", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
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
