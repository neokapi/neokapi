---
sidebar_position: 17
title: "Brand Voice Data Model"
---

# Brand Voice Data Model

This note provides implementation details for [AD-015](/architecture-decisions/015-server-ai-operations).

## Go Struct Definitions

### VoiceProfile

```go
// core/brand/profile.go
type VoiceProfile struct {
    ID          string                    `json:"id" yaml:"id,omitempty"`
    Name        string                    `json:"name" yaml:"name"`
    Description string                    `json:"description,omitempty" yaml:"description,omitempty"`
    Tone        ToneProfile               `json:"tone" yaml:"tone"`
    Style       StyleRules                `json:"style" yaml:"style"`
    Vocabulary  VocabularyRules           `json:"vocabulary" yaml:"vocabulary"`
    Examples    []VoiceExample            `json:"examples" yaml:"examples"`
    Locales     map[string]LocaleOverride `json:"locales,omitempty" yaml:"locales,omitempty"`
    Channels    map[string]ChannelOverride `json:"channels,omitempty" yaml:"channels,omitempty"`
    WorkspaceID string                    `json:"workspace_id" yaml:"workspace_id,omitempty"`
    Version     int                       `json:"version" yaml:"version,omitempty"`
    CreatedAt   time.Time                 `json:"created_at" yaml:"created_at,omitempty"`
    UpdatedAt   time.Time                 `json:"updated_at" yaml:"updated_at,omitempty"`
    CreatedBy   string                    `json:"created_by,omitempty" yaml:"created_by,omitempty"`
}

type ToneProfile struct {
    Personality []string `json:"personality" yaml:"personality"`
    Formality   string   `json:"formality" yaml:"formality"`
    Emotion     string   `json:"emotion" yaml:"emotion"`
    Humor       string   `json:"humor" yaml:"humor"`
    Guidelines  string   `json:"guidelines,omitempty" yaml:"guidelines,omitempty"`
}

type StyleRules struct {
    ActiveVoice        bool      `json:"active_voice" yaml:"active_voice"`
    SentenceLength     string    `json:"sentence_length" yaml:"sentence_length"`
    PersonPOV          string    `json:"person_pov" yaml:"person_pov"`
    Contractions       string    `json:"contractions" yaml:"contractions"`
    ProhibitedPatterns []Pattern `json:"prohibited_patterns,omitempty" yaml:"prohibited_patterns,omitempty"`
    RequiredPatterns   []Pattern `json:"required_patterns,omitempty" yaml:"required_patterns,omitempty"`
}

type VocabularyRules struct {
    PreferredTerms  []TermRule        `json:"preferred_terms,omitempty" yaml:"preferred_terms,omitempty"`
    ForbiddenTerms  []TermRule        `json:"forbidden_terms,omitempty" yaml:"forbidden_terms,omitempty"`
    CompetitorTerms []TermRule        `json:"competitor_terms,omitempty" yaml:"competitor_terms,omitempty"`
    Abbreviations   map[string]string `json:"abbreviations,omitempty" yaml:"abbreviations,omitempty"`
}
```

### Scoring Types

```go
// core/brand/scoring.go
type Dimension string
const (
    DimensionTone       Dimension = "tone"
    DimensionStyle      Dimension = "style"
    DimensionVocabulary Dimension = "vocabulary"
    DimensionClarity    Dimension = "clarity"
    DimensionBrand      Dimension = "brand_compliance"
)

type Severity string
const (
    SeverityNeutral  Severity = "neutral"   // weight: 0
    SeverityMinor    Severity = "minor"     // weight: 1
    SeverityMajor    Severity = "major"     // weight: 5
    SeverityCritical Severity = "critical"  // weight: 25
)

type BrandVoiceFinding struct {
    Dimension    Dimension       `json:"dimension"`
    Severity     Severity        `json:"severity"`
    Message      string          `json:"message"`
    Suggestion   string          `json:"suggestion,omitempty"`
    Position     model.TextRange `json:"position"`
    OriginalText string          `json:"original_text,omitempty"`
}

type BrandComplianceScore struct {
    Overall    int                 `json:"overall"` // 0-100
    Dimensions []DimensionScore   `json:"dimensions"`
    Findings   []BrandVoiceFinding `json:"findings"`
    WordCount  int                 `json:"word_count"`
    ProfileID  string              `json:"profile_id"`
}
```

### BrandStore Interface

```go
// core/brand/store.go
type BrandStore interface {
    CreateProfile(ctx context.Context, profile *VoiceProfile) error
    GetProfile(ctx context.Context, id string) (*VoiceProfile, error)
    UpdateProfile(ctx context.Context, profile *VoiceProfile) error
    DeleteProfile(ctx context.Context, id string) error
    ListProfiles(ctx context.Context, workspaceID string) ([]*VoiceProfile, error)

    StoreScore(ctx context.Context, score *StoredScore) error
    GetScores(ctx context.Context, projectID string, locale model.LocaleID) ([]*StoredScore, error)
    GetScoreTrends(ctx context.Context, projectID string, days int) ([]*ScoreTrend, error)

    StoreCorrection(ctx context.Context, correction *Correction) error
    GetSuggestedRules(ctx context.Context, workspaceID string, minCount int) ([]*SuggestedRule, error)

    Close() error
}
```

## Framework: SQLite Schema

```sql
-- cli/storage/brand/sqlite.go
CREATE TABLE IF NOT EXISTS brand_profiles (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    tone TEXT NOT NULL DEFAULT '{}',
    style TEXT NOT NULL DEFAULT '{}',
    vocabulary TEXT NOT NULL DEFAULT '{}',
    examples TEXT NOT NULL DEFAULT '[]',
    locales TEXT NOT NULL DEFAULT '{}',
    channels TEXT NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    UNIQUE (workspace_id, name)
);

CREATE TABLE IF NOT EXISTS brand_voice_scores (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    stream TEXT NOT NULL DEFAULT 'main',
    block_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    locale TEXT NOT NULL,
    score INTEGER NOT NULL,
    dimensions TEXT NOT NULL,     -- JSON array of DimensionScore
    findings TEXT NOT NULL,       -- JSON array of BrandVoiceFinding
    checked_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS brand_voice_corrections (
    id TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL,
    block_id TEXT NOT NULL,
    dimension TEXT NOT NULL,
    original_text TEXT NOT NULL,
    corrected_text TEXT NOT NULL,
    finding_id TEXT,
    corrected_by TEXT NOT NULL,
    corrected_at TEXT NOT NULL
);
```

## Server: PostgreSQL Schema

The server PostgreSQL schema mirrors the SQLite schema with PostgreSQL-specific types:

- `TEXT` columns for JSON data (tone, style, vocabulary, examples, locales, channels, dimensions, findings)
- `TIMESTAMP WITH TIME ZONE` for temporal columns
- Workspace isolation via `workspace_id` column with foreign key to workspaces table
- GIN indexes on JSON columns for efficient queries

## Scoring Algorithm

The `CalculateScore` function in `core/brand/scoring.go`:

1. Iterate all findings, accumulating penalties per dimension using `SeverityWeight()`
2. For each of the five dimensions, compute: `score = max(0, 100 - penalty)`
3. Compute overall: `overall = max(0, 100 - totalPenalty)`
4. Return `BrandComplianceScore` with overall, dimensions, findings, word count, profile ID

Severity weights follow MQM conventions:

- neutral=0 (informational)
- minor=1 (slight issue)
- major=5 (clear violation)
- critical=25 (severe violation, e.g., competitor term)

## Server: MCP Protocol Details

The server cloud MCP endpoint (`platform/server/mcp/`) uses Streamable HTTP transport:

```go
// server.go
ms.handler = mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return s },
    nil,
)
// Mounted at /mcp/* on the Echo server
e.Any("/mcp/*", echo.WrapHandler(http.StripPrefix("/mcp", s.handler)))
```

Tools use typed input/output structs with `jsonschema` tags for automatic schema generation. The `checkVocab()` function performs rule-based vocabulary checking (forbidden + competitor terms) as a fast path before LLM-based analysis.

## Starter Packs

Five YAML-based starter packs are embedded via `go:embed` in `core/brand/packs/`:

- `professional-b2b.yaml` -- formal, authoritative enterprise voice
- `friendly-dtc.yaml` -- casual, approachable consumer voice
- `technical-docs.yaml` -- precise, informative documentation voice
- `marketing-blog.yaml` -- engaging, creative content marketing voice
- `customer-support.yaml` -- empathetic, helpful support voice

Loaded via `packs.Load("professional-b2b")` which returns a `*brand.VoiceProfile`.

## Hierarchical Profile Binding

### Well-Known Property Keys

Profile bindings use well-known keys stored on existing entity maps:

| Entity     | Map               | Key                      | Purpose                         |
| ---------- | ----------------- | ------------------------ | ------------------------------- |
| Workspace  | (native field)    | `BrandVoiceProfileID`    | Default profile for workspace   |
| Project    | `Properties`      | `brand_voice_profile_id` | Override profile for project    |
| Project    | `Properties`      | `brand_voice_channel`    | Default channel override        |
| Stream     | `Properties`      | `brand_voice_profile_id` | Override profile for stream     |
| Stream     | `Properties`      | `brand_voice_channel`    | Channel override for stream     |
| Collection | `ConnectorConfig` | `brand_voice_profile_id` | Override profile for collection |
| Collection | `ConnectorConfig` | `brand_voice_channel`    | Channel override for collection |

### Resolution Algorithm

`ResolveProfileFromContext()` in `core/brand/resolve.go`:

```
1. If ExplicitProfileID set → fetch that profile
2. Check CollectionConfig["brand_voice_profile_id"]
3. Check StreamProperties["brand_voice_profile_id"]
4. Check ProjectProperties["brand_voice_profile_id"]
5. Check WorkspaceProfileID
6. None found → return nil (no enforcement)

Channel resolution (most specific wins):
  CollectionConfig > StreamProperties > ProjectProperties

Apply: ResolveProfile(profile, locale, channel)
```

### CLI Config (top-level on `<dir-name>.kapi`)

```yaml
brand_voice:
  profile: "prof_abc123"
  channel: "docs"
  collections:
    marketing-emails:
      profile: "prof_abc123"
      channel: "email"
    help-center:
      channel: "support"
```

## Profile Versioning Schema

```sql
-- New tables for profile versioning
CREATE TABLE IF NOT EXISTS brand_profile_versions (
    profile_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    snapshot TEXT NOT NULL,       -- JSON-encoded VoiceProfile
    note TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (profile_id, version)
);

CREATE TABLE IF NOT EXISTS brand_profile_tags (
    profile_id TEXT NOT NULL,
    name TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (profile_id, name)
);
```

`StoredScore` gains `profile_version INTEGER NOT NULL DEFAULT 0` for linking scores to profile snapshots.

## Evaluation Types

```go
// core/brand/evaluate.go
type BrandVoiceEvaluation struct {
    Stream            string                `json:"stream"`
    BaselineStream    string                `json:"baseline_stream"`
    StreamProfile     string                `json:"stream_profile"`
    BaselineProfile   string                `json:"baseline_profile"`
    BlocksEvaluated   int                   `json:"blocks_evaluated"`
    StreamScore       AggregateScore        `json:"stream_score"`
    BaselineScore     AggregateScore        `json:"baseline_score"`
    ScoreDelta        int                   `json:"score_delta"`
    BlastRadius       BlastRadius           `json:"blast_radius"`
    DimensionComparison []DimensionComparison `json:"dimension_comparison"`
    TopFindings       []EvaluationFinding   `json:"top_findings"`
}

type AggregateScore struct {
    Overall      float64        `json:"overall"`
    Min          int            `json:"min"`
    Max          int            `json:"max"`
    Median       int            `json:"median"`
    Distribution map[string]int `json:"distribution"`
}

type BlastRadius struct {
    TotalBlocks        int                     `json:"total_blocks"`
    AffectedBlocks     int                     `json:"affected_blocks"`
    ImprovedBlocks     int                     `json:"improved_blocks"`
    DegradedBlocks     int                     `json:"degraded_blocks"`
    NewViolations      int                     `json:"new_violations"`
    ResolvedViolations int                     `json:"resolved_violations"`
    CriticalCount      int                     `json:"critical_count"`
    Collections        []CollectionBlastRadius `json:"collections"`
}
```

## Implementation Files

### Framework (`core/`, `cli/`)

| File                           | Purpose                                                                |
| ------------------------------ | ---------------------------------------------------------------------- |
| `core/brand/profile.go`        | VoiceProfile data model with tone, style, vocabulary                   |
| `core/brand/scoring.go`        | Dimensions, severities, scoring algorithm                              |
| `core/brand/store.go`          | BrandStore interface + StoredScore, Correction types                   |
| `core/brand/annotation.go`     | BrandVoiceAnnotation (implements model.Annotation)                     |
| `core/brand/resolve.go`        | ResolveProfile + ResolveProfileFromContext for hierarchical resolution |
| `core/brand/binding.go`        | BrandVoiceBinding, ResolveContext, ProfileResolver interface           |
| `core/brand/evaluate.go`       | BrandVoiceEvaluation, BlastRadius, AggregateScore types                |
| `core/brand/workspace_tags.go` | TagDimension for workspace-configurable scoping                        |
| `core/brand/packs/*.yaml`      | Starter pack YAML definitions                                          |
| `core/brand/packs/embed.go`    | Embedded starter pack loader                                           |
| `core/ai/tools/brandvoice.go`  | LLM-based brand-voice-check pipeline tool                              |
| `core/tools/brandvocab.go`     | Rule-based brand-vocab-filter pipeline tool                            |
| `cli/storage/brand/sqlite.go`  | SQLite BrandStore implementation                                       |

### Server (`platform/`, `bowrain/`)

| File                                   | Purpose                        |
| -------------------------------------- | ------------------------------ |
| `bowrain/brand/`                       | PostgreSQL BrandStore (server) |
| `platform/server/mcp/server.go`        | Cloud MCP server bootstrap     |
| `platform/server/mcp/resources.go`     | MCP resource handlers          |
| `platform/server/mcp/tools.go`         | MCP tool handlers (Phase 1)    |
| `platform/server/mcp/tools_scoring.go` | MCP tool handlers (Phase 2)    |
| `platform/server/mcp/prompts.go`       | MCP prompt templates           |
