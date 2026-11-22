package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
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
