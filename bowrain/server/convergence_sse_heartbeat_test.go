package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stage of a run takes as long as it takes, and settling source over a large
// corpus takes minutes. The stream carried nothing at all for that whole time,
// and an idle connection is what a gateway closes: the nightly of 2026-09-16 ran
// four minutes of silence into a 504 and failed a run the server was still
// working on.
func TestConvergenceRunSSE_HeartbeatsThroughAQuietStage(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	ctx := t.Context()
	p := &platstore.Project{Name: "quiet-run", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, cs.CreateProject(ctx, p))

	runStore := bstore.NewConvergenceRunStore(cs.DB())
	s := &Server{ContentStore: cs, ConvergenceRunStore: runStore}
	s.convergence = newConvergenceOrchestrator(s)

	// A run with no events at all: the stage is under way and has said nothing.
	run := &bstore.ConvergenceRun{ProjectID: p.ID, Stream: "main", Trigger: "manual"}
	require.NoError(t, runStore.CreateRun(ctx, run))

	restore := convergenceHeartbeatEvery
	convergenceHeartbeatEvery = 50 * time.Millisecond
	t.Cleanup(func() { convergenceHeartbeatEvery = restore })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+p.ID+"/convergence/runs/"+run.ID+"/events", nil)
	reqCtx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(reqCtx)
	rec := newSyncRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "runID")
	c.SetParamValues(p.ID, run.ID)
	c.Set("project_permissions", platauth.PermAll)

	done := make(chan error, 1)
	go func() { done <- s.HandleConvergenceRunSSE(c) }()

	require.Eventually(t, func() bool {
		return strings.Count(rec.body(), ": heartbeat") >= 2
	}, 3*time.Second, 20*time.Millisecond,
		"a quiet stage keeps sending, so nothing in front of the stream reads it as idle")

	// A heartbeat says nothing about the run. It carries no id and no data, so
	// every SSE reader skips it, including the released client that folds each
	// data frame into the run it is rendering.
	body := rec.body()
	assert.NotContains(t, body, "data:", "a heartbeat is not an event")
	assert.NotContains(t, body, "id:", "a heartbeat does not move the resume point")

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after the request context was cancelled")
	}
}
