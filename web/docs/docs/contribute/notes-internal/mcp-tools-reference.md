---
sidebar_position: 13
title: "MCP Tools Reference"
description: Implementation note for AD-013 — the complete reference for the kapi MCP server's tool handlers, their JSON-RPC input and output schemas, and the file locations where each handler is implemented.
keywords: [MCP tools, kapi mcp, tool handlers, JSON-RPC, MCP server reference, implementation note, neokapi]
---

# MCP Tools Reference

This note provides implementation details for [AD-013](/contribute/architecture/013-kapi-cli).

## Kapi MCP Server

Started via `kapi mcp`. Provides ad-hoc file processing tools — no project directory needed.

**Server info:** `{"name": "kapi", "version": "<version>"}`

### `list_formats`

List all supported file formats with their extensions, MIME types, and read/write capabilities.

**Input:** none

**Output:**

```json
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
  ],
  "total": 15
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

Parse a file and extract translatable content blocks with source text and word counts.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to extract content from |
| `format` | string | no | Override format detection |
| `source_lang` | string | no | Source language (default: `en`) |

**Output:**

```json
{
  "format": "json",
  "word_count": 42,
  "blocks": [
    {
      "id": "greeting",
      "source_text": "Hello World",
      "word_count": 2
    }
  ]
}
```

### `word_count`

Count translatable words in a file.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | File path to count words in |
| `format` | string | no | Override format detection |
| `source_lang` | string | no | Source language (default: `en`) |

**Output:**

```json
{
  "format": "json",
  "word_count": 42,
  "block_count": 5
}
```

### `run_flow`

Execute a processing flow on a file. Available flows: `pseudo-translate`, `qa-check`, `segmentation`, `tm-leverage`. AI-powered flows (e.g. `ai-translate`) require API keys and are not available via MCP.

**Input:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `flow_name` | string | yes | Name of the flow (e.g. `pseudo-translate`) |
| `path` | string | yes | Input file path |
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

**Output:**

```json
{
  "flows": [
    { "name": "pseudo-translate", "description": "Generate pseudo-translations for testing" },
    { "name": "qa-check", "description": "Run rule-based quality checks on translations" },
    { "name": "ai-translate", "description": "Translate content using AI/LLM" }
  ],
  "total": 6
}
```

### `list_tools`

List all available processing tools (built-in and plugin-provided).

**Input:** none

**Output:**

```json
{
  "tools": [
    {
      "name": "word-count",
      "description": "Count translatable words in content",
      "source": "built-in"
    }
  ],
  "total": 12
}
```

---

## Implementation Files

| File                              | Purpose                              |
| --------------------------------- | ------------------------------------ |
| `cli/mcp.go`                      | Shared `mcp` subcommand + server bootstrap (`NewMCPCmd`) |
| `kapi/cmd/kapi/root.go`           | Wires the kapi root command, including `mcp`   |
| `kapi/cmd/kapi/mcp_tools.go`      | kapi MCP tool handlers + input/output types (`list_formats`, `detect_format`, `extract_content`, `run_flow`, `list_flows`, `word_count`, `list_tools`, `pseudo_translate`) |
| `kapi/cmd/kapi/mcp_tools_test.go` | Unit tests for kapi MCP handlers     |

## Testing

The MCP handshake can be verified manually:

```bash
echo '{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}},"id":1}' \
  | kapi mcp 2>/dev/null
```
