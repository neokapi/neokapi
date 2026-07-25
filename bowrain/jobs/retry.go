package jobs

import (
	"time"

	"github.com/neokapi/neokapi/bowrain/resilience"
)

// defaultMaxJobAttempts bounds how many times a translation job is retried on
// transient failures before it is marked failed. It is aligned with the queue
// redelivery cap (the SQS redrive policy's sqsMaxReceiveCount) so the in-flight
// NAK retry and the DB attempt budget agree on the same ceiling.
const defaultMaxJobAttempts = sqsMaxReceiveCount

// defaultMaxJobDeferrals bounds how long a job will wait out an unavailable
// dependency before it is failed. Deferrals are cheap — the job is parked, not
// attempted — so the ceiling is generous: at the AI breaker's 30 s cooldown
// this is roughly an hour of outage absorbed without losing work, which covers
// every provider incident short of a sustained one. It is deliberately separate
// from the retry budget so waiting never eats the attempts a job needs once the
// dependency returns.
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
