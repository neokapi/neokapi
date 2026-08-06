package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/mailer"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/terms"
)

// These tests pin the summons: submitting a change-set must reach the people
// who can approve it. Without one, a `kapi push` that proposes governed
// concepts flips a status and leaves the reviewer with nothing to find.

// summonsHarness wires a server with a SQLite task store, a fake knowledge
// store, a scripted auth store, and a recording mail sender — everything the
// submit path touches, with no Postgres and no network.
type summonsHarness struct {
	srv   *Server
	fake  *fakeKnowledgeStore
	sends *recordingSender
	auth  *summonsAuthStore
}

const (
	summonsWSID   = "ws-summons"
	summonsWSSlug = "acme"
)

func newSummonsHarness(t *testing.T) *summonsHarness {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "summons.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	srv.wsStores.tbFactory = func() terms.Store {
		return &testTermStore{terms.NewInMemoryStore()}
	}
	fake := newFakeKnowledgeStore()
	srv.KnowledgeStore = fake
	srv.TaskStore = bstore.NewTaskStore(cs.DB())

	as := &summonsAuthStore{
		workspace: &platauth.Workspace{ID: summonsWSID, Slug: summonsWSSlug, Name: "Acme"},
		members: []*platauth.Membership{
			{UserID: "author", WorkspaceID: summonsWSID, Role: platauth.RoleOwner},
			{UserID: "owner", WorkspaceID: summonsWSID, Role: platauth.RoleOwner},
			{UserID: "translator", WorkspaceID: summonsWSID, Role: platauth.RoleMember},
		},
		users: map[string]*platauth.User{
			"author":     {ID: "author", Email: "author@acme.test", Name: "Ada"},
			"owner":      {ID: "owner", Email: "owner@acme.test", Name: "Otto"},
			"translator": {ID: "translator", Email: "t@acme.test", Name: "Tan"},
		},
	}
	srv.AuthStore = as

	sends := &recordingSender{}
	m, err := mailer.New(sends)
	require.NoError(t, err)
	srv.Mailer = m

	return &summonsHarness{srv: srv, fake: fake, sends: sends, auth: as}
}

// ctx builds an echo context on the workspace route with full permissions.
func (h *summonsHarness) ctx(method, target, body, actor string, params ...string) (echo.Context, *httptest.ResponseRecorder) {
	var r *strings.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	var req *http.Request
	if r != nil {
		req = httptest.NewRequest(method, target, r)
		req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	c := h.srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", summonsWSID)
	c.Set("user_id", actor)

	names, values := []string{"ws"}, []string{summonsWSSlug}
	for i := 0; i+1 < len(params); i += 2 {
		names = append(names, params[i])
		values = append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c, rec
}

// draftWithGovernedOps creates a draft change-set authored by "author" carrying
// n governed concept deletions, and returns its id.
func (h *summonsHarness) draftWithGovernedOps(t *testing.T, name string, n int) string {
	t.Helper()
	ctx := context.Background()
	cs := &knowledge.ChangeSet{
		WorkspaceID: summonsWSID,
		Name:        name,
		Status:      knowledge.ChangeSetDraft,
		CreatedBy:   "author",
	}
	require.NoError(t, h.fake.CreateChangeSet(ctx, cs))
	for i := range n {
		payload, err := json.Marshal(knowledge.ConceptDeletePayload{ConceptID: "c" + string(rune('a'+i))})
		require.NoError(t, err)
		require.NoError(t, h.fake.AppendOp(ctx, &knowledge.ChangeSetOp{
			WorkspaceID: summonsWSID,
			ChangesetID: cs.ID,
			Op:          knowledge.OpConceptDelete,
			Payload:     payload,
			CreatedBy:   "author",
		}))
	}
	return cs.ID
}

func (h *summonsHarness) openTasks(t *testing.T) []bstore.Task {
	t.Helper()
	res, err := h.srv.TaskStore.List(context.Background(), bstore.TaskQuery{
		WorkspaceID: summonsWSID,
		Statuses:    []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
	})
	require.NoError(t, err)
	return res.Tasks
}

// TestSubmitChangeSetOpensReviewTaskAndMails pins what a governed change-set
// arriving by push must leave behind: a task in the queue and a mail in every
// eligible reviewer's inbox — and never in the author's, who cannot approve it.
func TestSubmitChangeSetOpensReviewTaskAndMails(t *testing.T) {
	h := newSummonsHarness(t)
	id := h.draftWithGovernedOps(t, "kapi push — governed concepts", 57)

	c, rec := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/submit", "", "author", "id", id)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	tasks := h.openTasks(t)
	require.Len(t, tasks, 1, "a submitted change-set opens exactly one review task")
	task := tasks[0]
	assert.Equal(t, bstore.TaskReviewTerms, task.Type)
	assert.Equal(t, bstore.TaskPriorityHigh, task.Priority, "a governed change-set is high priority")
	assert.Equal(t, "owner", task.AssigneeID, "assigned to an eligible reviewer, never the author")
	assert.Equal(t, id, task.Data["changeset_id"])
	assert.Equal(t, "57", task.Data["op_count"])
	assert.Contains(t, task.Title, "57 changes")

	require.Len(t, h.sends.messages(), 1, "one reviewer, one mail")
	msg := h.sends.messages()[0]
	assert.Equal(t, "owner@acme.test", msg.to)
	assert.Contains(t, msg.subject, "Waiting for your review")
	assert.Contains(t, msg.subject, "kapi push — governed concepts")
	assert.Contains(t, msg.body, "/acme/context/changes/"+id,
		"the mail links the change-set's real route, not a path the app does not serve")
	assert.Contains(t, msg.body, "57 changes")
	assert.Contains(t, msg.body, "Ada", "the mail names who proposed it")
}

// TestSubmitChangeSetSkipsAuthorAndNonReviewers pins the reach: only members who
// hold the permission the approve gate checks are summoned.
func TestSubmitChangeSetSkipsAuthorAndNonReviewers(t *testing.T) {
	h := newSummonsHarness(t)
	id := h.draftWithGovernedOps(t, "ban a term", 1)

	c, rec := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/submit", "", "author", "id", id)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var recipients []string
	for _, m := range h.sends.messages() {
		recipients = append(recipients, m.to)
	}
	assert.Equal(t, []string{"owner@acme.test"}, recipients,
		"the author cannot approve their own change-set, and a member has no manage_brand")
	assert.Contains(t, h.openTasks(t)[0].Title, "1 change", "one change reads as singular")
}

// TestSubmitChangeSetWithNoEligibleReviewer keeps the work visible even when
// nobody else can approve it: the task still lands, unassigned.
func TestSubmitChangeSetWithNoEligibleReviewer(t *testing.T) {
	h := newSummonsHarness(t)
	h.auth.members = []*platauth.Membership{
		{UserID: "author", WorkspaceID: summonsWSID, Role: platauth.RoleOwner},
	}
	id := h.draftWithGovernedOps(t, "solo", 2)

	c, rec := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/submit", "", "author", "id", id)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code)

	tasks := h.openTasks(t)
	require.Len(t, tasks, 1)
	assert.Empty(t, tasks[0].AssigneeID, "unassigned, but in the queue where it can be seen")
	assert.Empty(t, h.sends.messages(), "nobody to mail")
}

// TestChangeSetDecisionClosesReviewTask covers the other end of the loop: a
// decided change-set must clear its reviewer's queue.
func TestChangeSetDecisionClosesReviewTask(t *testing.T) {
	for _, tc := range []struct {
		name   string
		decide func(h *summonsHarness, id string) error
	}{
		{"approve", func(h *summonsHarness, id string) error {
			c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/approve", `{"comment":"ok"}`, "owner", "id", id)
			return h.srv.HandleApproveChangeSet(c)
		}},
		{"reject", func(h *summonsHarness, id string) error {
			c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/reject", `{"comment":"no"}`, "owner", "id", id)
			return h.srv.HandleRejectChangeSet(c)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newSummonsHarness(t)
			id := h.draftWithGovernedOps(t, "decide me", 3)

			c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+id+"/submit", "", "author", "id", id)
			require.NoError(t, h.srv.HandleSubmitChangeSet(c))
			require.Len(t, h.openTasks(t), 1)

			require.NoError(t, tc.decide(h, id))
			assert.Empty(t, h.openTasks(t), "the decision closes the review task")
		})
	}
}

// TestReviewRequestMailRespectsPreference pins the one knob: a reviewer who has
// turned task email off is still notified in-app, but not mailed. With no
// preference store wired the default (on) applies, which the other tests cover.
func TestReviewRequestMailRespectsPreference(t *testing.T) {
	h := newSummonsHarness(t)
	// No preference store: send-on-submit is the default.
	assert.True(t, h.srv.wantsTaskEmail(context.Background(), "owner", summonsWSSlug))
}

// TestChangeSetReviewPath pins the link every summons hands out — the review
// URL in the request email and the LinkURL on each reviewer's notification —
// against the route the web app actually serves.
//
// The assertion reads the route table rather than restating a string. A link
// that is merely self-consistent passes while pointing at a route the app does
// not serve, and following it is indistinguishable from nothing having
// happened — for a summons that reaches people who cannot debug a 404.
//
// bowrain/plugin/commands.TestChangesetURLMatchesTheWebRoute makes the same
// assertion for the CLI's producer of this path. Both must fail on a rename.
func TestChangeSetReviewPath(t *testing.T) {
	assert.Equal(t, "/bowrain-hq/context/changes/FCfv5QTy", changeSetReviewPath("bowrain-hq", "FCfv5QTy"))

	// The web app's route table is the authority for that path. Read it: a
	// rename on the frontend must break this test, not the reviewer's click.
	routes := filepath.Join("..", "packages", "app", "src", "routes", "index.tsx")
	src, err := os.ReadFile(routes)
	require.NoError(t, err,
		"the web route table is the authority for the summons link; if it moved, point this test at it")
	// assert.Contains on a whole route table prints the whole route table, and a
	// test whose failure is forty pages of unrelated TSX is one people learn to
	// scroll past. Reduce to a bool first so the failure is the sentence.
	table := string(src)
	assert.True(t, strings.Contains(table, `path: "changes/$id"`),
		"%s no longer declares path: \"changes/$id\" — the summons links /<ws>/context/changes/<id>, so that link now 404s for every reviewer it reaches", routes)
	assert.False(t, strings.Contains(table, `path: "changesets/$id"`),
		"%s declares path: \"changesets/$id\" — the route the summons wrongly used before; if it is back, changeSetReviewPath must be revisited", routes)
}

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// recordingSender captures every message a Mailer dispatches.
type recordingSender struct {
	mu   sync.Mutex
	msgs []sentMessage
}

type sentMessage struct{ to, subject, body string }

func (r *recordingSender) Send(_ context.Context, to, subject, htmlBody string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, sentMessage{to: to, subject: subject, body: htmlBody})
	return nil
}

func (r *recordingSender) messages() []sentMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sentMessage(nil), r.msgs...)
}

// summonsAuthStore is the slice of auth.AuthStore the summons reads: workspace
// membership, per-role overrides, and user records. Everything else is nil and
// would panic if reached, which is the point — it pins the reach.
type summonsAuthStore struct {
	auth.AuthStore
	workspace *platauth.Workspace
	members   []*platauth.Membership
	users     map[string]*platauth.User
	overrides map[platauth.Role]platauth.Permission
}

func (m *summonsAuthStore) ListMembers(context.Context, string) ([]*platauth.Membership, error) {
	return m.members, nil
}

func (m *summonsAuthStore) GetWorkspaceRoleOverride(_ context.Context, _ string, role platauth.Role) (platauth.Permission, bool, error) {
	p, ok := m.overrides[role]
	return p, ok, nil
}

func (m *summonsAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, assertNoUser
}

func (m *summonsAuthStore) GetWorkspace(context.Context, string) (*platauth.Workspace, error) {
	return m.workspace, nil
}

// GetWorkspaceBySlug is what the job-failure summons falls back to: the
// extraction queue's table has no workspace id, so its events carry the slug
// alone.
func (m *summonsAuthStore) GetWorkspaceBySlug(_ context.Context, slug string) (*platauth.Workspace, error) {
	if m.workspace != nil && m.workspace.Slug == slug {
		return m.workspace, nil
	}
	return nil, assertNoUser
}

// Close satisfies the store lifecycle the server's shutdown walks.
func (m *summonsAuthStore) Close() error { return nil }

// assertNoUser is the sentinel a missing user resolves to.
var assertNoUser = errNoSuchUser{}

type errNoSuchUser struct{}

func (errNoSuchUser) Error() string { return "no such user" }
