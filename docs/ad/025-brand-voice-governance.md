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

| Dimension          | What it measures                                   |
| ------------------ | -------------------------------------------------- |
| `tone`             | Voice personality, formality, emotion alignment    |
| `style`            | Writing rules (active voice, sentence length, POV) |
| `vocabulary`       | Preferred/forbidden/competitor term usage          |
| `clarity`          | Readability and comprehension                      |
| `brand_compliance` | Overall brand alignment                            |

**Severity weights (MQM-inspired):**

| Severity   | Weight | Example                   |
| ---------- | ------ | ------------------------- |
| `neutral`  | 0      | Informational note        |
| `minor`    | 1      | Slight tone inconsistency |
| `major`    | 5      | Wrong term used           |
| `critical` | 25     | Competitor term used      |

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

The framework provides:

1. **SQLite** (`cli/storage/brand/`) — persistent file-based storage for CLI tools. JSON columns for tone, style, vocabulary, examples, locales, channels.

The `BrandStore` interface supports server-side backends with workspace scoping and PostgreSQL storage.

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

Brand voice capabilities are exposed to AI agents via MCP ([AD-021](./021-mcp-integration.md)):

- **`kapi mcp`** (stdio) — brand voice checking via `run_flow` with the `brand-voice-check` and `brand-vocab-filter` tools. No server required.
- **Project CLI MCP** (stdio) — same capabilities within a project context.

Server deployments can extend this with a cloud MCP endpoint using Streamable HTTP transport, providing HTTP-based access to brand voice profiles, vocabulary tools, scoring, and guided prompts for AI agents.

### Starter Packs

Five built-in starter packs are embedded via `go:embed` in `core/brand/packs/`:

| Pack               | Formality | Personality                  | Use Case                |
| ------------------ | --------- | ---------------------------- | ----------------------- |
| `professional-b2b` | formal    | knowledgeable, authoritative | Enterprise software     |
| `friendly-dtc`     | casual    | friendly, approachable       | Consumer products       |
| `technical-docs`   | technical | precise, informative         | Developer documentation |
| `marketing-blog`   | neutral   | engaging, creative           | Content marketing       |
| `customer-support` | neutral   | empathetic, helpful          | Support interactions    |

Starter packs are YAML files loaded via `packs.Load(name)` and returned as `VoiceProfile` structs. They provide ready-to-use starting points that users can customize.

### Workspace-Scoped Tag Dimensions

`TagDimension` in `core/brand/workspace_tags.go` defines workspace-configurable dimensions (e.g., "market", "channel", "product") used with `graph.Validity` to scope edges by business criteria. Each dimension has a name, description, allowed values, and required flag.

### Hierarchical Profile Binding

Brand voice profiles are workspace-owned but can be **bound** at multiple levels of the content hierarchy. Each level stores a profile reference (ID), not a copy. The resolution chain applies the most specific binding:

```
Workspace (default profile)
  └─ Project (override via Properties map)
       └─ Stream (override via Properties map)
            └─ Collection (override via ConnectorConfig map)
```

**Resolution order** (most specific wins):

1. Explicit `ProfileID` parameter (always takes priority)
2. Collection-level: `ConnectorConfig["brand_voice_profile_id"]`
3. Stream-level: `Properties["brand_voice_profile_id"]`
4. Project-level: `Properties["brand_voice_profile_id"]`
5. Workspace-level: `Workspace.BrandVoiceProfileID`

Each binding also carries an optional **channel** key (e.g., `"email"`, `"social"`, `"docs"`) that maps to `VoiceProfile.Channels` for channel-specific tone and style overrides. Channel mapping is explicit only -- no convention-based name matching.

`ResolveProfileFromContext()` in `core/brand/resolve.go` performs the full inheritance chain lookup, channel resolution, and locale/channel override application in a single call. The `ResolveContext` struct carries all scope metadata (workspace, project, stream, collection properties) so resolution works identically in CLI and server contexts.

The `ProfileResolver` interface in `core/brand/binding.go` abstracts profile resolution for tools, enabling both explicit-profile and context-resolved construction:

```go
type ProfileResolver interface {
    ResolveProfile(ctx context.Context, rc ResolveContext) (*VoiceProfile, error)
}
```

**CLI integration**: The `.bowrain/config.yaml` supports a `brand_voice` section with project-level and per-collection bindings. Stream-level bindings are managed server-side via `Stream.Properties`.

### Profile Versioning and Tags

Profiles gain immutable version history and named tags, addressing two temporal dimensions:

- **Content streams** answer "what if we apply profile X to our content?" (forward experiments)
- **Profile versions** answer "what did profile X look like when we shipped v2.0?" (backward analysis)

Each `UpdateProfile()` call archives the previous state as an immutable `ProfileVersion` before applying the edit. The `Version` counter increments automatically.

`ProfileTag` is a lightweight named reference to a specific version (e.g., `"v1.0-launch"`, `"pre-rebrand"`). Tags enable temporal evaluation: "how does current content score under the old voice?"

`StoredScore` gains a `ProfileVersion` field linking each score to the exact profile version that produced it.

### Brand Voice Evaluation

The evaluation system enables comparing brand voice profiles against existing content without modifying anything. The `BrandVoiceEvaluation` struct in `core/brand/evaluate.go` captures:

- **Aggregate scores** (overall, min, max, median, distribution) for both experiment and baseline
- **Blast radius** -- affected blocks, improved vs degraded, new vs resolved violations, critical count, per-collection breakdown
- **Per-dimension comparison** (tone, style, vocabulary, clarity, brand compliance)
- **Top findings** with source/target text snippets for in-context review

The evaluation endpoint (`POST /projects/:id/brand-voice/evaluate`) accepts optional `profile_tag` and `baseline_profile_tag` parameters, enabling four evaluation modes:

1. Stream vs stream (current profiles)
2. Current vs historical (one tag)
3. Experimental vs historical (stream + tag)
4. Historical vs historical (both tags)

Scores from evaluation runs are stored as immutable records in `brand_voice_scores`, enabling trend tracking across iterations. `GetScoreTrends` filtered by stream shows experiment-specific history.

## Alternatives Considered

- **Separate brand service**: Would add deployment complexity and break the single-binary model for CLI use. Integrating brand voice into the existing pipeline and storage infrastructure is more consistent.

- **Hard-coded scoring dimensions**: The five dimensions (tone, style, vocabulary, clarity, brand_compliance) are fixed because they align with industry-standard quality frameworks. Custom dimensions would add complexity without clear benefit.

- **Store profiles in files only**: Would miss the workspace-scoped sharing model needed for teams. The BrandStore interface supports both file-based (SQLite) and server-based (PostgreSQL) workflows.

- **Separate MCP server for brand voice**: Would require agents to connect to multiple servers. Extending the cloud MCP server with brand resources, tools, and prompts keeps the integration surface simple.

- **Store bindings in a separate junction table**: Would add complexity for a simple ID reference. Using existing `Properties` and `ConnectorConfig` maps avoids schema changes for projects and collections while keeping bindings co-located with the entities they modify.

- **Convention-based channel mapping** (auto-match collection name to channel key): Too much implicit magic. Explicit `brand_voice_channel` keys are more predictable and debuggable.

- **Profile branching** (git-like branches for profiles): Overkill for the use case. Immutable version snapshots with named tags provide sufficient temporal coverage. Forward experiments use content stream bindings, not profile branches.

## Consequences

- Brand voice governance is integrated into the same pipeline as terminology, TM, and AI translation -- not a separate system
- MQM-inspired scoring provides quantitative brand compliance metrics with trend tracking over time
- The MCP distribution model lets AI agents (Claude, GPT, Cursor) consume brand voice profiles and check compliance without custom integration
- Locale and channel overrides enable a single profile to adapt across markets and content types
- Starter packs reduce time-to-value for new workspaces
- The correction feedback loop surfaces emerging vocabulary patterns from user behavior
- Brand vocabulary flows through existing terminology pipeline tools, avoiding parallel enforcement systems
- Hierarchical binding lets teams assign different voices to different projects, streams, and collections without duplicating profiles
- Profile versioning enables backward analysis and rebrand impact assessment
- Stream-based voice experiments provide safe, isolated evaluation of voice changes with blast radius visibility before merging
- The evaluation system makes brand voice changes measurable and reviewable, reducing the risk of unintended quality regressions
