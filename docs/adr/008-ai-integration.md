---
id: 008-ai-integration
sidebar_position: 8
title: "ADR-008: AI Integration"
---
# ADR-008: First-class AI and LLM integration

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
    Close() error
}
```

Three built-in implementations exist: Anthropic Claude (`ai/provider/anthropic.go`), OpenAI (`ai/provider/openai.go`), and Ollama for local models (`ai/provider/ollama.go`). Each is configured via `provider.Config` with API key, base URL, model name, and generation parameters. A mock provider (`ai/provider/mock.go`) enables deterministic testing without API calls.

### AI Tools

Six AI tools are implemented as standard Tools (see [ADR-006](./006-tool-system.md)) that embed `BaseTool`:

| Tool | Package | Purpose |
|---|---|---|
| `ai-translate` | `ai/tools/translate.go` | Translate untranslated Blocks using LLM |
| `ai-qa` | `ai/tools/qualitycheck.go` | Check translations for fluency, accuracy, terminology |
| `ai-terminology` | `ai/tools/terminology.go` | Extract terminology candidates from source Blocks |
| `ai-review` | `ai/tools/review.go` | Review translations with explanations |
| `entity-annotate` | planned | Annotate named entities (people, places, dates, products) |
| `brand-voice-check` | planned | Validate content against brand voice rules (see [ADR-010](./010-terminology.md)) |

The `ai-terminology` tool creates `TermAnnotation` entries with `status: proposed`, feeding the terminology lifecycle workflow. The `entity-annotate` tool will produce `EntityAnnotation` entries that serve as do-not-translate markers, localization hints, and context for AI translation. See [ADR-010](./010-terminology.md) for the full terminology and brand management design.

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

### Block Batching

Individual block-by-block translation wastes API quota on small calls. The block batcher groups blocks into batches for efficient API calls:

```go
type BlockBatcher struct {
    pool          *AIWorkerPool
    batchSize     int           // blocks per API call (default 10)
    flushInterval time.Duration // max wait before flushing partial batch
}
```

The batcher collects blocks from the streaming pipeline and flushes them when either the batch reaches `batchSize` or `flushInterval` elapses (whichever comes first). A single API call translates the entire batch, reducing overhead. Each submitter receives its result via a per-item response channel, preserving the streaming pipeline's concurrency model.

Partial batch failures are handled per-item: if the batch call succeeds but returns fewer translations than expected, only the missing items are retried individually.

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

## Consequences

- AI translation is a pipeline tool, not a separate system. It composes with all other tools ([ADR-006](./006-tool-system.md)).
- Tools can be ordered to maximize quality: TM leverage ([ADR-009](./009-translation-memory.md)) before AI avoids retranslating exact matches, reducing cost.
- Terminology context flows through the pipeline via annotations ([ADR-010](./010-terminology.md)), enabling AI tools to produce terminology-consistent translations from the start.
- The worker pool prevents API rate limit violations and handles transient failures gracefully, making AI translation reliable in production.
- Batching reduces API call count and improves throughput for large document processing.
- Provider abstraction enables cost optimization: local Ollama for development, Claude or OpenAI for production, MT connectors for simple translations.
- Prompt templates are centralized and testable. Mock providers enable deterministic testing without API calls.
- The content model ([ADR-002](./002-content-model.md)) carries AI-generated annotations (TM match scores, QA issues, term suggestions) alongside the translated content.
