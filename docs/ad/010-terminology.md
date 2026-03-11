---
id: 010-terminology
sidebar_position: 10
title: "AD-010: Terminology and Brand Management"
---
# AD-010: Terminology and brand management system

## Context

Terminology management in localization ranges from simple glossaries (CSV with
source/target pairs) to concept-oriented termbases (TBX, MultiTerm) to full
brand governance platforms (Acrolinx, Writer.com). No existing tool integrates
concept-oriented terminology with a streaming localization pipeline as
first-class tools. gokapi needs progressive complexity: start simple (CSV),
grow into concept management and brand governance.

Key gaps in the market:

- No tool integrates concept-oriented terminology with a streaming pipeline
- Multi-dimensional context (domain x product x market x time) requires
  separate termbases in existing tools rather than dimensions within entries
- What-if experimentation for terminology changes does not exist
- No open-source system bridges terminology management and brand governance
- AI-assisted term extraction and enforcement are bolted on rather than native

Standards: **TBX** (ISO 30042:2019) is the universal interchange format for
concept-oriented terminological data. **CSV/TSV** provides simple glossary
import. TBX is used for import/export; native storage uses SQLite or PostgreSQL.

## Decision

### Architecture Overview

Progressive complexity model: Terminology Store (Phase 1) -> Concept
Management (Phase 2) -> Brand Governance (Phase 3).

Shared storage infrastructure (SQLite and PostgreSQL) with Sievepen TM ([AD-009](./009-translation-memory.md))
and Content Store ([AD-003](./003-content-store.md)) via `bowrain/storage/`.

### Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept groups terms across languages, each with multi-dimensional context (products, markets, audiences, temporal validity). Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale.

### TermBase Interface

The TermBase interface provides concept CRUD, term lookup, search, and import/export. Three storage tiers:

1. **In-memory** (`core/termbase/`): Fast, ephemeral. Used for session-scoped batch processing.
2. **CLI SQLite** (`cli/storage/termbase/`): Persistent file-based storage for kapi and bowrain CLI. No project_id or stream columns — designed for single-user, file-based workflows. Resources are resolved via `--name` (KAPI_HOME), `--local` (cwd), or `--file` (explicit path). Created on demand.
3. **Server SQLite/PostgreSQL** (`bowrain/termbase/`): Persistent storage for Bowrain Server with project scoping, terminology streams, and workspace isolation (PostgreSQL). Uses the shared `bowrain/storage` layer from [AD-003](./003-content-store.md).

| Aspect | kapi CLI | Bowrain Server |
|--------|---------|---------------|
| Storage | SQLite files on disk | SQLite or PostgreSQL |
| Location | Named in KAPI_HOME, local dir, or file path | Server-managed per workspace |
| Scope | Single user, single machine | Multi-user, multi-workspace |
| Features | CRUD, import/export, lookup, search | + streams, project scoping, REST API |

See [Terminology Data Model](/docs/notes/terminology-data-model) for full Go struct definitions (Concept, Term, TermContext, TermBase interface).

### Term Lookup: Tiered Matching

Default pipeline: exact -> normalized -> fuzzy. Fuzzy matching uses
trigram-based candidate retrieval (FTS5 trigram tokenizer for SQLite,
pg_trgm GIN indexes for PostgreSQL) to reduce full table scans to ~200
candidates, followed by character-level Levenshtein scoring in Go. Text
normalization applies Unicode NFC (`golang.org/x/text/unicode/norm`) before
comparison. UI search uses ranked full-text search (FTS5 trigram with
substring matching for SQLite, pg_trgm similarity() ranking for PostgreSQL)
instead of unranked LIKE queries. Opt-in: stem matching (Snowball stemmers)
and AI-assisted matching (LLM provider from [AD-008](./008-ai-integration.md)).
The architecture is extensible via a `Matcher` interface.

### Pipeline Tools

Six pipeline tools integrate terminology, entity annotation, and privacy into the streaming pipeline: `term-lookup`, `term-enforce`, `term-extract`, `entity-annotate`, `redact`, and `unredact`.

### Concept Relations and Terminology Streams (Phase 2)

Concept relations (broader/narrower, related, supersedes) enable graph navigation. Terminology streams provide named what-if experiments for terminology changes, isolated from the active termbase until promoted.

See [Terminology Data Model](/docs/notes/terminology-data-model) for pipeline tool descriptions, concept relations, and streams details.

### Brand Voice (Phase 3)

Brand voice rules (tone, style) with a `brand-voice-check` pipeline tool
using LLM analysis ([AD-008](./008-ai-integration.md)). Positions gokapi as
the only open-source system bridging terminology and brand governance.

### Content Model Extensions

Two annotation types (`TermAnnotation`, `EntityAnnotation`) implement the `Annotation` interface with character-level `TextRange` positions for precise inline highlighting in Bowrain ([AD-012](./012-bowrain.md)). See [Terminology Data Model](/docs/notes/terminology-data-model) for details.

## Alternatives Considered

**Embed in Sievepen (TM)**: Terminology has fundamentally different data
requirements (concept-orientation, lifecycle, relations). Separate systems
sharing storage infrastructure is the right balance.

**External terminology server**: Adds deployment complexity and defeats the
single-binary goal.

**TBX as native format**: Verbose, hard to query, lacks performance for
real-time lookup. TBX for import/export only.

**Git-like branching**: Too complex. Streams provide essential what-if
capability without merge conflicts.

## Consequences

- Terminology is first-class in the pipeline, not a bolt-on
- Progressive complexity: CSV glossary to concept management to brand governance
- Shared storage infrastructure (SQLite and PostgreSQL) with TM
  ([AD-009](./009-translation-memory.md)) and Content Store
  ([AD-003](./003-content-store.md))
- Character-level annotation positions enable precise Bowrain highlighting
  ([AD-012](./012-bowrain.md))
- Entity annotation drives both terminology and TM generalization
  ([AD-009](./009-translation-memory.md))
- Terminology streams enable rebranding workflows with content preview
- TBX import/export provides interoperability with all major localization tools
