package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// removeBeforeWrite changes the store once: right after a handler's read, or
// right before a held read. Either way the change lands between what the
// handler decided on and what it writes.
type removeBeforeWrite struct {
	platstore.ContentStore
	once   sync.Once
	change func(ctx context.Context)
}

func (s *removeBeforeWrite) GetBlock(ctx context.Context, projectID, stream, blockID string) (*venue.StoredBlock, error) {
	sb, err := s.ContentStore.GetBlock(ctx, projectID, stream, blockID)
	s.once.Do(func() { s.change(ctx) })
	return sb, err
}

func (s *removeBeforeWrite) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.ContentStore.GetBlocks(ctx, q)
	s.once.Do(func() { s.change(ctx) })
	return blocks, err
}

func (s *removeBeforeWrite) UpdateBlock(ctx context.Context, projectID, stream, blockID string, update func(*venue.StoredBlock) error) (*venue.StoredBlock, error) {
	s.once.Do(func() { s.change(ctx) })
	return s.ContentStore.UpdateBlock(ctx, projectID, stream, blockID, update)
}

// seedGuardedWriteItem stores one block under en.json on stream and returns
// the id the store gave it.
func seedGuardedWriteItem(t *testing.T, cs platstore.ContentStore, projectID, stream string, b *model.Block) string {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.StoreItem(ctx, projectID, stream, &platstore.Item{ProjectID: projectID, Name: "en.json", Format: "json"}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, stream, "en.json", []*model.Block{b}))
	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: projectID, Stream: stream, ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	return stored[0].Block.ID
}

// writeTargetText stores wording for a locale the way another editor's save does.
func writeTargetText(t *testing.T, cs platstore.ContentStore, projectID, blockID, locale, text string) {
	t.Helper()
	_, err := cs.UpdateBlock(t.Context(), projectID, "main", blockID, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID(locale), text)
		return nil
	})
	require.NoError(t, err)
}

// historySeq is the block_history entry that recorded text for a locale.
func historySeq(t *testing.T, cs platstore.ContentStore, projectID, blockID, locale, text string) int64 {
	t.Helper()
	hist, err := cs.GetBlockHistory(t.Context(), projectID, "main", blockID, locale, 50)
	require.NoError(t, err)
	for _, h := range hist {
		if h.Text == text {
			return h.Seq
		}
	}
	t.Fatalf("no history entry for %q", text)
	return 0
}

// projectBlocks is every block a project holds on main.
func projectBlocks(t *testing.T, cs platstore.ContentStore, projectID string) []*venue.StoredBlock {
	t.Helper()
	blocks, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	return blocks
}

func callRollback(t *testing.T, srv *Server, pid, bid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id", "ref", "bid")
	c.SetParamValues(pid, "main", bid)
	require.NoError(t, srv.HandleRollbackBlock(c))
	return rec
}

func callCreateEntity(t *testing.T, srv *Server, pid, stream, bid, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/?item=en.json", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id", "ref", "bid")
	c.SetParamValues(pid, stream, bid)
	require.NoError(t, srv.HandleCreateEntity(c))
	return rec
}

// A rollback made against a read of the target that has since moved is
// refused with the block as it stands, and the newer wording stays.
func TestRollbackBlock_AStaleRollbackGetsTheCurrentBlock(t *testing.T) {
	srv, _ := newTestServer(t)
	cs := srv.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rbs", Name: "RB", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}}))
	b := model.NewBlock("k1", "hello")
	b.Translatable = true
	b.SetTargetText("fr", "bonjour-v1")
	bid := seedGuardedWriteItem(t, cs, "p-rbs", "main", b)
	writeTargetText(t, cs, "p-rbs", bid, "fr", "bonjour-v2")

	read, err := cs.GetBlock(ctx, "p-rbs", "main", bid)
	require.NoError(t, err)
	base := platstore.TargetRevision(read, "fr")
	writeTargetText(t, cs, "p-rbs", bid, "fr", "bonjour-v3")
	v1 := historySeq(t, cs, "p-rbs", bid, "fr", "bonjour-v1")

	answer := decodeBlockChanged(t, callRollback(t, srv, "p-rbs", bid,
		fmt.Sprintf(`{"locale":"fr","to_seq":%d,"base_revision":%q}`, v1, base)))
	assert.Equal(t, "bonjour-v3", answer.Current.Targets["fr"].Text)
	got, err := cs.GetBlock(ctx, "p-rbs", "main", bid)
	require.NoError(t, err)
	assert.Equal(t, "bonjour-v3", got.Block.TargetText("fr"), "the newer wording stays")
}

// A block removed while its rollback is under way stays removed.
func TestRollbackBlock_ABlockRemovedDuringTheRollbackStaysRemoved(t *testing.T) {
	srv, _ := newTestServer(t)
	cs := srv.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rbr", Name: "RB", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}}))
	b := model.NewBlock("k1", "hello")
	b.Translatable = true
	b.SetTargetText("fr", "bonjour-v1")
	bid := seedGuardedWriteItem(t, cs, "p-rbr", "main", b)
	writeTargetText(t, cs, "p-rbr", bid, "fr", "bonjour-v2")
	v1 := historySeq(t, cs, "p-rbr", bid, "fr", "bonjour-v1")

	srv.ContentStore = &removeBeforeWrite{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, "p-rbr", "main", "en.json"))
	}}
	rec := callRollback(t, srv, "p-rbr", bid, fmt.Sprintf(`{"locale":"fr","to_seq":%d}`, v1))
	assert.NotEqual(t, http.StatusOK, rec.Code, "a removed block is not rolled back: %s", rec.Body.String())
	assert.Empty(t, projectBlocks(t, cs, "p-rbr"), "the removal stands")
}

// A batch revert leaves a block removed during the revert removed, and does
// not count what it could not revert.
func TestApplyReverts_ABlockRemovedDuringTheRevertStaysRemovedAndIsNotCounted(t *testing.T) {
	srv, _ := newTestServer(t)
	cs := srv.ContentStore
	pg, ok := cs.(*bstore.PostgresStore)
	require.True(t, ok)
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rvr", Name: "Rev", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}}))
	require.NoError(t, cs.StoreItem(ctx, "p-rvr", "main", &platstore.Item{ProjectID: "p-rvr", Name: "en.json", Format: "json"}))

	mk := func(key, src, fr string) *model.Block {
		b := model.NewBlock(key, src)
		b.Translatable = true
		b.SetTargetText("fr", fr)
		return b
	}
	ctx0 := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "c0"})
	require.NoError(t, cs.StoreBlocksForItem(ctx0, "p-rvr", "main", "en.json", []*model.Block{mk("k1", "one", "un-v0"), mk("k2", "two", "deux-v0")}))
	ctxB := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "BATCH1"})
	require.NoError(t, cs.StoreBlocksForItem(ctxB, "p-rvr", "main", "en.json", []*model.Block{mk("k1", "one", "un-v1"), mk("k2", "two", "deux-v1")}))
	reverts, err := pg.ComputeBatchReverts(ctx, "p-rvr", "main", "BATCH1")
	require.NoError(t, err)
	require.NotEmpty(t, reverts)

	srv.ContentStore = &removeBeforeWrite{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, "p-rvr", "main", "en.json"))
	}}
	n, err := srv.applyReverts(ctx, "p-rvr", "main", "revert_batch:BATCH1", reverts)
	require.NoError(t, err)
	assert.Zero(t, n, "the removed blocks were not reverted")
	assert.Empty(t, projectBlocks(t, cs, "p-rvr"), "the removal stands")
}

// Approving a source proposal after the source changed is refused with the
// block as it stands, and the proposal stays open to be decided again.
func TestSourceProposal_ApprovingOverASourceThatChangedIsRefused(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	ctx := context.Background()

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Colour picker")
	projID, ids := seedMultiLocaleProject(t, s, wsID, []*model.Block{b1})
	blockID := ids["Colour picker"]
	require.NotEmpty(t, blockID)

	createBody := `{"block_id":"` + blockID + `","item_name":"ui.json","proposed_source":"Color picker","found_in_locale":"fr","rationale":"US spelling"}`
	cCreate, recCreate := spCtx(wsID, projID, "", platauth.PermReview|platauth.PermViewContent, "reviewer-1", createBody)
	require.NoError(t, s.HandleCreateSourceProposal(cCreate))
	require.Equal(t, http.StatusCreated, recCreate.Code)
	var created bstore.ProposedSourceChange
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &created))

	_, err := s.ContentStore.UpdateBlock(ctx, projID, "main", blockID, func(sb *venue.StoredBlock) error {
		sb.Block.SetSourceText("Colour chooser")
		return nil
	})
	require.NoError(t, err)

	cDecide, recDecide := spCtx(wsID, projID, created.ID, platauth.PermEditSource|platauth.PermViewContent, ownerID, `{"decision":"approve"}`)
	require.NoError(t, s.HandleDecideSourceProposal(cDecide))
	require.Equal(t, http.StatusConflict, recDecide.Code, recDecide.Body.String())
	var answer struct {
		Code    string `json:"code"`
		Current struct {
			Source string `json:"source"`
		} `json:"current"`
	}
	require.NoError(t, json.Unmarshal(recDecide.Body.Bytes(), &answer))
	assert.Equal(t, "block_changed", answer.Code)
	assert.Equal(t, "Colour chooser", answer.Current.Source, "the answer carries the source that stands")

	sb, err := s.ContentStore.GetBlock(ctx, projID, "main", blockID)
	require.NoError(t, err)
	assert.Equal(t, "Colour chooser", sb.Block.SourceText(), "the newer source stays")
	got, err := s.SourceProposalStore.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.SourceProposalOpen, got.Status, "the proposal stays open")
}

// An entity marked on a block of another stream lands on that stream's block.
func TestEntityCreate_OnAStreamLandsOnThatStream(t *testing.T) {
	srv, _ := newTestServer(t)
	cs := srv.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-ent", Name: "Ent", DefaultSourceLanguage: "en"}))
	pg, ok := cs.(*bstore.PostgresStore)
	require.True(t, ok)
	require.NoError(t, pg.CreateStream(ctx, &platstore.Stream{ProjectID: "p-ent", Name: "feature", Parent: "main"}))
	b := model.NewBlock("k1", "Acme Cloud is fast.")
	b.Translatable = true
	bid := seedGuardedWriteItem(t, cs, "p-ent", "feature", b)

	rec := callCreateEntity(t, srv, "p-ent", "feature", bid, `{"text":"Acme Cloud","type":"product","start":0,"end":10}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	sb, err := cs.GetBlock(ctx, "p-ent", "feature", bid)
	require.NoError(t, err)
	assert.NotNil(t, sb.Block.OverlaySpan(model.OverlayEntity, "entity:0"), "the entity is on the stream's block")
}

// A block removed while an entity is being marked on it stays removed.
func TestEntityCreate_ABlockRemovedDuringTheWriteStaysRemoved(t *testing.T) {
	srv, _ := newTestServer(t)
	cs := srv.ContentStore
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-entr", Name: "Ent", DefaultSourceLanguage: "en"}))
	b := model.NewBlock("k1", "Acme Cloud is fast.")
	b.Translatable = true
	bid := seedGuardedWriteItem(t, cs, "p-entr", "main", b)

	srv.ContentStore = &removeBeforeWrite{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, "p-entr", "main", "en.json"))
	}}
	rec := callCreateEntity(t, srv, "p-entr", "main", bid, `{"text":"Acme Cloud","type":"product","start":0,"end":10}`)
	assert.NotEqual(t, http.StatusCreated, rec.Code, "no entity lands on a removed block: %s", rec.Body.String())
	assert.Empty(t, projectBlocks(t, cs, "p-entr"), "the removal stands")
}

// leftOutLogRecorder keeps the log records a test is looking for.
type leftOutLogRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *leftOutLogRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (h *leftOutLogRecorder) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *leftOutLogRecorder) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *leftOutLogRecorder) WithGroup(string) slog.Handler { return h }

func (h *leftOutLogRecorder) attr(message, key string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message != message {
			continue
		}
		found, value := false, ""
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == key {
				found, value = true, fmt.Sprint(a.Value.Any())
				return false
			}
			return true
		})
		return value, found
	}
	return "", false
}

// The pull names the block rows it leaves out, so an operator can find them.
func TestSyncPull_LogsTheIdsOfTheBlocksItLeavesOut(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{{ID: "b1", Text: "Hello", ItemName: "en.json"}})
	orphan := model.NewBlock("orphan-1", "Orphan")
	orphan.Translatable = true
	require.NoError(t, srv.ContentStore.StoreBlocks(t.Context(), pid, "main", []*model.Block{orphan}))

	logs := &leftOutLogRecorder{}
	prev := slog.Default()
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(prev) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	ids, ok := logs.attr("sync pull: left out block rows that belong to no item", "block_ids")
	require.True(t, ok, "the log names the rows it left out")
	assert.Contains(t, ids, "orphan-1")
}
