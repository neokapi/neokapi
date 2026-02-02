---
id: 009-ai-integration
sidebar_position: 9
title: "ADR-009: AI Integration"
---
# ADR-009: First-class AI/LLM integration

## Context

Large language models have become capable translators and quality reviewers.
Rather than bolting AI capabilities onto the side as a separate service, we
wanted LLM-powered translation, QA, terminology extraction, and review to be
composable tools in the same pipeline as traditional processing steps.

## Decision

### LLMProvider Interface

AI capabilities are backed by an `LLMProvider` interface with three
implementations: Anthropic Claude, OpenAI, and Ollama (local).

```go
type LLMProvider interface {
    Translate(ctx, text, sourceLocale, targetLocale, glossary) (string, error)
    Chat(ctx, messages) (string, error)
}
```

### AI Tools

Four AI tools are implemented as standard Tools that embed `BaseTool`:

| Tool               | Purpose                                       |
|--------------------|-----------------------------------------------|
| `ai-translate`     | Translate untranslated Blocks using LLM       |
| `ai-qa`            | Check translations for fluency, accuracy, terminology |
| `ai-terminology`   | Extract terminology from source Blocks        |
| `ai-review`        | Review translations with explanations         |

Because AI tools are standard Tools, they compose naturally in flows:

```
Reader -> TM Leverage -> AI Translate -> AI QA -> Writer
```

### Prompt Engineering

Prompt templates live in `ai/prompt/` and are context-aware: they include
surrounding Blocks, glossary constraints, TM matches, and format metadata.
Templates are centralized for easy tuning without recompiling.

### Connector Layer

External translation services (DeepL, Google, Microsoft, MyMemory, ModernMT)
are integrated via `connectors/` as an alternative to LLM-based translation.

## Alternatives Considered

- **Separate AI service**: decoupled but loses composability; requires
  separate deployment.
- **AI as a post-processing step**: misses the opportunity to leverage TM
  context and inline QA.
- **Hard-coded provider**: the `LLMProvider` interface allows swapping
  providers per project or using local models via Ollama.

## Consequences

- AI translation is a pipeline tool, not a separate system
- Tools can be ordered to maximize quality: TM leverage before AI avoids
  retranslating exact matches
- Provider abstraction enables cost optimization (local Ollama for dev,
  Claude for production)
- Prompt templates are centralized and testable
- Mock providers enable deterministic testing without API calls
