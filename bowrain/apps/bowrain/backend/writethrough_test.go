package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// itemOpServer wires an App to a routed httptest server in the connected state.
// The handler dispatches on method+path so a single server can answer the
// upload, remove, pseudo/tm-translate, term-enforce, and blocks-refresh calls a
// single desktop op makes. hits records every "METHOD path" the server saw.
func itemOpServer(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) (*App, *[]string) {
	t.Helper()
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		hits = append(hits, key)
		// Longest-suffix match so callers key on the trailing route segment.
		for suffix, fn := range routes {
			parts := strings.SplitN(suffix, " ", 2)
			if len(parts) == 2 && r.Method == parts[0] && strings.HasSuffix(r.URL.Path, parts[1]) {
				fn(w, r)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.remoteHTTP = apiclient.NewEditorClient(srv.URL, "tok-xyz")
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok-xyz"}
	app.mu.Unlock()
	return app, &hits
}

func hitsContain(hits []string, method, suffix string) bool {
	for _, h := range hits {
		if strings.HasPrefix(h, method+" ") && strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

func TestAddItemsOnlineUploadsToServer(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /items/main": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorProject{ID: "p1", Name: "Proj"})
		},
	})

	dir := t.TempDir()
	writeTestFile(t, dir, "hello.txt", "Hello World")

	info, err := app.AddItems("p1", []string{dir + "/hello.txt"})
	require.NoError(t, err)
	assert.Equal(t, "p1", info.ID)
	assert.True(t, hitsContain(*hits, "POST", "/items/main"), "upload should hit server; hits=%v", *hits)
}

func TestAddItemsOfflineEnqueues(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	dir := t.TempDir()
	writeTestFile(t, dir, "hello.txt", "Hello World")
	_, err = app.AddItems(proj.ID, []string{dir + "/hello.txt"})
	require.NoError(t, err)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "add_items", changes[0].Operation)

	// Local cache was seeded so an offline reader sees the item.
	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)
}

func TestRemoveItemOnlineHitsServer(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"DELETE /items/main": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorProject{ID: "p1", Name: "Proj"})
		},
	})

	info, err := app.RemoveItem("p1", "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "p1", info.ID)
	assert.True(t, hitsContain(*hits, "DELETE", "/items/main"), "remove should hit server; hits=%v", *hits)
}

func TestRemoveItemOfflineEnqueues(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)
	dir := t.TempDir()
	writeTestFile(t, dir, "hello.txt", "Hello World")
	_, err = app.addItemsLocal(t.Context(), proj.ID, []string{dir + "/hello.txt"})
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	_, err = app.RemoveItem(proj.ID, "hello.txt")
	require.NoError(t, err)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "remove_item", changes[0].Operation)
}

func TestPseudoTranslateItemOnlineHitsServer(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /actions/main/pseudo-translate": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorTranslationStats{TotalBlocks: 3, TranslatedBlocks: 3, WordCount: 9})
		},
	})

	stats, err := app.PseudoTranslateItem("p1", "hello.txt", "fr")
	require.NoError(t, err)
	assert.Equal(t, 3, stats.TotalBlocks)
	assert.Equal(t, 3, stats.TranslatedBlocks)
	assert.True(t, hitsContain(*hits, "POST", "/actions/main/pseudo-translate"), "hits=%v", *hits)
}

func TestPseudoTranslateItemOfflineEnqueues(t *testing.T) {
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	proj, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)
	dir := t.TempDir()
	writeTestFile(t, dir, "hello.txt", "Hello World")
	_, err = app.addItemsLocal(t.Context(), proj.ID, []string{dir + "/hello.txt"})
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	stats, err := app.PseudoTranslateItem(proj.ID, "hello.txt", "fr")
	require.NoError(t, err)
	assert.Positive(t, stats.TotalBlocks)

	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "pseudo_translate_item", changes[0].Operation)

	// Local pseudo-translation was applied to the cache.
	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	assert.Contains(t, flattenTargetRuns(blocks[0], "fr"), "[")
}

func TestMemoryTranslateItemOnlineHitsServer(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /actions/main/tm-translate": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorTranslationStats{TotalBlocks: 2, TranslatedBlocks: 1, WordCount: 4})
		},
	})

	stats, err := app.MemoryTranslateItem("p1", "hello.txt", "fr")
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalBlocks)
	assert.True(t, hitsContain(*hits, "POST", "/actions/main/tm-translate"), "hits=%v", *hits)
}

func TestTermEnforceItemOnlineHitsServer(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /actions/main/term-enforce": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode([]apiclient.EditorTermEnforceResult{{
				BlockID: "b1", SourceTerm: "software", ConceptID: "c1",
				Expected: []string{"logiciel"}, SourceText: "software",
				TargetText: "programme", SourceLocale: "en", TargetLocale: "fr",
			}})
		},
	})

	results, err := app.TermEnforceItem("p1", "hello.txt", "fr")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "software", results[0].SourceTerm)
	assert.Equal(t, []string{"logiciel"}, results[0].Expected)
	assert.True(t, hitsContain(*hits, "POST", "/actions/main/term-enforce"), "hits=%v", *hits)
}

// TestReplayItemActions verifies the reconnect replay dispatches each queued
// item op to its core/client method.
func TestReplayItemActions(t *testing.T) {
	app, hits := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /items/main": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorProject{ID: "p1"})
		},
		"DELETE /items/main": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorProject{ID: "p1"})
		},
		"POST /actions/main/pseudo-translate": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorTranslationStats{})
		},
		"POST /actions/main/tm-translate": func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(apiclient.EditorTranslationStats{})
		},
	})

	ctx := t.Context()
	require.NoError(t, app.replayChange(ctx, PendingChange{Operation: "add_items", Payload: `{"project_id":"p1","files":{"a.txt":"aGk="}}`}))
	require.NoError(t, app.replayChange(ctx, PendingChange{Operation: "remove_item", Payload: `{"project_id":"p1","item_name":"a.txt"}`}))
	require.NoError(t, app.replayChange(ctx, PendingChange{Operation: "pseudo_translate_item", Payload: `{"project_id":"p1","item_name":"a.txt","target_locale":"fr"}`}))
	require.NoError(t, app.replayChange(ctx, PendingChange{Operation: "tm_translate_item", Payload: `{"project_id":"p1","item_name":"a.txt","target_locale":"fr"}`}))

	assert.True(t, hitsContain(*hits, "POST", "/items/main"))
	assert.True(t, hitsContain(*hits, "DELETE", "/items/main"))
	assert.True(t, hitsContain(*hits, "POST", "/actions/main/pseudo-translate"))
	assert.True(t, hitsContain(*hits, "POST", "/actions/main/tm-translate"))
}

// TestPermanentRejectionSurfacesToCaller verifies a connected mutation the
// server rejects outright (permanent 4xx — e.g. a 422 review of a block whose
// translation was deleted concurrently) is returned to the caller as-is: the
// app stays connected, nothing is enqueued for replay (retrying the identical
// request can never succeed), and the local cache is not touched.
func TestPermanentRejectionSurfacesToCaller(t *testing.T) {
	app, _ := itemOpServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"PUT /review": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"block has no fr translation to review"}`))
		},
	})
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	err := app.ReviewBlock("p1", "hello.txt", "b1", "fr", true, "")
	require.Error(t, err)
	var statusErr *apiclient.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusUnprocessableEntity, statusErr.StatusCode)

	assert.Equal(t, StateConnected, app.GetConnectionState().State,
		"a semantic rejection is not a connectivity failure")
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	assert.Empty(t, changes, "a permanently rejected op must not be queued for replay")
}

// TestTransportFailureStillGoesOffline locks the other half of the split the
// permanent-rejection path introduces: a connected mutation that fails at the
// transport level (server unreachable) still drops to offline mode and
// enqueues the op for replay.
func TestTransportFailureStillGoesOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	app := newTestApp(t)
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.remoteHTTP = apiclient.NewEditorClient(srv.URL, "tok-xyz")
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok-xyz"}
	app.mu.Unlock()
	srv.Close() // connection refused from here on

	// The local fallback also errors (no such cached block) — the assertions
	// below are about the offline transition and the queued replay op.
	_ = app.ReviewBlock("p1", "hello.txt", "b1", "fr", true, "")

	assert.Equal(t, StateOffline, app.GetConnectionState().State)
	changes, err := q.PeekPending(10)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "review_block", changes[0].Operation)
}
