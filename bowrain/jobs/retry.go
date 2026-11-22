package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/resilience"
)

// QuotaSkipPrefix starts the error recorded on a job skipped because another
// job in its batch found the AI provider's quota exhausted. The original
// provider message follows, so the row still classifies as a quota failure.
const QuotaSkipPrefix = "skipped: the AI provider's quota is exhausted"

// skipQueuedPushSiblings fails the still-queued jobs that share job's push
// once job has hit an exhausted provider quota. The quota is account-wide and
// resets in hours, so each of those jobs would make one more refused call.
// Only jobs read as queued are selected; one already processing belongs to
// another worker and ends on its own. A job claimed between the read and the
// write is cancelled too, which stops it at its next chunk. The skipped jobs'
// queue messages are acked unrun, because a claim needs a queued row. Returns
// how many were skipped.
func skipQueuedPushSiblings(ctx context.Context, store JobStore, job *TranslationJob, cause error) int {
	if job.PushID == "" || cause == nil {
		return 0
	}
	ctx = context.WithoutCancel(ctx)
	siblings, err := store.ListJobsByPushID(ctx, job.PushID)
	if err != nil {
		slog.WarnContext(ctx, "quota skip: list push jobs failed", "push_id", job.PushID, "error", err)
		return 0
	}
	reason := QuotaSkipPrefix + " (" + cause.Error() + ")"
	skipped := 0
	for _, sib := range siblings {
		if sib.ID == job.ID || sib.Status != StatusQueued {
			continue
		}
		ok, err := store.CancelJob(ctx, sib.ID, reason)
		if err != nil {
			slog.WarnContext(ctx, "quota skip: fail queued job failed", "job_id", sib.ID, "error", err)
			continue
		}
		if ok {
			skipped++
		}
	}
	if skipped > 0 {
		slog.WarnContext(ctx, "AI provider quota exhausted; skipped queued jobs in the push",
			"push_id", job.PushID, "skipped", skipped)
	}
	return skipped
}

// defaultMaxJobAttempts bounds how many times a translation job is retried on
// transient failures before it is marked failed. It is aligned with the queue
// redelivery cap (the SQS redrive policy's sqsMaxReceiveCount) so the in-flight
// NAK retry and the DB attempt budget agree on the same ceiling.
const defaultMaxJobAttempts = sqsMaxReceiveCount

// defaultMaxJobDeferrals limits waiting for unavailable dependencies separately
// from retry attempts. At a 30-second breaker cooldown, the limit allows roughly
// an hour of deferrals before failing the job.
const defaultMaxJobDeferrals = 120

// transientError marks a job failure the worker should NAK so the message
// broker redelivers it. The job row has been reset to 'queued' (see
// JobStore.RetryOrFail) so the redelivery can re-claim it. ACKing a transient
// failure would strand a job that would have succeeded on retry.
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// deferredError marks a job parked because a dependency's circuit was open —
// the call was never attempted. The worker re-enqueues it after retryAfter
// rather than immediately, so the redelivery lands when the breaker is ready to
// probe instead of hammering a queue with work that cannot run yet.
//
// It is distinct from transientError precisely because the two have different
// costs: a transient failure spends an attempt, a deferral does not.
type deferredError struct {
	err        error
	dependency string
	retryAfter time.Duration
}

func (e *deferredError) Error() string { return e.err.Error() }
func (e *deferredError) Unwrap() error { return e.err }

// isTransientError reports whether err is a retryable upstream failure — a
// provider 5xx / 429 / 529, a request timeout, or a transport-level network
// error — as opposed to a permanent error (4xx auth/validation, missing
// project, malformed config) that would fail identically on redelivery.
//
// It delegates to the platform's single classifier so the worker's retry
// verdict and the circuit breaker's failure count can never disagree about
// what "the upstream is in trouble" means.
func isTransientError(err error) bool { return resilience.IsUpstreamFailure(err) }

// deferralFor returns the deferral describing err when it is an open-circuit
// rejection, or nil when the job should be handled by the ordinary retry path.
func deferralFor(err error) *deferredError {
	u, ok := resilience.AsUnavailable(err)
	if !ok {
		return nil
	}
	return &deferredError{err: err, dependency: u.Dependency, retryAfter: u.RetryAfter}
}
