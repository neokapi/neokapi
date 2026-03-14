---
id: 025-brand-voice-governance
sidebar_position: 25
title: "AD-025: Brand Voice Governance"
---
# AD-025: Brand voice governance

## Context

Localization teams need more than terminology management -- they need to enforce consistent brand voice across languages, channels, and markets. Existing tools like Acrolinx and Writer.com are proprietary, expensive, and don't integrate with streaming localization pipelines. neokapi's progressive terminology model ([AD-010](./010-terminology.md)) already provides the foundation for brand vocabulary; this extends it into full brand governance with voice profiles, MQM-inspired scoring, and AI-powered compliance checking.

Key requirements:
- Define brand voice as structured, machine-readable profiles (tone, style, vocabulary)
- Score content against brand guidelines using MQM-inspired penalty weighting
- Distribute profiles to AI agents via MCP ([AD-021](./021-mcp-integration.md))
- Support locale-specific and channel-specific overrides
- Integrate with the existing terminology system for vocabulary enforcement
- Provide starter packs for common brand archetypes

## Decision

### VoiceProfile Data Model

Brand voice is defined by a `VoiceProfile` struct in `core/brand/profile.go`:

```go
type VoiceProfile struct {
    ID          string
    Name        string
    Description string
    Tone        ToneProfile
    Style       StyleRules
    Vocabulary  VocabularyRules
    Examples    []VoiceExample
    Locales     map[string]LocaleOverride
    Channels    map[string]ChannelOverride
    WorkspaceID string
    Version     int
    CreatedAt   time.Time
    UpdatedAt   time.Time
    CreatedBy   string
}
```

**ToneProfile** describes the desired personality characteristics:
- `Personality` -- trait keywords (e.g., "friendly", "knowledgeable", "direct")
- `Formality` -- "casual", "neutral", "formal", "technical"
- `Emotion` -- "warm", "neutral", "authoritative"
- `Humor` -- "none", "light", "frequent"
- `Guidelines` -- free-text tone guidance

**StyleRules** defines writing constraints:
- `ActiveVoice` -- prefer active voice
- `SentenceLength` -- "short", "medium", "varied"
- `PersonPOV` -- "first_plural", "second", "third"
- `Contractions` -- "always", "sometimes", "never"
- `ProhibitedPatterns` / `RequiredPatterns` -- regex-based pattern rules with severity

**VocabularyRules** defines term constraints:
- `PreferredTerms` -- terms to use, with optional replacement and note
- `ForbiddenTerms` -- terms to avoid, with replacement suggestions
- `CompetitorTerms` -- competitor brand terms to never use
- `Abbreviations` -- approved abbreviation mappings

**VoiceExample** provides before/after pairs with explanations, categorized by dimension (tone, style, vocabulary).

### Locale and Channel Overrides

Profiles support locale-specific overrides (`LocaleOverride`) that adjust formality, humor, person POV, vocabulary, and examples for specific markets. Channel overrides (`ChannelOverride`) replace tone and style rules entirely for specific content channels (e.g., social media vs. documentation).

`ResolveProfile()` in `core/brand/resolve.go` applies these overrides to produce a resolved profile for a given locale + channel combination.

### MQM-Inspired Scoring

Brand compliance is scored using an MQM-inspired penalty model with five dimensions and four severity levels:

**Dimensions:**

| Dimension | What it measures |
|-----------|-----------------|
| `tone` | Voice personality, formality, emotion alignment |
| `style` | Writing rules (active voice, sentence length, POV) |
| `vocabulary` | Preferred/forbidden/competitor term usage |
| `clarity` | Readability and comprehension |
| `brand_compliance` | Overall brand alignment |

**Severity weights (MQM-inspired):**

| Severity | Weight | Example |
|----------|--------|---------|
| `neutral` | 0 | Informational note |
| `minor` | 1 | Slight tone inconsistency |
| `major` | 5 | Wrong term used |
| `critical` | 25 | Competitor term used |

**Scoring algorithm** (`CalculateScore` in `core/brand/scoring.go`):
1. Each `BrandVoiceFinding` contributes a penalty to its dimension
2. Dimension score = 100 - sum of penalties for that dimension (clamped to 0)
3. Overall score = 100 - total penalties across all dimensions (clamped to 0)

The `BrandComplianceScore` struct holds the overall score, per-dimension breakdown, all findings, word count, and profile ID.

### BrandStore Interface

Brand voice profiles and scores are persisted via the `BrandStore` interface in `core/brand/store.go`:

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

Three storage tiers:

1. **CLI SQLite** (`cli/storage/brand/`) -- persistent file-based storage for kapi and bowrain CLI. JSON columns for tone, style, vocabulary, examples, locales, channels.
2. **Server PostgreSQL** (`bowrain/brand/`) -- workspace-scoped storage for Bowrain Server.

Score storage tracks compliance over time per block, enabling trend analysis. The correction feedback loop (`StoreCorrection` / `GetSuggestedRules`) surfaces repeated user corrections as candidate vocabulary rules.

### Content Model Extension

`BrandVoiceAnnotation` in `core/brand/annotation.go` implements the `Annotation` interface, carrying profile ID, overall score, findings with `TextRange` positions, and severity levels. This integrates with the existing annotation system on Blocks ([AD-002](./002-content-model.md)).

### Terminology Integration

The terminology system ([AD-010](./010-terminology.md)) is extended with brand-specific fields:

- **`TermSource`** -- distinguishes `"terminology"` from `"brand_vocabulary"` concepts
- **`CompetitorTerm`** -- boolean flag on `Term` marking competitor brand terms
- **`SourceFilter`** on `LookupOptions` -- filter lookups by term source
- **`ConceptRelation`** -- enables graph import/export of concept relationships using `graph.Label*` constants

This means brand vocabulary flows through the same pipeline tools (`term-lookup`, `term-enforce`) as traditional terminology, while the `SourceFilter` allows brand-specific filtering.

### MCP Distribution

Brand voice profiles are distributed to AI agents via a cloud MCP server (`platform/server/mcp/`) using Streamable HTTP transport. See [AD-021](./021-mcp-integration.md) for the full MCP design.

**Resources** (read-only data):
- `brand://profiles/{id}` -- full voice profile
- `brand://profiles/{id}/vocabulary` -- vocabulary rules only
- `brand://profiles/{id}/examples` -- before/after pairs
- `brand://terminology/{workspace}` -- workspace termbase index

**Tools** (actions):
- `check_vocabulary` -- validate text against brand terms (returns findings + score)
- `list_profiles` -- list profiles in a workspace
- `get_voice_guide` -- formatted markdown guide for LLM consumption
- `score_brand_compliance` -- full scoring with per-dimension breakdown
- `suggest_corrections` -- generate rewrites for findings
- `rewrite_in_voice` -- apply vocabulary rules and return rewritten text

**Prompts** (LLM workflows):
- `write_in_voice` -- write new content in a brand voice
- `rewrite_in_voice` -- rewrite text to match brand voice
- `check_draft` -- review draft against guidelines

### Starter Packs

Five built-in starter packs are embedded via `go:embed` in `core/brand/packs/`:

| Pack | Formality | Personality | Use Case |
|------|-----------|-------------|----------|
| `professional-b2b` | formal | knowledgeable, authoritative | Enterprise software |
| `friendly-dtc` | casual | friendly, approachable | Consumer products |
| `technical-docs` | technical | precise, informative | Developer documentation |
| `marketing-blog` | neutral | engaging, creative | Content marketing |
| `customer-support` | neutral | empathetic, helpful | Support interactions |

Starter packs are YAML files loaded via `packs.Load(name)` and returned as `VoiceProfile` structs. They provide ready-to-use starting points that users can customize.

### Workspace-Scoped Tag Dimensions

`TagDimension` in `core/brand/workspace_tags.go` defines workspace-configurable dimensions (e.g., "market", "channel", "product") used with `graph.Validity` to scope edges by business criteria. Each dimension has a name, description, allowed values, and required flag.

## Alternatives Considered

- **Separate brand service**: Would add deployment complexity and break the single-binary model for CLI use. Integrating brand voice into the existing pipeline and storage infrastructure is more consistent.

- **Hard-coded scoring dimensions**: The five dimensions (tone, style, vocabulary, clarity, brand_compliance) are fixed because they align with industry-standard quality frameworks. Custom dimensions would add complexity without clear benefit.

- **Store profiles in files only**: Would miss the workspace-scoped sharing model needed for teams. The BrandStore interface supports both file-based (SQLite) and server-based (PostgreSQL) workflows.

- **Separate MCP server for brand voice**: Would require agents to connect to multiple servers. Extending the cloud MCP server with brand resources, tools, and prompts keeps the integration surface simple.

## Consequences

- Brand voice governance is integrated into the same pipeline as terminology, TM, and AI translation -- not a separate system
- MQM-inspired scoring provides quantitative brand compliance metrics with trend tracking over time
- The MCP distribution model lets AI agents (Claude, GPT, Cursor) consume brand voice profiles and check compliance without custom integration
- Locale and channel overrides enable a single profile to adapt across markets and content types
- Starter packs reduce time-to-value for new workspaces
- The correction feedback loop surfaces emerging vocabulary patterns from user behavior
- Brand vocabulary flows through existing terminology pipeline tools, avoiding parallel enforcement systems
