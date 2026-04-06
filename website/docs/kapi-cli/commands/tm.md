---
sidebar_position: 7
title: tm
---

# kapi tm

Manage translation memory. Import/export TMX files, look up translations, search entries, and view statistics.

## Synopsis

```bash
kapi tm import <file> [flags]
kapi tm export [flags]
kapi tm lookup <text> [flags]
kapi tm search <query> [flags]
kapi tm stats [flags]
kapi tm list
```

## Description

A translation memory (TM) stores previously translated segments as a SQLite database. kapi's TM is content-aware: it stores full Fragments with inline markup and supports tiered matching (generalized, structural, plain) with entity adaptation.

## Resource Location

All TM commands (except `list`) accept these mutually exclusive flags for specifying which TM to use:

| Flag            | Resolves to                   | Example                    |
| --------------- | ----------------------------- | -------------------------- |
| `--name <n>`    | `~/.config/kapi/tm/<n>.db`    | `--name project-tm`        |
| `--local`       | `./tm.db` (current directory) | `--local`                  |
| `--file <path>` | Explicit file path            | `--file /shared/memory.db` |
| _(no flag)_     | Same as `--local`             |                            |

Databases are created on demand if they don't exist.

## Commands

### import

Import a TMX file into translation memory:

```bash
# Import into a named TM in KAPI_HOME
kapi tm import translations.tmx --name project-tm -s en -t fr

# Import into default local TM (./tm.db)
kapi tm import translations.tmx -s en -t fr

# Import into a specific file
kapi tm import translations.tmx --file /shared/memory.db -s en -t fr
```

### export

Export translation memory to TMX:

```bash
kapi tm export --name project-tm -s en -t fr -o backup.tmx
kapi tm export -s en -t fr -o translations.tmx
```

### lookup

Look up text in translation memory:

```bash
kapi tm lookup "Welcome to our platform" --name project-tm -s en -t fr
kapi tm lookup "Click here to continue" -s en -t fr --min-score 0.8
```

### search

Search translation memory entries:

```bash
kapi tm search "welcome" --name project-tm -s en -t fr
kapi tm search "authentication" -s en --limit 50
```

### stats

Show translation memory statistics:

```bash
kapi tm stats --name project-tm
kapi tm stats                      # uses ./tm.db
```

### list

List all named TMs in KAPI_HOME:

```bash
kapi tm list
```

## Use in Tool Commands

TM leverage can be integrated into translation commands using the `--tm` flag:

```bash
# Use a named TM for pre-filling translations
kapi tm-leverage -i input.html -o output.html -s en -t fr \
  --tm project-tm

# Combine TM + AI translation
kapi ai-translate -i input.html -o output.html -s en -t fr \
  --tm project-tm --termbase project-terms
```

## Match Types

kapi's TM uses a 6-tier matching pipeline, tried in order of reuse potential:

| Tier | Type              | Description                                           |
| ---- | ----------------- | ----------------------------------------------------- |
| 1    | Generalized Exact | Entity values replaced with placeholders (100% match) |
| 2    | Structural Exact  | Inline markup normalized (100% match)                 |
| 3    | Plain Exact       | Raw text exact match (100% match)                     |
| 4    | Generalized Fuzzy | Levenshtein with entity normalization                 |
| 5    | Structural Fuzzy  | Levenshtein with markup normalization                 |
| 6    | Plain Fuzzy       | Levenshtein on raw text                               |

See [Translation Memory features](/docs/features/translation-memory) for details.
