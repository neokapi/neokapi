# Bowrain: AI-Native Brand Voice Platform — Implementation Plan

This plan describes how we evolve neokapi (framework) and Bowrain (platform) from a localization engine into a first-class AI-native brand voice tool for both monolingual and multilingual content creation.

---

## Strategic positioning

Bowrain becomes the **portable brand voice infrastructure layer** for AI-native content creation. Configure brand voice rules once in Bowrain, enforce them everywhere — across Claude, ChatGPT, Copilot, Cursor, and any MCP-capable AI tool. The architecture serves both monolingual content teams (marketing, docs, support) and multilingual localization workflows.

**Core thesis:** The market splits into generators (Writer, Jasper) and governors (Acrolinx, Grammarly). No tool delivers a composable Guide → Write → QA closed loop that works across AI assistants. Bowrain occupies this gap by being model-agnostic infrastructure rather than another walled garden.

**Three defensible moats:**
1. **Portable brand voice** — configure once, deploy everywhere via MCP
2. **Terminology as wedge** — concrete, measurable, acknowledged as unsolved by Nimdzi
3. **Learning feedback loop** — QA corrections improve future generation guidance, which no competitor does

---

## Current state and what we build on

The existing architecture provides strong foundations that we extend rather than replace:

| Capability | Current state | Brand voice role |
|---|---|---|
| **Terminology** (AD-010) | Phase 1 shipped (concept-oriented termbase, tiered lookup, 6 pipeline tools). Phase 2 (concept relations, streams) planned. Phase 3 (brand voice) outlined. | Preferred/forbidden term lists become brand vocabulary enforcement. Extend to cover monolingual brand terminology. |
| **AI tools** (AD-008) | 5 tools shipped (ai-translate, ai-qa, ai-review, ai-terminology, ai-entity-extract). `brand-voice-check` listed as planned. | Brand voice check becomes the sixth AI tool. AI translate gains voice-aware prompting. |
| **MCP integration** (AD-021) | Two stdio servers shipped (kapi mcp, bowrain mcp) with file processing and project sync tools. | Add a third MCP server: `bowrain-server mcp` — cloud-hosted, OAuth 2.1, brand voice resources + tools. This is the primary distribution channel. |
| **Automation** (AD-011) | Event bus, automation rules, quality gates, webhooks — all shipped. | Brand compliance becomes a quality gate. Voice drift triggers automation rules. |
| **Content model** (AD-002) | Block annotations (TermAnnotation, EntityAnnotation) with character-level ranges. Block properties for QA issues. | Add BrandVoiceAnnotation for compliance findings. Block properties for voice scores. |
| **QA system** | Rule-based checks (whitespace, empty targets, span constraints). | Extend with ML-based brand voice dimensions alongside existing rule-based checks. |
| **Streams** (AD-024) | Git-like content branching with copy-on-write and TM/term scoping. | Brand voice profiles scoped to streams enable A/B testing of voice changes. |
| **Plugin system** (AD-007) | gRPC plugin architecture with format, tool, connector, provider types. | Brand voice providers (custom scoring backends) register as plugins. |

---

## Architecture overview

```
                         Brand Voice Profile
                    (tone, style, vocabulary, examples)
                                 |
               +-----------------+-----------------+
               |                 |                 |
          MCP Server        Pipeline Tools     REST API
     (AI assistant layer)  (processing layer) (platform layer)
               |                 |                 |
    Claude / ChatGPT /    Flow: Guide → Write    Web UI /
    Cursor / VS Code      → QA → Feedback        Desktop
```

**Three access layers, one profile:**
1. **MCP Server** — AI assistants access brand voice as resources and tools
2. **Pipeline Tools** — neokapi processing flows enforce brand voice during batch operations
3. **REST API + UI** — Teams manage profiles, review compliance, analyze drift

---

## Phase 1: Brand Voice Profile and Terminology Wedge

**Goal:** Ship the brand voice data model, profile management UI, and terminology enforcement via MCP. This is the "30-second aha moment" — connect Bowrain MCP → paste text → see brand vocabulary violations highlighted instantly.

### 1.1 Brand Voice Profile data model

Add to the framework content model (`core/`):

```go
// core/brand/profile.go
type VoiceProfile struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`          // "Bowrain Marketing", "Technical Docs"
    Description string            `json:"description"`
    Tone        ToneProfile       `json:"tone"`          // personality, formality, emotion
    Style       StyleRules        `json:"style"`         // sentence structure, voice, prohibited patterns
    Vocabulary  VocabularyRules   `json:"vocabulary"`    // preferred/forbidden terms, competitor terms
    Examples    []VoiceExample    `json:"examples"`      // before/after pairs demonstrating the voice
    Locales     map[string]LocaleOverride `json:"locales"` // per-locale voice adaptations
    Channels    map[string]ChannelOverride `json:"channels"` // blog, social, support, docs
    Version     int               `json:"version"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

type ToneProfile struct {
    Personality []string          `json:"personality"`   // ["friendly", "knowledgeable", "direct"]
    Formality   string            `json:"formality"`     // "casual", "neutral", "formal", "technical"
    Emotion     string            `json:"emotion"`       // "warm", "neutral", "authoritative"
    Humor       string            `json:"humor"`         // "none", "light", "frequent"
    Guidelines  string            `json:"guidelines"`    // free-text voice guidelines
}

type StyleRules struct {
    ActiveVoice     bool            `json:"active_voice"`     // prefer active over passive
    SentenceLength  string          `json:"sentence_length"`  // "short", "medium", "varied"
    PersonPOV       string          `json:"person_pov"`       // "first_plural", "second", "third"
    Contractions    string          `json:"contractions"`     // "always", "sometimes", "never"
    ProhibitedPatterns []Pattern    `json:"prohibited_patterns"` // regex patterns to flag
    RequiredPatterns   []Pattern    `json:"required_patterns"`
}

type VocabularyRules struct {
    PreferredTerms  []TermRule      `json:"preferred_terms"`   // use "start" not "commence"
    ForbiddenTerms  []TermRule      `json:"forbidden_terms"`   // never say "leverage", "synergy"
    CompetitorTerms []TermRule      `json:"competitor_terms"`  // flag competitor brand names
    Abbreviations   map[string]string `json:"abbreviations"`   // when to use/expand abbreviations
}
```

**Module placement:** `core/brand/` in the framework module — brand voice is a content concern, not a platform concern. The profile is a pure data type with JSON serialization, no database dependency. Storage backends (SQLite/PostgreSQL) live in `cli/storage/brand/` and `bowrain/brand/`.

**Why framework, not platform:** Kapi needs brand voice checking for standalone file processing (monolingual use case). A marketing writer using `kapi flow run brand-check -i draft.md --profile marketing.yaml` should work without a Bowrain server.

### 1.2 Terminology enforcement as brand vocabulary

Extend the existing terminology system (AD-010) to serve double duty:

- **Existing:** TermBase stores concepts with preferred/approved/deprecated terms across languages. `term-lookup` annotates blocks. `term-enforce` validates terminology consistency.
- **Extension:** Add `term_source` field to Concept: `"terminology"` (translation glossary) or `"brand_vocabulary"` (brand voice). Add `competitor_term` boolean to Term. Brand vocabulary terms participate in the same tiered lookup pipeline but produce `BrandVocabularyAnnotation` instead of `TermAnnotation`.

This reuses the entire term lookup infrastructure (exact → normalized → fuzzy matching, morphology-aware stemming, AI-assisted matching) without building a parallel system.

**New pipeline tool:** `brand-vocab-check` — runs `term-lookup` filtered to `term_source=brand_vocabulary`, then flags:
- Forbidden term usage (severity: major)
- Competitor term usage (severity: critical)
- Missing preferred term where a forbidden alternative was used (severity: minor, with suggestion)

### 1.3 Cloud MCP Server (bowrain-server mcp)

This is the primary distribution mechanism. A third MCP server, hosted by Bowrain Server, accessible over Streamable HTTP with OAuth 2.1:

**Resources** (structured context loaded before AI writes):
| Resource URI | Description |
|---|---|
| `brand://profiles/{id}` | Full voice profile (tone, style, vocabulary, examples) |
| `brand://profiles/{id}/vocabulary` | Preferred/forbidden/competitor term lists |
| `brand://profiles/{id}/examples` | Before/after example pairs |
| `brand://profiles/{id}/locale/{locale}` | Locale-specific voice adaptations |
| `brand://terminology/{workspace}` | Full termbase for the workspace |

**Tools** (model-controlled functions):
| Tool | Description |
|---|---|
| `check_vocabulary` | Validate text against preferred/forbidden/competitor terms. Returns flagged terms with suggestions. |
| `list_profiles` | List available brand voice profiles |
| `get_voice_guide` | Get the complete voice guide for a profile (formatted for LLM consumption) |

**Prompts** (user-controlled templates):
| Prompt | Description |
|---|---|
| `write_in_voice` | "Write [content type] in [brand] voice about [topic]" |
| `rewrite_in_voice` | "Rewrite this text to match [brand] voice" |
| `check_draft` | "Check this draft against [brand] voice guidelines" |

**Transport:** Streamable HTTP on the existing bowrain-server Echo endpoint (`/mcp/`), with OAuth 2.1 + PKCE using Bowrain's existing Keycloak OIDC infrastructure. The MCP authorization flow reuses the existing auth system (AD-015) — no separate credential management.

**Implementation:** Add `bowrain/mcp/` package in the bowrain module. The server registers as an Echo route group, reusing existing service layer (ProjectService, TermBase, FlowService). The brand profile storage is a new BrandStore interface alongside ContentStore and TermBase.

### 1.4 Brand voice profile management UI

Add to both Bowrain web app and desktop app:

- **Profile editor** — form-based UI for tone, style, vocabulary, examples. Each section maps to the VoiceProfile struct. Rich text for guidelines, table editor for vocabulary rules.
- **Example pairs** — side-by-side "before/after" editor showing how generic text should be rewritten in the brand voice. These examples serve as few-shot prompts for LLM-based tools.
- **Vocabulary table** — CRUD for preferred/forbidden/competitor terms with import from CSV. Integrates with existing termbase UI (term status, part of speech, notes).
- **Profile preview** — paste any text, see it annotated with vocabulary violations in real-time (via the `brand-vocab-check` pipeline tool).

**Workspace scoping:** Brand voice profiles are workspace-level resources (not project-level), because brand voice spans all projects. Projects can override specific profile settings via channel overrides.

### 1.5 Starter packs and 30-second aha moment

Ship 4–6 starter brand voice profile templates:
- **Professional B2B** — formal, active voice, no contractions, technical precision
- **Friendly DTC** — casual, second person, contractions, warm
- **Technical Documentation** — precise, imperative mood, short sentences
- **Marketing Blog** — conversational, storytelling, varied sentence length
- **Customer Support** — empathetic, solution-focused, clear

Each pack includes tone profile, style rules, vocabulary rules (with common "forbidden" corporate jargon), and 3–5 before/after examples.

The 30-second aha moment: User connects Bowrain MCP server in Claude Desktop → types "check this text against Professional B2B" → Bowrain highlights vocabulary violations, suggests replacements, shows a before/after diff.

### Phase 1 deliverables

| Component | Module | Key files |
|---|---|---|
| VoiceProfile types | framework (`core/brand/`) | `profile.go`, `vocabulary.go`, `scoring.go` |
| BrandStore interface | framework (`core/brand/`) | `store.go` |
| CLI SQLite brand store | cli (`cli/storage/brand/`) | `sqlite.go` |
| Server PostgreSQL brand store | bowrain (`bowrain/brand/`) | `postgres.go`, `migrations/` |
| `brand-vocab-check` tool | framework (`core/tools/`) | `brandvocab.go` |
| Term source extension | framework (`core/termbase/`) | update Concept struct |
| Cloud MCP server | bowrain (`bowrain/mcp/`) | `server.go`, `resources.go`, `tools.go`, `prompts.go` |
| MCP auth (OAuth 2.1) | bowrain (`bowrain/mcp/`) | `auth.go` (wraps existing OIDC) |
| Profile management UI | web + desktop | profile editor, vocabulary table, preview |
| Starter voice packs | framework (`core/brand/packs/`) | YAML files |
| REST API endpoints | bowrain (`bowrain/server/`) | `handlers_brand.go` |

---

## Phase 2: AI-Powered Brand Voice Scoring (Guide → Write → QA)

**Goal:** Ship the `brand-voice-check` AI tool and MQM-inspired compliance scoring. Content gets a measurable Brand Compliance Score. The Guide → Write → QA loop closes.

### 2.1 Brand Compliance Score (MQM-inspired)

Adapt the translation industry's MQM framework for brand voice:

**Scoring dimensions:**
| Dimension | What it measures | Example issues |
|---|---|---|
| Tone | Formality, personality, emotional register | Too formal for casual brand; humor where inappropriate |
| Style | Sentence structure, voice, POV | Passive voice in active-voice brand; wrong person POV |
| Vocabulary | Preferred/forbidden/competitor terms | Forbidden jargon; competitor brand names |
| Clarity | Readability, conciseness | Unnecessarily complex sentences; ambiguous phrasing |
| Brand Compliance | Overall guideline adherence | Off-brand messaging; inconsistent with examples |

**Severity weights** (from MQM):
| Severity | Weight | Meaning |
|---|---|---|
| Neutral | 0 | Observation, no penalty |
| Minor | 1 | Slight deviation, acceptable |
| Major | 5 | Clear violation, needs correction |
| Critical | 25 | Serious violation (competitor terms, legal risk) |

**Score calculation:**
```
Penalty = sum(severity_weight * count) per dimension
Raw Score = max(0, 100 - Penalty)
Brand Compliance Score = Raw Score (0-100)
```

Score stored in `Block.Properties["brand-voice-score"]` as JSON with per-dimension breakdown.

### 2.2 `brand-voice-check` AI tool

The sixth AI tool (AD-008), implemented as a standard pipeline tool:

```go
// core/ai/tools/brandvoice.go
type BrandVoiceCheckTool struct {
    tool.BaseTool
    pool     *ai.AIWorkerPool
    profile  *brand.VoiceProfile
}
```

**How it works:**
1. Receives Blocks from upstream in the pipeline
2. For each Block, constructs a prompt containing:
   - The brand voice profile (tone, style, vocabulary, examples)
   - The Block's source text (and target text if multilingual)
   - Any existing annotations (terminology, entities)
3. Calls `ChatStructured` with a JSON schema for structured findings
4. Returns `BrandVoiceAnnotation` entries with dimension, severity, character range, message, and suggestion
5. Calculates and stores the Brand Compliance Score

**Prompt design:** The prompt includes the voice profile as system context, before/after examples as few-shot demonstrations, and the specific text to check. The structured output schema enforces that the LLM returns typed findings matching the scoring dimensions.

**Pipeline composition:**
```
Reader → term-lookup → brand-vocab-check → brand-voice-check → Writer
         (rule-based)   (rule-based)        (LLM-based)
```

The rule-based `brand-vocab-check` (Phase 1) catches concrete vocabulary violations cheaply. The LLM-based `brand-voice-check` catches nuanced tone and style issues. Running vocabulary first means the LLM doesn't waste tokens on easily detectable term issues.

### 2.3 Cloud MCP Server — scoring tools

Extend the Phase 1 MCP server with scoring tools:

| Tool | Description |
|---|---|
| `score_brand_compliance` | Run full brand voice check (vocabulary + AI scoring). Returns per-dimension scores, overall score, and annotated findings. |
| `suggest_corrections` | Given findings from a check, generate specific rewrite suggestions for each issue. |
| `rewrite_in_voice` | Rewrite input text to match the brand voice profile. Returns rewritten text + diff. |

These compose into the closed loop: an AI assistant reads the voice guide (resource), writes content (using prompts), scores it (tool), and iterates on corrections (tool) — all within a single conversation.

### 2.4 Confidence-based routing

Add configurable thresholds to the automation system (AD-011):

```yaml
automations:
  - name: brand-voice-gate
    on: translation.updated
    actions:
      - flow: brand-voice-check
        config:
          profile: marketing
          auto_approve_threshold: 85    # score >= 85 → auto-approve
          review_threshold: 60          # 60 <= score < 85 → human review queue
          reject_threshold: 60          # score < 60 → auto-reject with feedback
```

Content scoring above `auto_approve_threshold` passes the quality gate automatically. Below `review_threshold` enters a human review queue. In between gets random spot-checking (configurable sample rate).

This uses the existing quality gate mechanism from AD-011 — `EventQualityGateFailed` triggers when content scores below threshold.

### 2.5 Brand compliance dashboard

Add to the web app and desktop app:

- **Compliance overview** — aggregate Brand Compliance Score across all project content, with per-dimension breakdowns and trend lines over time
- **Issue density** — issues per 1,000 words, tracked over time, broken down by dimension
- **Drill-down** — click a dimension to see all blocks with issues in that dimension, sorted by severity
- **Inline annotations** — in the editor, show brand voice findings with color-coded severity (neutral=gray, minor=yellow, major=orange, critical=red), one-click accept/reject for suggestions
- **Side-by-side** — AI draft vs. brand guidelines comparison view

### Phase 2 deliverables

| Component | Module | Key files |
|---|---|---|
| BrandVoiceAnnotation | framework (`core/model/`) | `annotation.go` (extend) |
| Brand Compliance Score | framework (`core/brand/`) | `scoring.go` |
| `brand-voice-check` AI tool | framework (`core/ai/tools/`) | `brandvoice.go` |
| MCP scoring tools | bowrain (`bowrain/mcp/`) | `tools_scoring.go` |
| Confidence-based routing | bowrain (`bowrain/event/`) | extend AutomationEngine |
| Compliance dashboard UI | web + desktop | dashboard, drill-down, inline annotations |
| Quality gate integration | bowrain (`bowrain/server/`) | extend `handlers_qa.go` |

---

## Phase 3: Multilingual Brand Voice

**Goal:** Extend brand voice governance across languages. A single voice profile governs all locales, with per-locale adaptations for cultural context. Translation preserves voice, not just meaning.

### 3.1 Per-locale voice adaptations

The `VoiceProfile.Locales` map stores locale-specific overrides:

```go
type LocaleOverride struct {
    Formality       string            `json:"formality,omitempty"`       // override for this market
    Humor           string            `json:"humor,omitempty"`           // cultural humor norms
    PersonPOV       string            `json:"person_pov,omitempty"`     // e.g., German formal "Sie"
    CulturalNotes   string            `json:"cultural_notes"`           // free-text adaptation guidance
    VocabularyOverrides []TermRule    `json:"vocabulary_overrides"`     // locale-specific term preferences
    ExampleOverrides    []VoiceExample `json:"example_overrides"`       // locale-specific before/after pairs
}
```

**Key design:** Locale overrides are additive. They override specific fields of the parent profile, inheriting everything else. A Japanese locale override might change `formality: "formal"` and `person_pov: "third"` while inheriting all other settings.

### 3.2 Voice-aware AI translation

Extend `ai-translate` (AD-008) to include brand voice context in translation prompts:

```
System: You are translating content for {brand_name}. The brand voice is:
- Tone: {tone.personality}, {tone.formality}
- Style: {style guidelines}
- This locale's specific adaptations: {locale_override.cultural_notes}

Translate the following content from {source_lang} to {target_lang}, preserving
the brand voice. Pay attention to:
- Using the preferred terminology (see glossary below)
- Maintaining the {formality} register in {target_lang}
- Adapting humor/cultural references appropriately

Glossary:
{preferred_terms_for_target_locale}

Example translations in this voice:
{locale_override.example_overrides}
```

This doesn't require a new tool — it extends the existing `ai-translate` tool to accept an optional `VoiceProfile` in its config. When present, the prompt includes voice context.

### 3.3 Cross-locale compliance dashboard

Extend the compliance dashboard with:

- **Language matrix** — Brand Compliance Scores per locale in a grid view, highlighting locales that drift from the voice
- **Drift detection** — automated comparison of scores between source and target languages, flagging locales where brand voice degrades most
- **Locale-specific issue breakdown** — which dimensions (tone, vocabulary, style) are hardest to maintain in each locale

### 3.4 MCP resources for multilingual voice

Add locale-aware resources to the MCP server:

```
brand://profiles/{id}/locale/{locale}        → locale-specific voice guide
brand://profiles/{id}/vocabulary/{locale}    → locale-specific term preferences
brand://terminology/{workspace}/{locale}     → workspace termbase filtered by locale
```

AI translation agents fetch the target locale's voice profile before translating, ensuring voice-aware output from the first draft.

### Phase 3 deliverables

| Component | Module | Key files |
|---|---|---|
| LocaleOverride types | framework (`core/brand/`) | `locale.go` |
| Voice-aware ai-translate | framework (`core/ai/tools/`) | extend `translate.go` |
| Cross-locale dashboard | web + desktop | language matrix, drift detection |
| Locale-aware MCP resources | bowrain (`bowrain/mcp/`) | extend `resources.go` |
| Multilingual voice scoring | framework (`core/ai/tools/`) | extend `brandvoice.go` |

---

## Phase 4: Feedback Loop and Learning System

**Goal:** Every human correction feeds back into the system. Brand voice governance becomes a learning system that improves over time, not a static checklist.

### 4.1 Correction capture

When a human reviewer corrects a brand voice issue in the editor:
1. The original text, the correction, and the associated brand voice finding are captured as a `BrandVoiceCorrection`
2. Corrections are stored in the BrandStore with the profile ID, dimension, and before/after text
3. The correction count per dimension is tracked over time (error density reporting)

### 4.2 Profile refinement from corrections

Corrections feed back into the voice profile:
- **Vocabulary refinement** — if reviewers consistently replace a term, it becomes a candidate for the preferred/forbidden list (surfaced in a "Suggested Rules" queue)
- **Example generation** — reviewer-approved corrections become candidate before/after examples for the voice profile (human-approved only)
- **Scoring calibration** — if auto-approved content gets corrected by reviewers, the system suggests lowering the auto-approve threshold for that dimension

### 4.3 Progressive autonomy

The automation system (AD-011) tracks correction rates per dimension over time:

```
Level 0: Full human review (all content enters review queue)
Level 1: AI-assisted review (suggestions shown, human decides)
Level 2: Auto-approve high-confidence (score >= threshold, spot-check)
Level 3: Auto-approve most (only flag critical issues for review)
Level 4: Full autonomy (all content auto-approved, alerts on drift)
```

Organizations start at Level 0 and can advance per content type. Legal content might stay at Level 1 forever; social media posts might reach Level 3 quickly. The thresholds per level are configurable in workspace settings.

### 4.4 Error density reporting

Track and display:
- **Issues per 1,000 words** — by dimension, over time, per locale
- **Correction rate** — percentage of auto-approved content that needed human correction
- **Systematic patterns** — recurring issues (same forbidden term, same tone violation) flagged for profile refinement
- **Writer-level metrics** — (optional) per-user compliance trends for coaching

### Phase 4 deliverables

| Component | Module | Key files |
|---|---|---|
| BrandVoiceCorrection model | framework (`core/brand/`) | `correction.go` |
| Correction capture in editor | bowrain (`bowrain/server/`) | extend `handlers_editor.go` |
| Suggested Rules queue UI | web + desktop | correction review, rule promotion |
| Progressive autonomy config | bowrain (`bowrain/event/`) | extend AutomationEngine |
| Error density reporting | bowrain (`bowrain/server/`) | `handlers_brand_analytics.go` |
| Drift alerting | bowrain (`bowrain/event/`) | new event: `brand.voice.drift` |

---

## Phase 5: Open Ecosystem and Distribution

**Goal:** Bowrain becomes the "brand voice infrastructure layer" through open-source distribution, community engagement, and ecosystem integrations.

### 5.1 Open-source brand voice MCP server

Extract a standalone, open-source MCP server from the Bowrain platform:

- **`kapi brand-check`** — standalone CLI command for single-file brand voice checking (no server needed)
- **`kapi mcp`** — extend with brand voice tools for local, file-based brand checking
- **Single-profile mode** — load a `voice-profile.yaml` from disk, check content against it. Genuine standalone utility under Apache 2.0.

**Cloud upsell:** Multiple profiles, team collaboration, analytics, cross-locale governance, SSO, audit trails — all on Bowrain Server.

### 5.2 Claude Skill

Publish a Bowrain Skill (agentskills.io) that teaches Claude how to:
- Structure brand voice guidelines effectively
- Apply voice consistency when writing content
- Self-check drafts against voice rules before presenting to users
- Adapt voice across content types (blog, social, docs, support)

The Skill provides procedural knowledge ("how to write in brand voice"), while the MCP server provides the specific guidelines and tools ("this brand's voice is..."). They complement each other.

### 5.3 Agent Skill for Claude Code

Publish a Bowrain agent skill that integrates brand voice checking into development workflows:
- Check documentation changes against brand voice before commit
- Validate user-facing strings in code against brand vocabulary
- Suggest rewrites for UI copy that violates brand guidelines

### 5.4 Registry and distribution

- List on PulseMCP, MCP.so, official MCP Registry, modelcontextprotocol/servers
- Publish Bowrain MCP server configuration in Claude Desktop, VS Code, and Cursor settings templates
- Ship the standalone brand voice tools in the existing Homebrew formulae (`kapi` and `bowrain`)
- Add brand voice profiles to the plugin registry alongside Okapi bridge filters

### 5.5 Template gallery

Create a community template gallery for brand voice profiles:
- Starter packs (Phase 1) as seeds
- User-contributed profiles (anonymized, shared with permission)
- Industry-specific templates (SaaS, healthcare, finance, e-commerce)
- Locale-specific adaptation guides

### Phase 5 deliverables

| Component | Module | Key files |
|---|---|---|
| `kapi brand-check` CLI command | kapi (`kapi/cmd/kapi/`) | `brand.go` |
| Kapi MCP brand tools | kapi (`kapi/cmd/kapi/`) | extend MCP registration |
| Claude Skill | standalone repo | `bowrain-skill/` |
| Claude Code agent skill | skills registry | `bowrain/` |
| Template gallery | bowrain web app | profile browser + share flow |

---

## Module placement and dependency boundaries

Strict adherence to the existing module architecture (AD-018):

```
core/brand/                    ← VoiceProfile types, scoring logic, store interface
                                  (framework module, Apache 2.0, no platform deps)

core/tools/brandvocab.go       ← brand-vocab-check tool (rule-based)
core/ai/tools/brandvoice.go    ← brand-voice-check tool (LLM-based)
                                  (framework module, standard pipeline tools)

cli/storage/brand/             ← SQLite brand store for kapi CLI
                                  (cli module, depends on framework only)

bowrain/brand/                 ← PostgreSQL brand store for server
bowrain/mcp/                   ← Cloud MCP server (Streamable HTTP + OAuth)
bowrain/server/handlers_brand* ← REST API for brand voice management
                                  (bowrain module, depends on framework + platform)
```

**Key constraint:** Brand voice types and scoring live in the framework. The framework has no database dependency — storage is injected via interfaces. This ensures `kapi brand-check` works standalone from a YAML file, while Bowrain Server uses PostgreSQL.

---

## Event types for brand voice

Extend the existing event system (AD-011):

```go
const (
    EventBrandVoiceCheckStarted   EventType = "brand.voice.check.started"
    EventBrandVoiceCheckCompleted EventType = "brand.voice.check.completed"
    EventBrandVoiceGateFailed     EventType = "brand.voice.gate.failed"
    EventBrandVoiceGatePassed     EventType = "brand.voice.gate.passed"
    EventBrandVoiceDrift          EventType = "brand.voice.drift"           // aggregate score drops
    EventBrandVoiceCorrected      EventType = "brand.voice.corrected"       // human correction captured
    EventBrandProfileUpdated      EventType = "brand.profile.updated"       // profile changed
)
```

These events plug into the existing automation engine — brand voice actions compose with existing trigger types (content.changed, translation.updated, connector.synced).

---

## Database schema additions

### Brand voice profile tables (PostgreSQL)

```sql
-- Workspace-scoped brand voice profiles
CREATE TABLE brand_profiles (
    id          TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    name        TEXT NOT NULL,
    description TEXT,
    tone        JSONB NOT NULL DEFAULT '{}',
    style       JSONB NOT NULL DEFAULT '{}',
    vocabulary  JSONB NOT NULL DEFAULT '{}',
    examples    JSONB NOT NULL DEFAULT '[]',
    locales     JSONB NOT NULL DEFAULT '{}',
    channels    JSONB NOT NULL DEFAULT '{}',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  TEXT NOT NULL REFERENCES users(id),
    UNIQUE (workspace_id, name)
);

-- Brand voice check results per block
CREATE TABLE brand_voice_scores (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    stream      TEXT NOT NULL DEFAULT 'main',
    block_id    TEXT NOT NULL,
    profile_id  TEXT NOT NULL REFERENCES brand_profiles(id),
    locale      TEXT NOT NULL,
    score       INTEGER NOT NULL,       -- 0-100
    dimensions  JSONB NOT NULL,          -- per-dimension breakdown
    findings    JSONB NOT NULL,          -- individual issues
    checked_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Human corrections for feedback loop
CREATE TABLE brand_voice_corrections (
    id              TEXT PRIMARY KEY,
    profile_id      TEXT NOT NULL REFERENCES brand_profiles(id),
    block_id        TEXT NOT NULL,
    dimension       TEXT NOT NULL,
    original_text   TEXT NOT NULL,
    corrected_text  TEXT NOT NULL,
    finding_id      TEXT,                -- links to the finding that was corrected
    corrected_by    TEXT NOT NULL REFERENCES users(id),
    corrected_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bvs_project_stream ON brand_voice_scores(project_id, stream);
CREATE INDEX idx_bvs_profile_score ON brand_voice_scores(profile_id, score);
CREATE INDEX idx_bvc_profile_dim ON brand_voice_corrections(profile_id, dimension);
```

SQLite equivalents follow the same schema with `TEXT` instead of `TIMESTAMPTZ` and no `JSONB` type (use `TEXT` with JSON validation in Go).

---

## REST API endpoints

```
# Brand voice profiles (workspace-scoped)
GET    /workspaces/:ws/brand-profiles              # list profiles
POST   /workspaces/:ws/brand-profiles              # create profile
GET    /workspaces/:ws/brand-profiles/:id           # get profile
PUT    /workspaces/:ws/brand-profiles/:id           # update profile
DELETE /workspaces/:ws/brand-profiles/:id           # delete profile
POST   /workspaces/:ws/brand-profiles/:id/duplicate # duplicate with new name

# Brand voice checking
POST   /workspaces/:ws/brand-profiles/:id/check     # check arbitrary text
GET    /projects/:pid/brand-voice/scores             # compliance scores for project
GET    /projects/:pid/brand-voice/scores/:locale     # scores for specific locale
GET    /projects/:pid/brand-voice/trends             # score trends over time

# Corrections (feedback loop)
POST   /projects/:pid/brand-voice/corrections        # record a correction
GET    /workspaces/:ws/brand-voice/suggested-rules   # suggested rule candidates from corrections

# Starter packs
GET    /brand-profiles/starters                      # list starter templates
POST   /workspaces/:ws/brand-profiles/from-starter   # create from starter
```

---

## MCP server endpoint

```
POST /mcp/                     # Streamable HTTP (JSON-RPC)
GET  /mcp/                     # SSE for server-initiated messages
GET  /.well-known/oauth-authorization-server   # OAuth 2.1 metadata
POST /mcp/token                # OAuth token endpoint (delegates to Keycloak)
```

The MCP endpoint mounts on the existing bowrain-server Echo instance. OAuth 2.1 discovery delegates to Keycloak's well-known endpoint. The MCP token endpoint wraps Keycloak's token endpoint to present the standard MCP OAuth flow to clients.

---

## Phasing and priorities

| Phase | Focus | Primary value | Estimated scope |
|---|---|---|---|
| **1** | Profile + Terminology + MCP | 30-second aha moment, vocabulary checking via MCP | New `core/brand/` package, extend termbase, cloud MCP server, profile UI |
| **2** | AI Scoring + Quality Gates | Measurable Brand Compliance Score, Guide→Write→QA loop | `brand-voice-check` AI tool, MQM scoring, compliance dashboard, confidence routing |
| **3** | Multilingual Voice | Voice-aware translation, cross-locale governance | Locale overrides, extend ai-translate prompts, language matrix dashboard |
| **4** | Feedback Loop | Learning system, progressive autonomy | Correction capture, suggested rules, error density reporting, drift alerts |
| **5** | Open Ecosystem | Community adoption, distribution | Open-source MCP server, Claude Skill, template gallery, registry listings |

**Phase 1 is the wedge.** Terminology enforcement is concrete, measurable, and ships fast because it extends existing infrastructure. The MCP server makes it instantly useful in Claude Desktop, Cursor, and VS Code. Everything else builds on this foundation.

**Phases overlap.** Phase 2 can start as soon as Phase 1's data model is stable. Phase 5's open-source extraction can happen alongside Phase 2 development. The phasing describes dependency order, not sequential timelines.

---

## Key architectural decisions

1. **Brand voice types in framework, not platform.** Enables standalone `kapi brand-check` for monolingual use without a server. Storage backends are injected.

2. **Reuse terminology infrastructure for brand vocabulary.** Don't build a parallel term matching system. Add `term_source` to distinguish terminology from brand vocabulary. Same tiered lookup, same annotations.

3. **Rule-based vocabulary check + LLM-based voice check, in sequence.** Cheap rule-based checking catches concrete violations first. Expensive LLM checking handles nuanced tone/style. Running vocabulary first means the LLM prompt doesn't waste tokens on detectable term issues.

4. **Cloud MCP server on Streamable HTTP, not just stdio.** Stdio servers require local CLI installation. A cloud MCP server with OAuth means users connect in 30 seconds without installing anything. The existing Keycloak OIDC infrastructure handles auth.

5. **MQM-inspired scoring, not binary pass/fail.** Five dimensions with four severity levels produce a nuanced Brand Compliance Score (0–100). This enables confidence-based routing and progressive autonomy — not possible with binary checks.

6. **Corrections as training data, not just audit trail.** Every human correction becomes a candidate for profile refinement (suggested rules, example pairs). This is the feature no competitor has — brand voice governance that gets better with use.

7. **Workspace-scoped profiles, not project-scoped.** Brand voice is an organizational concern that spans all projects. Projects inherit workspace profiles and can apply channel overrides.
