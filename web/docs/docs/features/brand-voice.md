---
title: Brand Voice
---

# Brand Voice

Where [terminology](/features/terminology) ensures you use the right words,
brand voice governance ensures you say them the right way — the personality,
formality, and writing patterns that make content recognizable. neokapi
captures a brand voice as a machine-readable profile, scores content against it
(0–100), and enforces it through the same pipeline and `Block` annotation system
used for terminology. The Go library lives in `core/brand/`.

This is how neokapi keeps your AI coding assistant on-brand: load the profile
into context (or expose it over [MCP](/kapi-cli/mcp)) so generated copy, docs,
and UI strings are on-voice from the first draft — then score and rewrite
anything that drifts, and carry the same voice through every translation.

## Brand voice with the CLI

The `kapi brand` command group works against a profile from a built-in starter
pack (`--pack`), the local brand store (`--profile`), or a standalone
git-shareable YAML file (`--profile-file`):

```bash
# Print the rendered guide (paste into an assistant, or pipe to a file)
kapi brand guide --pack friendly-dtc

# Score text: file argument, --text, or stdin. --min-score gates CI (exit 3).
kapi brand check --profile-file brand.yaml --min-score 80 release-notes.md

# Rewrite off-voice content (add --ai for tone/style as well as vocabulary)
kapi brand rewrite --profile-file brand.yaml --text "Leverage our solution"

# Manage profiles in the local store
kapi brand profiles
```

Both `check` and `rewrite` run a fast, offline rule-based vocabulary pass by
default; pass `--ai` to add an LLM analysis of tone, style, and clarity.

## Voice profiles

A profile captures tone, style, and vocabulary as rules:

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
  forbidden_terms:
    - term: "leverage"
      replacement: "use"
      severity: minor
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

Profiles support **locale overrides** (e.g. `formal` and third-person POV for
`ja`) and **channel overrides** (e.g. casual, frequent humor for
`social_media`). Channel overrides replace whole Tone/Style sections; locale
overrides merge individual fields.

## Compliance scoring

Compliance is scored 0–100 across five dimensions — Tone, Style, Vocabulary,
Clarity, and overall Brand compliance. Each finding reduces the score by its
severity weight:

| Severity   | Weight | Example                   |
| ---------- | ------ | ------------------------- |
| `Neutral`  | 0      | Informational note        |
| `Minor`    | 1      | Slight tone inconsistency |
| `Major`    | 5      | Wrong term used           |
| `Critical` | 25     | Competitor term used      |

## Starter packs

Built-in packs provide ready-to-use starting points — `professional-b2b`,
`friendly-dtc`, `technical-docs`, `marketing-blog`, and `customer-support` —
each with tone settings, style rules, vocabulary constraints, and before/after
examples to customize.

## Pipeline integration

The `brand-voice-check` tool runs in the pipeline alongside other tools:

```
Reader → TM Leverage → Term Lookup → AI Translate → Brand Voice Check → AI QA → Writer
```

It uses an LLM to analyze content against the profile and attaches compliance
scores and findings to each Block as annotations. The faster, rule-based
`brand-vocab-filter` tool checks forbidden and competitor terms without LLM
calls. Brand vocabulary also flows through ordinary terminology tools —
preferred terms surface in `term-lookup`, forbidden/competitor terms trigger
`term-enforce` violations — so brand governance and terminology share one
enforcement path.

## MCP integration

AI agents reach brand voice checking through the `kapi mcp` server:

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

Agents can run brand voice checks via `run_flow`, extract content and review it
against vocabulary rules, and score content for compliance. Server deployments
can expose an HTTP MCP endpoint so agents consume profiles and scoring without a
local CLI process.

## Go library

### BrandStore

```go
type BrandStore interface {
    CreateProfile(ctx context.Context, profile *VoiceProfile) error
    GetProfile(ctx context.Context, id string) (*VoiceProfile, error)
    UpdateProfile(ctx context.Context, profile *VoiceProfile) error
    DeleteProfile(ctx context.Context, id string) error
    ListProfiles(ctx context.Context, workspaceID string) ([]*VoiceProfile, error)

    StoreScore(ctx context.Context, score *StoredScore) error
    GetScores(ctx context.Context, projectID, locale string) ([]*StoredScore, error)
    GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)

    StoreCorrection(ctx context.Context, correction *Correction) error
    GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*SuggestedRule, error)

    Close() error
}
```

The framework ships a SQLite backend (`cli/storage/brand/sqlite.go`) built on
the shared `core/storage` migration system, with JSON columns for the complex
tone/style/vocabulary fields. The interface is designed for extension — server
deployments can add a workspace-scoped PostgreSQL backend.

### Scoring and resolution

```go
import "github.com/neokapi/neokapi/core/brand"

findings := []brand.BrandVoiceFinding{
    {Dimension: brand.DimensionVocabulary, Severity: brand.SeverityMajor,
        Message: "Forbidden term: leverage", Suggestion: "use"},
    {Dimension: brand.DimensionTone, Severity: brand.SeverityMinor,
        Message: "Tone is too formal for this profile"},
}
score := brand.CalculateScore(findings) // score.Overall = 94 (100 - 5 - 1)

// ResolveProfile applies locale then channel overrides to a base profile
resolved := brand.ResolveProfile(base, "ja", "")
```

### Pipeline tools

```go
import (
    aitool "github.com/neokapi/neokapi/core/ai/tools"
    "github.com/neokapi/neokapi/core/brand"
    "github.com/neokapi/neokapi/core/tools"
)

// LLM-based: structured findings scored via CalculateScore, attached as a
// BrandVoiceAnnotation plus brand-voice-score / brand-voice-findings properties
checkTool := aitool.NewBrandVoiceCheckTool(llmProvider, profile)

// Rule-based: fast forbidden/competitor-term enforcement, no LLM calls
vocabTool := tools.NewBrandVocabFilterTool(profile)
```

### Starter packs

```go
import "github.com/neokapi/neokapi/core/brand/packs"

names, _ := packs.List()          // the five built-in pack names
profile, _ := packs.Load("professional-b2b")
all, _ := packs.LoadAll()
```

Packs are YAML files embedded via `go:embed`; each returns a
`*brand.VoiceProfile` ready to use or customize.

### Content model integration

`BrandVoiceAnnotation` implements `model.Annotation`, so it integrates with the
Block annotation system alongside `TermAnnotation` and `EntityAnnotation` and
can drive inline highlighting in editors:

```go
type BrandVoiceAnnotation struct {
    ProfileID string              `json:"profile_id"`
    Score     int                 `json:"score"` // 0-100 overall
    Findings  []BrandVoiceFinding `json:"findings"`
    Position  model.TextRange     `json:"position"`
}

func (a *BrandVoiceAnnotation) AnnotationType() string { return "brand-voice" }
```

Profiles serialize as both JSON and YAML, so they can be authored by hand or
constructed programmatically as a `*brand.VoiceProfile`.
