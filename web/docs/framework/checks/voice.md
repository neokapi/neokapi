---
sidebar_position: 3
title: Voice profiles
description: "The voice profile is one checkset over neokapi's content-verification engine: a machine-readable profile of tone, style, and vocabulary whose findings annotate Blocks like every other check."
keywords: [voice profile, content checks, writing style, terminology, term rules, MCP, AI assistant]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# Voice profiles

Where [terminology](/framework/terminology) ensures you use the right words,
a voice profile describes how you say them: the personality, formality, and
writing patterns that make content recognizable. neokapi captures a voice
as a machine-readable profile and runs it as **one checkset over the same
[content-verification engine](/framework/checks)** that powers terminology,
do-not-translate, and placeholder integrity: every checker emits the same
findings into the same `Block` annotation, so voice is one check among
many. The Go library lives in `core/profile/`.

Used this way, a voice profile keeps an AI assistant on-voice the way a test keeps
code correct: load the profile into context (or expose it over
[MCP](/reference/mcp)) so generated copy is on-voice from the first draft, then
**check** anything that drifts and carry the same voice through every
translation. The findings (the specific terms and rules that broke) are the
substance; the 0–100 roll-up is a convenience, reliable only when calibrated
against a labeled set.

## Voice profiles with the CLI

The `kapi voice` command group works against a profile from a built-in starter
pack (`--pack`), the local voice store (`--profile`), or a standalone
git-shareable YAML file (`--profile-file`):

```bash
# Print the rendered guide (paste into an assistant, or pipe to a file)
kapi voice guide --pack friendly-dtc

# Score text: file argument, --input-text, or stdin. --min-score gates CI (exit 3).
# The default pass is rule-based and offline; --ai adds an LLM analysis of tone,
# style, and clarity.
kapi voice check --profile-file voice.yaml --min-score 80 release-notes.md

# Rewrite off-voice content: deterministic vocabulary substitution, no model
kapi voice rewrite --profile-file voice.yaml --input-text "Leverage our solution"

# Ask a model for the inflected forms of each vocabulary term, written as a diff
kapi voice expand --profile-file voice.yaml --language nb

# Manage profiles in the local store
kapi voice profiles
```

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
`ja`), **channel overrides** (e.g. casual, frequent humor for
`social_media`) and **persona overrides** (an individual author's voice, whose
vocabulary can only tighten the profile's). Channel and persona overrides
replace whole Tone/Style sections; locale overrides merge individual fields.

Three rule fields do most of the work beyond the example above:

- A vocabulary entry is a **term rule**: `term`, `replacement`, `severity`,
  and optionally `forms` (the inflections the exact matcher also recognises,
  which `kapi voice expand` fills in), `case_sensitive`, `scope` (`prose`,
  `code`, `heading`), `do_not_translate`, and a `concept_id` tying the rule to
  a concept in the terms store. The same shape is what a flow step accepts under
  `term_rules:` for `term-check`, `translate`, `recycle`, `dnt-check` and
  `pseudo-translate`, so a rule authored in the profile and a rule handed to a
  tool read identically.
- A `style.prohibited_patterns` or `required_patterns` entry is a **pattern**:
  `regex`, `description`, `severity`, and optionally a `rate` (`max` matches per
  `per_words`, default 1,000) that turns a ban into a ceiling, and a `scope`.
- `min_score` is the compliance bar a block must reach to count as compliant in
  roll-ups; unset, one critical vocabulary hit already drops a block below it.

Every field is listed in the
[voice profile reference](/reference/serialization/voice-profile).

## Compliance scoring

Compliance is scored 0–100 across five dimensions: Tone, Style, Vocabulary,
Clarity, and overall voice compliance. Each finding reduces the score by its
severity weight:

| Severity   | Weight | Example                   |
| ---------- | ------ | ------------------------- |
| `Neutral`  | 0      | Informational note        |
| `Minor`    | 1      | Slight tone inconsistency |
| `Major`    | 5      | Wrong term used           |
| `Critical` | 25     | Competitor term used      |

## Starter packs

Built-in packs provide ready-to-use starting points (`professional-b2b`,
`friendly-dtc`, `technical-docs`, `marketing-blog`, and `customer-support`),
each with tone settings, style rules, vocabulary constraints, and before/after
examples to customize.

## Pipeline integration

The `voice-check` tool runs in the pipeline alongside other tools:

<PipelineDiagram
  stages={[
    { label: "recycle", role: "translate" },
    { label: "term-check", role: "annotate" },
    { label: "translate", sub: "LLM", role: "translate" },
    { label: "voice-check", sub: "LLM", role: "qa" },
    { label: "qa", sub: "LLM", role: "qa" },
  ]}
/>

It uses an LLM to analyze content against the profile and attaches compliance
scores and findings to each Block as annotations. The faster, rule-based
`voice-vocab-check` tool checks forbidden and competitor terms without LLM
calls. Voice vocabulary also flows through the ordinary terminology tool:
`term-check` takes the profile's rules as `term_rules:`, so a forbidden or
competitor term fires there too, and voice guardrails and terminology share
one enforcement path.

## MCP integration

AI agents reach voice checking through the `kapi mcp` server:

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

Agents can score content for voice compliance with the `voice_check` MCP tool
and rewrite off-voice copy with `voice_rewrite`. The guide itself is read
rather than called: `kapi voice guide` prints it, and the `context://<path>`
resource returns it for the point a file sits at, with the terms bound there.
Server deployments can expose an HTTP MCP endpoint so agents consume profiles
and scoring without a local CLI process.

## Go library

### Store

```go
type Store interface {
    // Profile CRUD; scope is the storing host's partition key (empty for the local CLI)
    CreateProfile(ctx context.Context, profile *VoiceProfile) error
    GetProfile(ctx context.Context, id string) (*VoiceProfile, error)
    UpdateProfile(ctx context.Context, profile *VoiceProfile) error
    DeleteProfile(ctx context.Context, id string) error
    ListProfiles(ctx context.Context, scope string) ([]*VoiceProfile, error)

    // Version history and named tags
    ListProfileVersions(ctx context.Context, profileID string) ([]*ProfileVersion, error)
    GetProfileVersion(ctx context.Context, profileID string, version int) (*ProfileVersion, error)
    GetProfileAtTag(ctx context.Context, profileID, tagName string) (*VoiceProfile, error)
    CreateProfileTag(ctx context.Context, tag *ProfileTag) error
    ListProfileTags(ctx context.Context, profileID string) ([]*ProfileTag, error)
    DeleteProfileTag(ctx context.Context, profileID, tagName string) error

    // Scores
    StoreScore(ctx context.Context, score *StoredScore) error
    GetScores(ctx context.Context, projectID string, locale model.LocaleID) ([]*StoredScore, error)
    GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)
    GetScoresByStream(ctx context.Context, projectID, stream string) ([]*StoredScore, error)

    // Corrections and the candidate rules derived from them
    StoreCorrection(ctx context.Context, correction *Correction) error
    GetSuggestedRules(ctx context.Context, scope string, minCount int) ([]*SuggestedRule, error)
    RecordRuleDecision(ctx context.Context, d *RuleDecision) error
    GetRuleDecision(ctx context.Context, profileID, term string) (*RuleDecision, error)
    ListRuleDecisions(ctx context.Context, profileID string) ([]*RuleDecision, error)

    Close() error
}
```

`StoredScore`, `ScoreTrend`, and the other unqualified types are declared in the
`profile` package; `model.LocaleID` is the BCP-47 locale type from
`github.com/neokapi/neokapi/core/model`.

The framework ships a SQLite backend (`voice/sqlite.go`) built on
the shared `core/storage` migration system, with JSON columns for the complex
tone/style/vocabulary fields. Inside a project the voice tables live in the
shared `.kapi/work/store.db`; a standalone store is `voice.db`. The interface
is designed for extension: server deployments can add a scope-partitioned
PostgreSQL backend.

### Scoring and resolution

```go
import "github.com/neokapi/neokapi/core/profile"

findings := []profile.VoiceFinding{
    {Dimension: profile.DimensionVocabulary, Severity: profile.SeverityMajor,
        Message: "Forbidden term: leverage", Suggestion: "use"},
    {Dimension: profile.DimensionTone, Severity: profile.SeverityMinor,
        Message: "Tone is too formal for this profile"},
}
score := profile.CalculateScore(findings) // score.Overall = 94 (100 - 5 - 1)

// ResolveProfile layers locale, then channel, then persona overrides on a base
// profile; a persona's vocabulary can only tighten the profile's
resolved := profile.ResolveProfile(base, "ja", "", "")
```

### Pipeline tools

```go
import (
    aitool "github.com/neokapi/neokapi/core/ai/tools"
    "github.com/neokapi/neokapi/core/profile"
    "github.com/neokapi/neokapi/core/tools"
)

// LLM-based: structured findings scored via CalculateScore, attached as a
// VoiceAnnotation plus voice-score / voice-findings properties
checkTool := aitool.NewVoiceCheckTool(llmProvider, profile)

// Rule-based: fast forbidden/competitor-term enforcement, no LLM calls
vocabTool := tools.NewVoiceVocabCheckTool(profile, terminology)
```

### Starter packs

```go
import "github.com/neokapi/neokapi/core/profile/packs"

names, _ := packs.List()          // the five built-in pack names
profile, _ := packs.Load("professional-b2b")
all, _ := packs.LoadAll()
```

Packs are YAML files embedded via `go:embed`; each returns a
`*profile.VoiceProfile` ready to use or customize.

### Content model integration

`VoiceAnnotation` is a registered payload (`voice`) stored as a
block-scoped **annotation** ([F-02](/contribute/architecture/foundations/f-02-content-model)),
the counterpart to positional overlays like `term` and `entity`. It is reached
through the block's `Anno`/`SetAnno` helpers and registered for wire/store
rehydration via `model.RegisterPayload`:

```go
type VoiceAnnotation struct {
    ProfileID string              `json:"profile_id"`
    Score     int                 `json:"score"` // 0-100 overall
    Findings  []VoiceFinding `json:"findings"`
    Position  model.Anchor        `json:"position"`
}

func (a *VoiceAnnotation) AnnotationType() string { return "voice" }
```

Profiles serialize as both JSON and YAML, so they can be authored by hand or
constructed programmatically as a `*profile.VoiceProfile`.
