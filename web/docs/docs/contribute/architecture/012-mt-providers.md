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
text-out transformations — and the adapter lets MT composes with TM leverage,
term enforcement, AI review, and other pipeline tools.

## Context

Machine translation is a core localization capability with two common use
cases:

- **Lightweight alternative to LLM translation** — MT services are typically
  faster and cheaper for straightforward content where LLM-level quality or
  context awareness is not required.
- **Gap-filling after TM leverage** — TM handles exact matches; MT handles
  the remainder before optional AI refinement.

MT and LLM providers share the same pipeline role (translate blocks) but
have fundamentally different interfaces. LLMs are general-purpose language
models with rich request context (glossary, format hints, surrounding
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
| **ModernMT**             | APIKey, Hints, BaseURL           | `MMT-ApiKey` header                | Memory hints bias translations toward TMs |
| **MyMemory**             | Email, BaseURL                   | None (free tier)                   | Email unlocks higher rate limits          |

Each provider:

- Handles locale format conversion from BCP-47 (the framework's canonical
  form, [AD-002: Content Model](002-content-model.md)) to the provider's
  expected codes.
- Exposes a `BaseURL` override for tests and private cloud endpoints.
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

MT providers resolve credentials through the same mechanism as AI
providers:

1. CLI credential store — OS keychain with provider configs as JSON.
2. Environment variables (e.g. `DEEPL_API_KEY`, `GOOGLE_API_KEY`).
3. Explicit `--api-key` flag.

Keys never appear in flow definitions or project files.

### Flow composition

A typical production flow chains TM, MT, and AI refinement:

<PipelineDiagram
  stages={[
    { label: "Reader", role: "io" },
    { label: "tm-leverage", role: "translate" },
    { label: "deepl-translate", role: "translate" },
    { label: "ai-review", role: "qa" },
    { label: "qa-check", role: "qa" },
    { label: "Writer", role: "io" },
  ]}
/>

- `tm-leverage` fills exact and generalized matches at near-zero cost.
- `deepl-translate` translates the remainder quickly and cheaply.
- `ai-review` (optional) refines MT output using LLM reasoning with
  glossary and TM context.
- `qa-check` validates the result before writing.

Switching providers is a configuration change: replace `deepl-translate`
with `google-translate` or `ai-translate`. The rest of the flow is
unchanged.

## Consequences

- MT providers mirror the AI provider pattern: registered as tools,
  composed in flows, dispatched through `BaseTool`.
- Adding a new MT provider requires implementing `MTProvider`, defining a
  config type, and registering a tool — no pipeline changes.
- The clean separation between provider (API client) and tool (pipeline
  adapter) enables testing providers without the pipeline and testing
  tools with mock providers.
- MT slots naturally into TM-before-MT-before-AI pipelines, letting
  projects use the cheapest adequate engine per segment.
- Provider-specific features (formality, memory hints, regions) are
  first-class via typed configs, not buried in a generic parameter map.

## Related

- [AD-006: Tool System](006-tool-system.md) — Tool pattern and `BaseTool`
- [AD-009: Translation Memory](009-translation-memory.md) — TM leverage
  before MT
- [AD-011: AI Providers](011-ai-providers.md) — richer provider interface
  for LLMs
- [AD-013: Kapi CLI](013-kapi-cli.md) — credential store
