---
sidebar_position: 3
title: Translation Memory
---

# Translation Memory

neokapi includes **Sievepen**, a built-in content-aware translation memory (TM) system with tiered matching, fuzzy matching, and TMX import/export.

## Content-Aware Matching

Unlike traditional TMs that store plain strings, Sievepen works with the full content model. It stores `Fragment` objects (coded text with inline markup) and supports three matching tiers, tried in order:

| Tier | Match Type      | Description                                                                                                                                                                                  |
| ---- | --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Generalized** | Entity-aware: named entities (people, products, dates) are replaced with typed placeholders. Matches segments with different entity values (e.g., "Welcome, John" matches "Welcome, Alice"). |
| 2    | **Structural**  | Inline-code-aware: inline markup (`<b>`, `<a href>`, etc.) is normalized. Matches segments with different formatting.                                                                        |
| 3    | **Plain**       | Text-only: standard Levenshtein fuzzy matching on plain text.                                                                                                                                |

Each tier can produce exact (100%) or fuzzy matches. When a generalized exact match is found, entity values from the current source are adapted into the stored target.

## Storage Backends

Two storage tiers ship with the framework:

1. **In-memory** (`core/sievepen/`) — fast, ephemeral. Used for session-scoped batch processing.
2. **SQLite** (`cli/storage/sievepen/`) — persistent file-based storage for CLI tools. Designed for single-user, file-based workflows.

All backends implement the same `TranslationMemory` interface and support all matching tiers. The interface supports server-side backends for multi-user deployments with project scoping, streams, and workspace isolation.

## Fuzzy Matching

Sievepen uses Levenshtein edit distance with a configurable threshold (default 70%). Results are sorted by score (highest first) and by match tier (generalized > structural > plain).

## CLI Usage

### Resource Location

All TM commands (except `list`) accept these mutually exclusive flags:

| Flag            | Resolves to                   | Example                    |
| --------------- | ----------------------------- | -------------------------- |
| `--name <n>`    | `~/.config/kapi/tm/<n>.db`    | `--name project-tm`        |
| `--local`       | `./tm.db` (current directory) | `--local`                  |
| `--file <path>` | Explicit file path            | `--file /shared/memory.db` |
| _(no flag)_     | Same as `--local`             |                            |

Databases are created on demand if they don't exist.

### Commands

```bash
# Import TMX
kapi tm import translations.tmx --name project-tm -s en -t fr

# Export TMX
kapi tm export --name project-tm -s en -t fr -o output.tmx

# Look up text
kapi tm lookup "Welcome to our platform" --name project-tm -s en -t fr

# Search entries
kapi tm search "welcome" --name project-tm -s en

# Statistics
kapi tm stats --name project-tm

# List named TMs
kapi tm list
```

## Pipeline Integration

The `tm-leverage` tool queries the TM for each Block's source segments and applies matches:

```bash
# Use a named TM from KAPI_HOME
kapi ai-translate -i input.html -o output.html -s en -t fr \
  --tm project-tm

# TM leverage is automatic when --tm is specified
```

TM exact matches skip AI translation, reducing cost and latency. Fuzzy matches are attached as `AltTranslation` annotations for translator review.

## Configuration

```yaml
tools:
  tm-leverage:
    threshold: 0.70 # minimum match score (0.0-1.0)
    max_results: 10 # maximum matches per block
```

## Design Decision: Separate TM and Termbase

TM and terminology are **separate systems** in neokapi with fundamentally different data shapes:

- **TM entries** are segment pairs (source fragment → target fragment) with inline markup preservation
- **Termbase concepts** are multi-term, multi-locale knowledge units with lifecycle statuses

The `Block` annotation system serves as the integration point: both TM matches and term matches are attached as annotations during pipeline processing, making them available to any downstream tool or editor.
