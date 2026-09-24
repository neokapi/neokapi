// Package resilience provides circuit breakers for outbound dependencies used
// by the Bowrain server and worker, including AI providers, connectors, storage,
// mail, identity and billing services.
//
// Callers use [Breaker], [Registry] and [Do]. The implementation uses
// sony/gobreaker/v2 with shared per-dependency state, rolling failure windows
// and bounded half-open probes.
//
// # Failure classification
//
// A new breaker starts closed and allows requests. [IsUpstreamFailure] counts
// provider 5xx/429/529 responses, timeouts and transport faults. Other 4xx errors,
// such as invalid credentials, do not count toward the shared breaker.
// context.Canceled is excluded. Open breakers allow probes after Cooldown.
// Disabling [Settings.Enabled] makes Do pass requests through immediately.
//
// # Unavailable dependencies
//
// [Do] returns [UnavailableError] without calling an open dependency. The API
// returns 503 with the ai_unavailable envelope and Retry-After. The worker
// requeues the job after cooldown without consuming a retry attempt.
//
// # Observability
//
// Breakers report state, transitions, rejections and outcomes through
// bowrain/observe. Transitions produce structured logs, and opening a breaker
// produces a Sentry event. [Registry.States] supplies the ctrl Health page.
package resilience
