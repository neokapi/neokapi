package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// pageThroughPull pages a stream's pull from cursor 0, limit change entries per
// page, and counts how many times each block arrived. between, when set, runs
// after each page with the blocks that page served.
func pageThroughPull(t *testing.T, e *echo.Echo, authHeader, projectID, scope string, limit int,
	between func(page int, blocks []apiclient.SyncBlock),
) (map[string]int, int) {
	t.Helper()
	arrived := map[string]int{}
	cursor := int64(0)
	for page := 1; ; page++ {
		require.Less(t, page, 200, "the pull ends")
		req := httptest.NewRequest(http.MethodGet,
			fmt.Sprintf("/api/v1/projects/%s/sync/main/pull?cursor=%d&limit=%d%s", projectID, cursor, limit, scope), nil)
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp apiclient.RichPullResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		for _, b := range resp.Blocks {
			arrived[b.ID]++
		}
		cursor = resp.Cursor
		if between != nil {
			between(page, resp.Blocks)
		}
		if !resp.HasMore {
			return arrived, page
		}
	}
}

// writeBackTargets writes set's change to the stored blocks pick selects, the
// way a server job writes back what it produced.
func writeBackTargets(t *testing.T, srv *Server, projectID string, pick func(*venue.StoredBlock) bool, set func(*model.Block)) {
	t.Helper()
	ctx := t.Context()
	stored, err := srv.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", Limit: 1000})
	require.NoError(t, err)
	var reads []*venue.StoredBlock
	for _, sb := range stored {
		if pick(sb) {
			set(sb.Block)
			reads = append(reads, sb)
		}
	}
	require.NotEmpty(t, reads)
	res, err := srv.ContentStore.WriteBackBlocks(ctx, projectID, "main", reads)
	require.NoError(t, err)
	require.Equal(t, len(reads), res.Written)
}

// A pull pages through the change log, and each block arrives once however its
// changes fall across pages. Pages were cut by change row, and a block was
// served on every page holding one of its rows: a block that gained a target
// after its source arrived twice, and a project that had been drafted a few
// times over arrived many times over.
func TestSyncPull_ServesEachBlockOnceAcrossPages(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	var items []pushBlockItem
	for i := 1; i <= 12; i++ {
		items = append(items, pushBlockItem{ID: fmt.Sprintf("b%02d", i), Text: fmt.Sprintf("Sentence number %d.", i), ItemName: "en.json"})
	}
	pushBlocks(t, srv, e, authHeader, pid, items)

	// Every block gains a Norwegian target after its source, and four gain a
	// German one after that, so each block's change rows fall on several pages.
	writeBackTargets(t, srv, pid, func(*venue.StoredBlock) bool { return true },
		func(b *model.Block) { b.SetTargetText("nb", "nb "+b.SourceText()) })
	picked := 0
	writeBackTargets(t, srv, pid, func(*venue.StoredBlock) bool { picked++; return picked <= 4 },
		func(b *model.Block) { b.SetTargetText("de", "de "+b.SourceText()) })

	for _, scope := range []string{"", "&locales=nb", "&locales=de"} {
		arrived, pages := pageThroughPull(t, e, authHeader, pid, scope, 5, nil)
		assert.Greater(t, pages, 1, "scope %q is paged", scope)
		assert.Len(t, arrived, len(items), "scope %q serves every block", scope)
		for id, times := range arrived {
			assert.Equal(t, 1, times, "scope %q served block %s %d times", scope, id, times)
		}
	}
}

// A block that changes after the page that served it arrives again: the
// client holds the state it was served, and the change is newer than that.
func TestSyncPull_ABlockChangedDuringThePullArrivesAgain(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	var items []pushBlockItem
	for i := 1; i <= 6; i++ {
		items = append(items, pushBlockItem{ID: fmt.Sprintf("c%02d", i), Text: fmt.Sprintf("Line %d.", i), ItemName: "en.json"})
	}
	pushBlocks(t, srv, e, authHeader, pid, items)

	changed := ""
	arrived, _ := pageThroughPull(t, e, authHeader, pid, "", 2, func(page int, blocks []apiclient.SyncBlock) {
		if page != 1 {
			return
		}
		require.NotEmpty(t, blocks)
		changed = blocks[0].ID
		writeBackTargets(t, srv, pid,
			func(sb *venue.StoredBlock) bool { return sb.Block.ID == changed },
			func(b *model.Block) { b.SetTargetText("nb", "Endret") })
	})
	require.NotEmpty(t, changed)
	assert.Len(t, arrived, len(items))
	for id, times := range arrived {
		want := 1
		if id == changed {
			want = 2
		}
		assert.Equal(t, want, times, "block %s", id)
	}
}
