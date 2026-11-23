package server

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// testPlanSnapshotConfig refreshes in tens of milliseconds, so a test waits
// for the refresh rather than the production timings.
var testPlanSnapshotConfig = planSnapshotConfig{
	quiet:   150 * time.Millisecond,
	maxWait: 600 * time.Millisecond,
	poll:    20 * time.Millisecond,
	lease:   time.Minute,
	retry:   100 * time.Millisecond,
	maxAge:  time.Hour,
}

// startTestPlanSnapshots starts the snapshot refresh on s for the test.
func startTestPlanSnapshots(t *testing.T, s *Server) *planSnapshotRefresher {
	t.Helper()
	return startTestPlanSnapshotsWith(t, s, testPlanSnapshotConfig)
}

// startTestPlanSnapshotsWith starts the snapshot refresh on s with cfg.
func startTestPlanSnapshotsWith(t *testing.T, s *Server, cfg planSnapshotConfig) *planSnapshotRefresher {
	t.Helper()
	r := s.startPlanSnapshots(cfg)
	require.NotNil(t, r, "the PostgreSQL content store keeps snapshots")
	t.Cleanup(func() { r.Close(context.Background()) })
	return r
}

// planSnapshotRow reads a project's stored snapshot on main.
func planSnapshotRow(t *testing.T, s *Server, projID string) *platstore.ReviewPlanSnapshot {
	t.Helper()
	st, ok := platstore.ReviewPlanSnapshots(s.ContentStore)
	require.True(t, ok)
	row, err := st.GetReviewPlanSnapshot(t.Context(), projID, reviewInboxStream)
	require.NoError(t, err)
	return row
}

// settled waits until the project's snapshot has no change waiting.
func settled(t *testing.T, s *Server, projID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		row := planSnapshotRow(t, s, projID)
		return row != nil && !row.Changed
	}, 10*time.Second, 10*time.Millisecond)
}

// A project with no snapshot is counted on its first read and stored; the
// reads after it count nothing. The inbox reads the same snapshot.
//
// The refresh's clock is pinned just before a second turns over, so the
// first read's time and the stored row's time match only when the row
// stores the time the counts were stamped with.
func TestPlanSnapshot_MissingIsCountedOnceAndStored(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	at := time.Date(2026, 11, 20, 10, 0, 0, 999_999_999, time.UTC)
	cfg := testPlanSnapshotConfig
	cfg.clock = func() time.Time { return at }
	r := startTestPlanSnapshotsWith(t, s, cfg)
	require.Nil(t, planSnapshotRow(t, s, projID), "no snapshot before the first read")

	plan := readPlan(t, s, wsID, projID)
	require.Len(t, plan.Languages, 1)
	assert.Positive(t, plan.Languages[0].Waiting+plan.Languages[0].Held)
	assert.Equal(t, "2026-11-20T10:00:00Z", plan.AsOf, "the plan says when its counts were taken")
	assert.EqualValues(t, 1, r.computes.Load())
	row := planSnapshotRow(t, s, projID)
	require.NotNil(t, row)
	assert.NotEmpty(t, row.Data)
	assert.False(t, row.Changed)
	assert.True(t, at.Truncate(time.Microsecond).Equal(row.ComputedAt),
		"the row stores the time the counts were taken: %s", row.ComputedAt)

	again := readPlan(t, s, wsID, projID)
	assert.Equal(t, plan.Languages, again.Languages)
	inbox := readInbox(t, s, owner)
	require.NotEmpty(t, inbox.Projects)
	require.NotNil(t, inbox.Projects[0].Plan)
	assert.Equal(t, plan.AsOf, inbox.Projects[0].Plan.AsOf)
	assert.EqualValues(t, 1, r.computes.Load(), "later reads use the snapshot")
}

// A review decision reaches the plan once the burst it belongs to is quiet.
func TestPlanSnapshot_DecisionIsCountedAfterTheQuietPeriod(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, ids := containerProject(t, s, wsID)
	r := startTestPlanSnapshots(t, s)
	readPlan(t, s, wsID, projID)
	// Opening the deck classifies its translations, which is a change too.
	readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	time.Sleep(2 * testPlanSnapshotConfig.poll)
	settled(t, s, projID)

	before := readPlan(t, s, wsID, projID).Languages[0]
	computes := r.computes.Load()
	rec := decideFromBody(t, s, projID, ids["Intro text."], `{"target_locale":"fr","reviewed":true}`,
		changeCaller{user: owner, perms: platauth.PermAll, ws: wsID})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.Eventually(t, func() bool {
		fr := readPlan(t, s, wsID, projID).Languages[0]
		return fr.Waiting+fr.Held == before.Waiting+before.Held-1
	}, 10*time.Second, 20*time.Millisecond, "the decision is counted")
	assert.Greater(t, r.computes.Load(), computes)
}

// A burst of changes costs one compute.
//
// The test drives each pass of the refresh itself (poll is zero), so the
// burst and the quiet period are steps in order rather than sleeps raced
// against the runner. Every change is heard before the first pass writes
// them, so they become one mark. The quiet period is then only a lower bound:
// passes repeat until the mark is due, and a slower machine waits longer for
// the same single compute.
func TestPlanSnapshot_BurstComputesOnce(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	cfg := testPlanSnapshotConfig
	cfg.poll = 0
	r := startTestPlanSnapshotsWith(t, s, cfg)
	heard := make(chan struct{}, 64)
	hook := func(ev platev.Event) {
		if ev.Type == platev.EventReviewDecided && ev.ProjectID == projID {
			heard <- struct{}{}
		}
	}
	r.onHear.Store(&hook)
	readPlan(t, s, wsID, projID)
	require.EqualValues(t, 1, r.computes.Load())

	const burst = 20
	for range burst {
		s.EventBus.Publish(platev.Event{Type: platev.EventReviewDecided, ProjectID: projID})
	}
	for i := range burst {
		select {
		case <-heard:
		case <-time.After(10 * time.Second):
			t.Fatalf("heard %d of %d changes", i, burst)
		}
	}

	require.Eventually(t, func() bool {
		r.poll(t.Context())
		return r.computes.Load() > 1
	}, 10*time.Second, testPlanSnapshotConfig.quiet/4, "the burst is counted once it is quiet")
	for range 3 {
		r.poll(t.Context())
	}
	assert.EqualValues(t, 2, r.computes.Load(), "the burst is counted once")
	settled(t, s, projID)
}

// Under continuous change the snapshot is still counted at least once per
// maxWait.
func TestPlanSnapshot_ContinuousChangeIsCountedByMaxWait(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	r := startTestPlanSnapshots(t, s)
	readPlan(t, s, wsID, projID)

	deadline := time.Now().Add(3 * testPlanSnapshotConfig.maxWait)
	for time.Now().Before(deadline) {
		r.touch([]string{projID}, nil)
		time.Sleep(30 * time.Millisecond)
	}
	assert.Greater(t, r.computes.Load(), int64(1), "a change waits no longer than maxWait")
}

// A change that arrives while a compute runs leaves the snapshot due, and
// it is counted again.
func TestPlanSnapshot_ChangeDuringComputeComputesAgain(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	st, ok := platstore.ReviewPlanSnapshots(s.ContentStore)
	require.True(t, ok)
	r := startTestPlanSnapshots(t, s)
	readPlan(t, s, wsID, projID)

	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	hook := func(platstore.ReviewPlanSnapshotClaim) {
		once.Do(func() {
			close(started)
			<-release
		})
	}
	r.onCompute.Store(&hook)
	r.touch([]string{projID}, nil)
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the change was not computed")
	}
	// Another instance hears a change while this one computes.
	require.NoError(t, st.MarkReviewPlanSnapshotsChanged(t.Context(), []string{projID}, nil))
	close(release)

	settled(t, s, projID)
	assert.EqualValues(t, 3, r.computes.Load(), "the first read, the change, and the change during it")
}

// Two instances on one database count a change once between them.
func TestPlanSnapshot_TwoInstancesComputeOnce(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	other := &Server{
		ContentStore: s.ContentStore, AuthStore: s.AuthStore, VoiceStore: s.VoiceStore,
		ContextScanStore: s.ContextScanStore, wsStores: s.wsStores,
	}
	st, ok := platstore.ReviewPlanSnapshots(s.ContentStore)
	require.True(t, ok)
	a, b := startTestPlanSnapshots(t, s), startTestPlanSnapshots(t, other)
	readPlan(t, s, wsID, projID)
	require.EqualValues(t, 1, a.computes.Load()+b.computes.Load())

	// A slow compute widens the window in which a second claim could land.
	slow := func(platstore.ReviewPlanSnapshotClaim) { time.Sleep(300 * time.Millisecond) }
	a.onCompute.Store(&slow)
	b.onCompute.Store(&slow)
	require.NoError(t, st.MarkReviewPlanSnapshotsChanged(t.Context(), []string{projID}, nil))
	settled(t, s, projID)
	time.Sleep(2 * testPlanSnapshotConfig.quiet)
	assert.EqualValues(t, 2, a.computes.Load()+b.computes.Load(), "one instance counts the change")
}

// A snapshot taken under another involvement is counted again on the read
// that follows the change, before any refresh.
func TestPlanSnapshot_InvolvementChangeIsCountedOnTheNextRead(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	cfg := testPlanSnapshotConfig
	cfg.quiet, cfg.maxWait = time.Hour, time.Hour
	r := startTestPlanSnapshotsWith(t, s, cfg)
	before := readPlan(t, s, wsID, projID)
	require.NotEqual(t, settleNone, before.Policy.Settle)

	require.Equal(t, http.StatusNoContent,
		planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, platauth.PermAll).Code)
	after := readPlan(t, s, wsID, projID)
	assert.Equal(t, settleNone, after.Policy.Settle)
	assert.Zero(t, after.Languages[0].Held, "under every nothing is held")
	assert.EqualValues(t, 2, r.computes.Load())
}

// Without a refresh running the plan counts on every read and stores
// nothing.
func TestPlanSnapshot_NoRefreshCountsEachRead(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	plan := readPlan(t, s, wsID, projID)
	assert.NotEmpty(t, plan.AsOf)
	assert.Nil(t, planSnapshotRow(t, s, projID))
}

func TestPlanSnapshot_InterfaceOverrideRecountsPages(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID, _ := involvementProject(t, s, wsID)
	ctx := t.Context()
	const item = "app/settings.json"
	require.NoError(t, s.ContentStore.StoreItem(ctx, projID, "main", &platstore.Item{
		Name: item, Format: "txt", ItemType: "file", CollectionID: "col-app",
	}))
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, projID, "main", item, []*model.Block{
		pendingFrBlock("settings", "Settings", "Paramètres"),
	}))
	cfg := testPlanSnapshotConfig
	cfg.poll = time.Hour
	r := startTestPlanSnapshotsWith(t, s, cfg)
	before := readPlan(t, s, wsID, projID)
	require.Len(t, before.Languages, 1)
	require.Equal(t, 2, before.Languages[0].Pages)

	putOverrides(t, s, wsID, projID, owner, `{"overrides":[{"coordinates":{"product":"app"},"involvement":"interfaces"}]}`)
	after := readPlan(t, s, wsID, projID)
	require.Len(t, after.Languages, 1)
	assert.Equal(t, 3, after.Languages[0].Pages)
	assert.EqualValues(t, 2, r.computes.Load())
}
