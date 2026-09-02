package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newReviewLoopHarness builds a Postgres-backed Server wired for the full review
// → delivery loop: content/auth/task/convergence-run stores, a channel event
// bus, the convergence orchestrator, a live stub forge connector bound for
// delivery, and both durable subscriptions (forge delivery + review completion).
// It seeds a workspace with an owner and the default role templates and returns
// the server, the delivery stub, the workspace ID, and the owner user ID.
func newReviewLoopHarness(t *testing.T) (*Server, *stubForgeConnector, string, string) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	as, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)

	reg := platconn.NewRegistry()
	stub := &stubForgeConnector{id: "conn-rl"}
	reg.Register("forge", venueconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		return stub, nil
	})
	formatReg := registry.NewFormatRegistry()
	toolReg := registry.NewToolRegistry()
	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)

	s := &Server{
		ConnectorReg:         reg,
		FormatRegistry:       formatReg,
		ToolRegistry:         toolReg,
		ContentStore:         cs,
		AuthStore:            as,
		EventBus:             bus,
		TaskStore:            bstore.NewTaskStore(db.DB),
		ConvergenceRunStore:  bstore.NewConvergenceRunStore(db.DB),
		ConnectorConfigStore: bstore.NewConnectorConfigStore(db.DB, nil),
		Services:             service.NewServices(cs, reg, formatReg, toolReg),
		wsStores:             newWorkspaceStores(),
	}
	s.convergence = newConvergenceOrchestrator(s)
	s.subscribeForgeDelivery()
	s.subscribeReviewCompletion()

	ctx := context.Background()
	ws := &platauth.Workspace{ID: "ws-rl", Name: "RL", Slug: "rl"}
	require.NoError(t, as.CreateWorkspace(ctx, ws))
	owner := &platauth.User{ID: "owner-rl", Email: "owner@rl.test", Name: "Owner"}
	require.NoError(t, as.CreateUser(ctx, owner))
	require.NoError(t, as.AddMember(ctx, ws.ID, owner.ID, platauth.RoleOwner))
	require.NoError(t, as.SeedDefaultRoleTemplates(ctx, ws.ID))
	return s, stub, ws.ID, owner.ID
}

// seedGovernedProject creates a governed (workflow default) fr-only project with
// source_gate:none (so a completing run settles nothing), one item, and the
// given blocks. Returns the project ID and the stored block IDs keyed by source.
func seedGovernedProject(t *testing.T, s *Server, wsID string, blocks []*model.Block) (string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	proj := &platstore.Project{
		Name:                  "RL Proj",
		WorkspaceID:           wsID,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		Properties:            map[string]string{"source_gate": "none"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))
	require.NoError(t, s.ContentStore.StoreItem(ctx, proj.ID, "main", &platstore.Item{
		Name: "greetings.txt", Format: "txt", ItemType: "file",
	}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, proj.ID, "main", "greetings.txt", blocks))

	stored, err := s.ContentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: proj.ID, Stream: "main", ItemName: "greetings.txt",
	})
	require.NoError(t, err)
	ids := map[string]string{}
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}
	return proj.ID, ids
}

// bindForgeConnector persists a forge connector config bound to the project and
// registers the live stub, so a completed run's delivery is observable.
func bindForgeConnector(t *testing.T, s *Server, wsID, projID string) {
	t.Helper()
	ctx := context.Background()
	_, err := s.ConnectorConfigStore.Upsert(ctx, &bstore.ConnectorConfig{
		ID: "conn-rl", WorkspaceID: wsID, Type: "forge", Name: "site",
		Config: map[string]string{
			"id": "conn-rl", "repo": "https://github.com/acme/site.git",
			"branch": "main", "project_id": projID,
		},
	})
	require.NoError(t, err)
	_, err = s.Services.Connector.AddConnector(wsID, "forge", map[string]string{"id": "conn-rl"})
	require.NoError(t, err)
}

func seedReviewTask(t *testing.T, s *Server, wsID, projID, locale, assignee string) {
	t.Helper()
	require.NoError(t, s.TaskStore.Create(context.Background(), &bstore.Task{
		WorkspaceID: wsID, ProjectID: projID, Stream: "main", Type: bstore.TaskReview,
		Status: bstore.TaskStatusOpen, Priority: bstore.TaskPriorityNormal,
		Title: "Review " + locale, AssigneeID: assignee, CreatedBy: "system",
		Data: map[string]string{"locale": locale},
	}))
}

// callApprovePassing invokes HandleApprovePassing with PermReview.
func callApprovePassing(t *testing.T, s *Server, wsID, projID, userID, body string) (*httptest.ResponseRecorder, ApprovePassingResponse) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rl/"+projID+"/review/approve-passing", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("rl", projID)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", wsID)
	c.Set("user_id", userID)
	require.NoError(t, s.HandleApprovePassing(c))
	var resp ApprovePassingResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	}
	return rec, resp
}

func reviewRun(t *testing.T, s *Server, projID string) *bstore.ConvergenceRun {
	t.Helper()
	runs, err := s.ConvergenceRunStore.ListRuns(context.Background(), projID, 20)
	require.NoError(t, err)
	for _, r := range runs {
		if r.Trigger == "review" {
			return r
		}
	}
	return nil
}

// TestReviewLoop_LastApprovalAutoContinuesToDelivery (RV-B) is the end-to-end
// proof: a governed project with drafted blocks and an open review task ships
// nothing while a block is still pending review, then — the moment the last
// pending review is approved — auto-continues through review.completed → a
// completing convergence run → forge delivery of the approved content, WITHOUT a
// second round of review tasks (the anti-loop gate).
func TestReviewLoop_LastApprovalAutoContinuesToDelivery(t *testing.T) {
	s, stub, wsID, ownerID := newReviewLoopHarness(t)
	ctx := context.Background()

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b1.SetTargetText("fr", "Bonjour")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Goodbye")
	b2.SetTargetText("fr", "Au revoir")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})
	seedReviewTask(t, s, wsID, projID, "fr", ownerID)
	bindForgeConnector(t, s, wsID, projID)

	// Approve the FIRST of two: a block is still pending, so nothing continues.
	rec, err := callReviewBlockAs(t, s, projID, ids["Hello"], "fr", true, platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	time.Sleep(300 * time.Millisecond)
	assert.Nil(t, stub.Published(), "delivery must not fire while a block is still pending review")
	assert.Nil(t, reviewRun(t, s, projID), "no completing run before the last review is approved")
	open, err := s.TaskStore.CountOpenByType(ctx, wsID, projID, "review")
	require.NoError(t, err)
	assert.Equal(t, 1, open, "the fr review task stays open while a block is pending")

	// Approve the LAST: the project has zero pending review → auto-continue.
	rec, err = callReviewBlockAs(t, s, projID, ids["Goodbye"], "fr", true, platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// The completing run converges and delivery materializes the approved content.
	require.Eventually(t, func() bool {
		return len(stub.Published()) > 0
	}, 30*time.Second, 50*time.Millisecond, "approving the last review must auto-continue to delivery")

	published := stub.Published()
	require.Len(t, published, 1)
	assert.Equal(t, model.LocaleID("fr"), published[0].Locale)
	require.Len(t, published[0].Blocks, 2, "both approved blocks ship")

	run := reviewRun(t, s, projID)
	require.NotNil(t, run, "review completion must start a completing run")
	require.Eventually(t, func() bool {
		r := reviewRun(t, s, projID)
		return r != nil && r.FinishedAt != nil
	}, 30*time.Second, 50*time.Millisecond)
	run = reviewRun(t, s, projID)
	assert.Equal(t, bstore.ConvergenceRunConverged, run.State, "everything approved → the completing run converges")
	assert.Equal(t, 0, run.Passes, "the completing run produces nothing new (anti-loop gate)")

	// Anti-loop: the completing run must NOT fan out a second round of review
	// tasks. Only the original task exists, now completed.
	res, err := s.TaskStore.List(ctx, bstore.TaskQuery{WorkspaceID: wsID, ProjectID: projID, Type: "review"})
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1, "no new review tasks may be created by the completing run")
	assert.Equal(t, bstore.TaskStatusCompleted, res.Tasks[0].Status)
	open, err = s.TaskStore.CountOpenByType(ctx, wsID, projID, "review")
	require.NoError(t, err)
	assert.Equal(t, 0, open, "the review queue is empty and stays empty (no loop)")
}

// TestApprovePassing_ExcludesFailingBlocks (RV-D) proves the bulk endpoint
// approves only blocks that clear the ship bar and leaves failing ones pending:
// the approved block becomes deliverable, the failing one does not, the review
// queue is not yet empty (so nothing auto-delivers), and the counts are exact.
func TestApprovePassing_ExcludesFailingBlocks(t *testing.T) {
	s, stub, wsID, ownerID := newReviewLoopHarness(t)
	ctx := context.Background()

	// b1 passes the checks; b2's target carries a U+FFFD replacement character,
	// an error-severity check finding, so it must be excluded and left pending.
	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b1.SetTargetText("fr", "Bonjour")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Goodbye")
	b2.SetTargetText("fr", "Au revoir�") // U+FFFD → error-severity check finding
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})
	seedReviewTask(t, s, wsID, projID, "fr", ownerID)
	bindForgeConnector(t, s, wsID, projID)

	rec, resp := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, resp.Approved, "only the passing block is approved")
	assert.Equal(t, 1, resp.Skipped, "the failing block is excluded")
	assert.Equal(t, 1, resp.RemainingPending, "the failing block is still pending review")
	assert.False(t, resp.ReviewCompleted, "review is not complete while a block remains pending")

	// The approved block is reviewed; the failing one stays below reviewed.
	g1, err := s.ContentStore.GetBlock(ctx, projID, "main", ids["Hello"])
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusReviewed, g1.Block.Target("fr").Status)
	g2, err := s.ContentStore.GetBlock(ctx, projID, "main", ids["Goodbye"])
	require.NoError(t, err)
	assert.Less(t, g2.Block.Target("fr").Status.Rank(), model.TargetStatusReviewed.Rank(),
		"a failing block must not be approved")

	// The approved block is now deliverable; the failing one is withheld.
	proj, err := s.ContentStore.GetProject(ctx, projID)
	require.NoError(t, err)
	items, err := s.materializeDelivery(ctx, proj)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Len(t, items[0].Blocks, 1, "only the approved block is deliverable")
	assert.Equal(t, "Bonjour", items[0].Blocks[0].SourceText())

	// No auto-delivery yet: the review queue is not empty.
	time.Sleep(300 * time.Millisecond)
	assert.Nil(t, stub.Published(), "a mixed project does not auto-deliver until every block is reviewed")
	assert.Nil(t, reviewRun(t, s, projID))
}

// TestApprovePassing_AllPassingAutoContinues (RV-D + RV-B) proves the bulk path's
// auto-continuation: when every pending block clears the bar, approve-passing
// empties the review queue and the loop runs straight through to delivery.
func TestApprovePassing_AllPassingAutoContinues(t *testing.T) {
	s, stub, wsID, ownerID := newReviewLoopHarness(t)

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b1.SetTargetText("fr", "Bonjour")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Goodbye")
	b2.SetTargetText("fr", "Au revoir")
	projID, _ := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})
	seedReviewTask(t, s, wsID, projID, "fr", ownerID)
	bindForgeConnector(t, s, wsID, projID)

	rec, resp := callApprovePassing(t, s, wsID, projID, ownerID, `{}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 2, resp.Approved)
	assert.Equal(t, 0, resp.Skipped)
	assert.Equal(t, 0, resp.RemainingPending)
	assert.True(t, resp.ReviewCompleted, "clearing the whole queue continues the loop")

	require.Eventually(t, func() bool {
		return len(stub.Published()) > 0
	}, 30*time.Second, 50*time.Millisecond, "bulk-approving the whole queue must auto-continue to delivery")
	published := stub.Published()
	require.Len(t, published, 1)
	require.Len(t, published[0].Blocks, 2)
}

// TestAddProjectCreatorMembership (founder fix) proves a project creator is made
// a review-capable project member — the gap that left governed review with no
// one to assign to.
func TestAddProjectCreatorMembership(t *testing.T) {
	s, _, wsID, ownerID := newReviewLoopHarness(t)
	ctx := context.Background()

	proj := &platstore.Project{
		Name: "Created", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
	}
	require.NoError(t, s.ContentStore.CreateProject(ctx, proj))

	// No membership yet.
	_, err := s.AuthStore.GetProjectMembership(ctx, proj.ID, ownerID)
	require.Error(t, err)

	s.addProjectCreatorMembership(ctx, wsID, proj.ID, ownerID)

	pm, err := s.AuthStore.GetProjectMembership(ctx, proj.ID, ownerID)
	require.NoError(t, err, "creator must become a project member")
	rt, err := s.AuthStore.GetRoleTemplate(ctx, wsID, pm.RoleID)
	require.NoError(t, err)
	assert.True(t, rt.Permissions.Has(platauth.PermReview), "creator's role must be review-capable")

	// Idempotent: a second call does not error or duplicate.
	s.addProjectCreatorMembership(ctx, wsID, proj.ID, ownerID)
	members, err := s.AuthStore.ListProjectMembers(ctx, proj.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
}
