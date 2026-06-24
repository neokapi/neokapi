---
sidebar_position: 13
title: "MCP Tools Reference"
description: Implementation note for AD-013 — the complete reference for the kapi MCP server's tool handlers, their JSON-RPC input and output schemas, and the file locations where each handler is implemented.
keywords: [MCP tools, kapi mcp, tool handlers, JSON-RPC, MCP server reference, implementation note, neokapi]
---

# MCP Tools Reference

This note provides implementation details for [AD-013](/contribute/architecture/013-kapi-cli).

## Kapi MCP Server

Started via `kapi mcp`. The tools default to ad-hoc single-file processing, but optionally accept a `project` (`.kapi`) file for project-scoped defaults and content resolution.

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
| `project` | string | no | Path to `.kapi` project file for scoped format detection |

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

### `word_count`

Count translatable words in a file.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to count words in |
| `format` | string | no | Override format detection |
| `source_lang` | string | no | Source language (default: `en`) |
| `project` | string | no | Path to `.kapi` project file for scoped format detection |

**Output:**

```json
{
  "format": "json",
  "word_count": 42,
  "block_count": 5
}
```

### `run_flow`

Execute a processing flow on a file. The flow name is any built-in flow from `list_flows` (e.g. `pseudo-translate`, `qa`, `recycle`, `translate-qa`, `secure-translate`). AI-powered flows (e.g. `translate`, `translate-qa`) run only when the required provider API keys are configured.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `flow_name` | string | yes | Name of the flow (e.g. `pseudo-translate`) |
| `path` | string | yes* | Input file path (*optional when a `project` file with content patterns resolves the inputs) |
| `project` | string | no | Path to a `.kapi` project file for project-scoped execution (resolves inputs from content patterns) |
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

Shorthand for `run_flow` with `flow_name: "pseudo-translate"`. Pseudo-translates a file for localization QA testing.

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

**Output** (illustrative selection; `total` is `len(flow.BuiltInFlows())`):

```jsonc
{
  "flows": [
    { "name": "pseudo-translate", "description": "Generate pseudo-translations for testing" },
    { "name": "qa", "description": "Run rule-based quality checks on translations" },
    { "name": "translate", "description": "Translate content using AI/LLM" }
    // …one entry per built-in flow
  ],
  "total": 0 // = len(flow.BuiltInFlows()), runtime-dependent
}
```

### `list_tools`

List all available processing tools (built-in and plugin-provided).

**Input:** none

**Output** (one element shown; `total` is `len(tools)`, runtime-dependent —
see the generated [Tool Reference](/reference/tools/word-count)):

```jsonc
{
  "tools": [
    {
      "name": "word-count",
      "description": "Count translatable words in content",
      "source": "built-in"
    }
    // …one entry per registered tool
  ],
  "total": 0 // = len(tools), runtime-dependent
}
```

## Brand, terminology, and TM tools

The shared CLI base (`cli/mcp_brand.go`) registers a further set of offline
tools on the same `mcp` stdio server via `RegisterMCPToolFactory`, so any
binary built on the CLI base (including kapi) exposes them, so non-Claude MCP
clients get local parity with the brand tools. All run offline against local
files and SQLite stores.

### `brand_guide`

Render a brand voice guide (markdown) from a starter pack or a profile YAML,
to inject into context before generating content.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `profile_pack` | string | one of pack/file | Starter pack name (e.g. `marketing-blog`, `technical-docs`) |
| `profile_file` | string | one of pack/file | Path to a profile YAML |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `profile` | string | Resolved profile name |
| `guide` | string | Rendered markdown voice guide |

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

### `term_lookup`

Look up a term in a local termbase to enforce consistent terminology.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `term` | string | yes | The term to look up |
| `source_lang` | string | no | Source locale (e.g. `en`) |
| `target_lang` | string | no | Target locale (e.g. `fr`) |
| `termbase` | string | no | Path to the termbase db (default: `termbase.db`) |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `matches` | array | `{term, locale, status, match_type}` entries |
| `total` | int | `len(matches)` |

### `tm_search`

Search a local translation memory for prior translations of source text.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | yes | Source text to search for |
| `source_lang` | string | yes | Source locale (e.g. `en`) |
| `target_lang` | string | yes | Target locale (e.g. `fr`) |
| `min_score` | number | no | Minimum match score 0–1 (default: `0.7`) |
| `tm` | string | no | Path to the TM db (default: `tm.db`) |

**Output:**
| Field | Type | Description |
|-------|------|-------------|
| `matches` | array | `{source, target, score, match_type}` entries (max 10) |
| `total` | int | `len(matches)` |

---

## Implementation Files

| File                              | Purpose                              |
| --------------------------------- | ------------------------------------ |
| `cli/mcp.go`                      | Shared `mcp` subcommand + server bootstrap (`NewMCPCmd`) |
| `cli/mcp_brand.go`                | Shared brand/terminology/TM MCP tools registered via `RegisterMCPToolFactory` (`brand_guide`, `brand_check`, `brand_rewrite`, `term_lookup`, `tm_search`) + their input/output types |
| `kapi/cmd/kapi/root.go`           | Wires the kapi root command, including `mcp`   |
| `kapi/cmd/kapi/mcp_tools.go`      | kapi MCP tool handlers + input/output types (`list_formats`, `detect_format`, `extract_content`, `run_flow`, `list_flows`, `word_count`, `list_tools`, `pseudo_translate`) |
| `kapi/cmd/kapi/mcp_tools_test.go` | Unit tests for kapi MCP handlers     |

## Testing

The MCP handshake can be verified manually:

```bash
echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}' \
  | kapi mcp 2>/dev/null
```
