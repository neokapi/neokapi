---
id: 011-ai-providers
sidebar_position: 11
title: "AD-011: AI Providers"
description: "Architecture decision: LLM capabilities plug in through an LLMProvider interface in providers/ai/ with built-in backends for Anthropic Claude, OpenAI, Google Gemini, Azure OpenAI, and Ollama."
keywords: [AI providers, LLMProvider, Anthropic, OpenAI, Gemini, Ollama, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-011: AI Providers

## Summary

The framework integrates LLM capabilities through an `LLMProvider` interface
in `providers/ai/` (package `aiprovider`), with built-in implementations for
Anthropic, OpenAI, Azure OpenAI, Ollama, and Gemini (plus an offline `demo`
provider) and an optional `StreamingLLMProvider` extension for live thinking
progress. AI tools call providers directly; throughput comes from
config-driven batching and bounded concurrency inside the tool, not from a
separate worker-pool subsystem. A `ChatStructured` method with JSON Schema
enables reliable batch translation and other structured-output tasks.

## Context

Modern LLMs are capable translators, reviewers, and terminology extractors.
Treating them as a separate service loses the composability of the streaming
pipeline: AI tools should sit alongside TM leverage, term enforcement, and
QA in the same flow.

AI APIs come with practical constraints: rate limits, cost per token,
transient failures, and variable latency. The framework's answer is to keep
the provider interface thin and let the calling tool decide how much work to
batch into a single request and how many requests to run in parallel.
Workspace-scale orchestration — async job queues, multi-tenant quotas —
belongs to a platform layer, not to the framework primitives.

Providers also differ in their structured-output mechanism: OpenAI and
Azure OpenAI use `response_format: json_schema`, Anthropic uses tool-use
with `input_schema`, Ollama uses `format: json`, and Gemini uses
response schema hints. A single interface must paper over these details
while giving tools a predictable contract.

## Decision

### LLMProvider interface

```go
type LLMProvider interface {
    Name() ProviderID
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

Two non-network providers round out the registry: a mock provider
(`providers/ai/mock.go`) for deterministic tests, and a `demo` provider
(`providers/ai/demo.go`) registered as `demo` that returns illustrative
output so the browser playground can run AI commands with no API keys. The
provider list is generated from the registry in `providers/ai/provider.go`
(`Providers()`), not hardcoded — the live set surfaces as the `provider`
option in the [`ai-translate` reference](/reference/tools/ai-translate).

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
    ChatStream(ctx context.Context, messages []Message,
        onEvent func(ChatStreamEvent)) (*ChatResponse, error)
    ChatStructuredStream(ctx context.Context, messages []Message,
        schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error)
}

type ChatStreamEvent struct {
    Type    StreamEventType // StreamEventThinking | StreamEventContent | StreamEventDone
    Content string          // text chunk (thinking summary or output content)
    Usage   TokenUsage      // cumulative usage; populated on StreamEventDone
    Model   string          // model name; populated on StreamEventDone
}
```

The streaming methods deliver progress events through an `onEvent`
callback and return the final aggregated `*ChatResponse`, rather than
exposing a channel directly.

UIs and CLI tools display live thinking progress from providers that
support it (Anthropic extended thinking, Gemini `includeThoughts`). A
provider that does not implement `StreamingLLMProvider` can still be
used — callers that need streaming check for the extension with a type
assertion.

### Concurrency model

AI tools call the provider directly — `provider.Translate()` for a single
block, `provider.ChatStructured()` for a batch. There is no intervening
worker pool, rate limiter, or circuit breaker in the framework. Throughput is
a property of the tool's own configuration, illustrated by `ai-translate`
(`core/ai/tools/translate.go`):

```go
const (
    DefaultBatchSize        = 100
    DefaultBatchConcurrency = 1
)
```

`AITranslateConfig` exposes `BatchSize` and `BatchConcurrency` as schema
fields, so they surface as CLI flags and flow config like any other tool
option. The tool's `Process` method chooses a path from those values:

- **Block-by-block** (`batchSize <= 1` and `concurrency <= 1`) — the default
  `BaseTool.Process` drives one `provider.Translate()` call per translatable
  Block. Under a session it uses the simplest sequential skip/hydrate path
  (`sessionHandleBlock`): `GetOverlay` to skip already-translated Blocks,
  `PutOverlay` to write the result back. The batched path also honours session
  overlay caching, via `processBatchedWithSession`, which pre-filters cached
  Blocks and writes overlays on the way out.
- **Batched** (`processBatched`) — drains all input Parts into a slice,
  selects the translatable Blocks (skipping already-translated ones when
  `SkipMatched` is set), groups them into batches of `batchSize`, and
  translates each batch in a single `ChatStructured()` call. Batches run
  under a `chan struct{}` semaphore sized to `BatchConcurrency`, so at most
  that many LLM calls are in flight at once. All Parts are then written
  downstream in their original order; entries missing from the structured
  response fall back to individual per-block `translate()` calls (one
  `provider.Translate()` per missing Block).

Streaming mode is orthogonal: when the provider implements
`StreamingLLMProvider` and an `OnProgress` callback is supplied, the tool
routes calls through `ChatStream` / `ChatStructuredStream` to surface live
thinking summaries (see below). Transient-failure handling (retry, backoff)
is left to the individual provider implementations and the underlying SDK;
the framework does not impose a uniform retry policy.

This in-tool batching is distinct from the `ParallelBlockTool` concurrency in
[AD-004: Processing Engine](004-processing-engine.md), which parallelizes Part
dispatch across the pipeline rather than grouping Blocks into a single API
call.

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

<PipelineDiagram
  stages={[
    { label: "Source", role: "io" },
    { label: "tm-leverage", role: "translate" },
    { label: "term-lookup", role: "annotate" },
    { label: "ai-translate", role: "translate" },
    { label: "term-enforce", role: "qa" },
    { label: "ai-qa", role: "qa" },
    { label: "Sink", role: "io" },
  ]}
/>

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

### Structured batch output

The batched `ai-translate` path relies on `ChatStructured()` to make a
multi-block response unambiguous. The tool sends a numbered prompt
(`[1] …`, `[2] …`) and constrains the response to a JSON Schema that returns
`{ translations: [{ index, text }] }` with `additionalProperties: false` and
`strict: true`. Index-text pairs eliminate the text-parsing ambiguity of
free-form output and let the tool re-associate each translation with its
source Block. Blocks whose source carries inline codes are rendered as
placeholder-tagged text before the call and reconstructed from the response
via `ParseRunsPlaceholderText`, so inline markup survives the round trip.

### Prompt templates

Prompt templates live in `core/ai/prompt/` as versioned Go files
using `text/template`:

- `translate.go` — translation prompts with glossary and context (single
  and batched)
- `qa.go` — quality assurance check prompts

Tool-specific prompts that have not been factored into the shared `prompt`
package (e.g. the review prompt) are built inline in their tool, such as
`core/ai/tools/review.go`.

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

The framework's responsibility ends at the provider interface and the
pipeline tools that call it. Server-side asynchronous job queues,
multi-tenant quota enforcement, rate-limit budgets, and workspace-scale
translation orchestration are a platform layer's concern, built on top of
these framework primitives.

## Consequences

- AI translation is a pipeline tool, not a separate system. It composes
  with all other tools without special orchestration.
- Ordering is meaningful: TM leverage before AI translation avoids
  re-translating exact matches, reducing cost.
- Terminology context flows through the pipeline via annotations,
  enabling AI tools to produce terminology-consistent translations from
  the start.
- Throughput tuning lives on the tool, not in a hidden subsystem: a
  caller raises `BatchSize` to cut API call count and `BatchConcurrency` to
  run batches in parallel, with no separate worker pool to configure.
- Structured batch output gives the tool a reliable index-text contract, so
  large documents translate in far fewer calls without parsing ambiguity.
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
