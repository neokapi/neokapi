package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/platformconfig"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func TestTermScanDue(t *testing.T) {
	completed := &planScanDTO{Status: string(jobs.ContextScanStatusCompleted)}
	scanned := termScanCoverage{Blocks: 1000, Collections: []string{"app", "docs"}}
	tests := []struct {
		name        string
		last        *planScanDTO
		hasRecorded bool
		now         termScanCoverage
		concepts    int
		want        bool
	}{
		{name: "no content yet", now: termScanCoverage{}, want: false},
		{name: "first content", now: termScanCoverage{Blocks: 3}, want: true},
		{name: "first content but the workspace has concepts", now: termScanCoverage{Blocks: 3}, concepts: 2, want: false},
		{name: "a scan is queued", last: &planScanDTO{Status: "queued"}, hasRecorded: true, now: termScanCoverage{Blocks: 5000}, want: false},
		{name: "a scan is processing", last: &planScanDTO{Status: "processing"}, hasRecorded: true, now: termScanCoverage{Blocks: 5000}, want: false},
		{name: "the last scan failed", last: &planScanDTO{Status: "failed"}, hasRecorded: true, now: scanned, want: true},
		{name: "nothing changed", last: completed, hasRecorded: true, now: scanned, want: false},
		{name: "a new collection", last: completed, hasRecorded: true, now: termScanCoverage{Blocks: 1001, Collections: []string{"app", "blog", "docs"}}, want: true},
		{name: "a collection removed", last: completed, hasRecorded: true, now: termScanCoverage{Blocks: 900, Collections: []string{"app"}}, want: false},
		{name: "growth below a fifth", last: completed, hasRecorded: true, now: termScanCoverage{Blocks: 1199, Collections: scanned.Collections}, want: false},
		{name: "growth of a fifth", last: completed, hasRecorded: true, now: termScanCoverage{Blocks: 1200, Collections: scanned.Collections}, want: true},
		{name: "no recorded coverage", last: completed, now: termScanCoverage{Blocks: 9000}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, why := termScanDue(tt.last, scanned, tt.hasRecorded, tt.now, tt.concepts)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want, why != "", "a due scan names its trigger")
		})
	}

	// 500 new blocks make a scan due however many were scanned.
	big := termScanCoverage{Blocks: 10_000}
	due, _ := termScanDue(completed, big, true, termScanCoverage{Blocks: 10_499}, 0)
	assert.False(t, due)
	due, _ = termScanDue(completed, big, true, termScanCoverage{Blocks: 10_500}, 0)
	assert.True(t, due)
}

func TestQuotaResetsAt(t *testing.T) {
	assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		jobs.QuotaResetsAt(time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC)))
	assert.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		jobs.QuotaResetsAt(time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC)))
}

// newAutoScanHarness is the review harness with a real scan job store, an
// in-memory queue and a platform AI provider.
func newAutoScanHarness(t *testing.T) (*Server, string, *jobs.ChannelQueue) {
	t.Helper()
	s, wsID, _ := newRecheckHarness(t)
	q := wireTermProposalJobs(t, s)
	s.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{AIProvider: "openai"})
	return s, wsID, q
}

// emptyProject is a project with no content.
func emptyProject(t *testing.T, s *Server, wsID string) string {
	t.Helper()
	proj := &platstore.Project{
		Name: "Empty", WorkspaceID: wsID, DefaultSourceLanguage: "en",
		TargetLanguages: []model.LocaleID{"fr"},
	}
	require.NoError(t, s.ContentStore.CreateProject(t.Context(), proj))
	return proj.ID
}

func projectProps(t *testing.T, s *Server, projID string) map[string]string {
	t.Helper()
	p, err := s.ContentStore.GetProject(t.Context(), projID)
	require.NoError(t, err)
	return p.Properties
}

// finishScan records a completed scan as the project's last one.
func finishScan(t *testing.T, s *Server, wsID, projID string) {
	t.Helper()
	job := &jobs.ContextScanJob{
		ID: id.New(), WorkspaceID: wsID, WorkspaceSlug: "rc",
		Status: jobs.ContextScanStatusCompleted, Progress: 100,
	}
	require.NoError(t, s.ContextScanStore.CreateContextScanJob(t.Context(), job))
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{propTermScanJob: job.ID}))
}

// addSourceBlocks stores n more translatable blocks in a new item.
func addSourceBlocks(t *testing.T, s *Server, projID, item string, n int) {
	t.Helper()
	ctx := t.Context()
	blocks := make([]*model.Block, 0, n)
	for i := range n {
		b := &model.Block{ID: fmt.Sprintf("%s-%d", item, i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Sentence %d of %s.", i, item))
		blocks = append(blocks, b)
	}
	require.NoError(t, s.ContentStore.StoreItem(ctx, projID, "main", &platstore.Item{Name: item, Format: "txt", ItemType: "file"}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, projID, "main", item, blocks))
}

func keyStep(p reviewPlanResponse) planStepDTO {
	for _, st := range p.Steps {
		if st.Level == planContext {
			return st
		}
	}
	return planStepDTO{}
}

func TestAutoScan_StartsOnFirstContentAndNotAgainWhileRunning(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)

	job := s.maybeStartTermScan(t.Context(), projID, "main", "rc")
	require.NotNil(t, job, "first content starts a scan")
	props := projectProps(t, s, projID)
	assert.Equal(t, job.ID, props[propTermScanJob])
	assert.Equal(t, "12", props[propTermScanBlocks], "the scan records the blocks it reads")

	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"), "a queued scan is not started twice")

	step := keyStep(readPlan(t, s, wsID, projID))
	assert.Equal(t, stepNotStarted, step.State)
	assert.Equal(t, "queued", step.Scan)
}

func TestAutoScan_IgnoresAnotherStream(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "feature-x", "rc"))
}

func TestAutoScan_WaitsForContent(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID := emptyProject(t, s, wsID)
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, scanWaitingContent, plan.Context.ScanWaiting)
	assert.Equal(t, scanWaitingContent, keyStep(plan).Scan)
}

func TestAutoScan_NotWhenTheWorkspaceHasConcepts(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	tb, err := s.wsStores.getTerms(t.Context(), "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{ID: "c-server", Terms: []terms.Term{
		{Text: "server", Locale: "en", Status: model.TermPreferred},
	}}))
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
}

func TestAutoScan_NewCollectionStartsAnotherScan(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.NotNil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
	finishScan(t, s, wsID, projID)
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"), "nothing new since the scan")

	require.NoError(t, s.ContentStore.CreateCollection(t.Context(), &platstore.Collection{
		ProjectID: projID, Name: "blog", Kind: platstore.CollectionUploaded,
	}))
	assert.NotNil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
	assert.Contains(t, projectProps(t, s, projID)[propTermScanCollections], "blog")
}

func TestAutoScan_GrowthStartsAnotherScan(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.NotNil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
	finishScan(t, s, wsID, projID)

	addSourceBlocks(t, s, projID, "one.txt", 1)
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"), "one block in twelve is below a fifth")

	addSourceBlocks(t, s, projID, "more.txt", 2)
	assert.NotNil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"), "three blocks in twelve reach a fifth")
}

func TestAutoScan_WaitsForTheAILimit(t *testing.T) {
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
	projID, _ := containerProject(t, s, wsID)

	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"), "a scan over the limit would only fail")
	assert.Empty(t, projectProps(t, s, projID)[propTermScanJob])

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, scanWaitingQuota, plan.Context.ScanWaiting)
	want := jobs.QuotaResetsAt(time.Now()).Format(time.RFC3339)
	assert.Equal(t, want, plan.Context.QuotaResetsAt)
	step := keyStep(plan)
	assert.Equal(t, scanWaitingQuota, step.Scan)
	assert.Equal(t, want, step.QuotaResetsAt)
}

func TestAutoScan_WaitsForAnAIProvider(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	s.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{})
	projID, _ := containerProject(t, s, wsID)
	assert.Nil(t, s.maybeStartTermScan(t.Context(), projID, "main", "rc"))
	assert.Equal(t, scanNoAI, keyStep(readPlan(t, s, wsID, projID)).Scan)
}

func TestAutoScan_PlanSaysTheNextPushStartsIt(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	assert.Equal(t, scanWaitingPush, keyStep(readPlan(t, s, wsID, projID)).Scan)
}

func TestAutoScan_StartsFromAPushEvent(t *testing.T) {
	s, wsID, q := newAutoScanHarness(t)
	s.subscribeTermScanOnPush()
	projID, _ := containerProject(t, s, wsID)

	ev := platev.Event{
		ID: id.New(), Type: platev.EventPushCompleted, ProjectID: projID,
		Data: map[string]string{"stream": "main", "workspace_slug": "rc"},
	}
	s.EventBus.Publish(ev)
	bus, ok := s.EventBus.(*handledBus)
	require.True(t, ok)
	select {
	case err := <-bus.done(termScanOnPushGroup, ev.ID):
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("the term scan subscriber never handled the push")
	}

	req := nextTermProposalJob(t, s, q)
	assert.Equal(t, projID, req.Project)
	assert.NotEmpty(t, projectProps(t, s, projID)[propTermScanJob])
}

func TestHandleStartReviewPlanScan_RecordsTheCoverage(t *testing.T) {
	s, wsID, _ := newAutoScanHarness(t)
	projID, _ := containerProject(t, s, wsID)
	rec := planWrite(t, s, s.HandleStartReviewPlanScan, wsID, projID, "", platauth.PermAll)
	require.Equal(t, 202, rec.Code, rec.Body.String())
	props := projectProps(t, s, projID)
	assert.NotEmpty(t, props[propTermScanJob])
	assert.Equal(t, "12", props[propTermScanBlocks])
}

// A saved project whose only state is a context waiting for the AI limit
// carries its plan in brief, so a client shows the wait rather than nothing.
func TestReviewInbox_CarriesThePlanOfASavedProjectThatOnlyWaits(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	wireTermProposalJobs(t, s)
	s.PlatformConfig = platformconfig.NewService(nil, platformconfig.Defaults{AIProvider: "openai"})
	db := pgtest.NewTestDB(t)
	quota, err := jobs.NewQuotaStore(db)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO ai_quotas (workspace_slug, workspace_id, monthly_limit) VALUES ('rc', $1, 10)`, wsID)
	require.NoError(t, err)
	require.NoError(t, quota.RecordUsage(t.Context(), jobs.AIUsageRecord{
		WorkspaceSlug: "rc", WorkspaceID: wsID, ProjectID: "p", Model: "m", TotalTokens: 11,
	}))
	s.QuotaStore = quota
	projID, _ := containerProject(t, s, wsID)
	confirmAs(t, s, wsID, projID, owner)
	rec := languagesCall(t, s, s.HandlePutReviewLanguages, http.MethodPut, wsID, projID, owner, platauth.PermAll, nil, `{"followed":[]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	inbox := readInbox(t, s, owner)
	require.Len(t, inbox.Projects, 1)
	p := inbox.Projects[0]
	assert.True(t, p.Saved)
	assert.Zero(t, p.Waiting, "nothing waits in the languages followed")
	require.NotNil(t, p.Plan, "the step that waits is carried")
	assert.Empty(t, p.Plan.Next)
	want := jobs.QuotaResetsAt(time.Now()).Format(time.RFC3339)
	assert.Equal(t, []planWaitDTO{{Level: planContext, Reason: scanWaitingQuota, QuotaResetsAt: want}}, p.Plan.Waiting)
}
