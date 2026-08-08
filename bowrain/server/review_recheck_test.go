package server

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/brand"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRecheckHarness builds a Postgres-backed Server wired for the RV-E review
// re-check: content/auth/task/convergence-run stores, a channel event bus, a
// Postgres brand store, an in-memory workspace terms, the convergence
// orchestrator with the review-completion subscription (so a wrongful
// review.completed WOULD start a run — letting the tests assert RV-E starts
// none), and the RV-E re-check subscription itself. It seeds a workspace with an
// owner and default role templates and returns the server, the workspace id, and
// the owner user id.
func newRecheckHarness(t *testing.T) (*Server, string, string) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	as, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)
	bs, err := brand.NewPostgresBrandStore(db)
	require.NoError(t, err)

	reg := platconn.NewRegistry()
	formatReg := registry.NewFormatRegistry()
	toolReg := registry.NewToolRegistry()
	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)

	s := &Server{
		ConnectorReg:        reg,
		FormatRegistry:      formatReg,
		ToolRegistry:        toolReg,
		ContentStore:        cs,
		AuthStore:           as,
		BrandStore:          bs,
		EventBus:            bus,
		TaskStore:           bstore.NewTaskStore(db.DB),
		ConvergenceRunStore: bstore.NewConvergenceRunStore(db.DB),
		SourceProposalStore: bstore.NewSourceProposalStore(db.DB),
		Services:            service.NewServices(cs, reg, formatReg, toolReg),
		wsStores:            newWorkspaceStores(),
	}
	// In-memory workspace terms (no PgDB wired on wsStores), shared per slug.
	s.wsStores.tbFactory = func() terms.Store {
		return &testTermStore{terms.NewInMemoryStore()}
	}
	s.convergence = newConvergenceOrchestrator(s)
	s.subscribeReviewCompletion()
	s.subscribeReviewRecheck()

	ctx := context.Background()
	ws := &platauth.Workspace{ID: "ws-rc", Name: "RC", Slug: "rc"}
	require.NoError(t, as.CreateWorkspace(ctx, ws))
	owner := &platauth.User{ID: "owner-rc", Email: "owner@rc.test", Name: "Owner"}
	require.NoError(t, as.CreateUser(ctx, owner))
	require.NoError(t, as.AddMember(ctx, ws.ID, owner.ID, platauth.RoleOwner))
	require.NoError(t, as.SeedDefaultRoleTemplates(ctx, ws.ID))
	return s, ws.ID, owner.ID
}

// reviewedBlock builds a translatable block whose fr target is already reviewed —
// the "approved before the term was marked" starting point RV-E reacts to.
func reviewedBlock(id, source, frTarget string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("fr", frTarget)
	b.Target("fr").Status = model.TargetStatusReviewed
	return b
}

// frPendingCount is the block-derived pending-review count for fr — the review
// queue's truth (targetPendingReview), which RV-E's demotions must move.
func frPendingCount(t *testing.T, s *Server, projID string) int {
	t.Helper()
	blocks, err := s.ContentStore.GetBlocks(context.Background(), platstore.BlockQuery{ProjectID: projID, Stream: "main"})
	require.NoError(t, err)
	n := 0
	for _, sb := range blocks {
		if targetPendingReview(sb.Block, "fr") {
			n++
		}
	}
	return n
}

func frStatus(t *testing.T, s *Server, projID, blockID string) model.TargetStatus {
	t.Helper()
	sb, err := s.ContentStore.GetBlock(context.Background(), projID, "main", blockID)
	require.NoError(t, err)
	return sb.Block.Target("fr").Status
}

func openFrReviewTasks(t *testing.T, s *Server, wsID, projID string) int {
	t.Helper()
	res, err := s.TaskStore.List(context.Background(), bstore.TaskQuery{
		WorkspaceID: wsID, ProjectID: projID, Type: string(bstore.TaskReview),
		Statuses: []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
		Limit:    500,
	})
	require.NoError(t, err)
	n := 0
	for _, tk := range res.Tasks {
		if tk.Data["locale"] == "fr" {
			n++
		}
	}
	return n
}

// awaitFrReviewTasks waits for the fr review queue to hold want open tasks.
//
// The demotion and the review task are written by two INDEPENDENT subscribers
// of the same event — the re-check writes the block, the automation engine
// writes the task — so having watched the demotion land says nothing about the
// task. Reading the queue the instant the block flips is a race the test loses
// under load; give the second subscriber its own chance to arrive.
func awaitFrReviewTasks(t *testing.T, s *Server, wsID, projID string, want int) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		res, err := s.TaskStore.List(context.Background(), bstore.TaskQuery{
			WorkspaceID: wsID, ProjectID: projID, Type: string(bstore.TaskReview),
			Statuses: []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
			Limit:    500,
		})
		if err != nil {
			return false
		}
		n := 0
		for _, tk := range res.Tasks {
			if tk.Data["locale"] == "fr" {
				n++
			}
		}
		return n == want
	}, 20*time.Second, 50*time.Millisecond, "the fr review queue should hold %d open task(s)", want)
}

// TestReviewRecheck_ConceptForbiddenTermDemotesAndRequeues (RV-E) is the required
// end-to-end proof: a governed project with two APPROVED (reviewed) fr targets —
// one that will violate a term, one that stays clean — has a concept marked with a
// forbidden fr term. The resulting concept event pulls exactly the violating
// target back to draft and re-queues fr for review, leaves the conforming target
// alone, moves the block-derived pending count 0 → 1, and starts NO convergence
// run (no cascade). Replaying the event demotes nothing new and creates no
// duplicate task (idempotent).
func TestReviewRecheck_ConceptForbiddenTermDemotesAndRequeues(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// b1's fr target uses "Utiliser"; b2's is clean. Both reviewed.
	b1 := reviewedBlock("b1", "Use the app", "Utiliser l'application")
	b2 := reviewedBlock("b2", "Hello", "Bonjour")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})

	// Sanity: both approved → zero pending review, both persisted at reviewed.
	require.Equal(t, 0, frPendingCount(t, s, projID), "both targets start approved")
	require.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Use the app"]))
	require.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Hello"]))

	// Mark a concept with a forbidden fr term "utiliser" (the governed outcome; RV-E
	// reacts to the resulting event, downstream of the change-set gate).
	tb, err := s.wsStores.getTB("rc")
	require.NoError(t, err)
	cid := id.New()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     cid,
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "utiliser", Locale: "fr", Status: model.TermForbidden},
			{Text: "employer", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	// The governed term-status flip publishes concept.term.status_changed carrying
	// the concept id and the workspace id.
	publishConceptEvent := func() {
		s.EventBus.Publish(platev.Event{
			ID:          id.New(),
			Type:        knowledge.EventConceptTermStatusChanged,
			Source:      "knowledge",
			WorkspaceID: wsID,
			Data:        map[string]string{"concept_id": cid},
			Timestamp:   time.Now().UTC(),
		})
	}
	publishConceptEvent()

	// RV-E demotes exactly the violating target and re-queues fr.
	require.Eventually(t, func() bool {
		return frStatus(t, s, projID, ids["Use the app"]) == model.TargetStatusDraft
	}, 20*time.Second, 50*time.Millisecond, "the violating approved target must be demoted to draft")

	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Hello"]),
		"the conforming target is left untouched (no needless churn)")
	assert.Equal(t, 1, frPendingCount(t, s, projID), "the pending-review count goes 0 → 1")
	awaitFrReviewTasks(t, s, wsID, projID, 1) // fr re-enters the review queue with exactly one task

	// No cascade: RV-E only demotes + re-queues; it must not start a convergence run.
	runs, err := s.ConvergenceRunStore.ListRuns(ctx, projID, 20)
	require.NoError(t, err)
	assert.Empty(t, runs, "RV-E must not trigger a convergence run")

	// Idempotency: replaying the event re-checks the (now draft) target, which no
	// longer sits at reviewed, so nothing is demoted again and no duplicate task is
	// created.
	publishConceptEvent()
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, model.TargetStatusDraft, frStatus(t, s, projID, ids["Use the app"]))
	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Hello"]))
	assert.Equal(t, 1, frPendingCount(t, s, projID), "replay changes nothing")
	assert.Equal(t, 1, openFrReviewTasks(t, s, wsID, projID), "no duplicate review task on replay")
	runs, err = s.ConvergenceRunStore.ListRuns(ctx, projID, 20)
	require.NoError(t, err)
	assert.Empty(t, runs, "replay still starts no run (no loop)")
}

// TestReviewRecheck_ConceptMandatedTermAbsenceDemotesAndRequeues (RV-F) is the
// complementary end-to-end proof for the glossary-ABSENCE direction: a governed
// project with two APPROVED (reviewed) fr targets whose sources both use a
// concept — one target uses the mandated fr rendering, one does not — has that
// concept given a preferred fr term. The resulting concept event pulls exactly
// the target that FAILS term-check (missing the mandated rendering) back to
// draft, leaves the conforming one alone, moves the pending count 0 → 1, creates
// exactly one review task, starts NO convergence run, and is idempotent on
// replay. This is the direction RV-E did not cover (it caught only targets that
// CONTAIN a forbidden term).
func TestReviewRecheck_ConceptMandatedTermAbsenceDemotesAndRequeues(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// Both sources use "app"; b1's fr target omits the mandated "application",
	// b2's uses it. Both reviewed.
	b1 := reviewedBlock("b1", "Open the app", "Ouvrir le truc")
	b2 := reviewedBlock("b2", "Close the app", "Fermer l'application")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})
	require.Equal(t, 0, frPendingCount(t, s, projID), "both targets start approved")

	// Give the "app" concept a mandated (preferred) fr rendering "application". The
	// governed outcome; RV-F reacts to the resulting concept event.
	tb, err := s.wsStores.getTB("rc")
	require.NoError(t, err)
	cid := id.New()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:     cid,
		Source: terms.TermSourceTerminology,
		Terms: []terms.Term{
			{Text: "app", Locale: "en", Status: model.TermApproved},
			{Text: "application", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	publishConceptEvent := func() {
		s.EventBus.Publish(platev.Event{
			ID:          id.New(),
			Type:        knowledge.EventConceptTermStatusChanged,
			Source:      "knowledge",
			WorkspaceID: wsID,
			Data:        map[string]string{"concept_id": cid},
			Timestamp:   time.Now().UTC(),
		})
	}
	publishConceptEvent()

	// RV-F demotes exactly the target missing the mandated rendering.
	require.Eventually(t, func() bool {
		return frStatus(t, s, projID, ids["Open the app"]) == model.TargetStatusDraft
	}, 20*time.Second, 50*time.Millisecond, "the approved target missing the mandated term must be demoted to draft")

	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Close the app"]),
		"the target that already uses the mandated rendering is left untouched")
	assert.Equal(t, 1, frPendingCount(t, s, projID), "the pending-review count goes 0 → 1")
	awaitFrReviewTasks(t, s, wsID, projID, 1) // fr re-enters the review queue with exactly one task

	runs, err := s.ConvergenceRunStore.ListRuns(ctx, projID, 20)
	require.NoError(t, err)
	assert.Empty(t, runs, "RV-F must not trigger a convergence run")

	// Idempotency: the demoted target is now draft (< reviewed), so a replay
	// re-checks nothing new and creates no duplicate task.
	publishConceptEvent()
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, model.TargetStatusDraft, frStatus(t, s, projID, ids["Open the app"]))
	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Close the app"]))
	assert.Equal(t, 1, frPendingCount(t, s, projID), "replay changes nothing")
	assert.Equal(t, 1, openFrReviewTasks(t, s, wsID, projID), "no duplicate review task on replay")
}

// TestReviewRecheck_RulePromotionScopedToPromotedTerm (RV-E) proves the brand
// rule path: promoting a forbidden vocabulary rule demotes an approved target that
// contains the JUST-promoted term, while a target that only trips an OLDER rule is
// left alone — the re-check is scoped to the promoted term, so a promotion does
// not retroactively churn every pre-existing violation.
func TestReviewRecheck_RulePromotionScopedToPromotedTerm(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	ctx := context.Background()

	// A profile with two forbidden terms: "ancien" (old) and "utiliser" (just
	// promoted). Both are enforced rules, but only "utiliser" is being promoted now.
	profile := &coreprofile.VoiceProfile{
		ID:    "p-rc",
		Name:  "RC Voice",
		Scope: wsID,
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{
				{Term: "ancien"},
				{Term: "utiliser"},
			},
		},
	}
	require.NoError(t, s.BrandStore.CreateProfile(ctx, profile))

	// b1's target contains the just-promoted "utiliser"; b2's contains only the
	// older "ancien". Both approved.
	b1 := reviewedBlock("b1", "Use it", "Il faut utiliser ceci")
	b2 := reviewedBlock("b2", "Old flow", "L'ancien parcours")
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{b1, b2})
	require.Equal(t, 0, frPendingCount(t, s, projID))

	// Promote the "utiliser" rule: publishBrandRuleEvent stashes the workspace id on
	// ProjectID and the term on Data["term"].
	s.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      EventBrandVoiceRulePromoted,
		Source:    "brand",
		ProjectID: wsID,
		Data:      map[string]string{"profile_id": profile.ID, "term": "utiliser"},
		Timestamp: time.Now().UTC(),
	})

	require.Eventually(t, func() bool {
		return frStatus(t, s, projID, ids["Use it"]) == model.TargetStatusDraft
	}, 20*time.Second, 50*time.Millisecond, "the target with the promoted term must be demoted")

	assert.Equal(t, model.TargetStatusReviewed, frStatus(t, s, projID, ids["Old flow"]),
		"a target tripping only an OLDER rule is not swept up (scoped to the promoted term)")
	assert.Equal(t, 1, frPendingCount(t, s, projID), "only the promoted-term violation re-enters review")
	awaitFrReviewTasks(t, s, wsID, projID, 1)
}
