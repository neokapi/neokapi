package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase4_RollbackBlock proves per-edit undo: PG now records content history
// on target writes, and the rollback handler restores a prior version
// non-destructively (appending a new history entry).
func TestPhase4_RollbackBlock(t *testing.T) {
	s, _ := newTestServer(t)
	cs := s.ContentStore
	ctx := t.Context()
	fr := model.LocaleID("fr")

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "p-rb", Name: "RB", DefaultSourceLanguage: "en"}))

	blk := &model.Block{ID: "b-rb", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: "hello"}}}}
	blk.SetTargetText(fr, "bonjour-v1")
	require.NoError(t, cs.StoreBlocks(ctx, "p-rb", "main", []*model.Block{blk}))
	blk.SetTargetText(fr, "bonjour-v2")
	require.NoError(t, cs.StoreBlocks(ctx, "p-rb", "main", []*model.Block{blk}))

	hist, err := cs.GetBlockHistory(ctx, "p-rb", "main", "b-rb", "fr", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hist), 2, "two edits should produce >=2 history entries")

	var v1seq int64
	for _, h := range hist {
		if h.Text == "bonjour-v1" {
			v1seq = h.Seq
		}
	}
	require.NotZero(t, v1seq, "v1 history entry should exist")

	// Roll back to v1 via the handler.
	e := s.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"locale":"fr","to_seq":%d}`, v1seq)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.SetParamNames("id", "ref", "bid")
	c.SetParamValues("p-rb", "main", "b-rb")
	require.NoError(t, s.HandleRollbackBlock(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// The target is restored to v1.
	sb, err := cs.GetBlock(ctx, "p-rb", "main", "b-rb")
	require.NoError(t, err)
	assert.Equal(t, "bonjour-v1", sb.TargetText(fr))

	// The rollback is itself recorded (a new history entry).
	hist2, err := cs.GetBlockHistory(ctx, "p-rb", "main", "b-rb", "fr", 10)
	require.NoError(t, err)
	assert.Greater(t, len(hist2), len(hist), "rollback should append a new history entry")
}

// TestPhase4_RollbackRequiresPermission proves the rollback endpoint is gated by
// PermRollbackChanges (which only project-admins have by default).
func TestPhase4_RollbackRequiresPermission(t *testing.T) {
	s, _ := newTestServer(t)
	e := s.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"locale":"fr","to_seq":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// A translator-level permission set (no rollback permission).
	c.Set("project_permissions", platauth.PermViewContent|platauth.PermTranslate)
	c.SetParamNames("id", "ref", "bid")
	c.SetParamValues("p-rb", "main", "b-rb")
	require.Error(t, s.HandleRollbackBlock(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
