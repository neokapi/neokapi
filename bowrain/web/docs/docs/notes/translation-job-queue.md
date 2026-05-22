---
sidebar_position: 16
title: Translation Job Queue
---

# Translation Job Queue

Implementation details for the server-side async translation service
described in [AD-015](/architecture-decisions/015-server-ai-operations).

## Job Model

```go
type JobStatus string

const (
    StatusQueued     JobStatus = "queued"
    StatusProcessing JobStatus = "processing"
    StatusCompleted  JobStatus = "completed"
    StatusFailed     JobStatus = "failed"
)

type TranslationJob struct {
    ID               string
    WorkspaceSlug    string
    ProjectID        string
    ItemName         string        // file being translated
    TargetLocale     string
    ProviderConfigID string        // empty or "platform" = managed identity
    Model            string        // deployment name (e.g., "gpt-4o-mini")
    PushID           string        // links to originating push event
    Status           JobStatus     // queued | processing | completed | failed
    Progress         int           // 0 - 100
    TotalBlocks      int
    DoneBlocks       int
    BatchSize        int           // blocks per LLM call (default 20)
    Concurrency      int           // parallel batch calls (default 5)
    TokensUsed       int
    Error            string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}
```

`IsPlatformProvider()` returns true when `ProviderConfigID` is empty or
`"platform"`, indicating the job should use Azure OpenAI with Managed
Identity.

## Queue Implementations

Three implementations of the `Queue` interface:

| Implementation    | Backend                 | Use Case                 |
| ----------------- | ----------------------- | ------------------------ |
| `ChannelQueue`    | Go channels (in-memory) | Local development        |
| `ServiceBusQueue` | Azure Service Bus       | Production Azure         |
| `NATSQueue`       | NATS streaming          | Cloud-native deployments |

Interface:

```go
type Queue interface {
    Enqueue(ctx context.Context, jobID string) error
    Dequeue(ctx context.Context) (jobID string, ack func(), nack func(), err error)
    Close() error
}
```

`Dequeue()` blocks until a job is available. Workers call `ack()` after
successful processing; `nack()` re-queues the job for retry on transient
failures.

## Worker Algorithm

```
1. Dequeue job ID from queue
2. Load job from JobStore
3. Skip if status != "queued"
4. Check quota (if QuotaStore configured)
5. Mark status = "processing"
6. Load project and blocks from ContentStore
7. Resolve provider:
   - Platform → Azure OpenAI with Managed Identity
   - User-configured → credentials store lookup
8. Create AITranslateTool with batch/concurrency config
9. Process blocks in chunks of 50:
   a. Run tool on chunk
   b. Record token usage in QuotaStore
   c. Update progress in JobStore
10. Store translated blocks in ContentStore
11. Mark status = "completed" with total token count
12. Always ack (no retry on permanent failures)
```

## Provider Resolution

**Platform provider** (when `BOWRAIN_OPENAI_ENDPOINT` is set):

```go
PlatformProviderConfig{
    Endpoint: os.Getenv("BOWRAIN_OPENAI_ENDPOINT"),
    ClientID: os.Getenv("AZURE_CLIENT_ID"),  // optional
}
```

Uses `azidentity.ManagedIdentityCredential` to acquire tokens with scope
`https://cognitiveservices.azure.com/.default`. Tokens are cached and
refreshed automatically by the Azure SDK. Rate limit: 10 req/sec.

**User-configured provider**: loaded from the credential store by
`ProviderConfigID`. Supports OpenAI, Anthropic, Ollama, Azure OpenAI
with explicit API keys. Rate limits vary by provider.

## Quota Schema (PostgreSQL)

```sql
CREATE TABLE ai_usage (
    id              BIGSERIAL PRIMARY KEY,
    workspace_slug  TEXT NOT NULL,
    project_id      TEXT NOT NULL,
    job_id          TEXT NOT NULL DEFAULT '',
    model           TEXT NOT NULL,
    prompt_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ai_usage_workspace_period
    ON ai_usage(workspace_slug, created_at);

CREATE TABLE ai_quotas (
    workspace_slug  TEXT PRIMARY KEY,
    monthly_limit   BIGINT NOT NULL DEFAULT 10000000,  -- 10M tokens
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Billing period: calendar month (1st UTC to end of month). `CheckQuota()`
verifies `remaining_tokens >= 0` before allowing a new job. Usage is
recorded incrementally per chunk (every 50 blocks) to prevent overrun on
large jobs.

## API Endpoints

| Method | Path                                    | Description                          |
| ------ | --------------------------------------- | ------------------------------------ |
| POST   | `/api/v1/workspaces/:ws/jobs/translate` | Create async job (202 Accepted)      |
| POST   | `/api/v1/projects/:id/sync/translate`   | Project-scoped translate (anonymous) |
| GET    | `/api/v1/workspaces/:ws/jobs/:id`       | Poll job status and progress         |
| GET    | `/api/v1/workspaces/:ws/jobs`           | List recent 50 jobs                  |
| DELETE | `/api/v1/workspaces/:ws/jobs/:id`       | Cancel job                           |
| GET    | `/api/v1/workspaces/:ws/ai/usage`       | Quota summary                        |

## Environment Variables

| Variable                         | Required | Description                                       |
| -------------------------------- | -------- | ------------------------------------------------- |
| `BOWRAIN_DATABASE_URL`           | Yes      | PostgreSQL or SQLite connection string            |
| `BOWRAIN_DATABASE_AUTH`          | No       | `"azure"` for Entra ID auth                       |
| `BOWRAIN_OPENAI_ENDPOINT`        | No       | Azure OpenAI endpoint (enables platform provider) |
| `AZURE_CLIENT_ID`                | No       | User-assigned managed identity client ID          |
| `BOWRAIN_SERVICE_BUS_CONNECTION` | No       | Azure Service Bus connection string               |
| `BOWRAIN_NATS_URL`               | No       | NATS streaming URL                                |

If neither Service Bus nor NATS is configured, the server uses an
in-memory channel queue (suitable only for single-instance development).

## Automation Integration

Translation jobs are triggered automatically via built-in automation
rules ([AD-013](/architecture-decisions/013-automation-engine)):

- `auto-translate-on-push`: on `push.completed`, creates one job per
  (item, locale) pair pushed. Links via `push_id`.
- `auto-translate-new-locale`: on `project.updated` when new target
  locales are added, creates jobs for all items.

The `kapi sync` CLI command (and the GitHub Action) coordinates the
full round-trip: push → wait for translation jobs → pull results.
