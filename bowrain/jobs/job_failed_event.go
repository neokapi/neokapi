package jobs

import (
	"context"
	"log/slog"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

// The publisher of flow.failed (AD-013), without which a failed job ends at the
// row that recorded it: the notification dispatcher and the activity recorder
// both have a case for the event, and both are unreachable until something
// emits it. A translation that exhausts its retries, a push whose manifest will
// not parse, or an extraction that dies on the first block otherwise leaves a
// row with status='failed' and an error string, visible only to whoever thinks
// to look.
//
// It publishes once per TERMINAL failure: the worker loop calls it from the
// branch that has already ruled out both retry and deferral, so a job that will
// be tried again says nothing and only the last word travels.
//
// The event carries workspace_slug as well as the workspace id, because the
// consumers key on the slug — the activity recorder writes it as the activity's
// workspace, and notification preferences are stored against it. An event that
// carried only the id would file the failure in a workspace nobody reads.

// jobFailureSummaryLimit bounds the error text that travels on the bus. Provider
// errors can carry an entire response body; the summons puts this line in a
// notification and an email subject line, and a paragraph of JSON there helps
// nobody. The full text stays on the job row.
const jobFailureSummaryLimit = 240

// failureSummary reduces an error to the single line a human reads first:
// collapsed whitespace, truncated at jobFailureSummaryLimit.
func failureSummary(msg string) string {
	s := strings.Join(strings.Fields(msg), " ")
	if len(s) <= jobFailureSummaryLimit {
		return s
	}
	// Cut on a rune boundary — a truncated multi-byte error message must not
	// arrive as replacement characters.
	cut := jobFailureSummaryLimit
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut]) + "…"
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// publishJobFailed announces one terminally failed translation-queue job.
//
// It is a no-op without a bus, which is the normal state of a worker deployed
// without Redis: the failure is still recorded on the row and logged, it simply
// summons nobody. Publishing is synchronous but cheap, and the caller is on its
// way to ACKing a dead message either way.
func publishJobFailed(bus platev.EventBus, job *TranslationJob, cause string) {
	if bus == nil || job == nil {
		return
	}
	data := map[string]string{
		"workspace_slug": job.WorkspaceSlug,
		"workspace_id":   job.WorkspaceID,
		"job_id":         job.ID,
		"job_kind":       job.Kind(),
		"error":          failureSummary(cause),
	}
	// The optional fields are omitted rather than sent empty, so a consumer can
	// test presence instead of testing for "".
	putIfSet(data, "item", job.ItemName)
	putIfSet(data, "target_locale", job.TargetLocale)
	putIfSet(data, "stream", job.Stream)
	putIfSet(data, "push_id", job.PushID)
	putIfSet(data, "step_id", job.StepID)
	putIfSet(data, "initiator", job.CreatedBy)

	bus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventFlowFailed,
		Source:    job.Kind() + "-worker",
		ProjectID: job.ProjectID,
		// A job is workspace-scoped as well as project-scoped: the summons
		// resolves recipients from the workspace, not the project.
		WorkspaceID: job.WorkspaceID,
		// The initiator is the actor in the sense the bus means it — the person
		// whose request this work serves. Empty for platform-started jobs, which
		// the summons reads as "tell the workspace's owners".
		Actor:     job.CreatedBy,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
}

// publishExtractionJobFailed announces one terminally failed extraction job.
//
// Extraction runs on its own queue with its own row shape and no retry budget:
// the first failure is the only failure, so the emitter sits directly at the
// point that writes the status.
//
// The extraction row carries the workspace slug and no workspace id — the table
// never needed one — so the event carries the slug alone and the summons
// resolves the id from it. The consumers key on the slug regardless.
func publishExtractionJobFailed(bus platev.EventBus, job *ExtractionJob, cause string) {
	if bus == nil || job == nil {
		return
	}
	data := map[string]string{
		"workspace_slug": job.WorkspaceSlug,
		"job_id":         job.ID,
		"job_kind":       "extraction",
		"error":          failureSummary(cause),
	}
	putIfSet(data, "item", job.ItemName)
	putIfSet(data, "target_locale", job.Locale)
	putIfSet(data, "push_id", job.PushID)
	putIfSet(data, "step_id", job.StepID)

	bus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventFlowFailed,
		Source:    "extraction-worker",
		ProjectID: job.ProjectID,
		Timestamp: time.Now().UTC(),
		Data:      data,
	})
}

// announceJobFailure publishes flow.failed for a job the worker loop has just
// given up on — but only if the row agrees that it is failed.
//
// The loop's failure branch is reached by more than terminal failure: a claim
// that errored, a job row that would not load, a stale worker whose write was
// refused all arrive here with an error and a row that is still 'processing' or
// already owned by somebody else. Re-reading the row is what tells those apart,
// and it is also how the event gets the job's fields at all — the loop holds an
// id and an error and nothing else.
func announceJobFailure(ctx context.Context, deps *WorkerDeps, jobID string, cause error) {
	if deps == nil || deps.EventBus == nil || deps.JobStore == nil {
		return
	}
	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil || job == nil {
		slog.WarnContext(ctx, "job failed but could not be re-read; nobody will be told",
			"job_id", jobID, "error", err)
		return
	}
	if job.Status != StatusFailed {
		// Not this worker's failure to announce: the row is still running, or a
		// fresh owner has it. Saying "failed" here would summon a reviewer to a
		// job that is about to succeed.
		return
	}
	msg := job.Error
	if msg == "" && cause != nil {
		msg = cause.Error()
	}
	publishJobFailed(deps.EventBus, job, msg)
}

func putIfSet(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}
