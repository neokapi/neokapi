package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// These cover the editor-domain routes that answer server-side what the web
// surfaces used to assemble from every block in a file: the filtered blocks
// page, its counts, one item's metadata, and the two batch actions.

// newEditorBulkServer builds a Server over a SQLite content store with an
// in-memory workspace content memory — no container needed.
func newEditorBulkServer(t *testing.T) (*Server, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/store.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	srv.wsStores.memoryFactory = func() memory.Store { return memory.NewInMemoryStore() }
	return srv, cs
}

// bulkTestBlocks is the seeded corpus, one block per status bucket plus a
// non-translatable one, keyed by source text.
func seedEditorBulkProject(t *testing.T, cs *sqlitestore.SQLiteStore) (string, map[string]string) {
	t.Helper()
	ctx := t.Context()
	proj := &platstore.Project{
		ID: "p-bulk", Name: "Bulk", DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"}, WorkspaceID: "ws-1",
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "greetings.txt", Format: "txt", ItemType: "file",
	}))

	mk := func(id, source, target string, status model.TargetStatus, translatable bool) *model.Block {
		b := model.NewBlock(id, source)
		b.Translatable = translatable
		if target != "" {
			b.SetTargetText("fr", target)
			b.StampTargetProvenance("fr", status, model.Origin{Kind: model.OriginAI})
		}
		return b
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "greetings.txt", []*model.Block{
		mk("tu1", "Hello", "Bonjour", model.TargetStatusTranslated, true),
		mk("tu2", "Goodbye", "Au revoir", model.TargetStatusDraft, true),
		mk("tu3", "Thanks", "Merci", model.TargetStatusReviewed, true),
		mk("tu4", "Untranslated", "", model.TargetStatusNew, true),
		mk("tu5", "Frozen", "", model.TargetStatusNew, false),
	}))

	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: "greetings.txt"})
	require.NoError(t, err)
	ids := make(map[string]string, len(stored))
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}
	return proj.ID, ids
}

// editorCall invokes a handler the way the router would, with the route params
// the editor group binds and full project permissions.
func editorCall(t *testing.T, srv *Server, method, pid, path, body string, h echo.HandlerFunc) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	c.Set("project_permissions", platauth.PermTranslate|platauth.PermReview|platauth.PermViewContent)
	return rec, h(c)
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// The blocks route narrows server-side: the surfaces stop downloading a file
// to filter it in the browser.
func TestHandleGetFileBlocks_Filters(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, _ := seedEditorBulkProject(t, cs)

	base := "/api/v1/acme/" + pid + "/blocks/main?item=greetings.txt&locale=fr"

	t.Run("status keeps one bucket", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid, base+"&status=draft", "", srv.HandleGetFileBlocks)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		blocks := decodeJSON[[]BlockInfoResponse](t, rec)
		require.Len(t, blocks, 1)
		assert.Equal(t, "Goodbye", blocks[0].Source)
	})

	t.Run("not-started is the untranslated locale", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid, base+"&status=not-started&translatable=true", "", srv.HandleGetFileBlocks)
		require.NoError(t, err)
		blocks := decodeJSON[[]BlockInfoResponse](t, rec)
		require.Len(t, blocks, 1)
		assert.Equal(t, "Untranslated", blocks[0].Source)
	})

	t.Run("q searches source and target", func(t *testing.T) {
		for q, want := range map[string]string{"THANK": "Thanks", "revoir": "Goodbye"} {
			rec, err := editorCall(t, srv, http.MethodGet, pid,
				base+"&q="+url.QueryEscape(q), "", srv.HandleGetFileBlocks)
			require.NoError(t, err)
			blocks := decodeJSON[[]BlockInfoResponse](t, rec)
			require.Len(t, blocks, 1, "q=%q", q)
			assert.Equal(t, want, blocks[0].Source)
		}
	})

	t.Run("translatable=false keeps the frozen block", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid, base+"&translatable=false", "", srv.HandleGetFileBlocks)
		require.NoError(t, err)
		blocks := decodeJSON[[]BlockInfoResponse](t, rec)
		require.Len(t, blocks, 1)
		assert.Equal(t, "Frozen", blocks[0].Source)
	})

	t.Run("an unknown status is a 400, not an empty page", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid, base+"&status=approved", "", srv.HandleGetFileBlocks)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("a status without a locale is a 400", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid,
			"/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt&status=draft", "", srv.HandleGetFileBlocks)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// The progress bar is one query, not a file's worth of blocks.
func TestHandleGetBlockCounts(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, _ := seedEditorBulkProject(t, cs)

	rec, err := editorCall(t, srv, http.MethodGet, pid,
		"/api/v1/acme/"+pid+"/blocks/main/counts?item=greetings.txt&locale=fr", "", srv.HandleGetBlockCounts)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	counts := decodeJSON[BlockCountsResponse](t, rec)
	assert.Equal(t, 5, counts.Total)
	assert.Equal(t, 4, counts.Translatable)
	assert.Equal(t, "fr", counts.Locale)
	assert.Equal(t, BlockStatusCountsResponse{NotStarted: 1, Draft: 1, Translated: 1, Reviewed: 1}, counts.Status)
}

// Three UI routes wanted one item's name; this route answers without the
// project's whole item list.
func TestHandleGetItem(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, _ := seedEditorBulkProject(t, cs)

	rec, err := editorCall(t, srv, http.MethodGet, pid,
		"/api/v1/acme/"+pid+"/items/main/one?item=greetings.txt", "", srv.HandleGetItem)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	item := decodeJSON[ItemInfoResponse](t, rec)
	assert.Equal(t, "greetings.txt", item.Name)
	assert.Equal(t, "txt", item.Format)
	assert.Equal(t, 5, item.BlockCount)
	assert.Equal(t, 4, item.Translatable)

	t.Run("an unknown item is a 404", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid,
			"/api/v1/acme/"+pid+"/items/main/one?item=absent.txt", "", srv.HandleGetItem)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("no item name is a 400", func(t *testing.T) {
		rec, err := editorCall(t, srv, http.MethodGet, pid,
			"/api/v1/acme/"+pid+"/items/main/one", "", srv.HandleGetItem)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// The batch carries the single-block route's semantics exactly, and a block
// that refuses is recorded against itself rather than failing the request.
func TestHandleBulkReviewBlocks(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, ids := seedEditorBulkProject(t, cs)
	ctx := t.Context()

	body, err := json.Marshal(BulkReviewRequest{
		BlockIDs:     []string{ids["Hello"], ids["Goodbye"], ids["Untranslated"], "no-such-block"},
		TargetLocale: "fr",
		Approve:      true,
		ItemName:     "greetings.txt",
	})
	require.NoError(t, err)

	rec, err := editorCall(t, srv, http.MethodPost, pid,
		"/api/v1/acme/"+pid+"/blocks/main/bulk-review", string(body), srv.HandleBulkReviewBlocks)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSON[BulkReviewResponse](t, rec)
	assert.Equal(t, 2, resp.Succeeded)
	assert.Equal(t, 2, resp.Failed)
	require.Len(t, resp.Results, 4)
	assert.True(t, resp.Results[0].OK)
	assert.Equal(t, "reviewed", resp.Results[0].Status)
	assert.True(t, resp.Results[1].OK)
	// An untranslated block is the single-block route's 422, per block.
	assert.False(t, resp.Results[2].OK)
	assert.Contains(t, resp.Results[2].Error, "no fr translation to review")
	assert.False(t, resp.Results[3].OK)
	assert.Contains(t, resp.Results[3].Error, "block not found")

	// The approvals are on the target ladder, where coverage reads them.
	for _, source := range []string{"Hello", "Goodbye"} {
		sb, err := cs.GetBlock(ctx, pid, "main", ids[source])
		require.NoError(t, err)
		assert.Equal(t, model.TargetStatusReviewed, sb.Block.Target("fr").Status, source)
	}
	// The untranslated block gained nothing — no empty target was written.
	sb, err := cs.GetBlock(ctx, pid, "main", ids["Untranslated"])
	require.NoError(t, err)
	assert.Nil(t, sb.Block.Target("fr"))
}

// A rejection demotes to draft, re-opening the work, exactly as the
// single-block route's status:"draft" does.
func TestHandleBulkReviewBlocks_RejectDemotesToDraft(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, ids := seedEditorBulkProject(t, cs)

	body, err := json.Marshal(BulkReviewRequest{
		BlockIDs:     []string{ids["Thanks"]},
		TargetLocale: "fr",
		Approve:      false,
		Status:       "draft",
		Comment:      "Wrong register",
	})
	require.NoError(t, err)

	rec, err := editorCall(t, srv, http.MethodPost, pid,
		"/api/v1/acme/"+pid+"/blocks/main/bulk-review", string(body), srv.HandleBulkReviewBlocks)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeJSON[BulkReviewResponse](t, rec)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "draft", resp.Results[0].Status)

	sb, err := cs.GetBlock(t.Context(), pid, "main", ids["Thanks"])
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, sb.Block.Target("fr").Status)

	// The comment rides along as a block note.
	notes, err := cs.ListBlockNotes(t.Context(), pid, "main", ids["Thanks"])
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "Wrong register", notes[0].Text)
}

func TestHandleBulkReviewBlocks_Validation(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, _ := seedEditorBulkProject(t, cs)

	cases := map[string]string{
		"no locale":            `{"block_ids":["x"],"approve":true}`,
		"no blocks":            `{"target_locale":"fr","approve":true}`,
		"unknown demotion":     `{"block_ids":["x"],"target_locale":"fr","approve":false,"status":"published"}`,
		"status with approval": `{"block_ids":["x"],"target_locale":"fr","approve":true,"status":"draft"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec, err := editorCall(t, srv, http.MethodPost, pid,
				"/api/v1/acme/"+pid+"/blocks/main/bulk-review", body, srv.HandleBulkReviewBlocks)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// One request replaces the surface's lookup-then-update loop, and the write
// follows the same rule an ordinary edit does.
func TestHandleBulkApplyMemory(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, ids := seedEditorBulkProject(t, cs)
	ctx := t.Context()

	tm, err := srv.wsStores.getMemory("acme")
	require.NoError(t, err)
	require.NoError(t, tm.Add(ctx, memory.Entry{
		ID: "seed-untranslated",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Untranslated"}}},
			"fr": {{Text: &model.TextRun{Text: "Non traduit"}}},
		},
		HintSrcLang: "en",
	}))

	body, err := json.Marshal(BulkApplyMemoryRequest{
		BlockIDs:     []string{ids["Untranslated"], ids["Hello"], ids["Frozen"], "no-such-block"},
		TargetLocale: "fr",
	})
	require.NoError(t, err)

	rec, err := editorCall(t, srv, http.MethodPost, pid,
		"/api/v1/acme/"+pid+"/blocks/main/bulk-apply-memory", string(body), srv.HandleBulkApplyMemory)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSON[BulkApplyMemoryResponse](t, rec)
	require.Len(t, resp.Applied, 1)
	assert.Equal(t, ids["Untranslated"], resp.Applied[0].BlockID)
	assert.Equal(t, "Non traduit", resp.Applied[0].Text)
	assert.GreaterOrEqual(t, resp.Applied[0].Score, 1.0)

	reasons := map[string]string{}
	for _, s := range resp.Skipped {
		reasons[s.BlockID] = s.Reason
	}
	assert.Equal(t, "no match at or above the threshold", reasons[ids["Hello"]])
	assert.Equal(t, "block is not translatable", reasons[ids["Frozen"]])
	assert.Equal(t, "block not found", reasons["no-such-block"])

	sb, err := cs.GetBlock(ctx, pid, "main", ids["Untranslated"])
	require.NoError(t, err)
	assert.Equal(t, "Non traduit", sb.Block.TargetText("fr"))
}

// An empty content memory skips every block by name rather than failing.
func TestHandleBulkApplyMemory_EmptyMemory(t *testing.T) {
	srv, cs := newEditorBulkServer(t)
	pid, ids := seedEditorBulkProject(t, cs)

	body, err := json.Marshal(BulkApplyMemoryRequest{
		BlockIDs: []string{ids["Untranslated"]}, TargetLocale: "fr",
	})
	require.NoError(t, err)

	rec, err := editorCall(t, srv, http.MethodPost, pid,
		"/api/v1/acme/"+pid+"/blocks/main/bulk-apply-memory", string(body), srv.HandleBulkApplyMemory)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeJSON[BulkApplyMemoryResponse](t, rec)
	assert.Empty(t, resp.Applied)
	require.Len(t, resp.Skipped, 1)
	assert.Equal(t, "content memory is empty", resp.Skipped[0].Reason)
}
