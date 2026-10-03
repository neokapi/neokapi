package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// changeCaller is who sends a change set in a test: their user id, and the
// permissions and language scope the project access middleware would resolve
// for them.
type changeCaller struct {
	user  string
	name  string
	perms platauth.Permission
	langs []string
	// ws is the workspace the request is in; empty is ws-1.
	ws string
}

// fullCaller holds every permission.
var fullCaller = changeCaller{user: "user-1", name: "Ada", perms: platauth.PermAll}

// translateCaller may read and translate.
var translateCaller = changeCaller{user: "user-2", name: "Tom", perms: platauth.PermViewContent | platauth.PermTranslate}

// changesContext is the echo context the router hands the changes route for a
// project's main stream, as who.
func changesContext(t *testing.T, pid string, who changeCaller, body []byte) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/projects/"+pid+"/streams/main/changes", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "stream")
	c.SetParamValues("acme", pid, "main")
	c.Set("project_permissions", who.perms)
	if who.langs != nil {
		c.Set("project_languages", who.langs)
	}
	c.Set("user_id", who.user)
	c.Set("name", who.name)
	ws := who.ws
	if ws == "" {
		ws = "ws-1"
	}
	c.Set("workspace_id", ws)
	c.SetRequest(r.WithContext(bstore.WithChangeContext(r.Context(), bstore.ChangeContext{Actor: who.user})))
	return c, rec
}

// sendChanges posts set to the project's main stream the way the router calls
// the changes route, as who, and returns the response and its result.
func sendChanges(t *testing.T, srv *Server, pid string, who changeCaller, set change.Set) (*httptest.ResponseRecorder, change.Result) {
	t.Helper()
	body, err := json.Marshal(set)
	require.NoError(t, err)
	return sendChangesBody(t, srv, pid, who, body)
}

// sendChangesBody posts a raw body to the changes route.
func sendChangesBody(t *testing.T, srv *Server, pid string, who changeCaller, body []byte) (*httptest.ResponseRecorder, change.Result) {
	t.Helper()
	c, rec := changesContext(t, pid, who, body)
	require.NoError(t, srv.HandleApplyChanges(c))
	var res change.Result
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res), rec.Body.String())
	require.Equal(t, change.ResultSchemaID, res.Schema)
	return rec, res
}

// at addresses edition locale of block bid in the item; an empty locale is
// the source.
func at(item, bid, locale string) change.Ref {
	r := change.Ref{Doc: item, Block: bid}
	if locale != "" {
		r.Edition = model.EditionKey{Locale: model.LocaleID(locale)}
	}
	return r
}

// setText is a set_content of text on ref, guarded by rev.
func setText(ref change.Ref, rev, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: ref, IfMatch: rev, Body: &change.SetContent{Text: &text}}
}

// decide is a decision with outcome on ref, guarded by rev.
func decide(ref change.Ref, rev string, outcome change.Outcome) change.Op {
	return change.Op{Kind: change.KindDecide, At: ref, IfMatch: rev, Body: &change.Decide{Outcome: outcome}}
}

// targetRev is the revision of a block's translation as the stream holds it.
func targetRev(t *testing.T, cs *bstore.PostgresStore, pid, bid, locale string) string {
	t.Helper()
	b := getStoredBlock(t, cs, pid, bid)
	return model.EditionRevision(b, model.EditionKey{Locale: model.LocaleID(locale)})
}

// editAsPerson writes content as a person's edit of edition locale of a block
// through the stream's change service, guarded by the revision the stream
// holds, and requires it to land. An empty locale edits the source.
func editAsPerson(t *testing.T, s *Server, pid, bid, locale string, content change.Content) {
	t.Helper()
	ctx := t.Context()
	proj, err := s.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	sb, err := s.ContentStore.GetBlock(ctx, pid, "main", bid)
	require.NoError(t, err)
	sb.Block.SourceLocale = proj.DefaultSourceLanguage
	var k model.EditionKey
	if locale != "" {
		k = model.EditionKey{Locale: model.LocaleID(locale)}
	}
	sender := changeSender{userID: fullCaller.user, name: fullCaller.name}
	sc := s.newStreamChange(ctx, nil, proj, "main", proj.WorkspaceID, "", sender)
	res, err := sc.apply(ctx, change.Set{Gate: change.GateReport, Ops: []change.Op{{
		Kind: change.KindSetContent, At: at(sb.ItemName, bid, locale), IfMatch: model.EditionRevision(sb.Block, k),
		Body: &change.SetContent{Content: content},
	}}}, change.Actor{Kind: change.ActorPerson, Name: sender.userID})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// reviewBody is a decision as an editor names it: approve, or withdraw an
// approval, or with status draft reject it.
type reviewBody struct {
	TargetLocale string `json:"target_locale"`
	Reviewed     bool   `json:"reviewed"`
	Status       string `json:"status"`
}

// decideFromBody sends the decision body names on a block's translation,
// guarded by the revision the stream holds, as who.
func decideFromBody(t *testing.T, s *Server, pid, bid, body string, who changeCaller) *httptest.ResponseRecorder {
	t.Helper()
	var req reviewBody
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	outcome := change.OutcomeWithdraw
	switch {
	case req.Reviewed:
		outcome = change.OutcomeEstablish
	case req.Status == string(model.TargetStatusDraft):
		outcome = change.OutcomeReject
	}
	sb, err := s.ContentStore.GetBlock(t.Context(), pid, "main", bid)
	require.NoError(t, err)
	loc := model.LocaleID(req.TargetLocale)
	rev := model.RunsRevision(model.EditionKey{Locale: loc}, sb.Block.TargetRuns(loc))
	if sb.Block.Target(loc) == nil {
		rev = model.AbsentRevision
	}
	rec, _ := sendChanges(t, s, pid, who, change.Set{Ops: []change.Op{
		decide(at(sb.ItemName, bid, req.TargetLocale), rev, outcome)}})
	return rec
}

// commitFor is the commitDrafts of a project's main stream on cs, as a server
// action commits what a tool produced.
func commitFor(t *testing.T, cs platstore.ContentStore, pid string) commitDrafts {
	t.Helper()
	proj, err := cs.GetProject(t.Context(), pid)
	require.NoError(t, err)
	srv := &Server{ContentStore: cs}
	return srv.commitTo(nil, proj, "main", proj.WorkspaceID)
}

// textContent is content in text form.
func textContent(text string) change.Content { return change.Content{Text: &text} }

// sourceRev is the revision of a block's source as the stream holds it, in the
// project's source language.
func sourceRev(t *testing.T, cs *bstore.PostgresStore, pid, bid string) string {
	t.Helper()
	b := getStoredBlock(t, cs, pid, bid)
	b.SourceLocale = "en"
	return model.EditionRevision(b, model.EditionKey{})
}
