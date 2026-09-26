package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// readTargetRevision reads a block through the editor's blocks route and
// returns the revision the route serves for one locale's target.
func readTargetRevision(t *testing.T, srv *Server, pid, bid, locale string) string {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/blocks/main?item=greetings.txt", nil)
	w := httptest.NewRecorder()
	c := e.NewContext(r, w)
	c.SetParamNames("ws", "id", "ref")
	c.SetParamValues("acme", pid, "main")
	require.NoError(t, srv.HandleGetFileBlocks(c))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload []struct {
		ID              string            `json:"id"`
		TargetRevisions map[string]string `json:"target_revisions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	for _, b := range payload {
		if b.ID == bid {
			return b.TargetRevisions[locale]
		}
	}
	t.Fatalf("block %s is not served", bid)
	return ""
}

// withBaseRevision is req as JSON with base_revision set to base.
func withBaseRevision(t *testing.T, req any, base string) string {
	t.Helper()
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	fields["base_revision"] = base
	out, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(out)
}

// writeBlockAs calls a block write handler the way the router does, with every
// permission and a raw JSON body.
func writeBlockAs(t *testing.T, handler echo.HandlerFunc, pid, bid, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/acme/"+pid+"/blocks/main/"+bid+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "ref", "bid")
	c.SetParamValues("acme", pid, "main", bid)
	c.Set("project_permissions", platauth.PermAll)
	require.NoError(t, handler(c))
	return rec
}

// changeTargetText stores another person's wording for a locale.
func changeTargetText(t *testing.T, cs *bstore.PostgresStore, pid, bid, locale, text string) {
	t.Helper()
	sb, err := cs.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	sb.Block.SetTargetText(model.LocaleID(locale), text)
	require.NoError(t, cs.StoreBlocks(t.Context(), pid, "main", []*model.Block{sb.Block}))
}

// blockChangedAnswer is the body of a refused write: the reason, and the block
// as it stands.
type blockChangedAnswer struct {
	Code    string `json:"code"`
	Error   string `json:"error"`
	Current struct {
		ID      string `json:"id"`
		Targets map[string]struct {
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"targets"`
		TargetRevisions map[string]string `json:"target_revisions"`
	} `json:"current"`
}

func decodeBlockChanged(t *testing.T, rec *httptest.ResponseRecorder) blockChangedAnswer {
	t.Helper()
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var answer blockChangedAnswer
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &answer))
	assert.Equal(t, "block_changed", answer.Code)
	return answer
}

// A translator saves over a target someone else changed after the translator's
// copy was read. The save is refused with the block as it now stands, and the
// other person's wording stays. Saving again from that block lands.
func TestEditorTargetEdit_AStaleSaveGetsTheCurrentBlockAndOverwritesNothing(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	read := readTargetRevision(t, srv, pid, bid, "fr")
	changeTargetText(t, cs, pid, bid, "fr", "Salut")

	rec := writeBlockAs(t, srv.HandleUpdateBlockTarget, pid, bid, "",
		withBaseRevision(t, UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Coucou"}, read))
	answer := decodeBlockChanged(t, rec)
	assert.Equal(t, "Salut", answer.Current.Targets["fr"].Text, "the answer carries the wording that stands")
	assert.Equal(t, "Salut", getStoredBlock(t, cs, pid, bid).TargetText("fr"), "the other save stays")

	rec = writeBlockAs(t, srv.HandleUpdateBlockTarget, pid, bid, "",
		withBaseRevision(t, UpdateBlockTargetRequest{TargetLocale: "fr", Text: "Coucou"}, answer.Current.TargetRevisions["fr"]))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "Coucou", getStoredBlock(t, cs, pid, bid).TargetText("fr"))
}

// The Run-native save follows the same rule.
func TestEditorTargetRunsEdit_AStaleSaveGetsTheCurrentBlockAndOverwritesNothing(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	read := readTargetRevision(t, srv, pid, bid, "fr")
	changeTargetText(t, cs, pid, bid, "fr", "Salut")

	save := UpdateBlockTargetRunsRequest{TargetLocale: "fr", Runs: []model.Run{{Text: &model.TextRun{Text: "Coucou"}}}}
	answer := decodeBlockChanged(t, writeBlockAs(t, srv.HandleUpdateBlockTargetRuns, pid, bid, "/runs", withBaseRevision(t, save, read)))
	assert.Equal(t, "Salut", answer.Current.Targets["fr"].Text)
	assert.Equal(t, "Salut", getStoredBlock(t, cs, pid, bid).TargetText("fr"))

	rec := writeBlockAs(t, srv.HandleUpdateBlockTargetRuns, pid, bid, "/runs",
		withBaseRevision(t, save, answer.Current.TargetRevisions["fr"]))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "Coucou", getStoredBlock(t, cs, pid, bid).TargetText("fr"))
}

// A reviewer approves wording that changed after the reviewer read it. The
// approval is refused with the block as it now stands, and the changed wording
// stays unapproved.
func TestReviewBlock_AnApprovalOfWordingThatChangedIsRefused(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b})
	bid := ids["Hello"]

	read := readTargetRevision(t, srv, pid, bid, "fr")
	changeTargetText(t, cs, pid, bid, "fr", "Salut")

	approve := ReviewBlockRequest{TargetLocale: "fr", ItemName: "greetings.txt", Reviewed: true}
	answer := decodeBlockChanged(t, writeBlockAs(t, srv.HandleReviewBlock, pid, bid, "/review", withBaseRevision(t, approve, read)))
	assert.Equal(t, "Salut", answer.Current.Targets["fr"].Text, "the reviewer is shown the wording that stands")
	assert.NotEqual(t, model.TargetStatusEstablished, getStoredBlock(t, cs, pid, bid).Target("fr").Status,
		"wording the reviewer did not read is not approved")

	rec := writeBlockAs(t, srv.HandleReviewBlock, pid, bid, "/review",
		withBaseRevision(t, approve, answer.Current.TargetRevisions["fr"]))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, model.TargetStatusEstablished, getStoredBlock(t, cs, pid, bid).Target("fr").Status)
}
