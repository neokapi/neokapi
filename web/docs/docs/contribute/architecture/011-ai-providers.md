---
id: 011-ai-providers
sidebar_position: 11
title: "AD-011: AI Providers"
description: "Architecture decision: LLM capabilities plug in through an LLMProvider interface in providers/ai/ with built-in backends for Anthropic Claude, OpenAI, Google Gemini, Azure OpenAI, and Ollama."
keywords: [AI providers, LLMProvider, Anthropic, OpenAI, Gemini, Ollama, architecture decision, neokapi]
---

# AD-011: AI Providers

## Summary

The framework integrates LLM capabilities through an `LLMProvider` interface
in `providers/ai/` (package `aiprovider`), with five built-in implementations
(Anthropic, OpenAI, Azure OpenAI, Ollama, Gemini) and an optional
`StreamingLLMProvider` extension for live thinking progress. AI tools consume
providers through an `AIWorkerPool` that adds rate limiting, circuit
breaking, and retry with backoff. A `ChatStructured` method with JSON Schema
enables reliable batch translation and other structured-output tasks.

## Context

Modern LLMs are capable translators, reviewers, and terminology extractors.
Treating them as a separate service loses the composability of the streaming
pipeline: AI tools should sit alongside TM leverage, term enforcement, and
QA in the same flow.

AI APIs come with practical constraints: rate limits, cost per token,
transient failures, and variable latency. A production-grade integration
needs batching, rate limiting, circuit breakers, and retry logic to use
these APIs reliably at scale.

Providers also differ in their structured-output mechanism: OpenAI and
Azure OpenAI use `response_format: json_schema`, Anthropic uses tool-use
with `input_schema`, Ollama uses `format: json`, and Gemini uses
response schema hints. A single interface must paper over these details
while giving tools a predictable contract.

## Decision

### LLMProvider interface

```go
type LLMProvider interface {
    Name() string
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Chat(ctx context.Context, messages []Message) (*ChatResponse, error)
    ChatStructured(ctx context.Context, messages []Message,
        schema JSONSchema) (*ChatResponse, error)
    Close() error
}
```

`ChatStructured` extends `Chat` with a JSON Schema constraint that forces
the provider to return structured output. The `JSONSchema` type includes
`Name`, `Description`, `Schema` (the JSON Schema definition), and a
`Strict` flag for providers that support strict validation.

Provider configuration is schema-driven: fields in AI tool configs generate
CLI flags automatically via `schema.FromStruct()`, removing the need for
manual flag registration.

### Built-in providers

| Provider      | File                          | Default Model            | Notes                                |
| ------------- | ----------------------------- | ------------------------ | ------------------------------------ |
| Anthropic     | `providers/ai/anthropic.go`   | claude-sonnet-4-20250514 | Extended thinking support            |
| OpenAI        | `providers/ai/openai.go`      | gpt-4o                   | `response_format` JSON schema        |
| Azure OpenAI  | `providers/ai/azureopenai.go` | deployment-specific      | Managed Identity via `TokenProvider` |
| Ollama        | `providers/ai/ollama.go`      | llama3                   | Local models, `format: json`         |
| Google Gemini | `providers/ai/gemini.go`      | gemini-3-flash-preview   | SSE streaming with `includeThoughts` |

A mock provider (`providers/ai/mock.go`) enables deterministic testing
without API calls.

Each provider takes a `Config` struct with API key, base URL, model name,
and generation parameters (temperature, max tokens, etc.). Azure OpenAI
additionally accepts a `TokenProvider` function, enabling passwordless
access via Azure Managed Identity.

### StreamingLLMProvider

An optional extension interface surfaces live progress events for
providers that support them:

```go
type StreamingLLMProvider interface {
    LLMProvider
    ChatStream(ctx context.Context, messages []Message) (
        <-chan ChatStreamEvent, error)
    ChatStructuredStream(ctx context.Context, messages []Message,
        schema JSONSchema) (<-chan ChatStreamEvent, error)
}

type ChatStreamEvent struct {
    Type    StreamEventType // StreamEventThinking | StreamEventContent | StreamEventDone
    Content string
}
```

UIs and CLI tools display live thinking progress from providers that
support it (Anthropic extended thinking, Gemini `includeThoughts`). A
provider that does not implement `StreamingLLMProvider` can still be
used — callers that need streaming check for the extension with a type
assertion.

### AIWorkerPool

AI API calls flow through a worker pool that handles rate limiting,
concurrency, and failure recovery:

```go
type AIWorkerPool struct {
    provider    LLMProvider
    limiter     *rate.Limiter              // token-bucket rate limit
    breaker     *gobreaker.CircuitBreaker  // circuit breaker
    sem         *semaphore.Weighted        // concurrent request cap
    retryConfig RetryConfig
}
```

Components:

- **Rate limiting** — token bucket via `golang.org/x/time/rate`. Configured
  per provider (Anthropic and OpenAI have different RPM ceilings).
- **Concurrency control** — a weighted semaphore limits parallel requests
  to prevent overwhelming the provider.
- **Circuit breaker** — `sony/gobreaker` opens after consecutive failures,
  preventing cascading failure when an API is down. A timeout returns the
  breaker to half-open, allowing probe requests.
- **Retry with backoff** — exponential backoff with jitter for transient
  failures (429, 503). Non-retryable errors (400, 401) fail immediately.

A request flow: acquire a semaphore slot, wait for the rate limiter,
execute through the circuit breaker with retry, release the semaphore.

### AI tools

AI capabilities reach the pipeline as ordinary Tools
([AD-006: Tool System](006-tool-system.md)):

| Tool                | Purpose                                                 |
| ------------------- | ------------------------------------------------------- |
| `ai-translate`      | Translate untranslated Blocks using an LLM              |
| `ai-qa`             | Check translations for fluency, accuracy, terminology   |
| `ai-terminology`    | Extract terminology candidates from source Blocks       |
| `ai-review`         | Review translations with explanations                   |
| `ai-entity-extract` | Extract entities and term candidates (hybrid LLM + NER) |

Because AI tools are ordinary Tools, they compose naturally:

```
Reader → tm-leverage → term-lookup → ai-translate → term-enforce → ai-qa → Writer
```

### Terminology-aware prompts

AI tools receive terminology context from upstream stages:

- **Term annotations** — when `term-lookup` has run, matched terms and
  their preferred translations appear in the prompt.
- **Entity annotations** — when `ai-entity-extract` has run, identified
  entities (with DNT flags and locale formatting hints) appear in the
  prompt context.
- **Glossary constraints** — a dedicated glossary section lists
  preferred and forbidden terms applicable to the current Block's
  domain, product, and market.

Terminology enforcement is not just a post-translation validation step;
it actively guides AI translation quality from the start.

### Batch translation

The `ai-translate` tool has two modes:

- **Single-block** (default for small documents) — translates each Block
  individually via `provider.Translate()`. Simple, predictable, works with
  all providers.
- **Batch** (configurable) — groups translatable Blocks and translates
  them in a single `ChatStructured()` call using a JSON Schema that
  returns `{ translations: [{ index, text }] }`.

Batch configuration:

- `BatchSize` — Blocks per LLM call (default 20)
- `Concurrency` — parallel batch calls (default 5)

Batch mode drains input Parts into memory, identifies translatable Blocks
(skipping already-translated ones), groups them, processes batches
concurrently with a semaphore, and writes all Parts downstream in original
order. Missing entries in the structured response are retried
individually as single-block translations.

This application-level batching is distinct from the `ParallelBlockTool`
concurrency in [AD-004: Processing Engine](004-processing-engine.md), which
parallelizes Part dispatch within the pipeline.

### Prompt templates

Prompt templates live in `providers/ai/prompt/` as versioned Go files
using `text/template`:

- `translate.go` — translation prompts with glossary and context
- `qa.go` — quality assurance check prompts
- `review.go` — translation review with explanations

Templates are context-aware: they include surrounding Blocks for document
context, glossary constraints from term lookup, TM matches from
leveraging, and format metadata (HTML tag handling instructions, CDATA
boundaries, etc.).

### Credential resolution

AI providers read credentials at runtime from one of three sources:

1. The CLI credential store ([AD-013: Kapi CLI](013-kapi-cli.md)) —
   provider configs as JSON, API keys in the OS keychain.
2. Environment variables — `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.
3. Explicit `--api-key` flag on CLI invocation.

Flag overrides store overrides environment. API keys never appear in
project files.

### Scope boundary

The framework's responsibility ends at the provider interface, the
worker pool, and the pipeline tools. Server-side asynchronous job
queues, multi-tenant quota enforcement, and workspace-scale translation

## Consequences

- AI translation is a pipeline tool, not a separate system. It composes
  with all other tools without special orchestration.
- Ordering is meaningful: TM leverage before AI translation avoids
  re-translating exact matches, reducing cost.
- Terminology context flows through the pipeline via annotations,
  enabling AI tools to produce terminology-consistent translations from
  the start.
- The worker pool handles API rate limits, concurrent request caps, and
  transient failures gracefully, making AI tools reliable in production.
- Batching reduces API call count and improves throughput for large
  document processing.
- Provider abstraction enables cost optimization: local Ollama for
  development, Claude or OpenAI for production.
- Prompt templates are centralized and testable. The mock provider
  enables deterministic tests without API calls.
- Azure Managed Identity eliminates API key management for production
  Azure deployments while the same interface continues to support
  key-based auth elsewhere.
- `ChatStructured` gives tools a reliable JSON contract across providers
  with very different structured-output mechanisms.

## Related

- [AD-002: Content Model](002-content-model.md) — annotations on Blocks
- [AD-004: Processing Engine](004-processing-engine.md) — flow execution
  and `ParallelBlockTool`
- [AD-006: Tool System](006-tool-system.md) — Tool pattern
- [AD-009: Translation Memory](009-translation-memory.md) — `tm-leverage`
  feeds context to AI tools
- [AD-010: Terminology](010-terminology.md) — term annotations feed
  context to AI tools
- [AD-012: MT Providers](012-mt-providers.md) — complementary external MT
  services
- [AD-013: Kapi CLI](013-kapi-cli.md) — credential store
