package server

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
)

// These tests pin the source-review summons: a source change that needs a
// decision must reach everyone entitled to make it, and nobody who is not —
// not the FIRST project member whose role carries PermEditSource.

const (
	srcWSID    = "ws-source"
	srcWSSlug  = "acme"
	srcProject = "proj-source"
)

// srcAuthStore is a scripted auth store: a workspace roster with roles, an
// optional explicit project membership per user, and per-workspace role
// overrides — exactly the three inputs project permission resolution reads.
type srcAuthStore struct {
	auth.AuthStore
	workspace      *platauth.Workspace
	members        []*platauth.Membership
	projectMembers []*platauth.ProjectMembership
	// projectPerms holds the effective permissions of an EXPLICIT project
	// membership. A user absent from this map has none, which the real store
	// reports as an error and the resolver answers with the workspace role.
	projectPerms map[string]platauth.Permission
	overrides    map[platauth.Role]platauth.Permission
	users        map[string]*platauth.User
	byEmail      map[string]*platauth.User
}

var errNoProjectMembership = errors.New("no project membership")

func (a *srcAuthStore) ListMembers(context.Context, string) ([]*platauth.Membership, error) {
	return a.members, nil
}

func (a *srcAuthStore) ListProjectMembers(context.Context, string) ([]*platauth.ProjectMembership, error) {
	return a.projectMembers, nil
}

func (a *srcAuthStore) ResolveProjectPermissions(_ context.Context, _, userID string) (*platauth.ResolvedPermission, error) {
	perms, ok := a.projectPerms[userID]
	if !ok {
		return nil, errNoProjectMembership
	}
	return &platauth.ResolvedPermission{Permissions: perms}, nil
}

func (a *srcAuthStore) GetWorkspaceRoleOverride(_ context.Context, _ string, role platauth.Role) (platauth.Permission, bool, error) {
	p, ok := a.overrides[role]
	return p, ok, nil
}

func (a *srcAuthStore) GetWorkspace(context.Context, string) (*platauth.Workspace, error) {
	return a.workspace, nil
}

func (a *srcAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	if u, ok := a.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("no such user")
}

func (a *srcAuthStore) GetUserByEmail(_ context.Context, email string) (*platauth.User, error) {
	if u, ok := a.byEmail[email]; ok {
		return u, nil
	}
	return nil, errors.New("no such user")
}

func (a *srcAuthStore) Close() error { return nil }

// newSrcAuthStore builds the roster the fan-out tests reason about:
//
//   - owner, admin   — workspace roles that carry PermEditSource by default
//   - translator     — a member role that does not
//   - editor         — a member whose PROJECT membership grants it anyway
//   - stripped       — an owner whose PROJECT membership takes it away
//   - guest          — a project member absent from the workspace roster
func newSrcAuthStore() *srcAuthStore {
	member := func(id string, role platauth.Role) *platauth.Membership {
		return &platauth.Membership{UserID: id, WorkspaceID: srcWSID, Role: role}
	}
	users := map[string]*platauth.User{}
	byEmail := map[string]*platauth.User{}
	for _, id := range []string{"owner", "admin", "translator", "editor", "stripped", "guest", "reviewer"} {
		u := &platauth.User{ID: id, Email: id + "@acme.test", Name: id}
		users[id] = u
		byEmail[u.Email] = u
	}
	return &srcAuthStore{
		workspace: &platauth.Workspace{ID: srcWSID, Slug: srcWSSlug, Name: "Acme"},
		members: []*platauth.Membership{
			member("owner", platauth.RoleOwner),
			member("admin", platauth.RoleAdmin),
			member("translator", platauth.RoleMember),
			member("editor", platauth.RoleMember),
			member("stripped", platauth.RoleOwner),
			member("reviewer", platauth.RoleMember),
		},
		projectMembers: []*platauth.ProjectMembership{
			{ProjectID: srcProject, UserID: "editor", WorkspaceID: srcWSID, RoleID: "source-editor"},
			{ProjectID: srcProject, UserID: "stripped", WorkspaceID: srcWSID, RoleID: "translator"},
			{ProjectID: srcProject, UserID: "guest", WorkspaceID: srcWSID, RoleID: "source-editor"},
		},
		projectPerms: map[string]platauth.Permission{
			"editor":   platauth.PermViewContent | platauth.PermEditSource,
			"stripped": platauth.PermViewContent | platauth.PermTranslate,
			"guest":    platauth.PermViewContent | platauth.PermEditSource,
		},
		users:   users,
		byEmail: byEmail,
	}
}

// recordingNotifier captures every real-time notification push, which is the
// last step of a successful dispatch: a recipient here is a recipient in fact.
type recordingNotifier struct {
	mu   sync.Mutex
	sent []*bstore.Notification
}

func (r *recordingNotifier) NotifyUser(userID string, n *bstore.Notification) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *n
	c.UserID = userID
	r.sent = append(r.sent, &c)
}

func (r *recordingNotifier) recipients() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, n := range r.sent {
		out = append(out, n.UserID)
	}
	return out
}

func (r *recordingNotifier) titleFor(userID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.sent {
		if n.UserID == userID {
			return n.Title
		}
	}
	return ""
}

// srcSummonsHarness wires a Server with a real SQLite content/task/notification
// store, the scripted auth store, and a recording notifier — the whole summons
// path with no Postgres and no network.
type srcSummonsHarness struct {
	srv    *Server
	cs     *sqlitestore.SQLiteStore
	authSt *srcAuthStore
	notes  *recordingNotifier
}

func newSrcSummonsHarness(t *testing.T) *srcSummonsHarness {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "source-summons.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	notes := &recordingNotifier{}
	dispatcher := event.NewNotificationDispatcher(bus, bstore.NewNotificationStore(cs.DB()), nil, notes, nil)
	t.Cleanup(dispatcher.Close)

	as := newSrcAuthStore()
	srv := &Server{
		ContentStore:           cs,
		TaskStore:              bstore.NewTaskStore(cs.DB()),
		AuthStore:              as,
		NotificationDispatcher: dispatcher,
	}
	return &srcSummonsHarness{srv: srv, cs: cs, authSt: as, notes: notes}
}

func (h *srcSummonsHarness) project() *platstore.Project {
	return &platstore.Project{ID: srcProject, WorkspaceID: srcWSID, Name: "Source"}
}

// The reach: every member whose EFFECTIVE project permissions carry
// PermEditSource, and nobody else. Explicit project membership decides where it
// exists — in both directions — and the workspace role stands in where it does
// not, which is exactly how ProjectAccessMiddleware answers the same question.
func TestSourceOwners_ReachesEveryEligibleMemberAndNoOther(t *testing.T) {
	h := newSrcSummonsHarness(t)

	owners := h.srv.sourceOwners(t.Context(), h.project())

	assert.ElementsMatch(t, []string{"owner", "admin", "editor", "guest"}, owners,
		"the fan-out must reach every member entitled to edit the source")
	assert.NotContains(t, owners, "translator", "a member role carries no source-editing right")
	assert.NotContains(t, owners, "stripped", "an explicit project membership overrides the workspace role, downward too")

	// Order is stable and assignment-worthy: the workspace roster leads, so the
	// task's single assignee does not shuffle between runs.
	require.NotEmpty(t, owners)
	assert.Equal(t, "owner", owners[0])
}

// A per-workspace role override is part of the rights, so it must be part of
// the reach: a workspace that grants its members source editing summons them.
func TestSourceOwners_HonoursWorkspaceRoleOverride(t *testing.T) {
	h := newSrcSummonsHarness(t)
	h.authSt.overrides = map[platauth.Role]platauth.Permission{
		platauth.RoleMember: platauth.PermViewContent | platauth.PermEditSource,
	}

	owners := h.srv.sourceOwners(t.Context(), h.project())

	assert.Contains(t, owners, "translator", "the override grants the right, so it must grant the summons")
	assert.NotContains(t, owners, "stripped", "an explicit project membership still wins over the role default")
}

// The summons itself: one task with a name on it, and a notification for every
// eligible owner rather than for whoever happened to be listed first.
func TestCreateSourceProposalTask_NotifiesEveryEligibleOwner(t *testing.T) {
	h := newSrcSummonsHarness(t)
	ctx := t.Context()
	p := &bstore.ProposedSourceChange{
		ID: "prop-1", WorkspaceID: srcWSID, ProjectID: srcProject, Stream: "main",
		ItemName: "a.json", BlockID: "b1", Kind: bstore.SourceProposalTextFix,
		FinderUser: "reviewer",
	}

	h.srv.createSourceProposalTask(ctx, h.project(), p)

	res, err := h.srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: srcWSID, ProjectID: srcProject, Type: string(bstore.TaskSourceReview),
	})
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1, "one task, not one per owner")
	assert.Equal(t, "owner", res.Tasks[0].AssigneeID, "the task carries a name so it is somebody's")
	assert.Equal(t, "prop-1", res.Tasks[0].Data["proposal_id"])

	assert.ElementsMatch(t, []string{"owner", "admin", "editor", "guest"}, h.notes.recipients(),
		"every eligible owner hears about it, and no ineligible member does")
}

// The demoted approval's reviewer is told. Approving the proposal clears the
// block's targets for every locale, so the work being undone is theirs, and the
// decision ledger is where their name is recorded.
func TestCreateSourceProposalTask_TellsTheDemotedApprovalsReviewer(t *testing.T) {
	h := newSrcSummonsHarness(t)
	ctx := t.Context()

	require.NoError(t, h.cs.CreateProject(ctx, &platstore.Project{
		ID: srcProject, Name: "Source", DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages: []model.LocaleID{model.LocaleFrench},
	}))
	require.NoError(t, h.cs.StoreItem(ctx, srcProject, "main", &platstore.Item{
		ProjectID: srcProject, Name: "a.json", Format: "json",
	}))
	blk := &model.Block{ID: "b1", Translatable: true}
	blk.SetSourceText("Hello")
	require.NoError(t, h.cs.StoreBlocksForItem(ctx, srcProject, "main", "a.json", []*model.Block{blk}))

	// The store owns both ids a block carries — the row id it is addressed by
	// and the durable unit identity the ledger keys decisions on — so read them
	// back rather than assuming the reader id survived the write.
	sb := storedBlock(t, h.cs, srcProject)
	require.NotEmpty(t, sb.SourceID)

	// An approval by "reviewer", recorded the way the editor records one: the
	// decider named by email when the auth store could resolve it.
	_, err := h.cs.UpsertUnitDecisions(ctx, srcProject, "main", []platstore.UnitDecision{
		{
			ItemName: "a.json", Unit: sb.SourceID, Variant: "fr",
			ReviewState: "approved", DecidedBy: "reviewer@acme.test",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
		// A machine decision is nobody to notify.
		{
			ItemName: "a.json", Unit: sb.SourceID, Variant: "de",
			ReviewState: "approved", DecidedBy: "ai/claude",
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
		},
	})
	require.NoError(t, err)

	h.srv.createSourceProposalTask(ctx, h.project(), &bstore.ProposedSourceChange{
		ID: "prop-2", WorkspaceID: srcWSID, ProjectID: srcProject, Stream: "main",
		ItemName: "a.json", BlockID: sb.Block.ID, Kind: bstore.SourceProposalTextFix,
		FoundInLocale: "fr", FinderUser: "translator",
	})

	assert.ElementsMatch(t, []string{"owner", "admin", "editor", "guest", "reviewer"}, h.notes.recipients(),
		"the reviewer whose approval is at stake hears about it too")
	assert.Contains(t, h.notes.titleFor("reviewer"), "affects your approval",
		"they are told their decision is at stake, not asked to make a new one")
	assert.Equal(t, "Source change proposed", h.notes.titleFor("owner"),
		"the owners still get the owner's wording")
}

// priorApprovers reads the ledger, so a block nobody has approved names nobody
// — the common case, and the one that must not invent a recipient.
func TestPriorApprovers_EmptyWithoutAnApproval(t *testing.T) {
	h := newSrcSummonsHarness(t)
	ctx := t.Context()
	require.NoError(t, h.cs.CreateProject(ctx, &platstore.Project{
		ID: srcProject, Name: "Source", DefaultSourceLanguage: model.LocaleEnglish,
	}))
	require.NoError(t, h.cs.StoreItem(ctx, srcProject, "main", &platstore.Item{
		ProjectID: srcProject, Name: "a.json", Format: "json",
	}))
	blk := &model.Block{ID: "b1", Translatable: true}
	blk.SetSourceText("Hello")
	require.NoError(t, h.cs.StoreBlocksForItem(ctx, srcProject, "main", "a.json", []*model.Block{blk}))

	sb := storedBlock(t, h.cs, srcProject)
	assert.Empty(t, h.srv.priorApprovers(ctx, srcProject, "main", sb.Block.ID))
}

// storedBlock reads back the project's single stored block, so a test can
// address it by the id the store assigned rather than the id the reader used.
func storedBlock(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string) *platstore.StoredBlock {
	t.Helper()
	blocks, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	return blocks[0]
}
