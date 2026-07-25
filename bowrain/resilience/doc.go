// Package resilience is the single circuit-breaker abstraction for every
// outbound dependency the Bowrain server and worker call — AI and MT providers,
// CMS/forge connectors, object storage, the mail relay, the identity provider,
// and billing.
//
// # Why one package
//
// Before this package each call site handled upstream trouble on its own: some
// had a client timeout, a few had bounded retries, most had neither. A provider
// outage therefore surfaced as a pile of independent slow calls — every request
// paying the full timeout, every job burning its retry budget against a
// dependency that was known to be down. The fix is not more per-call-site
// logic; it is one shared breaker whose state is *per dependency* and whose
// behaviour is uniform, observable, and tunable in one place.
//
// # Why sony/gobreaker rather than a bespoke state machine
//
// The breaker state machine itself (closed → open → half-open, with a bounded
// number of half-open probes and a rolling failure window) is small but easy to
// get subtly wrong under concurrency. github.com/sony/gobreaker/v2 is the
// long-standing Go implementation of it: zero transitive dependencies, a
// generic Execute, a rolling-bucket window, and — decisively — the two hooks
// this platform needs, IsSuccessful (classify which errors count) and
// IsExcluded (ignore caller-side cancellations entirely). Vendoring a
// hand-rolled equivalent would buy nothing and cost the concurrency review.
//
// gobreaker is therefore an implementation detail: it is imported by this
// package and nowhere else. Call sites see [Breaker], [Registry] and [Do].
//
// # Safety posture
//
// The defaults are deliberately biased towards *not* interfering:
//
//   - Cold start is closed (fail-open). A fresh process never rejects a call it
//     has not yet observed failing.
//   - A breaker only ever opens on [IsUpstreamFailure] errors — provider
//     5xx/429/529, timeouts, transport faults. A 4xx (bad request, bad
//     credentials, quota) is a caller-side fault: it is counted as neither a
//     success nor a failure, so a workspace with a wrong API key can never trip
//     the shared breaker for everybody else.
//   - context.Canceled is excluded outright: a user closing a tab is not a
//     dependency signal.
//   - An open breaker always re-probes after Cooldown, so it cannot wedge a
//     dependency that has recovered.
//   - [Settings.Enabled] is a kill switch. Turning breakers off in ctrl makes
//     every Do a straight pass-through immediately.
//
// # Degradation, not failure
//
// [Do] returns an [UnavailableError] when the circuit is open. That is a
// distinct, typed outcome — "we did not call the dependency because it is
// known to be down" — not a generic error. Callers use it to degrade:
//
//   - The API returns 503 with the ai_unavailable envelope and a Retry-After,
//     which the web app renders as a queued state rather than a red failure.
//   - The worker defers the job — requeued after the cooldown *without*
//     consuming a retry attempt — so an outage costs waiting, not dead jobs.
//
// # Observability
//
// Every breaker reports through bowrain/observe: Prometheus series for state,
// transitions, rejections and call outcomes; a structured slog line on every
// transition; a Sentry event when one opens. [Registry.States] feeds the ctrl
// Health page so an operator sees the same state the code is acting on.
package resilience
