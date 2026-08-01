---
id: 012-mt-providers
sidebar_position: 12
title: "AD-012: Machine Translation Providers"
description: "Architecture decision: MT services plug in through an MTProvider interface in providers/mt/ with built-in backends for DeepL, Google Translate, Microsoft Translator, ModernMT, and MyMemory."
keywords: [MT providers, MTProvider, DeepL, Google Translate, Microsoft Translator, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-012: Machine Translation Providers

## Summary

Machine translation services (DeepL, Google, Microsoft, ModernMT, MyMemory)
plug into the pipeline through an `MTProvider` interface in `providers/mt/`
(package `mtprovider`) and an `MTTranslateTool` adapter. The interface is
intentionally simpler than `LLMProvider` — MT services are stateless text-in,
text-out transformations — and the adapter lets MT compose with memory leverage,
term enforcement, AI review, and other pipeline tools.

## Context

Machine translation is a core capability with two common use
cases:

- **Lightweight alternative to LLM translation** — MT services are typically
  faster and cheaper for straightforward content where LLM-level quality or
  context awareness is not required.
- **Gap-filling after memory leverage** — the memory handles exact matches; MT
  handles the remainder before optional AI refinement.

MT and LLM providers share the same pipeline role (translate blocks) but
have fundamentally different interfaces. LLMs are general-purpose language
models with rich request context (terminology, format hints, surrounding
blocks). MT services are deterministic translation engines with a minimal
surface: source text, source locale, target locale, translated text.
Forcing both through `LLMProvider` would waste parameters on MT and
obscure the difference.

MT services also have distinctive features — DeepL's formality control,
ModernMT's memory hints, Microsoft's regional endpoints — that are best
expressed as typed configs rather than a generic parameter map.

## Decision

### MTProvider interface

```go
type MTProvider interface {
    Name() ProviderID
    Translate(ctx context.Context, req TranslateRequest) (
        *TranslateResponse, error)
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

The interface is deliberately minimal. One method, plain text in and out.
Compared to `LLMProvider` ([AD-011: AI Providers](011-ai-providers.md)) —
which has `Translate`, `Chat`, and `ChatStructured` — MT providers do one
thing.

### Built-in providers

| Provider                 | Config Fields                    | Auth                               | Distinctive Feature                       |
| ------------------------ | -------------------------------- | ---------------------------------- | ----------------------------------------- |
| **DeepL**                | APIKey, Formality, BaseURL       | `DeepL-Auth-Key` header            | Formality control (more/less/prefer)      |
| **Google Translate**     | APIKey, ProjectID, BaseURL       | `X-Goog-Api-Key` header            | Cloud Translation API v2                  |
| **Microsoft Translator** | SubscriptionKey, Region, BaseURL | `Ocp-Apim-Subscription-Key` header | Optional region header                    |
| **ModernMT**             | APIKey, Hints, BaseURL           | `MMT-ApiKey` header                | Memory hints bias translations toward stored memories |
| **MyMemory**             | Email, BaseURL                   | None (free tier)                   | Email unlocks higher rate limits          |

Each provider:

- Handles locale format conversion from BCP-47 (the framework's canonical
  form, [AD-002: Content Model](002-content-model.md)) to the provider's
  expected codes.
- Accepts a `BaseURL` naming a self-hosted or private-cloud endpoint. It is
  **host configuration, not a recipe field** — see Credential resolution below.
- Returns structured errors with HTTP status codes.

### Pipeline integration via MTTranslateTool

Each provider is wrapped in an `MTTranslateTool`
(`core/mt/tools/translate.go`) that embeds `BaseTool`
([AD-006: Tool System](006-tool-system.md)):

```go
type MTTranslateTool struct {
    tool.BaseTool
    provider     mtprovider.MTProvider
    sourceLocale model.LocaleID
    targetLocale model.LocaleID
}
```

The tool:

1. Receives `*Part` from the input channel.
2. Extracts translatable Blocks.
3. Skips non-translatable blocks (`block.Translatable == false`) and
   blocks that already have a target for the configured locale.
4. Calls `provider.Translate()` with the block's source text.
5. Sets the translation on the block's target locale via
   `block.SetTargetText()`.
6. Passes the updated Part downstream.

### Provider configuration pattern

Each provider defines a `ToolConfig` type that embeds the provider config
and adds locale fields:

```go
type DeepLToolConfig struct {
    DeepLConfig
    SourceLocale model.LocaleID
    TargetLocale model.LocaleID
}
```

Tool configs implement `ToolName()`, `Reset()`, and `Validate()` for
integration with the flow definition system.

### Credential resolution

MT providers resolve credentials through the same mechanism as AI providers.
Highest precedence first, as `host/credentials.ResolveCredentials` applies it:

1. An inline `apiKey` — the `--api-key` flag, or a step's own config.
2. A saved credential named with `--credential`.
3. The provider's environment variable (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
   `GEMINI_API_KEY` then `GOOGLE_API_KEY`, `AZURE_OPENAI_API_KEY`).
4. Auto-detect from the store, when exactly one credential matches.

Keys never appear in flow definitions or project files.

**The endpoint travels with the key.** `BaseURL` is part of the same decision
as the secret sent to it, so it is stored alongside the credential (`kapi
credentials add --base-url`) and injected during credential resolution. A flow
step's `config:` cannot set it: `host/credentials.ResolveCredentials` clears the
key on the way in and re-sets it only from a resolved credential.

**So a custom endpoint arrives by exactly one route — step 2.** The two other
ways to supply a key resolve before an endpoint could be injected: an inline
key returns immediately, and the environment fallback builds a provider config
that has no endpoint to give. A self-hosted endpoint combined with `--api-key`,
or with `OPENAI_API_KEY` set, therefore calls the *public* host, saved
credential or not. This is the intended shape rather than a gap — the endpoint
and the secret are one decision, and only a saved credential holds both — but
it is the shape a user is most likely to be surprised by, so
`kapi credentials add --base-url` names it and
[Choose a translation provider](/kapi/recipes/choose-a-translation-provider)
states it where a user is choosing. Pinned by
`TestResolveCredentials_OnlyAStoredCredentialCarriesAnEndpoint`.

This is enforced in the resolver rather than by the tool's struct tags, because
the two kinds of tag mean different things. `schema:"-"` keeps a field off the
CLI and out of the generated form, but `core/schema.ApplyConfig` is a plain
json round-trip — a step's config map reaches every field with a `json` tag,
whatever its `schema` tag says. Hiding a field from the form is a presentation
choice; keeping a recipe out of it is a separate act.

### Flow composition

On the CLI surface there are no per-engine commands; every MT engine is
reached through the single `translate` command (and the matching `translate`
tool in a flow) by selecting it with `--provider` — the same command that
reaches the LLM providers. A typical production flow chains the memory, MT, and
AI refinement:

<PipelineDiagram
  stages={[
    { label: "source", sub: "binding", role: "io" },
    { label: "recycle", role: "translate" },
    { label: "translate", sub: "--provider deepl", role: "translate" },
    { label: "review", role: "qa" },
    { label: "qa", role: "qa" },
    { label: "sink", sub: "binding", role: "io" },
  ]}
/>

- `recycle` fills exact and generalized matches at near-zero cost.
- `translate --provider deepl` translates the remainder quickly and cheaply.
- `review` (optional) annotates a review property on MT output using LLM
  reasoning with term and memory context — it does not rewrite the target.
- `qa` validates the result before writing.

Switching providers is a configuration change: switch `--provider deepl`
to `--provider google` (or an LLM provider such as `--provider anthropic`).
The rest of the flow is unchanged.

## Consequences

- MT providers mirror the AI provider pattern: registered as tools,
  composed in flows, dispatched through `BaseTool`.
- Adding a new MT provider requires implementing `MTProvider`, defining a
  config type, and registering a tool — no pipeline changes.
- The clean separation between provider (API client) and tool (pipeline
  adapter) enables testing providers without the pipeline and testing
  tools with mock providers.
- MT slots naturally into memory-before-MT-before-AI pipelines, letting
  projects use the cheapest adequate engine per segment.
- Provider-specific features (formality, memory hints, regions) are
  first-class via typed configs, not buried in a generic parameter map.

## Related

- [AD-006: Tool System](006-tool-system.md) — Tool pattern and `BaseTool`
- [AD-009: Content memory](009-content-memory.md) — memory leverage
  before MT
- [AD-011: AI Providers](011-ai-providers.md) — richer provider interface
  for LLMs
- [AD-013: Kapi CLI](013-kapi-cli.md) — credential store
