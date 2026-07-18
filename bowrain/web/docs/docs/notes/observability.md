---
title: Observability & Incident Drill-Down
---

# Observability & Incident Drill-Down

How the Bowrain platform is observed in production, and — the point of it all —
how to go from a user-reported error to its root cause with a single ID.

The stack has four tools, each owning one job:

| Tool | Owns | Answers |
| --- | --- | --- |
| **Sentry** | Exceptions + traces (server, worker, web) | "What broke, where, with what stack trace?" |
| **PostHog** | Product analytics + session replay + funnels | "What was the user doing? Watch it." |
| **CloudWatch** | Infra metrics, logs, alarms | "Is the platform healthy? What fired?" |
| **ctrl → Health** | At-a-glance service readiness + deep-links | "Is anything red right now?" |

## The reference ID — one string, full drill-down

Every HTTP response carries an `X-Request-ID` (a UUID minted per request). The
web client surfaces it to the user as an error **reference** — the copyable
`ref <id>` chip on `ErrorNotice`. That same ID is:

- stamped on **every server log line** for the request (via the slog
  `ContextHandler`, on any `*Context` log call),
- returned in the JSON error body as `reference`,
- set as the `reference` / `request_id` **tag on the Sentry event**.

So given a reference a user quotes you:

1. **Sentry** → search `reference:<id>` → the exact issue, stack trace, release,
   and (for web errors) the linked **PostHog session replay** (`posthog_replay_url`
   tag) — watch what they did.
2. **CloudWatch Logs Insights** → `filter request_id = "<id>"` over
   `/bowrain/prod/server` (saved query "drill-by-reference") → every log line.

Background runs (convergence, jobs) have no HTTP request ID, so the **run/job ID**
is seeded as the `request_id` instead: a convergence run's logs all carry
`request_id=<run.ID>`, and a worker job's carry `request_id=<jobID>`. The run/job
ID is what the user sees in the URL, so the same drill-down works.

## What lives where

### Sentry (errors + traces)

- Config (server & worker): `SENTRY_DSN` (empty = disabled), `SENTRY_ENVIRONMENT`,
  `SENTRY_TRACES_SAMPLE_RATE`. Wired in `observe.InitSentryFromEnv`; capture flows
  through the unified `HTTPErrorHandler` (5xx + recovered panics) and the worker
  job loop (permanent failures). PII off; auth headers/cookies scrubbed in
  `BeforeSend`.
- Config (web): `VITE_SENTRY_DSN` (publish-safe, ships in the bundle). The single
  EU DSN serves both server and browser — events are separated by the
  `service`/`surface` tag. The web app is wrapped in a `Sentry.ErrorBoundary`.

### PostHog (product + replay)

- Config (all SPAs): `VITE_POSTHOG_KEY`, `VITE_POSTHOG_HOST` (default
  `https://eu.i.posthog.com`). Keyless builds are silent no-ops.
- **Session replay** is on for the web app with `maskAllInputs` — every text
  input is masked. Mark extra-sensitive DOM with `[data-sensitive]` (blackout) or
  `[data-ph-no-capture]` (drop). Every event carries a `surface` super-property.
- Feature flags are **not** PostHog's — the platform has its own flag system
  (ctrl → Platform + per-workspace overrides). Do not duplicate it in PostHog.

### CloudWatch (infra) — see `bowrain-infra`

- Container logs → per-service log groups `/bowrain/<env>/{server,worker,pulse}`
  (JSON, 30-day retention).
- Golden-signal **alarms** wired to the `bowrain-<env>-alerts` SNS topic (email):
  ALB 5xx / latency / unhealthy hosts, ECS CPU/mem/task-count, RDS
  CPU/storage/connections, Redis CPU/memory/evictions, SQS message-age +
  **DLQ depth**, and an ERROR-log metric filter. A composite "site health" alarm
  rolls them into one signal.
- A CloudWatch **dashboard** shows the golden signals; **saved Logs Insights
  queries** provide drill-by-reference.
- Code-level alerts (error spikes, regressions) come from **Sentry** and product
  regressions from **PostHog**, so infra and app alerting stay separate.

### App metrics (Prometheus `/metrics`)

- Server exposes HTTP metrics; the **worker** now exposes job metrics
  (`bowrain_jobs_processed_total{queue,outcome}`, `_duration_seconds`,
  `_in_flight`), the **convergence** loop emits
  `bowrain_convergence_runs_total{outcome}`, and DB-pool saturation gauges are
  registered. Both endpoints are gated (bearer token via `BOWRAIN_METRICS_TOKEN`,
  else private-IP only, `404` on refusal). Go-runtime + process metrics come free.

## ctrl → Health page

`ctrl.bowrain.cloud` → **Health** shows live per-component readiness for the
server (db / queue / ai / session store / email) and the worker (db / queue /
blob), plus deep-links out to Sentry, PostHog, and the CloudWatch dashboard.
Backed by `GET /api/admin/health`.

## Environment variables (summary)

| Var | Service | Purpose |
| --- | --- | --- |
| `SENTRY_DSN` | server, worker | Error tracking (empty = off) |
| `SENTRY_ENVIRONMENT` | server, worker | `production` / `staging` / `dev` |
| `SENTRY_TRACES_SAMPLE_RATE` | server, worker | Perf tracing 0..1 (default 0) |
| `VITE_SENTRY_DSN` | web build | Browser error tracking |
| `VITE_POSTHOG_KEY` / `_HOST` | SPA builds | Product analytics + replay |
| `BOWRAIN_METRICS_TOKEN` | server, worker | Bearer gate for `/metrics` |
| `BOWRAIN_LOG_LEVEL` / `_FORMAT` | server, worker | slog level / json\|text |
| `BOWRAIN_WORKER_HEALTH_URL` | server | Internal URL the Health page probes for `/readyz` |
| `OBSERVABILITY_SENTRY_URL` | server | ctrl Health deep-link |
| `OBSERVABILITY_POSTHOG_URL` | server | ctrl Health deep-link |
| `OBSERVABILITY_CLOUDWATCH_URL` | server | ctrl Health deep-link |

## Runbook: a user reports an error

1. Ask for the **reference** (the `ref <id>` on the error notice).
2. Paste it into **Sentry** search → stack trace + release + linked session
   replay. Watch the replay if you need to see what they did.
3. If you need raw logs, run the **CloudWatch Logs Insights** "drill-by-reference"
   saved query with the same ID.
4. For a background run/job failure, the reference *is* the run/job ID — same flow.
