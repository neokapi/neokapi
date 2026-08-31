---
id: e-07-model-providers
sidebar_position: 7
title: "E-07: Model and translation providers"
description: "Language-model capabilities plug in through an LLMProvider interface in providers/ai and machine-translation engines through a narrower MTProvider interface in providers/mt; both are reached from ordinary tools and share one credential resolver."
keywords: [neokapi, architecture decision, LLMProvider, MTProvider, model catalog, multimodal, streaming, credentials, batching, providers]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# E-07: Model and translation providers

## Summary

Two provider interfaces sit under the framework's tools, in two packages:
`providers/ai` (package `aiprovider`) for language models, and `providers/mt`
(package `mtprovider`) for machine-translation engines. They share the tools that
call them, the credential resolver, and the registration pattern; they differ in
surface area, because an LLM and an MT engine are asked for different things.

`LLMProvider` carries translation, chat, and schema-constrained structured
output, with **multimodal** messages (content is an ordered list of text, image,
audio, or video parts) and an optional streaming extension for live thinking
progress. `MTProvider` is one method: plain text in, plain text out. The
framework ships built-in LLM backends and **no** classic MT
engines: the translation core is LLM-first, and MT engines arrive as plugins that
register a config factory. On the CLI surface there is one `translate` command
and one `qa` command across every backend; `--provider` selects.

## Context

Modern language models are capable translators, reviewers, and term extractors.
Treating them as a separate service would lose the composability of the streaming
pipeline: model-backed tools should sit alongside memory leverage, term
enforcement, and checks in the same flow.

Model APIs come with practical constraints: rate limits, cost per token,
transient failures, and variable latency. The framework's answer is to keep the
provider interface thin and let the calling tool decide how much work to batch
into a single request and how many requests to run in parallel. Workspace-scale
orchestration (asynchronous job queues, multi-tenant quotas) belongs to a
platform layer rather than to the framework primitives.

Providers also differ in their structured-output mechanism: some use a
`response_format` JSON schema, some use tool-use with an input schema, some a
`format: json` switch, some a response-schema hint. A single interface must paper
over these details while giving tools a predictable contract.

Machine translation is a different shape. An MT engine is a deterministic
translation service with a minimal surface (source text, source locale, target
locale, translated text) and no notion of terminology context, format hints, or
surrounding blocks. Forcing both through `LLMProvider` would waste parameters on
MT and obscure the difference.

## Decision

### The LLM provider interface

```go
type LLMProvider interface {
    Name() ProviderID
    InputModalities() []Modality // non-text inputs accepted; text always
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Chat(ctx context.Context, messages []Message) (*ChatResponse, error)
    ChatStructured(ctx context.Context, messages []Message,
        schema JSONSchema) (*ChatResponse, error)
    Close() error
}
```

`ChatStructured` extends `Chat` with a JSON Schema constraint that forces the
provider to return structured output. The `JSONSchema` type carries `Name`,
`Description`, `Schema`, and a `Strict` flag for providers that support strict
validation.

Provider configuration is schema-driven: fields in a model-backed tool's config
generate CLI flags automatically via `schema.FromStruct()`, so no manual flag
registration is needed ([E-03](e-03-tool-system.md)).

#### Multimodal content

A `Message`'s content is an ordered list of typed parts, so one interface carries
text, images, audio, and video:

```go
type Message struct {
    Role  string        // "system" | "user" | "assistant"
    Parts []ContentPart
}

type ContentPart struct {
    Kind  ContentKind  // closed set, named type
    Text  string       // Kind == ContentText
    Media *model.Media // otherwise: a bounded slice, carried by reference
}

type ContentKind string

const (
    ContentText  ContentKind = "text"
    ContentImage ContentKind = "image"
    ContentAudio ContentKind = "audio"
    ContentVideo ContentKind = "video"
)
```

A media part carries its payload as a **`model.Media`** (the framework's
binary-reference type, [F-02](../foundations/f-02-content-model.md)) with
precedence `BlobKey > URI > Data`, rather than a bare `[]byte`. A small slice rides
inline; a larger one, such as a video clip, is a blob key or URI and is never
forced into memory. A single helper at the provider's HTTP boundary resolves the
`Media` to the backend's wire form (base64 inline, or a fetchable URL where the
provider supports one), so provider implementations stay **storage-agnostic**:
they never read a file or the blob store. This keeps one binary idiom across
`Media`, plugin I/O, and provider content.

A text-only message is a single `text` part, so translation, check, and term
tools use the interface with no media parts; the common path carries no media
ceremony. Image, audio, and video parts carry a Block's media anchor into the
prompt, which is what the multimodal refinement tier sends
([M-03](../multilingual/m-03-multimodal-content.md)).

Backends differ in which input modalities they accept, so `InputModalities()`
advertises a provider's reach (`Modality` being the image/audio/video subset of
`ContentKind`; text is always accepted) and a caller selects a provider that fits
rather than discovering the limit at call time. The live per-provider set is on
the generated [AI Models reference](/models).

#### Built-in LLM providers

The framework registers backends for Anthropic, OpenAI, Azure OpenAI, Gemini,
and Ollama, plus two non-network providers: a mock provider for deterministic
tests, and a `demo` provider that returns illustrative output so the browser
playground can run model-backed commands with no API keys. A `claude-code`
provider runs prompts through a locally installed Claude Code CLI, billing the
user's subscription instead of an API key: keyless, though not local, since
content still travels to the vendor.

Ollama is the on-device backend, driving a local runtime over HTTP with no key
and managed through `kapi models ollama`. `aiprovider.IsLocalProvider` is a
*registration* property, not a hardcoded list: a provider sets it when it
registers, so plugin-registered providers declare themselves local the same way
the built-ins do, and the transformer placement pass
([E-03](e-03-tool-system.md)) can refine remote-egress away for them.

Plugins register further providers at runtime, so the live provider set is
whatever `aiprovider.Providers()` returns, not a fixed list; it surfaces as the
`provider` option in the [`translate` reference](/reference/tools/translate).
Default models are not listed in prose, because they change with every model
generation. They live in the model catalog, below.

Each provider takes a `Config` struct with API key, base URL, model name, and
generation parameters. Azure additionally accepts a `TokenProvider` function,
enabling passwordless access via Managed Identity.

#### The model catalog

The models kapi supports are described in one place: `providers/ai/models.json`,
a curated catalog embedded into the framework and read as `aiprovider.Models()`.
It is the single source of truth, and the rest derives from it.

**Why data, and why curated.** One catalog rather than model knowledge spread
over a default-model constant per provider, a prefix-to-ceilings map, and a
separate price table; none of which answers the question a user actually asks:
*is this model current, superseded, or retired, and since when?* The catalog
carries the defaults and the ceilings (`LimitsForModel` resolves through it; a
test asserts every provider default is catalogued and marked as that provider's
default) as well as the lifecycle. It is **data** because a model list hardcoded
in Go goes stale silently; it is **curated** because vendor APIs return only what
is live today as a flat list of ids; they do not say when a model entered
neokapi, what replaced it, or when it retires. Those are facts about *our*
support, and only a human, or an agent reading a model card, can supply them.

**Each entry** carries the model's provider, label, aliases, output and context
ceilings, and its lifecycle: `status` (`active` | `superseded`), `introduced`,
`superseded_by`, and `retirement_date`. There is no `retired` status: a model the
provider stops serving is **removed** from the catalog rather than kept as a
tombstone: the catalog is the list of models kapi supports, and a dead model supports
nothing. An announced *future* retirement is a date on a still-live entry, shown
as a warning. A model can be one provider's current default while superseded
elsewhere, and the catalog records that rather than papering over it; `kapi models
list` and the `/models` page both surface it.

**Recommended vs known.** The catalog is *descriptive*, not an allowlist: naming
a model it does not list is never rejected: the string goes to the provider API
as-is, and `LimitsForModel` falls back to a conservative batch size until an entry
exists. So the catalog serves two audiences at once, and a `recommended` flag
(absent = yes) separates them. It stays true for the models most projects should
reach for, and is set false for a model that is fully supported but a poor
default: capable-but-premium, overkill for faithful content work, or tuned for a
different task. A non-recommended model keeps its ceilings and is callable by
name; it just sits under "Advanced" on the `/models` page rather than in the
primary list, which is sorted Recommended → Advanced → Legacy (superseded). An
*active default* can never be non-recommended; a test enforces it.

**Staying honest.** Curation rots, so `make check-models` (`scripts/modelcheck`)
is the alarm: it lists what each provider serves today and reports any catalogued
model that is gone, or, with `-candidates`, any live model the catalog omits. The
live half needs provider credentials and stays a manual or scheduled tool: a
rate-limited provider must never be mistaken for a retired one, the same
false-cliff trap the [batch eval](/batch-eval) guards against. The keyless half
(every published price must be for a catalogued model) is an ordinary unit test,
so it runs in `make test`. The `/models` page is generated from the catalog and
gated by `make check-reference-docs`, so it cannot describe a model the catalog
no longer lists.

#### StreamingLLMProvider

An optional extension interface surfaces live progress events for providers that
support them:

```go
type StreamingLLMProvider interface {
    LLMProvider
    ChatStream(ctx context.Context, messages []Message,
        onEvent func(ChatStreamEvent)) (*ChatResponse, error)
    ChatStructuredStream(ctx context.Context, messages []Message,
        schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error)
}

type ChatStreamEvent struct {
    Type    StreamEventType // thinking | content | done
    Content string          // text chunk (thinking summary or output content)
    Usage   TokenUsage      // cumulative usage; populated on done
    Model   string          // model name; populated on done
}
```

The streaming methods deliver progress events through an `onEvent` callback and
return the final aggregated `*ChatResponse`, rather than exposing a channel
directly. UIs and CLI tools display live thinking progress from providers that
support it. A provider that does not implement `StreamingLLMProvider` can still
be used; callers that need streaming check for the extension with a type
assertion.

#### Concurrency model

Model-backed tools call the provider directly: `Translate()` for a single block,
`ChatStructured()` for a batch. There is no intervening worker pool, rate
limiter, or circuit breaker in the framework. Throughput is a property of the
tool's own configuration, illustrated by `translate` (`core/ai/tools/translate.go`):

- **Batching intent rather than a magic number.** The user-facing knob is `--batching`
  with two values: `auto`, the default, hands sizing to a packer that fits blocks
  to the model's context and output ceilings from the catalog; `single` pins one
  block per call. An explicit `batchSize` in a flow step's config overrides both.
- **Two automatic downgrades to per-block.** A *local* provider is forced to
  `single`, because a small on-device model tends to ignore the "return JSON"
  instruction and emit plain numbered text, and a very large prompt is a slow
  generation with no intermediate feedback. Do-not-translate enforcement also
  forces `single` (and drops neighbour context), because each protected span is
  masked with a sentinel that must survive one generation intact, which the batch
  path cannot track per span.
- **Bounded parallelism.** `BatchConcurrency` (default 1) sizes a semaphore, so
  at most that many calls are in flight at once.

The block-by-block path drives one `Translate()` call per translatable Block;
under a session it uses the sequential skip-and-hydrate path, reading an existing
overlay to skip already-translated Blocks and writing the result back. The
batched path drains the input Parts, selects the translatable Blocks, groups them,
and translates each group in a single `ChatStructured()` call, then writes all
Parts downstream in their original order; entries missing from the structured
response fall back to individual per-block calls. The batched path honours the
same session overlay cache, pre-filtering cached Blocks and writing overlays on
the way out.

Streaming mode is orthogonal: when the provider implements
`StreamingLLMProvider` and a progress callback is supplied, the tool routes calls
through the streaming methods to surface live thinking summaries.
Transient-failure handling (retry, backoff) is left to the individual provider
implementations and the underlying SDK; the framework imposes no uniform retry
policy.

What the framework does add is observation. `aiprovider.Traced` wraps a
provider so that each call opens one span on the observation seam
([E-01](e-01-processing-engine.md#observation-seam)) with GenAI attribute names,
recording the model, token usage, cost and truncation, and never prompt content.
A host that wants a circuit breaker or a budget wraps the provider outside the
framework, with tracing outside the breaker so a refused call is still measured.

This in-tool batching is distinct from the `ParallelBlockTool` concurrency in
[E-01](e-01-processing-engine.md), which parallelizes Part dispatch across the
pipeline rather than grouping Blocks into a single API call. Prompt construction
and the structured batch contract are
[M-05](../multilingual/m-05-prompts-and-batching.md).

### The MT provider interface

```go
type MTProvider interface {
    Name() ProviderID
    Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)
    Close() error
}

type TranslateRequest struct {
    Source       string
    SourceLocale model.LocaleID
    TargetLocale model.LocaleID
}

type TranslateResponse struct {
    Translation string
}
```

The interface is minimal. One method, plain text in and out, against
`LLMProvider`'s translate, chat, structured chat, and modality declaration.

**The framework ships no classic MT engines.** The translation core is LLM-first,
and the only built-in `MTProvider` is the offline `demo` provider. A real MT
engine is hosted by a plugin, which registers a `ConfigFactory` with
`mtprovider.RegisterConfigFactory(id, factory)` (and, for a credential-free
engine, a provider factory). `mtprovider.NewProviderWithConfig(id, MTConfig{...})`
constructs one from a generic, credential-bearing config, so the unified
`translate` tool can build any registered engine from a flow or CLI config map
without knowing anything about it. `mttools.Providers` is the canonical list of
engines reachable through `kapi translate --provider <id>`; a plugin appends to
it, which is how an engine becomes selectable and appears in the tool's provider
enum.

An MT provider handles locale-format conversion from BCP-47 (the framework's
canonical form) to whatever codes it expects, accepts a `BaseURL` naming a
self-hosted or private-cloud endpoint, and returns structured errors carrying the
HTTP status.

#### Pipeline integration

An MT engine reaches the pipeline through `MTTranslateTool`
(`core/mt/tools/translate.go`), which embeds `BaseTool`
([E-03](e-03-tool-system.md)) and sets `Produce`:

1. Receive a `*Part` from the input channel.
2. Extract translatable Blocks.
3. Skip non-translatable blocks (`Translatable == false`) and blocks that already
   have a target for the configured locale.
4. Call `provider.Translate()` with the block's source text.
5. Set the translation on the block's target locale.
6. Pass the updated Part downstream.

Because the tool only ever writes the target, source stays read-only by
construction, exactly like the LLM translate path.

### Credential resolution

Both provider families resolve credentials through one function,
`host/credentials.ResolveCredentials`. Highest precedence first:

1. A tool that does not declare `credentials` in `Requires` is returned
   unchanged.
2. An inline `apiKey`: the `--api-key` flag, or a step's own config.
3. A saved credential named with `--credential`.
4. Nothing at all, for a **keyless local provider** (Ollama, demo): a local run
   is never failed for want of a saved credential.
5. The provider's environment variable, injected as the API key.
6. Auto-detection from the store, when exactly one credential matches.

Keys never appear in flow definitions or project files.

**The endpoint travels with the key.** `baseURL` is part of the same decision as
the secret sent to it, so it is stored alongside the credential (`kapi
credentials add --base-url`) and injected during credential resolution. A flow
step's `config:` cannot set it: `ResolveCredentials` clears the key on the way in,
unconditionally and before every return, and re-sets it only from a resolved
credential.

**So a custom endpoint arrives by exactly one route: the saved credential.** The
other ways to supply a key resolve before an endpoint could be injected: an
inline key returns immediately, and the environment fallback builds a provider
config that has no endpoint to give. A self-hosted endpoint combined with
`--api-key`, or with a provider environment variable set, therefore calls the
*public* host, saved credential or not. This is the intended shape rather than a
gap (the endpoint and the secret are one decision, and only a saved credential
holds both), but it is the shape a user is most likely to be surprised by, so
`kapi credentials add --base-url` names it and
[Choose a translation provider](/kapi/recipes/choose-a-translation-provider)
states it where a user is choosing.

This is enforced in the resolver rather than by the tool's struct tags, because
the two kinds of tag mean different things. `schema:"-"` keeps a field off the
CLI and out of the generated form, but `core/schema.ApplyConfig` is a plain JSON
round-trip: a step's config map reaches every field with a `json` tag, whatever
its `schema` tag says. Hiding a field from the form is a presentation choice;
keeping a recipe out of it is a separate act.

### Default provider resolution

*Which* provider a run uses when nothing names one is a separate question from
*how it authenticates*, and it has exactly one resolver:
`config.ResolveAIDefault`. It returns the provider, the model, and **where the
value came from** (`env` | `config` | `none`), and `config.SetAIDefault` /
`config.ClearAIDefault` are the matching single write path.

Precedence, matching every other config key: an explicit `--provider`/`--model`
flag or an inline recipe value first, then `KAPI_AI_PROVIDER` / `KAPI_AI_MODEL`,
then the stored `ai.provider` / `ai.model`.

One resolver rather than six read sites, because six read sites had no shared
answer to "what is configured, and where did it come from", so a scope bug in
the reader was invisible in all of them at once. Reporting the *source* alongside
the value is what lets a diagnostic name the file or the environment variable to
change, instead of asserting that nothing is configured.

The app config file is pinned to `config.GlobalConfigFilePath()` (the same
function the writers use), and never resolved through a search path. A recipe is
project configuration and app config is per-machine, so the working directory is
not a config location: a search path reaching it would load the *recipe* as the
app config inside any project, since a kapi project's recipe is also named
`kapi.yaml`, and every stored default would read as empty.

`$HOME/.config/kapi/kapi.yaml` and `/etc/kapi/kapi.yaml` are read as
lower-precedence layers beneath the pinned file, so a hand-written config works.
On Linux the first of those *is* the pinned path, since `os.UserConfigDir`
honours XDG; on macOS it resolves to `~/Library/Application Support`, which is
why the two differ at all.

### The tools that call them

Both families reach the pipeline as ordinary tools
([E-03](e-03-tool-system.md)). On the CLI surface, translation is a single
`translate` command across every backend and quality a single `qa` command; the
backend is selected with `--provider`, while the two provider interfaces stay
distinct underneath. `review`, `term-extract`, `entity-extract`, `voice-check`,
`voice-infer`, and `media-refine` are the other model-backed tools; the
authoritative list is the [Tool Reference](/tools).

Because they are ordinary tools, they compose naturally:

<PipelineDiagram
  stages={[
    { label: "source", sub: "binding", role: "io" },
    { label: "recycle", sub: "memory", role: "translate" },
    { label: "translate", sub: "--provider", role: "translate" },
    { label: "review", role: "qa" },
    { label: "qa", role: "qa" },
    { label: "sink", sub: "binding", role: "io" },
  ]}
/>

- `recycle` fills exact and generalized matches from the content memory at
  near-zero cost.
- `translate` translates the remainder through whichever backend `--provider`
  named.
- `review` annotates a review assessment using model reasoning with term and
  memory context; it does not rewrite the target.
- `qa` validates the result before it is written.

Switching backends is a configuration change (one `--provider` value for
another), and the rest of the flow is unchanged.

Model-backed tools receive terminology context from upstream stages: matched
terms and their preferred translations, identified entities with their
do-not-translate flags and locale formatting hints, and a dedicated term-constraint
section for the current Block's coordinates. Terminology is therefore not only a
post-translation check; it guides the generation from the start
([C-08](../context/c-08-terms.md)).

### Scope boundary

The framework's responsibility ends at the provider interfaces and the pipeline
tools that call them. Server-side asynchronous job queues, multi-tenant quota
enforcement, rate-limit budgets, and workspace-scale translation orchestration
are a platform layer's concern, built on top of these framework primitives.

## Consequences

- Translation is a pipeline tool, not a separate system. It composes with all
  other tools without special orchestration.
- Ordering is meaningful: memory leverage before generation avoids
  re-translating exact matches, reducing cost.
- Term and entity context flows through the pipeline via overlays, so
  model-backed tools produce terminology-consistent output from the start.
- Throughput tuning lives on the tool, not in a hidden subsystem: a caller
  chooses a batching intent and a batch concurrency, with no worker pool to
  configure, and the two automatic downgrades keep correctness ahead of the
  batch win where they conflict.
- `ChatStructured` gives tools a reliable JSON contract across providers with
  very different structured-output mechanisms.
- The provider abstraction enables cost choices: a local runtime for development,
  a hosted model for production, an MT engine where the content does not need
  model-level context.
- The mock and demo providers make deterministic tests and a keyless browser
  playground possible without API calls.
- Azure Managed Identity eliminates API-key management for that deployment while
  the same interface continues to support key-based auth elsewhere.
- Keeping MT engines out of the framework keeps the default binary free of
  vendor API clients and their credentials, and makes adding one a plugin
  decision rather than a framework change.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): overlays on Blocks; the media anchor a multimodal message carries
- [E-01: The processing engine](e-01-processing-engine.md): flow execution and `ParallelBlockTool`
- [E-03: The tool system](e-03-tool-system.md): the tool pattern, provider injection, and the remote-egress side effect
- [E-05: The plugin system](e-05-plugin-system.md): how a plugin registers a provider
- [M-03: Multimodal content](../multilingual/m-03-multimodal-content.md): the refinement tier that sends image, audio, and video parts
- [M-05: Prompts and batching](../multilingual/m-05-prompts-and-batching.md): prompt templates and the structured batch contract
- [C-09: Content memory](../context/c-09-content-memory.md): `recycle` runs before generation and feeds it context
- [C-08: Terms](../context/c-08-terms.md): term context feeds the prompt and the post-check
- [S-01: The kapi CLI](../surfaces/s-01-kapi-cli.md): the credential store
