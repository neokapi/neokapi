package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase4_RevertBatch proves a whole batch of changes (grouped by correlation
// id) can be reverted to the pre-batch state across multiple blocks.
func TestPhase4_RevertBatch(t *testing.T) {
	s, _ := newTestServer(t)
	cs := s.ContentStore
	fr := model.LocaleID("fr")
	ctx := t.Context()

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rev", Name: "Rev", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{fr}}))
	require.NoError(t, cs.StoreItem(ctx, "p-rev", "main", &platstore.Item{Name: "en.json", Format: "json"}))

	mk := func(id, src string) *model.Block {
		return model.NewBlock(id, src)
	}

	// Initial state (correlation c0): two blocks translated.
	b1, b2 := mk("b1", "one"), mk("b2", "two")
	b1.SetTargetText(fr, "un-v0")
	b2.SetTargetText(fr, "deux-v0")
	ctx0 := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "c0"})
	require.NoError(t, cs.StoreBlocksForItem(ctx0, "p-rev", "main", "en.json", []*model.Block{b1, b2}))
	id1, id2 := rowIDOf(t, cs, "p-rev", "b1"), rowIDOf(t, cs, "p-rev", "b2")

	// A batch (correlation BATCH1) re-translates both.
	b1.SetTargetText(fr, "un-v1")
	b2.SetTargetText(fr, "deux-v1")
	ctxB := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "BATCH1"})
	b1.ID, b2.ID = id1, id2
	require.NoError(t, cs.StoreBlocksForItem(ctxB, "p-rev", "main", "en.json", []*model.Block{b1, b2}))

	// Sanity: both are at v1.
	sb1, _ := cs.GetBlock(ctx, "p-rev", "main", id1)
	require.Equal(t, "un-v1", sb1.TargetText(fr))

	// Revert the batch via the handler.
	e := s.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"correlation_id":"BATCH1","stream":"main"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id")
	c.SetParamValues("p-rev")
	require.NoError(t, s.HandleRevertBatch(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Both blocks restored to their pre-batch (v0) values.
	sb1, _ = cs.GetBlock(ctx, "p-rev", "main", id1)
	sb2, _ := cs.GetBlock(ctx, "p-rev", "main", id2)
	assert.Equal(t, "un-v0", sb1.TargetText(fr))
	assert.Equal(t, "deux-v0", sb2.TargetText(fr))
}

// TestPhase4_RevertBatchClearsAdded proves a target first created by the batch
// is blanked on revert (no prior value).
func TestPhase4_RevertBatchClearsAdded(t *testing.T) {
	s, _ := newTestServer(t)
	cs := s.ContentStore
	fr := model.LocaleID("fr")
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rev2", Name: "Rev2", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{fr}}))
	require.NoError(t, cs.StoreItem(ctx, "p-rev2", "main", &platstore.Item{Name: "en.json", Format: "json"}))

	b := model.NewBlock("bx", "hi")
	b.SetTargetText(fr, "added-in-batch")
	ctxB := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "ADD1"})
	require.NoError(t, cs.StoreBlocksForItem(ctxB, "p-rev2", "main", "en.json", []*model.Block{b}))

	e := s.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"correlation_id":"ADD1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id")
	c.SetParamValues("p-rev2")
	require.NoError(t, s.HandleRevertBatch(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	sb, _ := cs.GetBlock(ctx, "p-rev2", "main", rowIDOf(t, cs, "p-rev2", "bx"))
	assert.False(t, holdsTarget(sb.Block, fr), "a batch-added target is removed on revert")
}

// TestPhase4_RestoreToVersion proves point-in-time restore: a stream is rolled
// back to the state captured at a named version.
func TestPhase4_RestoreToVersion(t *testing.T) {
	s, _ := newTestServer(t)
	cs := s.ContentStore
	fr := model.LocaleID("fr")
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-pit", Name: "PIT", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{fr}}))
	require.NoError(t, cs.StoreItem(ctx, "p-pit", "main", &platstore.Item{Name: "en.json", Format: "json"}))

	blk := model.NewBlock("bp", "hi")
	blk.SetTargetText(fr, "v1")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p-pit", "main", "en.json", []*model.Block{blk}))
	bid := rowIDOf(t, cs, "p-pit", "bp")
	blk.ID = bid

	// Separating the three timestamps, and nothing more: the history rows, the
	// version and the cursor are all stamped by this process, so ordering them
	// only needs them to be distinct. They used to straddle two clocks — history
	// on the database's NOW(), the version on this one's — and no sleep here
	// could have fixed that; see recordTargetHistoryPg.
	time.Sleep(5 * time.Millisecond)
	ver, err := cs.CreateVersion(ctx, "p-pit", "main", "snap", "")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)

	// Change after the version.
	blk.SetTargetText(fr, "v2")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p-pit", "main", "en.json", []*model.Block{blk}))
	sb, _ := cs.GetBlock(ctx, "p-pit", "main", bid)
	require.Equal(t, "v2", sb.TargetText(fr))

	// Restore to the version.
	e := s.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"to_version":%q}`, ver.ID)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id")
	c.SetParamValues("p-pit")
	require.NoError(t, s.HandleRestoreToPoint(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	sb, _ = cs.GetBlock(ctx, "p-pit", "main", bid)
	assert.Equal(t, "v1", sb.TargetText(fr), "restore-to-version should roll the target back to the version's state")
}

// testServerCaller holds every permission in the test server's workspace.
var testServerCaller = changeCaller{user: "user-1", name: "Ada", perms: platauth.PermAll, ws: "test-ws"}

// removeTranslation removes a block's translation as a person's change set.
func removeTranslation(t *testing.T, s *Server, pid, item, bid, locale string) {
	t.Helper()
	rev := targetRev(t, s.ContentStore.(*bstore.PostgresStore), pid, bid, locale)
	rec, res := sendChanges(t, s, pid, testServerCaller, change.Set{Ops: []change.Op{{
		Kind: change.KindRemoveEdition, At: at(item, bid, locale), IfMatch: rev, Body: &change.RemoveEdition{}}}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// sendRestore calls a restore route (revert or rollback) with body as a
// project manager and returns the response.
func sendRestore(t *testing.T, s *Server, handler func(echo.Context) error, pid, bid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := s.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id", "ref", "bid")
	c.SetParamValues(pid, "main", bid)
	require.NoError(t, handler(c))
	return rec
}

// History rows that record no wording (a removal, a decision) restore what
// they record: a revert to before a batch that re-added a removed translation
// removes it again, and a decision filed before a batch leaves the wording it
// was made on as what the revert restores.
func TestRevert_HistoryRowsThatRecordNoWording(t *testing.T) {
	fr := model.LocaleID("fr")
	setup := func(t *testing.T, pid string) (*Server, string) {
		s, _ := newTestServer(t)
		require.NoError(t, s.ContentStore.CreateProject(t.Context(), &platstore.Project{ID: pid, Name: pid, DefaultSourceLanguage: "en",
			TargetLanguages: []model.LocaleID{fr}, WorkspaceID: "test-ws"}))
		blk := model.NewBlock("b", "hello")
		blk.SetTargetText(fr, "bonjour-v0")
		return s, storeItemBlock(t, s.ContentStore, pid, "en.json", blk)
	}
	batch := func(t *testing.T, s *Server, pid, bid, corr, text string) {
		ctx := bstore.WithChangeContext(t.Context(), bstore.ChangeContext{Actor: "u", CorrelationID: corr})
		_, err := s.ContentStore.UpdateBlock(ctx, pid, "main", bid, func(sb *venue.StoredBlock) error {
			sb.Block.SetTargetText(fr, text)
			return nil
		})
		require.NoError(t, err)
	}

	t.Run("a translation removed before the batch is removed by its revert", func(t *testing.T) {
		s, bid := setup(t, "p-rm")
		removeTranslation(t, s, "p-rm", "en.json", bid, "fr")
		batch(t, s, "p-rm", bid, "READD", "bonjour-v1")

		rec := sendRestore(t, s, s.HandleRevertBatch, "p-rm", bid, `{"correlation_id":"READD"}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.False(t, holdsTarget(getStoredBlock(t, s.ContentStore.(*bstore.PostgresStore), "p-rm", bid), fr),
			"the block holds no translation, as before the batch")
	})

	t.Run("a decision filed before the batch restores the wording before it", func(t *testing.T) {
		s, bid := setup(t, "p-dec")
		rec, res := sendChanges(t, s, "p-dec", testServerCaller, change.Set{Ops: []change.Op{
			decide(at("en.json", bid, "fr"), targetRev(t, s.ContentStore.(*bstore.PostgresStore), "p-dec", bid, "fr"), change.OutcomeEstablish)}})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
		hist, err := s.ContentStore.GetBlockHistory(t.Context(), "p-dec", "main", bid, "fr", 10)
		require.NoError(t, err)
		require.Equal(t, "decision", hist[0].ChangeType, "the premise: the decision is the last history row before the batch")
		batch(t, s, "p-dec", bid, "AFTER", "bonjour-v1")

		rec = sendRestore(t, s, s.HandleRevertBatch, "p-dec", bid, `{"correlation_id":"AFTER"}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "bonjour-v0", getStoredBlock(t, s.ContentStore.(*bstore.PostgresStore), "p-dec", bid).TargetText(fr))
	})

	t.Run("a rollback to a removal removes the translation", func(t *testing.T) {
		s, bid := setup(t, "p-rb-rm")
		removeTranslation(t, s, "p-rb-rm", "en.json", bid, "fr")
		hist, err := s.ContentStore.GetBlockHistory(t.Context(), "p-rb-rm", "main", bid, "fr", 10)
		require.NoError(t, err)
		require.Equal(t, bstore.HistoryTargetRemoved, hist[0].ChangeType)
		removal := hist[0].Seq
		batch(t, s, "p-rb-rm", bid, "READD", "bonjour-v1")

		rec := sendRestore(t, s, s.HandleRollbackBlock, "p-rb-rm", bid, fmt.Sprintf(`{"locale":"fr","to_seq":%d}`, removal))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.False(t, holdsTarget(getStoredBlock(t, s.ContentStore.(*bstore.PostgresStore), "p-rb-rm", bid), fr))
	})
}

// rowIDOf is the row id of the block a project's main stream keys key.
func rowIDOf(t *testing.T, cs platstore.ContentStore, projectID, key string) string {
	t.Helper()
	stored, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	for _, sb := range stored {
		if sb.SourceID == key {
			return sb.Block.ID
		}
	}
	t.Fatalf("no block keyed %s", key)
	return ""
}
