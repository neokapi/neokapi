---
id: 008-ai-integration
sidebar_position: 8
title: "AD-008: AI Integration"
---
# AD-008: First-class AI and LLM integration

## Context

Large language models have become capable translators and quality reviewers. Rather than bolting AI capabilities onto the side as a separate service, LLM-powered translation, QA, terminology extraction, and review should be composable tools in the same pipeline as traditional processing steps.

AI APIs come with practical constraints: rate limits, cost per token, transient failures, and varying latency. A production system needs batching, rate limiting, circuit breakers, and retry logic to use these APIs reliably at scale. External machine translation services (DeepL, Google, Microsoft) serve as lightweight alternatives to LLM-based translation and have their own operational characteristics.

## Decision

### LLMProvider Interface

AI capabilities are backed by an `LLMProvider` interface defined in `ai/provider/provider.go`:

```go
type LLMProvider interface {
    Name() string
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Chat(ctx context.Context, messages []Message) (*ChatResponse, error)
    ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error)
    Close() error
}
```

`ChatStructured` extends `Chat` with a JSON Schema constraint that forces
the provider to return structured output. Each provider implements this
differently: OpenAI and Azure OpenAI use `response_format: json_schema`,
Anthropic uses tool_use with `input_schema`, and Ollama uses `format: json`.
The `JSONSchema` type includes `Name`, `Description`, `Schema` (the JSON
Schema definition), and a `Strict` flag for providers that support strict
validation.

Four built-in implementations exist: Anthropic Claude (`ai/provider/anthropic.go`), OpenAI (`ai/provider/openai.go`), Azure OpenAI (`ai/provider/azureopenai.go`), and Ollama for local models (`ai/provider/ollama.go`). Each is configured via `provider.Config` with API key, base URL, model name, and generation parameters. Azure OpenAI additionally supports token-based authentication via a `TokenProvider` function, enabling Azure Managed Identity for passwordless access. A mock provider (`ai/provider/mock.go`) enables deterministic testing without API calls.

### AI Tools

Six AI tools are implemented as standard Tools (see [AD-006](./006-tool-system.md)) that embed `BaseTool`:

| Tool | Package | Purpose |
|---|---|---|
| `ai-translate` | `ai/tools/translate.go` | Translate untranslated Blocks using LLM |
| `ai-qa` | `ai/tools/qualitycheck.go` | Check translations for fluency, accuracy, terminology |
| `ai-terminology` | `ai/tools/terminology.go` | Extract terminology candidates from source Blocks |
| `ai-review` | `ai/tools/review.go` | Review translations with explanations |
| `entity-annotate` | planned | Annotate named entities (people, places, dates, products) |
| `brand-voice-check` | planned | Validate content against brand voice rules (see [AD-010](./010-terminology.md)) |

The `ai-terminology` tool creates `TermAnnotation` entries with `status: proposed`, feeding the terminology lifecycle workflow. The `entity-annotate` tool will produce `EntityAnnotation` entries that serve as do-not-translate markers, localization hints, and context for AI translation. See [AD-010](./010-terminology.md) for the full terminology and brand management design.

Because AI tools are standard Tools, they compose naturally in flows:

```
Reader -> TM Leverage -> Term Lookup -> AI Translate -> Term Enforce -> AI QA -> Writer
```

### Terminology-Aware Prompts

AI tools receive terminology context when available. The prompt system includes:

- **Term annotations**: When `term-lookup` has run before AI translation, the matched terms and their preferred translations are included in the prompt, guiding the LLM toward consistent terminology.
- **Entity annotations**: When `entity-annotate` has run, the identified entities (with DNT flags, locale formatting hints) are included in the prompt context.
- **Glossary constraints**: A dedicated glossary section in the prompt template lists preferred/forbidden terms applicable to the current Block's context (domain, product, market).

This composability means terminology enforcement is not just a validation step -- it actively guides AI translation quality from the start.

### AI Worker Pool

AI API calls are managed through a worker pool that handles rate limiting, concurrency control, and failure recovery:

```go
type AIWorkerPool struct {
    provider    provider.LLMProvider
    limiter     *rate.Limiter          // Token bucket rate limiting
    breaker     *gobreaker.CircuitBreaker // Circuit breaker for failures
    sem         *semaphore.Weighted    // Concurrent request limit
    retryConfig RetryConfig
}
```

The worker pool provides:

- **Rate limiting**: Token bucket via `golang.org/x/time/rate` to stay within API rate limits. Configured per provider (e.g. Anthropic allows different RPM than OpenAI).
- **Concurrency control**: Weighted semaphore limits the number of parallel requests to prevent overwhelming the provider.
- **Circuit breaker**: Opens after consecutive failures (via `sony/gobreaker`), preventing cascade failures when an API is down. After a timeout period, half-open state allows probe requests.
- **Retry with backoff**: Exponential backoff with jitter for transient failures (429 rate limit, 503 service unavailable). Non-retryable errors (400 bad request, 401 unauthorized) fail immediately.

A typical request flows through the pool as: acquire semaphore slot, wait for rate limiter, execute through circuit breaker with retry, release semaphore.

### Batch Translation

The `ai-translate` tool supports two execution modes:

**Single-block mode** (default for small documents): translates each Block
individually via `provider.Translate()`. Simple, predictable, works with
all providers.

**Batch mode** (configurable): groups translatable Blocks and translates
them with a single `ChatStructured()` call using a JSON Schema that
returns `{ translations: [{ index, text }] }`. Configuration:

- `BatchSize` -- Blocks per LLM call (default 20)
- `Concurrency` -- parallel batch calls (default 5)

Batch mode drains all input Parts into memory, identifies translatable
Blocks (skipping already-translated ones), groups them into batches,
processes batches concurrently with a semaphore, and writes all Parts to
output in original order. Missing entries in the structured response are
retried individually as single-block translations.

This is distinct from the `ParallelBlockTool` concurrency in
[AD-004](./004-processing-engine.md), which parallelizes Part dispatch
within the pipeline. Batch translation is application-level batching of
the LLM call itself -- multiple source texts in one prompt, one structured
response back.

### Server-Side Translation Service

Bowrain Server provides an asynchronous job-based translation service
separate from the pipeline-based AI tools. This enables workspace-scale
translation with progress tracking, quota enforcement, and automation
triggers.

**Job lifecycle**: queued → processing → completed/failed

**Key components:**

- **JobStore** -- persists translation jobs with status, progress, and
  token usage. Backed by SQLite or PostgreSQL.
- **JobQueue** -- abstracts async job dispatch. Three implementations:
  in-memory channels (dev), Azure Service Bus (production Azure), and
  NATS (cloud-native).
- **Worker** -- dequeues jobs, resolves providers, processes blocks in
  progressive chunks (default 50 blocks), records usage, updates progress.
- **QuotaStore** -- tracks token usage per workspace with configurable
  monthly limits (default 10M tokens). Enforced before each translation
  job starts.

**Provider resolution:**

- **Platform provider** -- uses Azure OpenAI with Managed Identity. No API
  keys; the worker acquires Entra ID tokens automatically. Enabled when
  `BOWRAIN_OPENAI_ENDPOINT` is set.
- **User-configured provider** -- API keys stored in the credential store.
  Supports OpenAI, Anthropic, Ollama, and Azure OpenAI with explicit keys.

**API endpoints:**

- `POST /api/v1/workspaces/:ws/jobs/translate` -- create async job (202 Accepted)
- `GET /api/v1/workspaces/:ws/jobs/:id` -- poll job status and progress
- `GET /api/v1/workspaces/:ws/ai/usage` -- quota summary (used, remaining, period)

The translation service is triggered by automation rules
([AD-011](./011-automation.md)) on push events or manually via the API.
It complements the pipeline-based `ai-translate` tool: the pipeline tool
runs synchronously within a flow, while the server service runs
asynchronously with progress tracking and quota management.

See [Translation Job Queue](/docs/notes/translation-job-queue) for the
job model, worker algorithm, and quota schema.

### MT Service Integration

External translation services (DeepL, Google, Microsoft, MyMemory, ModernMT) are integrated via `connectors/` as lightweight alternatives to LLM-based translation. These connectors are typically faster and cheaper for straightforward translation tasks. They implement a simpler interface than `LLMProvider` and are useful when LLM-level quality or context awareness is not required.

### Prompt Engineering

Prompt templates live in `ai/prompt/` and are context-aware: they include surrounding Blocks for document context, glossary constraints from term lookup, TM matches from leveraging, and format metadata (e.g. HTML tag handling instructions). Templates are centralized Go files, enabling version control and easy tuning without recompiling when used with `text/template`.

Current prompt templates:

- `ai/prompt/translate.go` -- translation prompts with glossary and context
- `ai/prompt/qa.go` -- quality assurance check prompts

## Alternatives Considered

- **Separate AI service**: Decoupled but loses composability with the pipeline; requires separate deployment and orchestration.
- **AI as post-processing**: Misses the opportunity to leverage TM context and inline QA within the pipeline.
- **Hard-coded provider**: The `LLMProvider` interface allows swapping providers per project or using local models via Ollama for development.
- **No batching**: Simpler implementation but wastes API quota on individual small calls, increasing cost and reducing throughput.
- **No circuit breaker**: Simpler but cascading failures from API outages would stall the entire pipeline.
- **Synchronous translation in server API**: Simpler but blocks the HTTP request for large projects. Async jobs with progress polling enables workspace-scale translation.
- **Single queue implementation**: Hard-coding Redis/NATS limits deployment flexibility. The queue interface allows in-memory for dev, Service Bus for Azure, NATS for cloud-native.

## Consequences

- AI translation is a pipeline tool, not a separate system. It composes with all other tools ([AD-006](./006-tool-system.md)).
- Tools can be ordered to maximize quality: TM leverage ([AD-009](./009-translation-memory.md)) before AI avoids retranslating exact matches, reducing cost.
- Terminology context flows through the pipeline via annotations ([AD-010](./010-terminology.md)), enabling AI tools to produce terminology-consistent translations from the start.
- The worker pool prevents API rate limit violations and handles transient failures gracefully, making AI translation reliable in production.
- Batching reduces API call count and improves throughput for large document processing.
- Provider abstraction enables cost optimization: local Ollama for development, Claude or OpenAI for production, MT connectors for simple translations.
- Prompt templates are centralized and testable. Mock providers enable deterministic testing without API calls.
- The content model ([AD-002](./002-content-model.md)) carries AI-generated annotations (TM match scores, QA issues, term suggestions) alongside the translated content.
- Structured output via `ChatStructured` enables reliable batch translation and other tasks requiring typed JSON responses from LLMs.
- Azure Managed Identity eliminates API key management for production Azure deployments while the same LLMProvider interface supports key-based auth for other environments.
- The server-side translation service enables workspace-scale async translation with quota enforcement, complementing the pipeline tool for real-time processing.
- Token quota tracking per workspace prevents unbounded AI spending in multi-tenant deployments.
