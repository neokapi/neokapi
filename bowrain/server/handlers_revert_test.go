package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
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

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rev", Name: "Rev", DefaultSourceLanguage: "en"}))

	mk := func(id, src string) *model.Block {
		return &model.Block{ID: id, Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: src}}}}
	}

	// Initial state (correlation c0): two blocks translated.
	b1, b2 := mk("b1", "one"), mk("b2", "two")
	b1.SetTargetText(fr, "un-v0")
	b2.SetTargetText(fr, "deux-v0")
	ctx0 := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "c0"})
	require.NoError(t, cs.StoreBlocks(ctx0, "p-rev", "main", []*model.Block{b1, b2}))

	// A batch (correlation BATCH1) re-translates both.
	b1.SetTargetText(fr, "un-v1")
	b2.SetTargetText(fr, "deux-v1")
	ctxB := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "BATCH1"})
	require.NoError(t, cs.StoreBlocks(ctxB, "p-rev", "main", []*model.Block{b1, b2}))

	// Sanity: both are at v1.
	sb1, _ := cs.GetBlock(ctx, "p-rev", "main", "b1")
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
	sb1, _ = cs.GetBlock(ctx, "p-rev", "main", "b1")
	sb2, _ := cs.GetBlock(ctx, "p-rev", "main", "b2")
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
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rev2", Name: "Rev2", DefaultSourceLanguage: "en"}))

	b := &model.Block{ID: "bx", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}}}
	b.SetTargetText(fr, "added-in-batch")
	ctxB := bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: "u", CorrelationID: "ADD1"})
	require.NoError(t, cs.StoreBlocks(ctxB, "p-rev2", "main", []*model.Block{b}))

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

	sb, _ := cs.GetBlock(ctx, "p-rev2", "main", "bx")
	assert.Equal(t, "", sb.TargetText(fr), "a batch-added target should be blanked on revert")
}
