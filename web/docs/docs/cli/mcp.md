---
sidebar_position: 5
title: MCP Server
description: kapi exposes its file-processing capabilities as an MCP server so AI tools — Claude, Cursor, GitHub Copilot, and other MCP-compatible agents — can parse files, count words, translate, check brand voice, and look up terminology.
keywords: [MCP, Model Context Protocol, kapi mcp, AI tools, Claude, Cursor, brand voice, localization agent]
---

# MCP server

kapi exposes its file-processing capabilities as an [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) server. This lets AI tools like Claude, GitHub Copilot, Cursor, Windsurf, and other MCP-compatible agents parse files, count words, run translation flows, check brand voice, and look up terminology — all through structured tool calls.

For the agent-skills path (Claude Code calling the `kapi` CLI), see [using kapi with Claude](/get-started/use-with-claude). The two can be used together.

## Quick Start

Start the MCP server:

```bash
kapi mcp
```

This launches a JSON-RPC server on stdio. You don't run it manually — your AI tool starts it as a subprocess.

## Setup

### Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Desktop. Kapi tools will appear in the tool picker.

### Claude Code

Add to your project's `.mcp.json` file (or create it at the repository root):

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

Claude Code will automatically discover and connect to the kapi MCP server.

### VS Code (GitHub Copilot / Copilot Chat)

Add to your VS Code settings (`.vscode/settings.json` or user settings):

```json
{
  "mcp": {
    "servers": {
      "kapi": {
        "command": "kapi",
        "args": ["mcp"]
      }
    }
  }
}
```

Or add to `.vscode/mcp.json` in your project:

```json
{
  "servers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

### Cursor

Add to your Cursor MCP config (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

### Windsurf

Add to your Windsurf MCP config (`~/.windsurf/mcp.json`):

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

:::tip
If `kapi` is not in your `$PATH`, use the full path to the binary (e.g. `/usr/local/bin/kapi` or `$HOME/go/bin/kapi`).
:::

## Available Tools

Once connected, your AI assistant can call these tools:

| Tool               | What it does                                                     |
| ------------------ | ---------------------------------------------------------------- |
| `list_formats`     | List all supported file formats with extensions and capabilities |
| `detect_format`    | Detect a file's format from its path                             |
| `extract_content`  | Parse a file and return translatable content blocks              |
| `word_count`       | Count translatable words in a file                               |
| `run_flow`         | Run a processing flow (pseudo-translate, QA check, etc.)         |
| `pseudo_translate` | Pseudo-translate a file for localization QA                      |
| `list_flows`       | List available processing flows                                  |
| `list_tools`       | List available processing tools                                  |
| `brand_guide`      | Render a brand voice guide from a starter pack or profile YAML   |
| `brand_check`      | Score text against a brand voice profile (rule-based)            |
| `brand_rewrite`    | Rewrite text to fix forbidden/competitor terms                   |
| `term_lookup`      | Look up a term in a local termbase                               |
| `tm_search`        | Search a local translation memory                                |

## Example Conversations

### "How many words need translating?"

Ask your AI assistant:

> How many translatable words are in `src/locales/en.json`?

The assistant calls `word_count` with the file path and returns a structured answer with word and block counts.

### "What formats can you handle?"

> What file formats does kapi support?

The assistant calls `list_formats` and returns a table of formats with their extensions, MIME types, and read/write support.

### "Extract the content from this file"

> Show me the translatable strings in `messages.json`

The assistant calls `extract_content`, parses the file, and returns each translatable block with its ID, source text, and word count.

### "Pseudo-translate for QA testing"

> Pseudo-translate `src/locales/en.json` so I can test for UI truncation

The assistant calls `pseudo_translate`, generates a pseudo-translated file with expanded characters, and tells you where the output was written.

### "Run a QA check"

> Run quality checks on my French translation at `src/locales/fr.json`

The assistant calls `run_flow` with `flow_name: "qa-check"` and returns the output file path.

## Tool Reference

### extract_content

Parse a file and return translatable content blocks with source text and word counts.

| Parameter     | Type   | Required | Description                         |
| ------------- | ------ | -------- | ----------------------------------- |
| `path`        | string | yes      | File path to parse                  |
| `format`      | string | no       | Override automatic format detection |
| `source_lang` | string | no       | Source language (default: `en`)     |

### run_flow

Execute a processing flow on a file.

| Parameter     | Type   | Required | Description                                                              |
| ------------- | ------ | -------- | ------------------------------------------------------------------------ |
| `flow_name`   | string | yes      | Flow name: `pseudo-translate`, `qa-check`, `tm-leverage`, `ai-translate-qa` |
| `path`        | string | yes      | Input file path                                                          |
| `source_lang` | string | no       | Source language (default: `en`)                                          |
| `target_lang` | string | yes\*    | Target language (\*optional for `pseudo-translate`, defaults to `qps`)   |
| `output_path` | string | no       | Output file path (default: auto-generated as `<base>_<lang><ext>`)       |

### word_count

| Parameter     | Type   | Required | Description                     |
| ------------- | ------ | -------- | ------------------------------- |
| `path`        | string | yes      | File path to count              |
| `format`      | string | no       | Override format detection       |
| `source_lang` | string | no       | Source language (default: `en`) |

### detect_format

| Parameter | Type   | Required | Description         |
| --------- | ------ | -------- | ------------------- |
| `path`    | string | yes      | File path to detect |

### pseudo_translate

Shorthand for `run_flow` with `pseudo-translate`.

| Parameter     | Type   | Required | Description                           |
| ------------- | ------ | -------- | ------------------------------------- |
| `path`        | string | yes      | File path to pseudo-translate         |
| `target_lang` | string | no       | Target language (default: `qps`)      |
| `output_path` | string | no       | Output path (default: auto-generated) |

## How It Works

Kapi MCP uses the same infrastructure as the CLI commands — `FormatRegistry` for format detection, `Executor` for pipeline orchestration, and the same built-in tools. The MCP server simply exposes these as typed, discoverable tools over the [Model Context Protocol](https://modelcontextprotocol.io/) stdio transport.

No server process, ports, or authentication needed. Your AI tool launches `kapi mcp` as a child process, communicates over stdin/stdout, and shuts it down when the session ends.

:::note
LLM-backed tools like `ai-translate` need API keys and run from the CLI, not over MCP: `kapi ai-translate -i file.json --target-lang fr`. The `brand_check`, `brand_rewrite`, `term_lookup`, and `tm_search` tools above are rule-based and need no key.
:::

## Related

- [Using kapi with Claude](/get-started/use-with-claude) — the agent-skills path.
- [kapi CLI overview](/cli/overview)
- [Run command](/commands?id=run)
- [Formats](/commands?id=formats)
