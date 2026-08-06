package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oneJobStore answers GetJob with a fixed row (or a fixed error) and refuses
// everything else, which is all announceJobFailure asks of a store.
type oneJobStore struct {
	JobStore
	job *TranslationJob
	err error
}

func (s *oneJobStore) GetJob(_ context.Context, _ string) (*TranslationJob, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.job, nil
}

func TestJobKind(t *testing.T) {
	tests := []struct {
		item string
		want string
	}{
		{item: "en.json", want: "translation"},
		{item: SyncPushItemName, want: "sync"},
		{item: ForgeIngestItemName, want: "ingest"},
		{item: ModelSweepItemName, want: "model-sweep"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			j := &TranslationJob{ItemName: tt.item}
			assert.Equal(t, tt.want, j.Kind())
		})
	}
}

func TestFailureSummaryCollapsesAndTruncates(t *testing.T) {
	assert.Equal(t, "openai: API error 503: down",
		failureSummary("openai: API error 503:\n   down\n"))

	long := strings.Repeat("a", jobFailureSummaryLimit*2)
	got := failureSummary(long)
	assert.LessOrEqual(t, len(got), jobFailureSummaryLimit+len("…"))
	assert.True(t, strings.HasSuffix(got, "…"))

	// A truncation that lands mid-rune must not emit a broken one. "é" is two
	// bytes, so a limit-length prefix of them cuts one in half.
	multibyte := strings.Repeat("é", jobFailureSummaryLimit)
	assert.True(t, isValidUTF8(failureSummary(multibyte)),
		"truncated summary must stay valid UTF-8")
}

func TestPublishJobFailedCarriesWhatTheSummonsNeeds(t *testing.T) {
	bus := &recordingBus{}
	job := &TranslationJob{
		ID:            "job-1",
		WorkspaceSlug: "acme",
		WorkspaceID:   "ws-1",
		ProjectID:     "proj-1",
		ItemName:      "en.json",
		TargetLocale:  "nb",
		Stream:        "main",
		PushID:        "push-1",
		StepID:        "step-1",
		CreatedBy:     "user-1",
	}

	publishJobFailed(bus, job, "openai: API error 401: invalid api key")

	events := bus.published()
	require.Len(t, events, 1)
	ev := events[0]

	assert.Equal(t, platev.EventFlowFailed, ev.Type)
	assert.Equal(t, "translation-worker", ev.Source)
	assert.Equal(t, "proj-1", ev.ProjectID)
	assert.Equal(t, "ws-1", ev.WorkspaceID)
	assert.Equal(t, "user-1", ev.Actor)
	assert.NotEmpty(t, ev.ID)
	assert.False(t, ev.Timestamp.IsZero())

	// The consumers key on the slug: the activity recorder files the activity
	// under it and notification preferences are stored against it.
	assert.Equal(t, "acme", ev.Data["workspace_slug"])
	assert.Equal(t, "ws-1", ev.Data["workspace_id"])
	assert.Equal(t, "job-1", ev.Data["job_id"])
	assert.Equal(t, "translation", ev.Data["job_kind"])
	assert.Equal(t, "en.json", ev.Data["item"])
	assert.Equal(t, "nb", ev.Data["target_locale"])
	assert.Equal(t, "user-1", ev.Data["initiator"])
	assert.Contains(t, ev.Data["error"], "invalid api key")
}

func TestPublishJobFailedOmitsUnsetFields(t *testing.T) {
	bus := &recordingBus{}
	publishJobFailed(bus, &TranslationJob{
		ID:            "job-2",
		WorkspaceSlug: "acme",
		ItemName:      SyncPushItemName,
	}, "blob store not configured")

	ev := bus.published()[0]
	assert.Equal(t, "sync", ev.Data["job_kind"])
	assert.Equal(t, "sync-worker", ev.Source)
	// A platform-started job names no initiator; the summons reads that as
	// "tell the workspace's owners".
	_, hasInitiator := ev.Data["initiator"]
	assert.False(t, hasInitiator)
	_, hasLocale := ev.Data["target_locale"]
	assert.False(t, hasLocale)
}

func TestPublishJobFailedWithoutBusIsNoOp(t *testing.T) {
	assert.NotPanics(t, func() {
		publishJobFailed(nil, &TranslationJob{ID: "job-3"}, "boom")
	})
}

// announceJobFailure must publish for a row that is failed, and stay silent for
// one that is not. The loop's failure branch is also reached by a claim that
// errored and by a stale worker whose write was refused — in both, the row is
// still running and a summons would call people to a job that has not failed.
func TestAnnounceJobFailureOnlyWhenTheRowAgrees(t *testing.T) {
	tests := []struct {
		name        string
		status      JobStatus
		wantPublish bool
	}{
		{name: "failed row is announced", status: StatusFailed, wantPublish: true},
		{name: "still processing is not", status: StatusProcessing, wantPublish: false},
		{name: "raced to completed is not", status: StatusCompleted, wantPublish: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &recordingBus{}
			deps := &WorkerDeps{
				EventBus: bus,
				JobStore: &oneJobStore{job: &TranslationJob{
					ID: "job-1", WorkspaceSlug: "acme", Status: tt.status,
					Error: "retries exhausted",
				}},
			}
			announceJobFailure(t.Context(), deps, "job-1", errors.New("retries exhausted"))
			assert.Len(t, bus.published(), boolToInt(tt.wantPublish))
		})
	}
}

func TestAnnounceJobFailureSurvivesAnUnreadableRow(t *testing.T) {
	bus := &recordingBus{}
	deps := &WorkerDeps{
		EventBus: bus,
		JobStore: &oneJobStore{err: errors.New("connection refused")},
	}
	assert.NotPanics(t, func() {
		announceJobFailure(t.Context(), deps, "job-1", errors.New("boom"))
	})
	assert.Empty(t, bus.published())
}

func TestPublishExtractionJobFailed(t *testing.T) {
	bus := &recordingBus{}
	publishExtractionJobFailed(bus, &ExtractionJob{
		ID:            "ext-1",
		WorkspaceSlug: "acme",
		ProjectID:     "proj-1",
		ItemName:      "en.json",
		Locale:        "en",
	}, "extract terms: model refused")

	ev := bus.published()[0]
	assert.Equal(t, platev.EventFlowFailed, ev.Type)
	assert.Equal(t, "extraction-worker", ev.Source)
	assert.Equal(t, "extraction", ev.Data["job_kind"])
	assert.Equal(t, "acme", ev.Data["workspace_slug"])
	assert.Equal(t, "ext-1", ev.Data["job_id"])
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func isValidUTF8(s string) bool { return strings.ToValidUTF8(s, "�") == s }
