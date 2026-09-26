---
sidebar_position: 3
title: Voice profiles
description: "The voice profile is one checkset over neokapi's content-verification engine: a machine-readable profile of tone, style measures and pattern rules whose findings annotate Blocks like every other check."
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

# Score text: file argument, --input-text, or stdin. Exits 3 when a finding
# fails, so it gates CI. The default pass is rule-based and offline; --ai adds
# an LLM analysis of tone, style, and clarity, whose findings report.
kapi voice check --profile-file voice.yaml release-notes.md

# Rewrite off-voice content: deterministic substitution of the terms the
# profile's file carries, no model. A rule with no replacement is reported
# under "skipped" rather than applied.
kapi voice rewrite --profile-file voice.yaml --input-text "Leverage our solution"

# Ask a model for the inflected forms of each term, written as a diff
kapi terms expand --locale nb

# Manage profiles in the local store
kapi voice profiles
```

## What voice checks assess

The default word-rule and pattern checks assess encoded rules. `kapi check
--voice` adds advisory similarity to profile examples through the `kapi-check`
plugin; it does not request an LLM judgment. `kapi voice check --ai` and the raw
`voice-check` tool explicitly request semantic model review.

A clean rule result means no findings in those rules. It does not establish
that a service description, promise or factual claim is correct. The canonical
check report identifies analyses that ran, were not requested or cannot be
assessed by exact rules. Reports without execution metadata have unreported
coverage. Applicable prose guidance is recorded as unsupported by deterministic
checks; an encoded prohibited expression can produce a rule finding.

Requested analysis failures are operation errors. Missing profiles, unavailable
plugins, provider errors, timeout and cancellation cannot yield a new successful
score. The LLM tool validates structured findings locally: malformed JSON,
missing or null findings, invalid fields and invalid finding values are errors.
Only an explicit empty findings array is a completed clean model response.
Empty content skips model analysis and emits no score.

## Voice profiles

A profile captures tone, style measures and pattern rules. Word rules ("write
this, not that") are [terms](/framework/terminology), and a voice file may
carry them beside the voice under a top-level `terms:` list:

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

terms:
  - replacement: "workspace"
    note: "Use instead of 'account' or 'organization'"
  - term: "leverage"
    replacement: "use"
    advisory: true
  - term: "Slack"
    replacement: "messaging platform"
    competitor: true

examples:
  - before: "Users can leverage the platform to achieve synergy."
    after: "Your team can use the workspace to collaborate more effectively."
    explanation: "Active voice, preferred terms, removed jargon"
    category: style
```

Profiles support **locale overrides** (e.g. `formal` and third-person POV for
`ja`), **channel overrides** (e.g. casual, frequent humor for
`social_media`) and **persona overrides** (an individual author's voice).
Channel and persona overrides replace whole Tone/Style sections; locale
overrides merge individual fields. No override carries word rules.

Three rule fields do most of the work beyond the example above:

- A `terms:` entry is a **term rule**: `term`, `replacement`, `note`,
  `advisory`, `competitor` for a rival's name, and optionally `forms` (the
  inflections the exact matcher also recognises), `case_sensitive` and `scope`
  (`prose`, `code`, `heading`). A rule with only a `replacement` names a
  preferred form and rejects nothing. `kapi voice import` and
  `kapi context import` move a file's `terms:` into the project's terms store
  and report how many; a file that lists its words under `vocabulary:` is
  converted the same way. The same shape is what a flow step accepts under
  `term_rules:` for `term-check`, `translate`, `recycle`, `dnt-check` and
  `pseudo-translate`. A use of the term fails a check unless the rule is
  marked `advisory: true`, which makes it report. A rule whose `replacement`
  is capitalised, such as a product name (`term: QuickCast`, `replacement:
  Quickcast`), matches case-sensitively, so a lower-case URL slug `quickcast`
  is left alone; so does a rule whose `term` differs from its `replacement`
  only in case. Any other rule matches regardless of case, and
  `case_sensitive: true` or `false` sets it either way.
- A `style.prohibited_patterns` or `required_patterns` entry is a **pattern**:
  `regex`, `description`, `advisory`, and optionally a `rate` (`max` matches per
  `per_words`, default 1,000) that turns a ban into a ceiling, and a `scope`.
- `min_score` is the compliance bar a block must reach to count as compliant in
  roll-ups; unset, one failing finding already drops a block below it.

Every field is listed in the
[voice profile reference](/reference/serialization/voice-profile).

## Compliance scoring

Compliance is scored 0–100 across five dimensions: Tone, Style, Vocabulary
(the word-rule findings), Clarity, and overall voice compliance. Each finding reduces the score by a
weight set by what it does to a check:

| Finding                        | Weight | Example                        |
| ------------------------------ | ------ | ------------------------------ |
| Fails                          | 25     | Forbidden or competitor term   |
| Reports                        | 1      | Advisory rule, tone reading    |
| Raised by a suggested rule     | 0      | A proposed rule not yet confirmed |

The score is reported beside the findings. Whether a check fails depends only
on whether a finding fails.

## Starter packs

Built-in packs provide ready-to-use starting points (`professional-b2b`,
`friendly-dtc`, `technical-docs`, `marketing-blog`, and `customer-support`),
each with tone settings, style rules, before/after examples to customize, and
terms under `terms:`. Where a pack is bound as the voice, its terms apply beside
the project's own, and a finding they raise names the pack (`pack
technical-docs`).

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
`voice-vocab-check` tool checks the voice's patterns and the forbidden,
competitor and retired terms that govern the text without LLM calls. Word
rules are terms, so they also flow through the ordinary terminology tools:
`term-check` and `dnt-check` take them as `term_rules:`, and voice guardrails
and terminology share one enforcement path.
The `term-lookup` and `term-enforce` stages in `terms/tool.go` are library
code rather than recipe names: the runner appends them behind any tool whose
schema requires terms, and neither appears in `kapi tools`.

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
and rewrite off-voice copy with `voice_rewrite`, which lists under `skipped`
the terms it matched and could not replace. The guide itself is read
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
tone and style fields. Inside a project the voice tables live in the
project's own store; a standalone store is `voice.db`. The interface
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
// profile
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

// Rule-based: word rules and prohibited patterns, no LLM calls
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
`*profile.VoiceProfile` ready to use or customize, carrying the pack's terms
(`CarriedTerms`, from `pack <name>`).

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
