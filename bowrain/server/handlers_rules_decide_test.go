package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
)

// decideParityDir is the fixture host's TestDecideParity holds the desktop's
// Decide to: one project, one ledger state, and the view they give.
var decideParityDir = filepath.Join("..", "..", "host", "screens", "testdata", "decide-parity")

// decideCaller is who the Decide routes are signed in as; a test changes it
// between requests.
type decideCaller struct {
	userID string
	perms  platauth.Permission
	reach  platauth.CoordinateReach
}

// decideSQLiteServer serves the Decide and context object routes over a
// SQLite content store, signed in as whoever caller names.
func decideSQLiteServer(t *testing.T, caller *decideCaller) (*echo.Echo, *sqlitestore.SQLiteStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	srv := &Server{ContentStore: cs, wsStores: newWorkspaceStores()}
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", caller.userID)
			c.Set("workspace_id", "ws")
			c.Set("project_permissions", caller.perms)
			c.Set("project_coordinates", caller.reach)
			return next(c)
		}
	})
	srv.registerContextObjectRoutes(g)
	srv.registerRulesDecideRoutes(g)
	return e, cs
}

// decideParityPushed is pushed.json: what a push of the parity project
// carries.
type decideParityPushed struct {
	Collections []struct {
		Name        string            `json:"name"`
		Coordinates map[string]string `json:"coordinates"`
	} `json:"collections"`
	Items []struct {
		Name       string   `json:"name"`
		Collection string   `json:"collection"`
		Blocks     []string `json:"blocks"`
	} `json:"items"`
}

// seedDecideParity stores the parity project as a push leaves it: its
// collections at their points, each file's blocks, and its ledger.
func seedDecideParity(t *testing.T, e *echo.Echo, cs platstore.ContentStore) string {
	t.Helper()
	ctx := t.Context()
	p := &platstore.Project{
		Name: "decide-parity", DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
		WorkspaceID: "ws", Properties: map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, p))

	data, err := os.ReadFile(filepath.Join(decideParityDir, "pushed.json"))
	require.NoError(t, err)
	var pushed decideParityPushed
	require.NoError(t, json.Unmarshal(data, &pushed))
	collectionIDs := map[string]string{}
	for _, c := range pushed.Collections {
		col := &platstore.Collection{
			ProjectID: p.ID, Name: c.Name, Kind: platstore.CollectionUploaded, ItemLabel: "file",
			Stream: "main", Context: c.Coordinates, Owner: "recipe",
		}
		require.NoError(t, cs.CreateCollection(ctx, col))
		collectionIDs[c.Name] = col.ID
	}
	for _, it := range pushed.Items {
		require.NoError(t, cs.StoreItem(ctx, p.ID, "main", &platstore.Item{
			Name: it.Name, Format: "markdown", ItemType: "file", CollectionID: collectionIDs[it.Collection],
		}))
		var blocks []*model.Block
		for i, text := range it.Blocks {
			blocks = append(blocks, model.NewBlock(fmt.Sprintf("%s#%d", it.Name, i), text))
		}
		require.NoError(t, cs.StoreBlocksForItem(ctx, p.ID, "main", it.Name, blocks))
	}

	segment, err := os.ReadFile(filepath.Join(decideParityDir, "log.jsonl"))
	require.NoError(t, err)
	ops, err := workspace.DecodeSegment(segment, "prj_decideparityaaaaaaaa")
	require.NoError(t, err)
	r := &routeRemote{h: e, base: "/api/v1/ws/" + p.ID + "/context/objects"}
	require.NoError(t, r.Put(ctx, workspace.Object{Name: workspace.SegmentName("desktop", ops[0].ID), Data: segment}))
	return p.ID
}

// decideRoute requests one Decide route of the project and decodes the JSON
// it answers with into out, returning the status.
func decideRoute(t *testing.T, e *echo.Echo, method, pid, route string, body, out any) int {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		require.NoError(t, err)
	}
	r := &routeRemote{h: e}
	rec := r.do(t.Context(), method, "/api/v1/ws/"+pid+route, raw)
	if out != nil && rec.Code < 300 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out), rec.Body.String())
	}
	if out != nil && rec.Code >= 300 {
		_ = json.Unmarshal(rec.Body.Bytes(), out)
	}
	return rec.Code
}

// readDecideView reads the project's Decide view.
func readDecideView(t *testing.T, e *echo.Echo, pid string) screens.RulesDecideViewDTO {
	t.Helper()
	var view screens.RulesDecideViewDTO
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, pid, "/rules/main/decide", nil, &view))
	return view
}

// decisionOf returns the question of the kind given.
func decisionOf(t *testing.T, view screens.RulesDecideViewDTO, kind string) screens.RuleDecisionDTO {
	t.Helper()
	for _, d := range view.Decisions {
		if d.Kind == kind {
			return d
		}
	}
	require.Failf(t, "no question", "the view asks no %s question", kind)
	return screens.RuleDecisionDTO{}
}

const decideAll = platauth.PermViewContent | platauth.PermManageTerms | platauth.PermManageVoice | platauth.PermManageMemory

// The server's Decide view of the parity ledger is the JSON the desktop's
// Decide reads for it (host TestDecideParity), apart from the assignee, which
// only the server holds.
func TestRulesDecide_TheDesktopsViewOfOneLedger_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)

	view := readDecideView(t, e, pid)
	for i := range view.Decisions {
		view.Decisions[i].Assignee = nil
	}
	got, err := json.MarshalIndent(view, "", "  ")
	require.NoError(t, err)
	want, err := os.ReadFile(filepath.Join(decideParityDir, "decide-view.json"))
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got)+"\n")
}

// One answer at a time: later records nothing, a keep of a name writes the
// form picked, a keep of one side drops the other, and a drop sets a
// suggestion aside. Who kept what is the member.
func TestRulesDecide_RecordsOneAnswerAtATime_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	view := readDecideView(t, e, pid)
	name := decisionOf(t, view, "name")
	note := decisionOf(t, view, "note")
	side := decisionOf(t, view, "disagreement")

	var res screens.RuleDecideResultDTO
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: note.ID, Answer: "later"}, &res))
	assert.Equal(t, screens.RuleDecideResultDTO{Answer: "later", Later: 1}, res)
	assert.Len(t, readDecideView(t, e, pid).Decisions, len(view.Decisions), "later records nothing")

	var consequence ruleConsequence
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, pid,
		"/rules/main/decide/consequence?id="+name.ID+"&form="+url.QueryEscape("Sign-in"), nil, &consequence))
	for _, o := range name.Options {
		if o.Form == "Sign-in" {
			assert.Equal(t, o.Consequence, consequence.Consequence, "the consequence of a form is the one its option states")
		}
	}

	res = screens.RuleDecideResultDTO{}
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: name.ID, Answer: "keep", Form: "Sign In"}, &res))
	assert.Equal(t, screens.RuleDecideResultDTO{Answer: "keep", Kept: 1}, res)

	employ := side.Options[len(side.Options)-1]
	res = screens.RuleDecideResultDTO{}
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: employ.ID, Answer: "keep"}, &res))
	assert.Equal(t, screens.RuleDecideResultDTO{Answer: "keep", Kept: 1, Dropped: 1}, res, "choosing a side drops the other")

	res = screens.RuleDecideResultDTO{}
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: note.ID, Answer: "drop"}, &res))
	assert.Equal(t, screens.RuleDecideResultDTO{Answer: "drop", Dropped: 1}, res)

	after := readDecideView(t, e, pid)
	assert.Empty(t, after.Decisions, "every question is answered")
	assert.Equal(t, 3, after.Rules)

	var refusal ErrorResponse
	assert.Equal(t, http.StatusNotFound, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: name.ID, Answer: "keep"}, &refusal), "a decided suggestion is not answered twice")

	var kept []screens.KeptRuleDTO
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, pid, "/rules/main/kept", nil, &kept))
	require.Len(t, kept, 3)
	byRule := map[string]screens.KeptRuleDTO{}
	for _, k := range kept {
		byRule[k.Rule] = k
	}
	signIn, ok := byRule["Write Sign In, not Sign in, Sign-in, Signin or SignIn."]
	require.True(t, ok, "the form picked is the one the rule writes: %v", kept)
	assert.Equal(t, "name", signIn.Kind)
	assert.Empty(t, signIn.From, "the server holds the rule in the ledger alone")
	assert.Equal(t, "docs", signIn.Applies)
	assert.Equal(t, screens.ContextActorDTO{Kind: "person"}, signIn.KeptBy, "what the reader kept reads as theirs")
	widget, ok := byRule["Write widget, not gadget."]
	require.True(t, ok)
	assert.Equal(t, screens.ContextActorDTO{Kind: "person", Name: "asgeir"}, widget.KeptBy, "another person's keep carries their name")
	assert.Equal(t, "word", widget.Kind)
	chosen, ok := byRule[employ.Sentence]
	require.True(t, ok, "%v", kept)
	assert.Equal(t, "docs", chosen.Applies, "the side chosen keeps the point it was suggested at")
}

// A decision needs the permission its subject takes over the point the
// suggestion holds at: a note needs PermManageVoice, a word rule
// PermManageTerms, and a member bounded to help decides nothing in docs.
func TestRulesDecide_PermissionAndReach_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	view := readDecideView(t, e, pid)
	note := decisionOf(t, view, "note")
	name := decisionOf(t, view, "name")

	caller.perms = platauth.PermViewContent | platauth.PermManageTerms
	var refusal ErrorResponse
	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: note.ID, Answer: "keep"}, &refusal))
	assert.Contains(t, refusal.Error, "needs the manage_voice permission")
	assert.Equal(t, contextOutsideReachCode, refusal.Code)

	caller.perms = platauth.PermViewContent | platauth.PermManageVoice
	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: name.ID, Answer: "drop"}, &refusal), "a name rule needs PermManageTerms")
	var res screens.RuleDecideResultDTO
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: note.ID, Answer: "keep"}, &res), "a note needs PermManageVoice alone")
	assert.Equal(t, 1, res.Kept)

	caller.perms = decideAll
	caller.reach = platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"surface": "help"})
	refusal = ErrorResponse{}
	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: name.ID, Answer: "keep"}, &refusal))
	assert.Contains(t, refusal.Error, "outside the coordinates you hold")
	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/assign",
		ruleAssignRequest{ID: name.ID, UserID: new("member-b")}, &refusal), "assigning needs what deciding needs")
	assert.Len(t, readDecideView(t, e, pid).Decisions, len(view.Decisions)-1, "a refused answer records nothing")
}

// An assignment shows on the question it names and in the assignee's queue,
// and assigning to no one removes it.
func TestRulesDecide_AssignAndQueue_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	view := readDecideView(t, e, pid)
	name := decisionOf(t, view, "name")
	side := decisionOf(t, view, "disagreement")

	var assigned ruleAssignment
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/assign",
		ruleAssignRequest{ID: name.ID, UserID: new("member-b")}, &assigned))
	assert.Equal(t, ruleAssignment{ID: name.ID, Assignee: &screens.ContextActorDTO{Kind: "person", Name: "member-b"}}, assigned)
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/assign",
		ruleAssignRequest{ID: side.Options[0].ID, UserID: new("member-a")}, &assigned))

	view = readDecideView(t, e, pid)
	assert.Equal(t, &screens.ContextActorDTO{Kind: "person", Name: "member-b"}, decisionOf(t, view, "name").Assignee)
	assert.Equal(t, &screens.ContextActorDTO{Kind: "person"}, decisionOf(t, view, "disagreement").Assignee,
		"a question assigned to the reader reads as theirs")
	assert.Nil(t, decisionOf(t, view, "note").Assignee)

	var queue contextQueue
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, "", "context/queue", nil, &queue))
	assert.Equal(t, contextQueue{Projects: []contextQueueProject{
		{ProjectID: pid, ProjectName: "decide-parity", Waiting: 3, Assigned: 1},
	}}, queue)

	caller.userID = "member-b"
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, "", "context/queue", nil, &queue))
	assert.Equal(t, 1, queue.Projects[0].Assigned)

	caller.userID = "member-a"
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/assign",
		ruleAssignRequest{ID: name.ID}, &assigned))
	assert.Nil(t, assigned.Assignee)
	assert.Nil(t, decisionOf(t, readDecideView(t, e, pid), "name").Assignee, "assigning to no one removes it")

	caller.perms = 0
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodGet, "", "context/queue", nil, &queue))
	assert.Empty(t, queue.Projects, "a project the caller cannot view is left out")
}

// A project whose context no push has bound asks nothing, an answer to it is
// refused, and a project the store does not hold is not found.
func TestRulesDecide_NoContextKey_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedContextObjectProject(t, cs)
	view := readDecideView(t, e, pid)
	assert.Empty(t, view.Decisions)
	assert.Empty(t, view.Project)
	var refusal ErrorResponse
	assert.Equal(t, http.StatusConflict, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: "x", Answer: "keep"}, &refusal))
	assert.Equal(t, ErrNoContextKey.Error(), refusal.Error)
	assert.Equal(t, http.StatusNotFound, decideRoute(t, e, http.MethodGet, "nope", "/rules/main/decide", nil, &refusal))
	assert.True(t, strings.Contains(refusal.Error, "not found"))
}

// pushSuggestions records, on a desktop sharing the parity project, two
// rules about one word that disagree: one across the brand, one in docs.
func pushSuggestions(t *testing.T, e *echo.Echo, pid string) {
	t.Helper()
	d := newDesktop(t, "prj_decideparityaaaaaaaa")
	d.observeAt(t, "use", "leverage", map[string]string{"brand": "acme"})
	d.observeAt(t, "apply", "leverage", map[string]string{"brand": "acme", "surface": "docs"})
	r := &routeRemote{h: e, base: "/api/v1/ws/" + pid + "/context/objects"}
	require.NoError(t, r.Put(t.Context(), d.push(t)...))
}

// sideOf returns the option of a disagreement whose sentence is sentence.
func sideOf(t *testing.T, d screens.RuleDecisionDTO, sentence string) screens.RuleOptionDTO {
	t.Helper()
	for _, o := range d.Options {
		if o.Sentence == sentence {
			return o
		}
	}
	require.Failf(t, "no side", "the disagreement has no side %q", sentence)
	return screens.RuleOptionDTO{}
}

// leverageQuestion is the disagreement pushSuggestions records.
func leverageQuestion(t *testing.T, view screens.RulesDecideViewDTO) screens.RuleDecisionDTO {
	t.Helper()
	for _, d := range view.Decisions {
		if d.Kind == "disagreement" && d.Label == "leverage" {
			return d
		}
	}
	require.Fail(t, "the view asks nothing about leverage")
	return screens.RuleDecisionDTO{}
}

// A collection the workspace owns declares no point: it does not stop a rule
// across the brand from applying to the whole project.
func TestRulesDecide_WorkspaceCollectionsAreNoPlace_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	require.NoError(t, cs.CreateCollection(t.Context(), &platstore.Collection{
		ProjectID: pid, Name: "default", Kind: platstore.CollectionUploaded, ItemLabel: "file",
		Stream: "main", IsDefault: true, Owner: "workspace",
	}))
	pushSuggestions(t, e, pid)
	d := leverageQuestion(t, readDecideView(t, e, pid))
	wide := sideOf(t, d, "Write use, not leverage.")
	assert.Contains(t, wide.Consequence, "Applies to the whole project.")
	narrow := sideOf(t, d, "Write apply, not leverage.")
	assert.Contains(t, narrow.Consequence, "Applies to docs.")
}

// One answer is written whole or not at all: keeping the brand-wide side
// drops the docs side and keeps the other, and a member bounded to docs is
// refused the keep, so the drop is not written either. Assigning the
// question needs every side in reach.
func TestRulesDecide_AnAnswerIsWrittenWhole_SQLite(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs := decideSQLiteServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	pushSuggestions(t, e, pid)
	d := leverageQuestion(t, readDecideView(t, e, pid))
	wide := sideOf(t, d, "Write use, not leverage.")
	narrow := sideOf(t, d, "Write apply, not leverage.")

	caller.reach = platauth.CoordinateReach{}.Add(platauth.CoordinateFilter{"surface": "docs"})
	var refusal ErrorResponse
	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: wide.ID, Answer: "keep"}, &refusal))
	assert.Equal(t, contextOutsideReachCode, refusal.Code)
	again := leverageQuestion(t, readDecideView(t, e, pid))
	assert.Len(t, again.Options, 2, "the drop of the docs side is not written without the keep")

	require.Equal(t, http.StatusForbidden, decideRoute(t, e, http.MethodPost, pid, "/rules/main/assign",
		ruleAssignRequest{ID: narrow.ID, UserID: new("member-b")}, &refusal),
		"the side asked by is in reach, the other side is not")

	caller.reach = nil
	var res screens.RuleDecideResultDTO
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/rules/main/decide",
		ruleDecideRequest{ID: wide.ID, Answer: "keep"}, &res))
	assert.Equal(t, screens.RuleDecideResultDTO{Answer: "keep", Kept: 1, Dropped: 1}, res)
	for _, q := range readDecideView(t, e, pid).Decisions {
		assert.NotEqual(t, "leverage", q.Label, "both sides are decided")
	}
}
