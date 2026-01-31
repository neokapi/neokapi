---
sidebar_position: 3
title: Translation Memory
---

# Translation Memory

gokapi includes Pensieve, a built-in translation memory (TM) system with fuzzy matching and TMX import/export.

## Storage Backends

- **In-memory** — fast, ephemeral; for session-scoped leverage during batch processing
- **SQLite** — persistent; pure Go with no CGo dependencies; supports import/export of TMX files

## Fuzzy Matching

Pensieve uses Levenshtein edit distance with a configurable threshold (default 75%). Match types:

| Match Type | Description |
|-----------|-------------|
| `exact` | 100% match |
| `fuzzy` | Above threshold (default 75%) |
| `mt` | Machine translation |
| `ai` | AI-generated translation |

## Pipeline Integration

The `tm-leverage` tool queries the TM for each Block's source segments and attaches `AltTranslation` annotations when matches are found:

```bash
kapi flow run --input docs/ --output out/ \
  --tools tm-leverage,ai-translate \
  -s en -t fr \
  --tm translations.tmx
```

TM exact matches skip AI translation, reducing cost and latency.

## TMX Import/Export

```bash
# Import TMX file into a project TM
kapi tm import translations.tmx

# Export project TM as TMX
kapi tm export -o project-tm.tmx
```

## Configuration

```yaml
tools:
  tm-leverage:
    threshold: 0.75
    storage: sqlite
    path: ./project.tm
```
