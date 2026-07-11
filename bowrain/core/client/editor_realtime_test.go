package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewBlock(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"PUT /api/v1/acme/p1/blocks/main/b1/review": {200, `{"ok":true,"block_id":"b1","reviewed":true}`},
	})

	err := c.ReviewBlock(context.Background(), "acme", "p1", "a.json", "b1", "fr", true, "")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, got.method)
	assert.Equal(t, "/api/v1/acme/p1/blocks/main/b1/review", got.path)

	var body map[string]any
	require.NoError(t, json.Unmarshal(got.body, &body))
	assert.Equal(t, "fr", body["target_locale"])
	assert.Equal(t, "a.json", body["item_name"])
	assert.Equal(t, true, body["reviewed"])
	_, hasStatus := body["status"]
	assert.False(t, hasStatus, "empty status must be omitted from the wire body")
	assert.Equal(t, "Bearer tok", got.auth)
}

// TestReviewBlockReject: a reviewer rejection travels as reviewed=false plus
// status:"draft" — the rung the server demotes the target to so the unit
// re-enters the work queue.
func TestReviewBlockReject(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"PUT /api/v1/acme/p1/blocks/main/b1/review": {200, `{"ok":true,"block_id":"b1","reviewed":false}`},
	})

	err := c.ReviewBlock(context.Background(), "acme", "p1", "a.json", "b1", "fr", false, "draft")
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(got.body, &body))
	assert.Equal(t, false, body["reviewed"])
	assert.Equal(t, "draft", body["status"])
}

func TestReportPresence(t *testing.T) {
	c, got := editorServer(t, map[string]route{
		"POST /api/v1/acme/p1/presence": {204, ""},
	})

	err := c.ReportPresence(context.Background(), "acme", "p1", "a.json", "b1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "/api/v1/acme/p1/presence", got.path)

	var body map[string]any
	require.NoError(t, json.Unmarshal(got.body, &body))
	assert.Equal(t, "a.json", body["item_name"])
	assert.Equal(t, "b1", body["block_id"])
}

func TestStreamProjectEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/acme/events", r.URL.Path)
		assert.Equal(t, "p1", r.URL.Query().Get("project"))
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		fmt.Fprint(w, ": connected\n\n")
		fmt.Fprint(w, "event: change\ndata: {\"type\":\"editor.block.updated\",\"blockId\":\"b1\",\"itemName\":\"a.json\"}\n\n")
		fmt.Fprint(w, "event: change\ndata: {\"type\":\"editor.presence.moved\",\"userId\":\"u1\",\"userName\":\"Alice\"}\n\n")
		if fl != nil {
			fl.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	c := NewEditorClient(srv.URL, "tok")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var got []EditorChangeEvent
	err := c.StreamProjectEvents(ctx, "acme", "p1", func(ev EditorChangeEvent) {
		got = append(got, ev)
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "editor.block.updated", got[0].Type)
	assert.Equal(t, "b1", got[0].BlockID)
	assert.Equal(t, "editor.presence.moved", got[1].Type)
	assert.Equal(t, "u1", got[1].UserID)
	assert.Equal(t, "Alice", got[1].UserName)
}

func TestStreamProjectEventsBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	t.Cleanup(srv.Close)

	c := NewEditorClient(srv.URL, "tok")
	err := c.StreamProjectEvents(context.Background(), "acme", "p1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}
