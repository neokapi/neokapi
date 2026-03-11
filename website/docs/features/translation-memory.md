---
sidebar_position: 3
title: Translation Memory
---

# Translation Memory

gokapi includes **Sievepen**, a built-in content-aware translation memory (TM) system with tiered matching, fuzzy matching, and TMX import/export.

## Content-Aware Matching

Unlike traditional TMs that store plain strings, Sievepen works with the full content model. It stores `Fragment` objects (coded text with inline markup) and supports three matching tiers, tried in order:

| Tier | Match Type | Description |
|------|-----------|-------------|
| 1 | **Generalized** | Entity-aware: named entities (people, products, dates) are replaced with typed placeholders. Matches segments with different entity values (e.g., "Welcome, John" matches "Welcome, Alice"). |
| 2 | **Structural** | Inline-code-aware: inline markup (`<b>`, `<a href>`, etc.) is normalized. Matches segments with different formatting. |
| 3 | **Plain** | Text-only: standard Levenshtein fuzzy matching on plain text. |

Each tier can produce exact (100%) or fuzzy matches. When a generalized exact match is found, entity values from the current source are adapted into the stored target.

## Storage Backends

- **In-memory** — fast, ephemeral; ideal for session-scoped leverage during batch processing
- **SQLite** — persistent; pure Go implementation (no CGo); supports import/export of TMX files

Both backends implement the same `TranslationMemory` interface and support all matching tiers.

## Fuzzy Matching

Sievepen uses Levenshtein edit distance with a configurable threshold (default 70%). Results are sorted by score (highest first) and by match tier (generalized > structural > plain).

## Pipeline Integration

The `tm-leverage` flow queries the TM for each Block's source segments and applies matches:

```bash
kapi flow run tm-leverage -i input.html -o output.html --source-lang en --target-lang fr
```

TM exact matches skip AI translation, reducing cost and latency. Fuzzy matches are attached as `AltTranslation` annotations for translator review.

## Configuration

```yaml
tools:
  tm-leverage:
    threshold: 0.70     # minimum match score (0.0-1.0)
    max_results: 10     # maximum matches per block
    storage: sqlite     # "memory" or "sqlite"
    path: ./project.tm  # SQLite database path
```

## Design Decision: Separate TM and Termbase

TM and terminology are **separate systems** in gokapi with fundamentally different data shapes:

- **TM entries** are segment pairs (source fragment → target fragment) with inline markup preservation
- **Termbase concepts** are multi-term, multi-locale knowledge units with lifecycle statuses

The `Block` annotation system serves as the integration point: both TM matches and term matches are attached as annotations during pipeline processing, making them available to any downstream tool or editor.
