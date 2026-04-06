---
id: 019-mt-providers
sidebar_position: 19
title: "AD-019: Machine Translation Providers"
---

# AD-019: Machine Translation Providers

## Context

Machine translation is a core capability in localization workflows. Traditional approaches treat MT services as external integrations — users configure API keys, call a service, and paste results. neokapi integrates MT as first-class pipeline tools, mirroring the AI provider architecture ([AD-008](./008-ai-integration.md)).

The MT provider system was originally placed under `connectors/`, but connectors are bidirectional integrations with content systems (CMS, design tools, code repositories). MT services are stateless transformation providers — they take text in and return text out. This mismatch led to the `mt/provider/` reorganization.

## Decision

### MTProvider Interface

Machine translation is modeled as a minimal provider interface in `core/mt/provider/`:

```go
type MTProvider interface {
    Name() string
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

The interface is intentionally simpler than the AI provider (`LLMProvider`):

- **MT**: One method (`Translate`), plain text in/out
- **AI**: Two methods (`Translate` + `Chat`), richer request types with context, glossary, format, confidence

This reflects the fundamental difference: MT providers are deterministic translation services; LLM providers are general-purpose language models that can translate, review, and converse.

### Five Built-in Providers

| Provider                 | Config Fields                    | Auth Mechanism                     | Notes                                                 |
| ------------------------ | -------------------------------- | ---------------------------------- | ----------------------------------------------------- |
| **DeepL**                | APIKey, Formality, BaseURL       | `DeepL-Auth-Key` header            | Supports formality control (more/less/prefer)         |
| **Google Translate**     | APIKey, ProjectID, BaseURL       | API key in query string            | Cloud Translation API v2                              |
| **Microsoft Translator** | SubscriptionKey, Region, BaseURL | `Ocp-Apim-Subscription-Key` header | Optional region header                                |
| **ModernMT**             | APIKey, Hints, BaseURL           | `MMT-ApiKey` header                | Memory hints bias translations toward specific TMs    |
| **MyMemory**             | Email, BaseURL                   | None (free)                        | Email unlocks higher rate limits; no API key required |

Each provider implementation:

- Handles locale format conversion (BCP-47 → provider-specific codes)
- Includes a `BaseURL` override for testing
- Returns structured errors with HTTP status codes

### Pipeline Integration via MTTranslateTool

Each provider is wrapped in an `MTTranslateTool` (`core/mt/tools/translate.go`) that embeds `BaseTool`:

```go
type MTTranslateTool struct {
    tool.BaseTool
    provider     provider.MTProvider
    sourceLocale model.LocaleID
    targetLocale model.LocaleID
}
```

The tool:

1. Receives `*Part` from the pipeline channel
2. Extracts translatable `Block` resources
3. Skips non-translatable blocks (`block.Translatable == false`)
4. Calls `provider.Translate()` with the block's source text
5. Sets the translation on the block's target locale via `block.SetTargetText()`
6. Passes the updated Part downstream

This makes MT providers composable with all other pipeline tools ([AD-006](./006-tool-system.md)). A typical flow might chain: `tm-leverage` → `deepl-translate` → `qa-check`, where TM handles exact matches and DeepL translates the remainder.

### Provider Configuration Pattern

Each provider defines a `ToolConfig` type that embeds the provider config and adds locale fields:

```go
type DeepLToolConfig struct {
    DeepLConfig
    SourceLocale model.LocaleID
    TargetLocale model.LocaleID
}
```

Tool configs implement `ToolName()`, `Reset()`, and `Validate()` for integration with the flow definition system.

## Alternatives Considered

- **MT as connectors**: The original placement. Rejected because connectors are bidirectional content integrations, not stateless transformations.
- **Single provider abstraction**: One generic provider with config-driven behavior. Rejected because each MT service has unique features (formality, memory hints, free tier) that are better expressed in typed configs.
- **External service calls only**: No pipeline integration, just a wrapper library. Rejected because pipeline integration enables TM-before-MT optimization and composability with QA tools.

## Consequences

- MT providers mirror the AI provider pattern — both are registered as tools, composed in flows, and use the same `BaseTool` dispatch mechanism.
- Adding a new MT provider requires: implement `MTProvider`, create a config type, register a tool. No changes to the pipeline.
- Provider credentials are managed through the credential store ([AD-015](./015-auth-and-workspaces.md)), keeping API keys out of flow definitions.
- The clean separation between provider (API client) and tool (pipeline adapter) enables testing providers without the pipeline and testing tools with mock providers.
