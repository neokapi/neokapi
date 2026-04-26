---
sidebar_position: 8
title: Brand Voice
---

# Brand Voice Governance

neokapi includes a brand voice governance system that helps teams maintain consistent tone, style, and vocabulary across all content and languages. It bridges terminology management and brand governance in a single framework.

## What is Brand Voice?

Brand voice defines _how_ your organization communicates -- the personality, formality, word choices, and writing patterns that make your content recognizable. While terminology management ensures you use the right words, brand voice governance ensures you say them the right way.

neokapi's brand voice system provides:

- **Voice profiles** -- structured definitions of tone, style, and vocabulary rules
- **Compliance scoring** -- quantitative measurement of brand consistency (0-100)
- **AI-powered checking** -- LLM analysis of content against brand guidelines
- **MCP integration** -- AI agents can consume profiles and check content automatically
- **Starter packs** -- ready-to-use profiles for common brand archetypes

## Voice Profiles

A voice profile captures your brand voice as machine-readable rules:

```yaml
name: "Acme Corp"
description: "Professional yet approachable B2B SaaS voice"

tone:
  personality: [knowledgeable, helpful, confident]
  formality: neutral
  emotion: warm
  humor: light

style:
  active_voice: true
  sentence_length: medium
  person_pov: second # "you" / "your"
  contractions: sometimes

vocabulary:
  preferred_terms:
    - term: "workspace"
      note: "Use instead of 'account' or 'organization'"
    - term: "team member"
      note: "Use instead of 'user'"
  forbidden_terms:
    - term: "leverage"
      replacement: "use"
      severity: minor
    - term: "synergy"
      replacement: "collaboration"
  competitor_terms:
    - term: "Slack"
      replacement: "messaging platform"
      severity: critical

examples:
  - before: "Users can leverage the platform to achieve synergy."
    after: "Your team can use the workspace to collaborate more effectively."
    explanation: "Active voice, preferred terms, removed jargon"
    category: style
```

### Locale Overrides

Profiles can be adjusted for specific markets:

```yaml
locales:
  ja:
    formality: formal # Japanese content should be more formal
    person_pov: third # Avoid direct "you" in Japanese
    cultural_notes: "Use keigo (honorific language) for UI text"
  de:
    contractions: never # German UI text avoids contractions
```

### Channel Overrides

Different content channels can have different rules:

```yaml
channels:
  social_media:
    tone:
      humor: frequent
      formality: casual
  documentation:
    tone:
      formality: technical
    style:
      contractions: never
```

## Compliance Scoring

Brand compliance is scored on a 0-100 scale across five dimensions:

| Dimension        | What it measures                                   |
| ---------------- | -------------------------------------------------- |
| Tone             | Voice personality, formality, emotion alignment    |
| Style            | Writing rules (active voice, sentence length, POV) |
| Vocabulary       | Preferred/forbidden/competitor term usage          |
| Clarity          | Readability and comprehension                      |
| Brand compliance | Overall brand alignment                            |

Issues are rated by severity:

| Severity | Weight | Example                   |
| -------- | ------ | ------------------------- |
| Neutral  | 0      | Informational note        |
| Minor    | 1      | Slight tone inconsistency |
| Major    | 5      | Wrong term used           |
| Critical | 25     | Competitor term used      |

A score of 100 means no issues found. Each finding reduces the score by its severity weight.

## Starter Packs

Five built-in starter packs provide ready-to-use starting points:

| Pack               | Best for                                | Formality |
| ------------------ | --------------------------------------- | --------- |
| `professional-b2b` | Enterprise software, B2B SaaS           | Formal    |
| `friendly-dtc`     | Consumer products, D2C brands           | Casual    |
| `technical-docs`   | Developer documentation, API references | Technical |
| `marketing-blog`   | Content marketing, blog posts           | Neutral   |
| `customer-support` | Help center, support responses          | Neutral   |

Each pack includes tone settings, style rules, vocabulary constraints, and before/after examples. Customize them to match your brand.

## Pipeline Integration

The `brand-voice-check` tool runs in the pipeline alongside other tools:

```
Reader -> TM Leverage -> Term Lookup -> AI Translate -> Brand Voice Check -> AI QA -> Writer
```

It uses an LLM to analyze content against the voice profile and attaches compliance scores and findings to each Block as annotations. The `brand-vocab-filter` tool provides faster rule-based vocabulary checking without LLM calls.

## MCP Integration

AI agents (Claude, Cursor, etc.) can access brand voice capabilities via MCP. The `kapi mcp` server exposes brand voice checking as part of its processing toolkit:

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

This enables agents to:

- Run brand voice checks on files via `run_flow`
- Extract content and review against vocabulary rules
- Score content for brand compliance

Server deployments can extend this with a cloud MCP endpoint that provides HTTP-based access to brand voice profiles, vocabulary tools, scoring, and guided prompts — enabling AI agents to consume brand voice data without running a local CLI process.

## Terminology Integration

Brand vocabulary flows through the same pipeline as traditional terminology:

- **Preferred terms** appear in `term-lookup` results with high priority
- **Forbidden terms** trigger violations in `term-enforce`
- **Competitor terms** are flagged as critical-severity issues
- The `SourceFilter` on lookups distinguishes brand vocabulary from traditional terminology

This means brand governance and terminology management share the same infrastructure -- no parallel enforcement systems needed.
