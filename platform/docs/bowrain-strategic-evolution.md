# Bowrain Strategic Evolution Plan

How we evolve Neokapi (framework) and Bowrain (platform) from a localization engine into the AI-native brand voice infrastructure layer for content creation.

This document bridges the market intelligence with the technical implementation plan at `platform/docs/brand-voice-platform-plan.md`.

---

## Strategic thesis

**The market opportunity:** No production-grade, model-agnostic solution exists that embeds brand governance directly into AI writing workflows via MCP. The market splits into generators (Writer, Jasper) and governors (Acrolinx, Grammarly). Nobody delivers a composable Guide → Write → QA closed loop that works across AI assistants.

**Bowrain's position:** Portable brand voice infrastructure layer — configure once, enforce everywhere via MCP across Claude, ChatGPT, Copilot, Cursor, and any MCP-capable tool. Not another walled garden; open infrastructure.

**Three defensible moats:**
1. **Portable brand voice** — universal format via MCP, not locked to one AI platform
2. **Terminology as wedge** — concrete, measurable, acknowledged as unsolved by Nimdzi
3. **Learning feedback loop** — QA corrections improve future generation guidance (no competitor does this)

---

## What we already have

The existing Neokapi/Bowrain architecture provides strong foundations:

| Capability | Current state | Brand voice extension |
|---|---|---|
| **Two MCP servers** (AD-021) | `kapi mcp` (file processing, stdio) and `bowrain mcp` (project sync, stdio) | Add third: `bowrain-server mcp` — cloud-hosted, Streamable HTTP + OAuth 2.1 |
| **Terminology** (AD-010) | Concept-oriented termbase, tiered lookup (exact → normalized → fuzzy), morphology-aware. SQLite + PostgreSQL. | Extend with `term_source: brand_vocabulary`. Preferred/forbidden/competitor terms reuse same lookup pipeline. |
| **5 AI tools** (AD-008) | ai-translate, ai-qa, ai-review, ai-terminology, ai-entity-extract. Worker pool with rate limiting, circuit breakers. | Add `brand-voice-check` as sixth AI tool. Extend ai-translate with voice-aware prompting. |
| **80+ pipeline tools** | QA checks, term enforcement, word count, segmentation, encoding. Channel-based streaming. | Add `brand-vocab-check` (rule-based) before `brand-voice-check` (LLM-based). |
| **Event bus + automation** (AD-011) | Event types, automation rules, quality gates, webhooks. | Brand compliance becomes quality gate. Voice drift triggers automation rules. |
| **Content model** (AD-002) | Block annotations (TermAnnotation, EntityAnnotation), coded text fragments, content-addressable blocks. | Add BrandVoiceAnnotation. Store compliance scores in Block.Properties. |
| **REST API** | 50+ handlers, WebSocket collab editing, gRPC multiplexed. Keycloak OIDC auth. | Add brand profile CRUD, compliance scoring, correction endpoints. MCP endpoint on `/mcp/`. |
| **Plugin system** (AD-007) | gRPC plugins, Okapi Bridge (40+ format filters). Format, tool, connector, provider types. | Brand voice scoring backends register as provider plugins. |

---

## Competitive positioning

### Direct competitors

| Competitor | Strength | Weakness vs. Bowrain |
|---|---|---|
| **Writer.com** ($47M ARR, 194% YoY) | Deep brand governance, personality profiles, proprietary LLMs | Closed ecosystem — requires adopting their LLMs. No MCP. No open API for external AI tools. |
| **Acrolinx/Markup AI** | Deepest content governance, machine-enforceable style rules, quality gates | Enterprise-only, complex implementation, no MCP server, no feedback loop. |
| **Grammarly Business** (30M+ DAU) | Broadest reach via browser extensions | Shallow brand governance, post-generation overlay, no composable architecture. |
| **Jasper AI** ($35-55M ARR, declining) | Brand voice learning from samples, multi-model | Thin-moat AI wrapper, no governance layer, peaked and declining. |

### Why Bowrain wins

1. **Model-agnostic via MCP** — works with Claude, ChatGPT, Copilot, Cursor, any MCP client. Writer requires its own LLMs. Acrolinx has no MCP. Grammarly is a browser extension.
2. **Open infrastructure** — Apache 2.0 framework, open-source MCP server. Enterprises can self-host. Not locked in.
3. **Existing localization engine** — 15+ format readers/writers, 80+ pipeline tools, terminology management already production-grade. Brand voice is a natural extension, not a ground-up build.
4. **Feedback loop** — every human correction feeds back into profile refinement. Static checklists vs. learning system.

### Market gaps we fill

| Gap | Bowrain's answer |
|---|---|
| No brand voice MCP server exists (across 5,800+ servers) | Cloud MCP server with OAuth 2.1, shipping in Phase 1 |
| Guide → Write → QA loop is broken (30-40% time re-establishing brand context) | Resources (guide) + Prompts (write) + Tools (QA) in one MCP server |
| Terminology vetting has no API-first solution | Existing termbase + `check_vocabulary` MCP tool |
| Multilingual brand voice unsolved | Per-locale voice adaptations, voice-aware ai-translate (Phase 3) |
| Brand voice not portable across AI tools | Universal voice profile format, MCP as distribution |

---

## Five-phase roadmap

### Phase 1: Brand Voice Profile + Terminology Wedge + MCP

**Goal:** Ship the 30-second aha moment. User connects Bowrain MCP → pastes text → sees brand vocabulary violations highlighted with suggestions.

**Why terminology first:** It's concrete, measurable, and extends existing infrastructure. No new AI models needed — rule-based matching against preferred/forbidden term lists.

**What we build:**

1. **`core/brand/` package** (framework module)
   - `VoiceProfile` — tone (personality, formality, emotion), style rules (active voice, sentence length, POV, contractions), vocabulary rules (preferred/forbidden/competitor terms), examples (before/after pairs), locale overrides, channel overrides
   - `BrandStore` interface — profile CRUD, profile-by-workspace lookup
   - Pure types with JSON serialization, no database dependency

2. **Terminology extension** (framework module)
   - Add `term_source` field to `Concept`: `"terminology"` or `"brand_vocabulary"`
   - Add `competitor_term` boolean to `Term`
   - Brand vocabulary terms reuse the entire tiered lookup pipeline

3. **`brand-vocab-check` pipeline tool** (framework module)
   - Run term-lookup filtered to `term_source=brand_vocabulary`
   - Flag: forbidden term (major), competitor term (critical), missing preferred term (minor with suggestion)

4. **Cloud MCP server** (`bowrain/mcp/`)
   - Streamable HTTP on existing bowrain-server Echo endpoint (`/mcp/`)
   - OAuth 2.1 + PKCE via existing Keycloak OIDC
   - Resources: `brand://profiles/{id}`, vocabulary, examples
   - Tools: `check_vocabulary`, `list_profiles`, `get_voice_guide`
   - Prompts: `write_in_voice`, `rewrite_in_voice`, `check_draft`

5. **Profile management UI** (web + desktop)
   - Profile editor (tone, style, vocabulary, examples)
   - Vocabulary table with CSV import
   - Real-time preview (paste text → see violations)

6. **Starter packs** (`core/brand/packs/`)
   - Professional B2B, Friendly DTC, Technical Documentation, Marketing Blog, Customer Support
   - Each with tone profile, style rules, vocabulary, 3-5 before/after examples

7. **Storage backends**
   - SQLite for kapi CLI (`cli/storage/brand/`)
   - PostgreSQL for bowrain-server (`bowrain/brand/`)

**Distribution (day one):**
- List on PulseMCP, MCP.so, official MCP Registry, modelcontextprotocol/servers
- Ship in existing Homebrew formulae (kapi, bowrain)
- Claude Desktop, Cursor, VS Code configuration templates

### Phase 2: AI-Powered Scoring + Quality Gates

**Goal:** Close the Guide → Write → QA loop with measurable Brand Compliance Scores. Content gets scored, not just checked.

**What we build:**

1. **`brand-voice-check` AI tool** (`core/ai/tools/brandvoice.go`)
   - Sixth AI tool using existing AIWorkerPool
   - Prompt includes voice profile + examples as few-shot context
   - `ChatStructured` returns typed findings per MQM-inspired dimensions
   - Pipeline: `term-lookup → brand-vocab-check → brand-voice-check`
   - Rule-based catches cheap violations; LLM handles nuanced tone/style

2. **Brand Compliance Score** (MQM-inspired, 0-100)
   - Five dimensions: Tone, Style, Vocabulary, Clarity, Brand Compliance
   - Four severities: Neutral (0), Minor (1), Major (5), Critical (25)
   - Score = max(0, 100 - penalty_sum)
   - Stored in Block.Properties, BrandVoiceAnnotation for findings

3. **Confidence-based routing** (extend automation engine)
   - auto_approve_threshold: 85+ → passes quality gate
   - review_threshold: 60-84 → human review queue
   - reject_threshold: \<60 → auto-reject with feedback
   - Uses existing quality gate mechanism (EventQualityGateFailed)

4. **MCP scoring tools**
   - `score_brand_compliance` — full vocabulary + AI check, returns scores + findings
   - `suggest_corrections` — generate rewrites for each finding
   - `rewrite_in_voice` — full rewrite with before/after diff

5. **Compliance dashboard** (web + desktop)
   - Aggregate scores with per-dimension breakdowns and trends
   - Issue density (per 1,000 words over time)
   - Inline annotations in editor (color-coded severity)
   - Side-by-side: AI draft vs. guidelines

### Phase 3: Multilingual Brand Voice

**Goal:** Voice-aware translation. A single profile governs all locales with per-locale cultural adaptations.

**What we build:**

1. **Per-locale voice adaptations** (`LocaleOverride`)
   - Override formality, humor, person POV per market
   - Cultural notes, locale-specific vocabulary, locale-specific examples
   - Additive overrides — inherit parent, override specific fields

2. **Voice-aware ai-translate**
   - Extend existing ai-translate to accept optional VoiceProfile in config
   - Prompt includes brand voice context, locale adaptations, glossary, examples
   - No new tool — configuration extension of existing one

3. **Cross-locale compliance dashboard**
   - Language matrix (scores per locale in grid)
   - Drift detection (which locales degrade brand voice most)
   - Per-locale dimension breakdown

4. **Locale-aware MCP resources**
   - `brand://profiles/{id}/locale/{locale}` → locale-specific voice guide
   - `brand://profiles/{id}/vocabulary/{locale}` → locale-specific terms
   - Translation agents fetch target locale profile before translating

### Phase 4: Feedback Loop + Learning System

**Goal:** Brand voice governance that gets better with every piece of content reviewed.

**What we build:**

1. **Correction capture**
   - Every human edit captured as `BrandVoiceCorrection` (original, corrected, dimension, finding)
   - Corrections feed suggested-rules queue

2. **Profile refinement**
   - Vocabulary: consistently replaced terms → candidate for preferred/forbidden list
   - Examples: reviewer-approved corrections → candidate before/after examples
   - Scoring calibration: auto-approved content that gets corrected → lower threshold suggestion

3. **Progressive autonomy** (5 levels)
   - Level 0: Full human review
   - Level 1: AI-assisted (suggestions shown, human decides)
   - Level 2: Auto-approve high-confidence, spot-check medium
   - Level 3: Auto-approve most, flag critical only
   - Level 4: Full autonomy, alerts on drift
   - Per content type, configurable in workspace settings

4. **Error density reporting**
   - Issues per 1,000 words by dimension, over time, per locale
   - Correction rate (auto-approved then corrected)
   - Systematic pattern detection
   - Drift alerting (aggregate score drops)

### Phase 5: Open Ecosystem + Distribution

**Goal:** Bowrain becomes the standard brand voice infrastructure layer.

**What we build:**

1. **Open-source MCP server** (Apache 2.0)
   - `kapi brand-check` — standalone CLI, no server needed
   - `kapi mcp` extended with brand tools — local file-based brand checking
   - Single-profile mode from disk (`voice-profile.yaml`)
   - Cloud upsell: multiple profiles, teams, analytics, SSO, audit

2. **Claude Skill** (agentskills.io)
   - Teaches Claude how to write in brand voice, self-check, adapt across types
   - Skill = procedural knowledge ("how"), MCP = specific guidelines ("what")

3. **Claude Code agent skill**
   - Brand voice checking in dev workflows
   - Validate user-facing strings, documentation, UI copy

4. **Template gallery**
   - Community-contributed voice profiles
   - Industry-specific templates (SaaS, healthcare, finance, e-commerce)
   - Locale-specific adaptation guides

---

## Architecture: three access layers, one profile

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

### MCP Server (primary distribution channel)

```
POST /mcp/                              # Streamable HTTP (JSON-RPC)
GET  /mcp/                              # SSE for server-initiated messages
GET  /.well-known/oauth-authorization-server  # OAuth 2.1 metadata
POST /mcp/token                         # Token endpoint (delegates to Keycloak)
```

Mounts on existing bowrain-server Echo instance. OAuth delegates to Keycloak.

### Pipeline (batch processing)

```
Reader → term-lookup → brand-vocab-check → brand-voice-check → Writer
         (rule-based)   (rule-based)        (LLM-based)
```

Rule-based catches cheap violations. LLM handles nuanced tone/style. Running vocabulary first means LLM doesn't waste tokens on detectable term issues.

### REST API (management + UI)

```
# Profile CRUD
GET/POST   /workspaces/:ws/brand-profiles
GET/PUT/DELETE /workspaces/:ws/brand-profiles/:id
POST /workspaces/:ws/brand-profiles/:id/check    # ad-hoc text checking

# Compliance scores
GET /projects/:pid/brand-voice/scores
GET /projects/:pid/brand-voice/scores/:locale
GET /projects/:pid/brand-voice/trends

# Feedback loop
POST /projects/:pid/brand-voice/corrections
GET  /workspaces/:ws/brand-voice/suggested-rules

# Starter packs
GET  /brand-profiles/starters
POST /workspaces/:ws/brand-profiles/from-starter
```

---

## Module placement (strict dependency boundaries)

```
framework/
├── core/brand/                 ← VoiceProfile types, scoring, store interface
│                                  (Apache 2.0, no platform deps, no database)
├── core/tools/brandvocab.go    ← brand-vocab-check (rule-based pipeline tool)
├── core/ai/tools/brandvoice.go ← brand-voice-check (LLM-based pipeline tool)
└── core/termbase/              ← extend Concept with term_source

framework/cli/
└── storage/brand/              ← SQLite brand store (kapi standalone)

platform/
├── brand/                      ← PostgreSQL brand store (bowrain-server)
├── mcp/                        ← Cloud MCP server (Streamable HTTP + OAuth)
└── server/handlers_brand*.go   ← REST API for brand management
```

**Key constraint:** Brand voice types live in the framework. No database dependency. Storage injected via interfaces. `kapi brand-check` works standalone from YAML; bowrain-server uses PostgreSQL.

---

## Database schema (PostgreSQL)

```sql
CREATE TABLE brand_profiles (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    name         TEXT NOT NULL,
    description  TEXT,
    tone         JSONB NOT NULL DEFAULT '{}',
    style        JSONB NOT NULL DEFAULT '{}',
    vocabulary   JSONB NOT NULL DEFAULT '{}',
    examples     JSONB NOT NULL DEFAULT '[]',
    locales      JSONB NOT NULL DEFAULT '{}',
    channels     JSONB NOT NULL DEFAULT '{}',
    version      INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by   TEXT NOT NULL REFERENCES users(id),
    UNIQUE (workspace_id, name)
);

CREATE TABLE brand_voice_scores (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    stream      TEXT NOT NULL DEFAULT 'main',
    block_id    TEXT NOT NULL,
    profile_id  TEXT NOT NULL REFERENCES brand_profiles(id),
    locale      TEXT NOT NULL,
    score       INTEGER NOT NULL,
    dimensions  JSONB NOT NULL,
    findings    JSONB NOT NULL,
    checked_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE brand_voice_corrections (
    id              TEXT PRIMARY KEY,
    profile_id      TEXT NOT NULL REFERENCES brand_profiles(id),
    block_id        TEXT NOT NULL,
    dimension       TEXT NOT NULL,
    original_text   TEXT NOT NULL,
    corrected_text  TEXT NOT NULL,
    finding_id      TEXT,
    corrected_by    TEXT NOT NULL REFERENCES users(id),
    corrected_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_bvs_project_stream ON brand_voice_scores(project_id, stream);
CREATE INDEX idx_bvs_profile_score ON brand_voice_scores(profile_id, score);
CREATE INDEX idx_bvc_profile_dim ON brand_voice_corrections(profile_id, dimension);
```

---

## Event types

```go
EventBrandVoiceCheckStarted   = "brand.voice.check.started"
EventBrandVoiceCheckCompleted = "brand.voice.check.completed"
EventBrandVoiceGateFailed     = "brand.voice.gate.failed"
EventBrandVoiceGatePassed     = "brand.voice.gate.passed"
EventBrandVoiceDrift          = "brand.voice.drift"
EventBrandVoiceCorrected      = "brand.voice.corrected"
EventBrandProfileUpdated      = "brand.profile.updated"
```

Plug into existing automation engine — compose with content.changed, translation.updated, connector.synced.

---

## Go-to-market: open core + cloud upsell

### Open-source (Apache 2.0)

- `kapi brand-check` — standalone CLI, single profile from YAML
- `kapi mcp` with brand tools — local file-based checking
- Starter brand voice packs

### Cloud platform (Bowrain Server)

- Multiple profiles per workspace
- Team collaboration, shared vocabulary
- Version history, audit trails
- Compliance analytics and trends
- SSO (Keycloak OIDC)
- API tokens for CI/CD integration

### Enterprise tier

- Custom model fine-tuning for scoring
- Multilingual voice profiles with per-locale adaptations
- CMS/DAM connector integration
- Compliance workflows with progressive autonomy
- SLA, dedicated support

### Viral adoption tactics

1. **Day one:** List on PulseMCP, MCP.so, official MCP Registry, modelcontextprotocol/servers
2. **30-second aha:** Connect MCP → paste text → see violations with suggestions
3. **Starter packs:** Professional B2B, Friendly DTC, Technical Docs, Marketing Blog, Customer Support
4. **Template gallery:** Users share/discover brand voice configurations (Notion templates playbook)
5. **Launch weeks:** Ship a new feature daily for one week per quarter (Supabase playbook)

---

## Key architectural decisions

1. **Brand voice types in framework, not platform.** Enables standalone `kapi brand-check` for monolingual use without a server.

2. **Reuse terminology infrastructure for brand vocabulary.** Same tiered lookup pipeline, same annotations. Don't build a parallel system.

3. **Rule-based + LLM-based, in sequence.** Cheap rule-based catches concrete violations. Expensive LLM handles nuanced tone/style. Running vocabulary first saves LLM tokens.

4. **Cloud MCP server on Streamable HTTP.** 30-second connection without installing anything. Existing Keycloak handles auth.

5. **MQM-inspired scoring (0-100), not binary.** Five dimensions, four severities. Enables confidence-based routing and progressive autonomy.

6. **Corrections as training data.** Every human correction → candidate for profile refinement. The feature no competitor has.

7. **Workspace-scoped profiles.** Brand voice spans all projects. Projects inherit and can apply channel overrides.

---

## Phasing summary

| Phase | Focus | What ships | Primary value |
|---|---|---|---|
| **1** | Profile + Terminology + MCP | core/brand types, termbase extension, cloud MCP server, profile UI, starter packs | 30-second aha moment, vocabulary checking via MCP |
| **2** | AI Scoring + Quality Gates | brand-voice-check AI tool, MQM scoring, confidence routing, compliance dashboard | Measurable Brand Compliance Score, Guide→Write→QA loop closes |
| **3** | Multilingual Voice | Locale overrides, voice-aware ai-translate, cross-locale dashboard | Voice-aware translation, cross-locale governance |
| **4** | Feedback Loop | Correction capture, suggested rules, progressive autonomy, drift alerts | Learning system that improves with use |
| **5** | Open Ecosystem | Open-source MCP server, Claude Skill, template gallery | Community adoption, distribution |

Phase 1 is the wedge. Terminology enforcement ships fast because it extends existing infrastructure. Everything else builds on this foundation. Phases overlap — Phase 2 starts as soon as Phase 1's data model is stable.
