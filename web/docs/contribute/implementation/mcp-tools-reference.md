---
sidebar_position: 13
title: "MCP Tools Reference"
description: Implementation note for AD-013 — the complete reference for the kapi MCP server's tool handlers, their JSON-RPC input and output schemas, and the file locations where each handler is implemented.
keywords: [MCP tools, kapi mcp, tool handlers, JSON-RPC, MCP server reference, implementation note, neokapi]
---

# MCP Tools Reference

This note provides implementation details for [AD-013](/contribute/architecture/013-kapi-cli).

## Kapi MCP Server

Started via `kapi mcp`. The tools default to ad-hoc single-file processing, but optionally accept a `project` (`kapi.yaml`) file for project-scoped defaults and content resolution.

**Server info:** `{"name": "kapi", "version": "<version>"}`

### `list_formats`

List all supported file formats with their extensions, MIME types, and read/write capabilities.

**Input:** none

**Output** (one element shown; `total` is `len(formats)`, set at runtime from
the live registry, so the real value tracks the registered formats — see the
generated [Format Reference](/reference/formats/html)):

```jsonc
{
  "formats": [
    {
      "name": "json",
      "display_name": "JSON",
      "extensions": [".json"],
      "mime_types": ["application/json"],
      "has_reader": true,
      "has_writer": true,
      "source": "built-in"
    }
    // …one entry per registered format
  ],
  "total": 0 // = len(formats), runtime-dependent
}
```

### `detect_format`

Detect the file format from a file path based on its extension.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to detect format from |

**Output:**

```json
{
  "format": "json",
  "extensions": [".json"],
  "has_reader": true,
  "has_writer": true
}
```

### `extract_content`

Parse a file into translatable content blocks — the read leg of the edit loop.
Each block carries its `id`, its `content_hash` (canonical identity over the
plain source text, the drift anchor), its `source_text` with inline codes
rendered as `<x id="…"/>` placeholders, and its `word_count`. Pair it with
`apply_edits` (or `kapi apply`) to round-trip an edit faithfully.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to extract content from |
| `format` | string | no | Override format detection |
| `source_lang` | string | no | Source language (default: `en`) |
| `project` | string | no | Path to `kapi.yaml` project file for scoped format detection |

**Output:**

```json
{
  "format": "json",
  "word_count": 42,
  "blocks": [
    {
      "id": "greeting",
      "content_hash": "a3f82c…",
      "source_text": "Hello <x id=\"1\"/>World<x id=\"/1\"/>",
      "word_count": 2
    }
  ]
}
```

### `apply_edits`

Apply a typed change-set — the one write verb, the write leg of the edit loop.
Content edits land through the byte-faithful round-trip (structure and inline
codes preserved, drift-guarded by `content_hash`); asset edits (`term`, `tm`,
`brand`, `recipe`) are written to their committed source artifact and compiled
into the cache. No AI provider is used.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `changeset` | array | yes | Typed change-set entries (`kind`: `content` / `term` / `tm` / `brand` / `recipe`) |

**Output:** `ok` plus the per-block content outcome (`applied` / `skipped` / `stale` / `guard_failed`) and a per-entry `assets` result. `ok` is false when an edit drifted or was rejected, signalling the caller to re-read and retry.

### `stats`

Size files before processing them: per-file and total content metrics —
blocks (translatable and not), words, characters (with and without spaces,
plus the unique-character inventory), segments when available, and a by-role
breakdown. Returns the same JSON `kapi stats --json` emits.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `files` | array | yes | Paths of the files to summarize |
| `format` | string | no | Input format override applied to every file (default: auto-detect) |

**Output:**

```json
{
  "files": [
    {
      "file": "messages.json",
      "blocks": 5,
      "translatable": 5,
      "non_translatable": 0,
      "words": 42,
      "characters": 230,
      "characters_no_space": 195,
      "unique_characters": 31,
      "segments": 0
    }
  ],
  "total": { "blocks": 5, "translatable": 5, "non_translatable": 0, "words": 42, "characters": 230, "characters_no_space": 195, "unique_characters": 31, "segments": 0 }
}
```

### `run_flow`

Execute a processing flow on a file. The flow name is any built-in flow from `list_flows` (e.g. `pseudo-translate`, `qa`, `recycle`, `translate-qa`, `secure-translate`). AI-powered flows (e.g. `translate`, `translate-qa`) run only when the required provider API keys are configured.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `flow_name` | string | yes | Name of the flow (e.g. `pseudo-translate`) |
| `path` | string | yes* | Input file path (*optional when a `project` file with content patterns resolves the inputs) |
| `project` | string | no | Path to a `kapi.yaml` project file for project-scoped execution (resolves inputs from content patterns) |
| `source_lang` | string | no | Source language (default: `en`) |
| `target_lang` | string | yes* | Target language (*optional for `pseudo-translate`, defaults to `qps`) |
| `output_path` | string | no | Output file path (default: auto-generated as `<base>_<lang><ext>`) |

**Output:**

```json
{
  "flow_name": "pseudo-translate",
  "input_path": "locales/en.json",
  "output_path": "locales/en_qps.json"
}
```

### `pseudo_translate`

Shorthand for `run_flow` with `flow_name: "pseudo-translate"`. Pseudo-translates a file so QA can test it before real translations exist.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to pseudo-translate |
| `target_lang` | string | no | Target language (default: `qps`) |
| `output_path` | string | no | Output file path (default: auto-generated) |

**Output:** same as `run_flow`.

### `list_flows`

List all available processing flows.

**Input:** none

**Output** (illustrative selection; `total` is `len(flowdef.BuiltInFlows())`):

```jsonc
{
  "flows": [
    { "name": "pseudo-translate", "description": "Generate pseudo-translations for testing" },
    { "name": "qa", "description": "Run rule-based quality checks on translations" },
    { "name": "translate", "description": "Translate content using AI/LLM" }
    // …one entry per built-in flow
  ],
  "total": 0 // = len(flowdef.BuiltInFlows()), runtime-dependent
}
```

### `list_tools`

List all available processing tools (built-in and plugin-provided).

**Input:** none

**Output** (one element shown; `total` is `len(tools)`, runtime-dependent —
see the generated [Tool Reference](/reference/tools/translate)):

```jsonc
{
  "tools": [
    {
      "name": "pseudo-translate",
      "description": "Generate pseudo-translations for testing",
      "source": "built-in"
    }
    // …one entry per registered tool
  ],
  "total": 0 // = len(tools), runtime-dependent
}
```

## Brand, terminology, and content-memory tools

The host runtime (`host/mcp_brand.go`) registers a further set of offline tools
on the same `mcp` stdio server via `RegisterMCPToolFactory`, so any binary built
on the shared base (including kapi) exposes them, and non-Claude MCP clients get
local parity with the brand tools. All run offline against local files and
SQLite stores.

### `brand_check`

Score text against a brand voice profile using deterministic vocabulary
rules; returns a 0–100 compliance score and findings.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | yes | The text to check |
| `profile_pack` | string | one of pack/file | Starter pack name |
| `profile_file` | string | one of pack/file | Path to a profile YAML |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Resolved profile name |
| `score` | int | Overall 0–100 compliance score |
| `dimensions` | array | Per-dimension scores |
| `findings` | array | Vocabulary findings |

### `brand_rewrite`

Rewrite text to comply with a brand voice profile by substituting
forbidden/competitor terms (deterministic, offline).

**Input:** same as `brand_check`.

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Resolved profile name |
| `original` | string | Input text |
| `rewritten` | string | Rewritten text |
| `changes` | array | `{from, to, count}` substitutions made |

## Retired: `brand_guide`, `term_lookup`, `tm_search`

All three are replaced by **`context_search`** (AD-037). They were
asset-shaped — one call per store — which forced a caller to know where an
answer lived before it could ask, and returned partial answers that read as
whole ones: `brand_guide` rendered a profile's own vocabulary while the
project's terms store went unread.

`context_search` asks the question once and answers from every store the
project binds, grouped by kind, and says what it could not reach.

`kapi brand guide` remains as a CLI verb: a human asking to see a profile
rendered is a reasonable thing to type.
