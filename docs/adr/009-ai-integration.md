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

Six AI tools are implemented as standard Tools that embed `BaseTool`:

| Tool               | Purpose                                       |
|--------------------|-----------------------------------------------|
| `ai-translate`     | Translate untranslated Blocks using LLM       |
| `ai-qa`            | Check translations for fluency, accuracy, terminology |
| `ai-terminology`   | Extract terminology candidates from source Blocks |
| `ai-review`        | Review translations with explanations         |
| `entity-annotate`  | Annotate named entities (people, places, dates, products) |
| `brand-voice-check`| Validate content against brand voice rules (Phase 3, ADR-016) |

The `ai-terminology` tool creates `TermAnnotation` entries with
`status: proposed`, feeding the terminology lifecycle workflow. The
`entity-annotate` tool produces `EntityAnnotation` entries that serve as
do-not-translate markers, localization hints, and context for AI
translation. See ADR-016 for the full terminology and brand management
design.

Because AI tools are standard Tools, they compose naturally in flows:

```
Reader -> TM Leverage -> Term Lookup -> AI Translate -> Term Enforce -> AI QA -> Writer
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

### Terminology-Aware Prompts

AI tools receive terminology context when available. The prompt system
includes:

- **Term annotations**: When `term-lookup` has run before AI translation,
  the matched terms and their preferred translations are included in the
  prompt, guiding the LLM toward consistent terminology.
- **Entity annotations**: When `entity-annotate` has run, the identified
  entities (with DNT flags, locale formatting hints) are included in the
  prompt context.
- **Glossary constraints**: A dedicated glossary section in the prompt
  template lists preferred/forbidden terms applicable to the current
  Block's context (domain, product, market).

This composability means terminology enforcement is not just a validation
step — it actively guides AI translation quality.

## Consequences

- AI translation is a pipeline tool, not a separate system
- Tools can be ordered to maximize quality: TM leverage before AI avoids
  retranslating exact matches
- Terminology context flows through the pipeline via annotations, enabling
  AI tools to produce terminology-consistent translations from the start
- Provider abstraction enables cost optimization (local Ollama for dev,
  Claude for production)
- Prompt templates are centralized and testable
- Mock providers enable deterministic testing without API calls
