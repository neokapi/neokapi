package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// addConcept stores a concept in the harness workspace's terms store.
func addConcept(t *testing.T, s *Server, cp terms.Concept) {
	t.Helper()
	tb, err := s.wsStores.getTerms(t.Context(), "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), cp))
}

// keptConcept is a concept with an English preferred term and nothing else.
func keptConcept(cid, term string) terms.Concept {
	return terms.Concept{ID: cid, Terms: []terms.Term{{Text: term, Locale: "en", Status: model.TermPreferred}}}
}

// queuedJob reads the next job the queue holds, failing after a few seconds.
func queuedJob(t *testing.T, s *Server, q *jobs.ChannelQueue) (jobs.ContextScanRequest, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	jobID, ack, _, err := q.Dequeue(ctx)
	if err != nil {
		return jobs.ContextScanRequest{}, false
	}
	ack()
	job, err := s.ContextScanStore.GetContextScanJob(t.Context(), jobID)
	require.NoError(t, err)
	var req jobs.ContextScanRequest
	require.NoError(t, json.Unmarshal(job.Request, &req))
	return req, true
}

func TestTermProposalTrigger(t *testing.T) {
	added := platev.RecipeChange{Source: "project", Targets: &platev.TargetsChange{Before: []string{"fr"}, After: []string{"fr", "nb"}}}
	proposed := platev.RecipeChange{Source: recipeChangeSourceProposal, Targets: added.Targets}
	removed := platev.RecipeChange{Source: "project", Targets: &platev.TargetsChange{Before: []string{"fr", "nb"}, After: []string{"fr"}}}
	cases := []struct {
		name string
		ev   platev.Event
		want bool
	}{
		{"a term scan completed", platev.Event{Type: platev.EventContextScanCompleted}, true},
		{"a push landed", platev.Event{Type: platev.EventPushCompleted}, true},
		{"a member added a target language", platev.Event{Type: platev.EventRecipeChanged, Data: added.Data()}, true},
		{"a target language proposed for the recipe", platev.Event{Type: platev.EventRecipeChanged, Data: proposed.Data()}, false},
		{"a target language removed", platev.Event{Type: platev.EventRecipeChanged, Data: removed.Data()}, false},
		{"a project update naming new locales", platev.Event{Type: platev.EventProjectUpdated, Data: map[string]string{"new_locales": "nb"}}, true},
		{"another project update", platev.Event{Type: platev.EventProjectUpdated}, false},
		{"an unrelated event", platev.Event{Type: platev.EventReviewDecided}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, termProposalTrigger(tc.ev))
		})
	}
}

func TestProjectDebouncer_RunsOnceAfterTheCallsStop(t *testing.T) {
	var d projectDebouncer
	var runs atomic.Int32
	for range 5 {
		d.after("p1", 50*time.Millisecond, func() { runs.Add(1) })
	}
	d.after("p2", 50*time.Millisecond, func() { runs.Add(10) })
	require.Eventually(t, func() bool { return runs.Load() == 11 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(11), runs.Load(), "five calls for one project run once")
}

// TestAutoPropose_StartsWhenAConceptWaits: a concept with a source term and
// no term in the project's target language starts the whole-project job,
// which the project records and the plan's context step shows. No second job
// starts while the first is queued.
func TestAutoPropose_StartsWhenAConceptWaits(t *testing.T) {
	s, wsID, q := newAutoScanHarness(t)
	projID := emptyProject(t, s, wsID)
	addConcept(t, s, keptConcept("c-flow", "flowgraph"))

	jobID := s.maybeProposeTerms(t.Context(), projID, "rc")
	require.NotEmpty(t, jobID)
	assert.Equal(t, jobID, projectProps(t, s, projID)[propTermProposalJob])
	req, ok := queuedJob(t, s, q)
	require.True(t, ok)
	assert.Equal(t, projID, req.Project)
	require.NotNil(t, req.TermProposals)
	assert.Empty(t, req.TermProposals.Concepts, "the job covers the whole project")

	assert.Empty(t, s.maybeProposeTerms(t.Context(), projID, "rc"), "a queued job is not started twice")

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, 1, plan.Context.Proposable)
	require.NotNil(t, plan.Context.Proposals)
	assert.Equal(t, "queued", keyStep(plan).Proposals)
}

// TestAutoPropose_NotWhenNothingWaits: a translated concept, a candidate, a
// proposal already filed and another project's concept start nothing.
func TestAutoPropose_NotWhenNothingWaits(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID := emptyProject(t, s, wsID)
	done := keptConcept("c-done", "server")
	done.Terms = append(done.Terms, terms.Term{Text: "serveur", Locale: "fr", Status: model.TermPreferred})
	addConcept(t, s, done)
	addConcept(t, s, terms.Concept{ID: "c-cand", Terms: []terms.Term{{Text: "dashpanel", Locale: "en", Status: model.TermProposed}}})
	other := keptConcept("c-other", "pipeline")
	other.Properties = map[string]string{termcandidate.PropProject: "another-project"}
	addConcept(t, s, other)
	filed := keptConcept("c-filed", "workspace")
	filed.Properties = map[string]string{termcandidate.PropTranslationPrefix + "fr": "espace de travail"}
	addConcept(t, s, filed)

	assert.Empty(t, s.maybeProposeTerms(t.Context(), projID, "rc"))
	plan := readPlan(t, s, wsID, projID)
	assert.Zero(t, plan.Context.Proposable)
	assert.Empty(t, keyStep(plan).Proposals)
}

// TestAutoPropose_WaitsForTheAILimit: over the monthly AI limit no job
// starts, and the plan's context step says it waits for the limit to reset.
func TestAutoPropose_WaitsForTheAILimit(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	db := pgtest.NewTestDB(t)
	quota, err := jobs.NewQuotaStore(db)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO ai_quotas (workspace_slug, workspace_id, monthly_limit) VALUES ('rc', $1, 10)`, wsID)
	require.NoError(t, err)
	require.NoError(t, quota.RecordUsage(t.Context(), jobs.AIUsageRecord{
		WorkspaceSlug: "rc", WorkspaceID: wsID, ProjectID: "p", Model: "m", TotalTokens: 11,
	}))
	s.QuotaStore = quota
	projID := emptyProject(t, s, wsID)
	addConcept(t, s, keptConcept("c-flow", "flowgraph"))

	assert.Empty(t, s.maybeProposeTerms(t.Context(), projID, "rc"), "a job over the limit would only fail")
	assert.Empty(t, projectProps(t, s, projID)[propTermProposalJob])

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, scanWaitingQuota, plan.Context.ProposalsWaiting)
	want := jobs.QuotaResetsAt(time.Now()).Format(time.RFC3339)
	step := keyStep(plan)
	assert.Equal(t, scanWaitingQuota, step.Proposals)
	assert.Equal(t, want, step.QuotaResetsAt)
}

// TestAutoPropose_WaitsForAnAIProvider: with no platform AI provider no job
// starts, and the step says so.
func TestAutoPropose_WaitsForAnAIProvider(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	s.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{})
	projID := emptyProject(t, s, wsID)
	addConcept(t, s, keptConcept("c-flow", "flowgraph"))
	assert.Empty(t, s.maybeProposeTerms(t.Context(), projID, "rc"))
	assert.Equal(t, scanNoAI, keyStep(readPlan(t, s, wsID, projID)).Proposals)
}

// TestAutoPropose_EventsInAQuietPeriodStartOneJob: a completed scan and a
// new target language that arrive together start one job, after the quiet
// period.
func TestAutoPropose_EventsInAQuietPeriodStartOneJob(t *testing.T) {
	quiet := termProposalQuiet
	termProposalQuiet = 50 * time.Millisecond
	t.Cleanup(func() { termProposalQuiet = quiet })

	s, wsID, q := newAutoScanHarness(t)
	s.subscribeTermProposals()
	projID := emptyProject(t, s, wsID)
	addConcept(t, s, keptConcept("c-flow", "flowgraph"))

	added := platev.RecipeChange{WorkspaceSlug: "rc", Source: "project", Targets: &platev.TargetsChange{After: []string{"fr"}}}
	events := []platev.Event{
		{ID: id.New(), Type: platev.EventContextScanCompleted, ProjectID: projID, Data: map[string]string{"workspace_slug": "rc"}},
		{ID: id.New(), Type: platev.EventRecipeChanged, ProjectID: projID, Data: added.Data()},
	}
	bus, ok := s.EventBus.(*handledBus)
	require.True(t, ok)
	for _, ev := range events {
		s.EventBus.Publish(ev)
		select {
		case err := <-bus.done(termProposalsGroup, ev.ID):
			require.NoError(t, err)
		case <-time.After(20 * time.Second):
			t.Fatal("the term proposal subscriber never handled the event")
		}
	}

	req, ok := queuedJob(t, s, q)
	require.True(t, ok, "the quiet period ends in one job")
	require.NotNil(t, req.TermProposals)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, _, _, err := q.Dequeue(ctx)
	assert.Error(t, err, "no second job starts")
}

// TestContextDeck_KeepTranslationStartsTheForms: keeping a proposed target
// term starts a job that writes the kept term's forms, for that concept and
// language alone. With no AI provider, the keep starts none.
func TestContextDeck_KeepTranslationStartsTheForms(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb, srv := deckServer(t, caller)
	q := wireTermProposalJobs(t, srv)
	srv.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{AIProvider: "openai"})
	pid := seedDecideParity(t, e, cs)
	proposeTranslation(t, tb, "c-memory", "fr", "mémoire de contenu")

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), translationCard("c-memory", "fr"))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	req := nextTermProposalJob(t, srv, q)
	assert.Equal(t, pid, req.Project)
	require.NotNil(t, req.TermForms)
	assert.Equal(t, []string{"c-memory"}, req.TermForms.Concepts)
	assert.Equal(t, []model.LocaleID{"fr"}, req.TermForms.Locales)
}

// TestContextDeck_KeepTranslationWithoutAIStartsNoForms: with no platform
// AI provider the keep stands and no forms job starts.
func TestContextDeck_KeepTranslationWithoutAIStartsNoForms(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb, srv := deckServer(t, caller)
	q := wireTermProposalJobs(t, srv)
	srv.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{})
	pid := seedDecideParity(t, e, cs)
	proposeTranslation(t, tb, "c-memory", "fr", "mémoire de contenu")

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), translationCard("c-memory", "fr"))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	assert.Equal(t, deckOutcomeKept, out.Outcome)
	_, queued := queuedJob(t, srv, q)
	assert.False(t, queued)
}
