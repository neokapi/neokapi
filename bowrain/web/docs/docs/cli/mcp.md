---
sidebar_position: 6
title: MCP Server
---

# Using the bowrain plugin with AI assistants

kapi (with the bowrain plugin) exposes project management capabilities as an [MCP](https://modelcontextprotocol.io/) (Model Context Protocol) server. This lets AI tools like Claude, GitHub Copilot, Cursor, Windsurf, and other MCP-compatible agents check project status, list tracked files, push and pull, manage flows, and consult the workspace's context graph, all through structured tool calls.

## Quick start

Start the MCP server:

```bash
kapi mcp
```

This launches a JSON-RPC server on stdio. You don't run it manually; your AI tool starts it as a subprocess. With the bowrain plugin installed, the server's tool set includes the project and context tools below. It requires a `.kapi` project (it walks upward looking for a `kapi.yaml` recipe, like git).

:::tip
For ad-hoc file processing without a project, the same `kapi mcp` serves the [kapi MCP tools](https://neokapi.github.io/reference/mcp).
:::

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

Restart Claude Desktop. The tools appear in the tool picker.

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

Claude Code discovers and connects to the server automatically.

### VS Code (GitHub Copilot / Copilot Chat)

Add to `.vscode/mcp.json` in your project:

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

Or add to your VS Code settings (`.vscode/settings.json`):

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
If `kapi` is not in your `$PATH`, use the full path to the binary (for example `/usr/local/bin/kapi` or `$HOME/go/bin/kapi`).
:::

## Available tools

Once connected, your AI assistant can call these tools:

| Tool                | What it does                                                                          |
| ------------------- | ------------------------------------------------------------------------------------- |
| `project_config`    | Read project configuration from the `kapi.yaml` recipe                                |
| `project_status`    | Show sync status: pending push/pull counts, server connection                         |
| `project_ls`        | List tracked files with optional stats (word counts, dirty detection)                 |
| `project_push`      | Upload local changes to Bowrain Server                                                |
| `project_pull`      | Download results from Bowrain Server                                                  |
| `list_flows`        | List available flows (built-in and project-defined)                                   |
| `concept_search`    | Search the workspace context graph for governed concepts                              |
| `concept_story`     | Show the chronological timeline of a governed concept                                 |
| `experiment_status` | Report context-graph change-sets, with detail and blast radius for one change-set     |

The three concept tools read the workspace [context graph](/server/context); they require a project connected to a workspace on a Bowrain server.

## Example conversations

### "What's the state of my project?"

Ask your AI assistant:

> What's the status of this project?

The assistant calls `project_status` and returns a summary: how many files, words, and blocks are tracked, how many changes are pending push or pull, and whether the project is synced with the server.

### "Which files have changed?"

> Which content files have local changes?

The assistant calls `project_ls` with `dirty: true` and returns only files with uncommitted changes, along with block and word counts.

### "How big is this project?"

> How many words and files are in this project?

The assistant calls `project_ls` with `stats: true` and returns a breakdown of every tracked file with block counts, word counts, and totals.

### "Show me the project config"

> What locales are configured for this project?

The assistant calls `project_config` and returns the project name, source locale, target locales, server URL, and file mapping count.

### "Push my changes"

> Push the latest changes to the server

The assistant calls `project_push` and returns how many blocks were uploaded, the word count, and how many files were scanned.

### "Pull latest results"

> Pull the latest French and German results from the server

The assistant calls `project_pull` with `locales: ["fr", "de"]` and returns how many blocks were downloaded and files were updated.

### "Preview before pushing"

> What would get pushed if I push now? Don't actually push yet.

The assistant calls `project_push` with `dry_run: true` and shows what would be uploaded without making any changes.

### "Is this term allowed here?"

> Is "e-shop" a term we still use?

The assistant calls `concept_search` with the query and reads the concept's status, and `concept_story` for how it got there.

## Tool reference

### project_status

Show project sync status. Returns local project info when no server is configured.

No parameters.

### project_config

Read project configuration from the `kapi.yaml` recipe at the project root.

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

Download results from Bowrain Server.

| Parameter | Type     | Required | Description                                  |
| --------- | -------- | -------- | -------------------------------------------- |
| `locales` | string[] | no       | Languages to download (for example `["fr", "de"]`) |
| `force`   | bool     | no       | Re-download everything even if unchanged     |
| `dry_run` | bool     | no       | Show what would change without writing files |

### list_flows

List available processing flows. Returns both built-in flows and project-defined flows (inline on the recipe and from `.kapi/flows/`).

No parameters.

### concept_search

Search the workspace [context graph](/server/context) for governed concepts (terms, status, domain) matching a query.

| Parameter | Type   | Required | Description                                                              |
| --------- | ------ | -------- | ------------------------------------------------------------------------ |
| `query`   | string | no       | Free-text query against the term text                                    |
| `status`  | string | no       | Filter by term lifecycle status (preferred, admitted, deprecated, forbidden) |
| `market`  | string | no       | Filter by market validity tag                                            |
| `domain`  | string | no       | Filter by subject-field domain                                           |
| `limit`   | int    | no       | Maximum number of concepts to return (default 50)                        |

### concept_story

Show the chronological timeline of a governed concept: revisions, observations, comments, and change-sets.

| Parameter    | Type   | Required | Description                          |
| ------------ | ------ | -------- | ------------------------------------ |
| `concept_id` | string | yes      | The concept ID whose timeline to fetch |

### experiment_status

Report context-graph change-sets. With a `changeset_id`, returns that change-set's detail and a blast-radius summary (affected blocks, new violations, resolved violations, words); without one, lists the workspace's change-sets.

| Parameter      | Type   | Required | Description                                                            |
| -------------- | ------ | -------- | ---------------------------------------------------------------------- |
| `changeset_id` | string | no       | A change-set ID to detail; omit to list all change-sets                |
| `status`       | string | no       | When listing, filter by status (draft, in_review, approved, merged, abandoned) |

## How it works

No server process, ports, or additional authentication is needed. Your AI tool starts `kapi mcp` as a subprocess, communicates over stdin/stdout, and shuts it down when the session ends. It discovers your project the same way the CLI does, by walking up the directory tree to find the nearest `kapi.yaml` recipe.

## Related

- [CLI Overview](/cli/overview)
- [Project Model](/cli/project-model)
- [Commands Reference](/cli/commands/init)
- [kapi MCP Server](https://neokapi.github.io/reference/mcp): the file-processing and context tools
