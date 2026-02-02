---
id: 010-translation-memory
sidebar_position: 10
title: "ADR-010: Translation Memory"
---
# ADR-010: Pensieve built-in translation memory

## Context

Translation memory (TM) is essential for localization: previously translated
segments are reused to maintain consistency and reduce cost. Okapi's TM
support relies on external tools (Olifant, Trados). We wanted TM to be
built-in and usable from CLI, server, and desktop app without external
dependencies.

## Decision

### Storage Backends

Implement the Pensieve translation memory library (`lib/pensieve/`) with two
storage backends:

1. **In-memory**: fast, ephemeral; for session-scoped leverage during batch
   processing.
2. **SQLite** (via `modernc.org/sqlite`): persistent; supports import/export
   of TMX files; pure Go with no CGo dependencies.

### Fuzzy Matching

Levenshtein edit distance with a configurable threshold (default 75%). Match
types are classified as: exact, fuzzy, MT, or AI.

### Pipeline Integration

TM is integrated into the pipeline via the `tm-leverage` tool, which queries
the TM for each Block's source segments and attaches `AltTranslation`
annotations when matches are found. Downstream tools (AI translate, QA) can
use these annotations for context.

### TMX Import/Export

Standard TMX format for interoperability with other TM systems.

## Alternatives Considered

- **External TM server** (e.g., Moses, Trados): adds deployment complexity;
  defeats the single-binary goal.
- **BoltDB / BadgerDB**: key-value stores lack the query flexibility needed
  for fuzzy matching.
- **PostgreSQL**: overkill for local TM; requires external service.
- **`mattn/go-sqlite3`**: CGo dependency; breaks cross-compilation. Chose
  `modernc.org/sqlite` (pure Go) instead.

## Consequences

- TM works out of the box with zero external dependencies
- TMX import/export enables interchange with other TM systems
- SQLite backend persists across sessions; suitable for project-level TM
- Levenshtein fuzzy matching is language-independent but not linguistically
  aware (no stemming); acceptable for localization where exact and near-exact
  matches dominate
- The `tm-leverage` tool composes with AI tools: TM exact matches skip AI
  translation, reducing cost and latency
