package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryReq sends one request through the router and returns the response.
func memoryReq(t *testing.T, s *Server, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	return rec
}

// promotedEntry sets up a project whose one block's translation was approved,
// so the workspace memory holds the pair, and returns a function that sends a
// decision on the block and the entry as the memory page lists it.
func promotedEntry(t *testing.T) (*Server, string, func(change.Outcome) int, MemoryEntryInfoResponse) {
	t.Helper()
	s, token := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p-mem", Name: "Memory", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"nb"}, WorkspaceID: "test-ws",
	}))
	blk := &model.Block{ID: "greeting", Translatable: true}
	blk.SetSourceText("Hello")
	blk.SetTargetText("nb", "Hei")
	require.NoError(t, cs.StoreItem(ctx, "p-mem", "main", &platstore.Item{Name: "en.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p-mem", "main", "en.json", []*model.Block{blk}))
	rows, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p-mem", Stream: "main", ItemName: "en.json", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	bid := rows[0].ID

	decideVia := func(outcome change.Outcome) int {
		t.Helper()
		sb, err := cs.GetBlock(ctx, "p-mem", "main", bid)
		require.NoError(t, err)
		body, err := json.Marshal(change.Set{Ops: []change.Op{
			decide(at("en.json", bid, "nb"), platstore.TargetRevision(sb, "nb"), outcome)}})
		require.NoError(t, err)
		return do(t, s, http.MethodPost, "/api/v1/test/projects/p-mem/streams/main/changes", token, string(body))
	}
	require.Equal(t, http.StatusOK, decideVia(change.OutcomeEstablish))

	entries := listMemory(t, s, token)
	require.Len(t, entries, 1)
	return s, token, decideVia, entries[0]
}

func listMemory(t *testing.T, s *Server, token string) []MemoryEntryInfoResponse {
	t.Helper()
	rec := memoryReq(t, s, http.MethodGet, "/api/v1/test/translation-memory?source_locale=en&target_locale=nb", token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var page MemorySearchResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	return page.Entries
}

func editMemory(t *testing.T, s *Server, token, id, target, base string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(MemoryUpdateRequest{
		Source: "Hello", Target: target, SourceLocale: "en", TargetLocale: "nb", BaseRevision: base,
	})
	require.NoError(t, err)
	return memoryReq(t, s, http.MethodPut, "/api/v1/test/translation-memory/"+id, token, string(body))
}

// TestRejectionKeepsAnEntryAPersonEdited is the second of the two ways the
// promotion lost a person's work: a rejection of the approved translation
// deleted the entry by its id, with whatever a person had since written in it.
// It now removes the approval's own origin, and the person's edit holds the
// entry.
func TestRejectionKeepsAnEntryAPersonEdited(t *testing.T) {
	s, token, decideVia, entry := promotedEntry(t)

	rec := editMemory(t, s, token, entry.ID, "Hallo", entry.Revision)
	require.Less(t, rec.Code, 300, rec.Body.String())

	require.Equal(t, http.StatusOK, decideVia(change.OutcomeReject))

	entries := listMemory(t, s, token)
	require.Len(t, entries, 1, "a rejection does not delete an entry a person edited")
	assert.Equal(t, "Hallo", entries[0].Target)
}

// TestMemoryEditIsBasedOnARevision: an edit names the revision it was made on,
// and the store refuses one based on an entry that changed since, one that
// names none, and one to an entry deleted since.
func TestMemoryEditIsBasedOnARevision(t *testing.T) {
	s, token, _, entry := promotedEntry(t)
	require.NotEmpty(t, entry.Revision, "every read carries the revision an edit names")

	rec := editMemory(t, s, token, entry.ID, "Hallo", entry.Revision)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var saved MemoryEntryInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &saved))
	assert.Equal(t, "Hallo", saved.Target)
	assert.NotEqual(t, entry.Revision, saved.Revision)

	// A second person who opened the entry before that edit is refused, and
	// told what the entry holds now.
	rec = editMemory(t, s, token, entry.ID, "Heisann", entry.Revision)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var changed struct {
		Code    string                  `json:"code"`
		Current MemoryEntryInfoResponse `json:"current"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &changed))
	assert.Equal(t, "memory_entry_changed", changed.Code)
	assert.Equal(t, "Hallo", changed.Current.Target)
	assert.Equal(t, saved.Revision, changed.Current.Revision)

	rec = editMemory(t, s, token, entry.ID, "Heisann", "")
	assert.Equal(t, http.StatusPreconditionRequired, rec.Code, "an edit that names no revision is refused")

	require.Equal(t, http.StatusNoContent,
		memoryReq(t, s, http.MethodDelete, "/api/v1/test/translation-memory/"+entry.ID, token, "").Code)
	rec = editMemory(t, s, token, entry.ID, "Heisann", saved.Revision)
	assert.Equal(t, http.StatusNotFound, rec.Code, "an edit never brings a deleted entry back")
	assert.Empty(t, listMemory(t, s, token))
}

// An entry the client named (one it made offline) is stored once under that
// id: the same add sent again is answered with the entry as stored, and an add
// of other text under the id is refused.
func TestAnAddThatNamesItsIDIsStoredOnce(t *testing.T) {
	s, token := newTestServer(t)
	add := func(target string) *httptest.ResponseRecorder {
		body, err := json.Marshal(MemoryAddRequest{ID: "offline-1", Source: "Hello", Target: target, SourceLocale: "en", TargetLocale: "nb"})
		require.NoError(t, err)
		return memoryReq(t, s, http.MethodPost, "/api/v1/test/translation-memory", token, string(body))
	}

	rec := add("Hei")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var first MemoryEntryInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
	assert.Equal(t, "offline-1", first.ID)
	assert.NotEmpty(t, first.Revision)

	rec = add("Hei")
	require.Equal(t, http.StatusOK, rec.Code, "the same add sent again stores nothing new: %s", rec.Body.String())
	var again MemoryEntryInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &again))
	assert.Equal(t, first.Revision, again.Revision)

	rec = add("Hallo")
	assert.Equal(t, http.StatusConflict, rec.Code, "other text never replaces the stored entry")
	entries := listMemory(t, s, token)
	require.Len(t, entries, 1)
	assert.Equal(t, "Hei", entries[0].Target)

	body, err := json.Marshal(MemoryAddRequest{ID: "not an id", Source: "a", Target: "b", SourceLocale: "en", TargetLocale: "nb"})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest,
		memoryReq(t, s, http.MethodPost, "/api/v1/test/translation-memory", token, string(body)).Code)
}

// Concurrent copies of one add (a replay racing a retry) over HTTP: one caller
// gets the created entry and every other the stored-once answer, never a 404.
func TestConcurrentCopiesOfOneAddStoreItOnce(t *testing.T) {
	s, token := newTestServer(t)
	// The workspace memory is opened as production opens it, on first use from
	// the database, so the first round races that opening too.
	pg, ok := s.ContentStore.(*bstore.PostgresStore)
	require.True(t, ok)
	s.wsStores.pgDB = pg.DB()
	s.wsStores.memoryFactory = nil
	srv := httptest.NewServer(s.GetEcho())
	t.Cleanup(srv.Close)

	for round := range 20 {
		entryID := fmt.Sprintf("offline-race-%d", round)
		body, err := json.Marshal(MemoryAddRequest{ID: entryID, Source: "Hello", Target: "Hei", SourceLocale: "en", TargetLocale: "nb"})
		require.NoError(t, err)

		const callers = 8
		codes := make([]int, callers)
		bodies := make([]string, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/test/translation-memory", strings.NewReader(string(body)))
				if err != nil {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					bodies[i] = err.Error()
					return
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				codes[i], bodies[i] = resp.StatusCode, string(b)
			})
		}
		wg.Wait()

		created := 0
		for i, code := range codes {
			switch code {
			case http.StatusCreated:
				created++
			case http.StatusOK:
			default:
				t.Fatalf("round %d caller %d: %d %s", round, i, code, bodies[i])
			}
		}
		assert.Equal(t, 1, created, "round %d: the entry is created once", round)
	}
}

// The first requests a server sees can arrive together. Each is answered by
// its route, never by a router whose routes are still being registered.
func TestTheFirstConcurrentRequestsReachTheirRoute(t *testing.T) {
	s, token := newTestServer(t)
	body, err := json.Marshal(MemoryAddRequest{ID: "first-race", Source: "Hello", Target: "Hei", SourceLocale: "en", TargetLocale: "nb"})
	require.NoError(t, err)
	const callers = 32
	codes := make([]int, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			codes[i] = memoryReq(t, s, http.MethodPost, "/api/v1/test/translation-memory", token, string(body)).Code
		})
	}
	wg.Wait()
	created := 0
	for i, code := range codes {
		require.Contains(t, []int{http.StatusCreated, http.StatusOK}, code, "caller %d", i)
		if code == http.StatusCreated {
			created++
		}
	}
	assert.Equal(t, 1, created)
}
