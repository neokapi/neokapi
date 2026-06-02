---
sidebar_position: 6
title: MCP Server
---

# Using the bowrain plugin with AI assistants

kapi (with the bowrain plugin) exposes project management capabilities as an [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) server. This lets AI tools like Claude, GitHub Copilot, Cursor, Windsurf, and other MCP-compatible agents check project status, list tracked files, push and pull translations, and manage flows — all through structured tool calls.

## Quick Start

Start the MCP server:

```bash
kapi mcp
```

This launches a JSON-RPC server on stdio. You don't run it manually — your AI tool starts it as a subprocess. The server requires a `.kapi` project (it walks upward looking for a `*.kapi` recipe, like git).

:::tip
For ad-hoc file processing without a project, use the [Kapi MCP server](https://neokapi.github.io/web/neokapi/docs/kapi-cli/mcp) instead.
:::

## Setup

### Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "bowrain": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Desktop. The bowrain plugin's tools will appear in the tool picker.

**Using both servers together:** You can register kapi and bowrain side by side. Use kapi for standalone file operations and bowrain for project workflows:

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    },
    "bowrain": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

### Claude Code

Add to your project's `.mcp.json` file (or create it at the repository root):

```json
{
  "mcpServers": {
    "bowrain": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

Claude Code will automatically discover and connect to the bowrain MCP server.

### VS Code (GitHub Copilot / Copilot Chat)

Add to `.vscode/mcp.json` in your project:

```json
{
  "servers": {
    "bowrain": {
      "command": "kapi",
      "args": ["mcp"]
    }
  }
}
```

Or add to your VS Code settings (`.vscode/settings.json`):

```json
{
  "mcp": {
    "servers": {
      "bowrain": {
        "command": "kapi",
        "args": ["mcp"]
      }
    }
  }
}
```

### Cursor

Add to your Cursor MCP config (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "bowrain": {
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
    "bowrain": {
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

| Tool             | What it does                                                          |
| ---------------- | --------------------------------------------------------------------- |
| `project_config` | Read project configuration from the `.kapi` recipe                    |
| `project_status` | Show sync status — pending push/pull counts, server connection        |
| `project_ls`     | List tracked files with optional stats (word counts, dirty detection) |
| `project_push`   | Upload local changes to Bowrain Server                                |
| `project_pull`   | Download translations from Bowrain Server                             |
| `list_flows`     | List available flows (built-in and project-defined)                   |

## Example Conversations

### "What's the state of my project?"

Ask your AI assistant:

> What's the translation status of this project?

The assistant calls `project_status` and returns a summary: how many files, words, and blocks are tracked, how many changes are pending push or pull, and whether the project is synced with the server.

### "Which files have changed?"

> Which localization files have local changes?

The assistant calls `project_ls` with `dirty: true` and returns only files with uncommitted changes, along with block and word counts.

### "How big is this project?"

> How many words and files are in this translation project?

The assistant calls `project_ls` with `stats: true` and returns a breakdown of every tracked file with block counts, word counts, and totals.

### "Show me the project config"

> What locales are configured for this project?

The assistant calls `project_config` and returns the project name, source locale, target locales, server URL, and file mapping count.

### "Push my changes"

> Push the latest translation changes to the server

The assistant calls `project_push` and returns how many blocks were uploaded, the word count, and how many files were scanned.

### "Pull latest translations"

> Pull the latest French and German translations from the server

The assistant calls `project_pull` with `locales: ["fr", "de"]` and returns how many blocks were downloaded and files were updated.

### "Preview before pushing"

> What would get pushed if I push now? Don't actually push yet.

The assistant calls `project_push` with `dry_run: true` and shows what would be uploaded without making any changes.

## Tool Reference

### project_status

Show project sync status. Returns local project info when no server is configured.

No parameters.

### project_config

Read project configuration from the `.kapi` recipe at the project root.

No parameters.

### project_ls

List files tracked by the project.

| Parameter | Type     | Required | Description                            |
| --------- | -------- | -------- | -------------------------------------- |
| `paths`   | string[] | no       | Filter by path prefixes                |
| `stats`   | bool     | no       | Include block and word counts per file |
| `dirty`   | bool     | no       | Show only files with local changes     |

### project_push

Upload local changes to Bowrain Server.

| Parameter | Type     | Required | Description                                 |
| --------- | -------- | -------- | ------------------------------------------- |
| `paths`   | string[] | no       | Specific file paths to push (default: all)  |
| `force`   | bool     | no       | Re-upload everything even if unchanged      |
| `dry_run` | bool     | no       | Show what would be uploaded without sending |

### project_pull

Download translations from Bowrain Server.

| Parameter | Type     | Required | Description                                  |
| --------- | -------- | -------- | -------------------------------------------- |
| `locales` | string[] | no       | Languages to download (e.g. `["fr", "de"]`)  |
| `force`   | bool     | no       | Re-download everything even if unchanged     |
| `dry_run` | bool     | no       | Show what would change without writing files |

### list_flows

List available processing flows. Returns both built-in flows and project-defined flows (inline on the recipe and from `.kapi/flows/`).

No parameters.

## How It Works

No server process, ports, or additional authentication is needed. Your AI tool starts `kapi mcp` as a subprocess, communicates over stdin/stdout, and shuts it down when the session ends. It discovers your project the same way the CLI does — by walking up the directory tree to find the nearest `*.kapi` recipe.

## Related

- [CLI Overview](/cli/overview)
- [Project Model](/cli/project-model)
- [Commands Reference](/cli/commands/init)
- [kapi MCP Server](https://neokapi.github.io/web/neokapi/docs/kapi-cli/mcp) — for standalone file processing
