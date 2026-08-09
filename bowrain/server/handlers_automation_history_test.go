package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAutomationHistoryServer wires only what the history endpoint reads: a
// migrated PostgreSQL schema and the rule store over it.
func newAutomationHistoryServer(t *testing.T) *Server {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return &Server{AutomationRuleStore: event.NewRuleStore(db.DB)}
}

// The table used to render every row a project had ever produced; the endpoint
// now answers a bounded page with a cursor.
func TestHandleListAutomationHistory_Paging(t *testing.T) {
	s := newAutomationHistoryServer(t)
	ctx := t.Context()

	started := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 7 {
		require.NoError(t, s.AutomationRuleStore.RecordExecution(ctx, &event.HistoryEntry{
			ID:        id.New(),
			RuleID:    "rule-1",
			ProjectID: "proj-1",
			Status:    "success",
			StartedAt: started.Add(-time.Duration(i) * time.Minute),
			EndedAt:   started,
		}))
	}

	get := func(t *testing.T, query string) event.HistoryPage {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/automations/history"+query, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("proj-1")
		require.NoError(t, s.HandleListAutomationHistory(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var page event.HistoryPage
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
		return page
	}

	t.Run("limit bounds the page and hands back a cursor", func(t *testing.T) {
		page := get(t, "?limit=3")
		assert.Len(t, page.Entries, 3)
		assert.NotEmpty(t, page.NextCursor)
	})

	t.Run("the cursor continues where the page ended", func(t *testing.T) {
		first := get(t, "?limit=3")
		second := get(t, "?limit=3&cursor="+url.QueryEscape(first.NextCursor))
		require.Len(t, second.Entries, 3)
		assert.NotEqual(t, first.Entries[0].ID, second.Entries[0].ID)
		assert.True(t, second.Entries[0].StartedAt.Before(first.Entries[2].StartedAt))
	})

	t.Run("the last page carries no cursor", func(t *testing.T) {
		page := get(t, "?limit=50")
		assert.Len(t, page.Entries, 7)
		assert.Empty(t, page.NextCursor)
	})

	t.Run("a project with no history is an empty list, not null", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/automations/history", nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("proj-none")
		require.NoError(t, s.HandleListAutomationHistory(c))

		var raw map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		assert.Equal(t, []any{}, raw["entries"])
	})
}

func TestHandleListAutomationHistory_NoStore(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/automations/history", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	require.NoError(t, s.HandleListAutomationHistory(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.Equal(t, []any{}, raw["entries"])
}
