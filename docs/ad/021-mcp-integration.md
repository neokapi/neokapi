---
id: 021-mcp-integration
sidebar_position: 21
title: "AD-021: MCP Integration"
---
# AD-021: MCP Integration

## Context

AI agents (Claude, GPT, Cursor, etc.) benefit from structured access to localization tools and project state. Both the **kapi** and **bowrain** CLIs need a machine-friendly interface that goes beyond shell scripting — one that lets agents discover capabilities, call tools with typed parameters, and receive structured JSON responses.

The [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) is an open standard for exposing tool capabilities to AI agents over a JSON-RPC transport. It provides tool discovery, typed input/output schemas, and a simple stdio transport that integrates with Claude Desktop, VS Code, and other MCP clients.

## Decision

### Two MCP Servers, One Protocol

Each CLI exposes its own MCP server via a `mcp` subcommand:

- **`kapi mcp`** — Ad-hoc file processing tools. No project directory required. Operates on individual files.
- **`bowrain mcp`** — Project management tools. Requires a `.bowrain/` project directory ([AD-016](./016-kapi-project-model.md)). Manages sync state with Bowrain Server.

Both servers use the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk`) and communicate over newline-delimited JSON-RPC on stdio (`StdioTransport`).

### Why Separate Servers

The separation mirrors the existing CLI architecture ([AD-013](./013-cli-and-server.md)):
- **Kapi** = standalone file tool, no project or server dependency
- **Bowrain CLI** = project sync companion, `.bowrain/` context required

Combining them would force agents to reason about when project context is needed. Keeping them separate lets agents connect to the appropriate server based on the task.

### Kapi MCP Tools

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `list_formats` | List supported file formats | — |
| `detect_format` | Detect format from file path | `path` |
| `extract_content` | Parse file, return translatable blocks | `path`, `format?`, `source_lang?` |
| `word_count` | Count translatable words | `path`, `format?`, `source_lang?` |
| `run_flow` | Execute a processing flow on a file | `flow_name`, `path`, `target_lang?`, `output_path?` |
| `pseudo_translate` | Pseudo-translate a file for QA | `path`, `target_lang?`, `output_path?` |
| `list_flows` | List available processing flows | — |
| `list_tools` | List available processing tools | — |

Kapi tools reuse the same infrastructure as CLI commands: `FormatRegistry` for format detection and reader/writer creation, `FlowExecutor` for pipeline orchestration, and built-in tool constructors for flow chains.

### Bowrain CLI MCP Tools

| Tool | Description | Key Parameters |
|------|-------------|----------------|
| `project_config` | Read project configuration | — |
| `project_status` | Show sync status (pending push/pull) | — |
| `project_ls` | List tracked files with optional stats | `paths?[]`, `stats?`, `dirty?` |
| `project_push` | Upload local changes to server | `paths?[]`, `force?`, `dry_run?` |
| `project_pull` | Download translations from server | `locales?[]`, `force?`, `dry_run?` |
| `list_flows` | List available flows (built-in + project) | — |

Bowrain CLI tools reuse `project.FindProject("")`, `project.NewSourceConnector()`, `project.NewLocalConnector()`, and `connector.PushOptions`/`PullOptions` — the same functions as the existing CLI commands.

### Wiring Pattern

Each CLI adds a Cobra `mcp` subcommand. In `RunE`:

```go
server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: version.Version}, nil)
registerKapiTools(server, app)
return server.Run(cmd.Context(), &mcp.StdioTransport{})
```

The `PersistentPreRun` hook initializes `app.FormatReg` and `app.PluginLoader` before the MCP server starts. Stdout is owned by the MCP transport — `Init()` only writes to stderr, which is safe.

### Handler Pattern

Each tool gets typed Input/Output structs with `json` and `jsonschema` struct tags. The MCP SDK generates JSON Schema from these types automatically:

```go
type ExtractContentInput struct {
    Path       string `json:"path" jsonschema:"File path to extract content from"`
    Format     string `json:"format,omitempty" jsonschema:"Override format detection"`
    SourceLang string `json:"source_lang,omitempty" jsonschema:"Source language (default: en)"`
}

func handleExtractContent(ctx context.Context, a *cli.App, input ExtractContentInput) (
    *mcp.CallToolResult, ExtractContentOutput, error) {
    // 1. Detect format (or use override)
    // 2. Create reader via FormatRegistry
    // 3. Open document, iterate blocks
    // 4. Return blocks with IDs, source text, word counts
}
```

### Client Configuration

MCP clients configure the servers in their settings. For Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "kapi": {
      "command": "kapi",
      "args": ["mcp"]
    },
    "bowrain": {
      "command": "bowrain",
      "args": ["mcp"]
    }
  }
}
```

### Dependency Placement

The MCP SDK is added independently to `kapi/go.mod` and `bowrain-cli/go.mod`. No shared `platform/mcp/` package — the server bootstrap is trivial (5 lines) and doesn't warrant a shared abstraction. This preserves the module isolation established in [AD-018](./018-four-module-architecture.md).

See [MCP Tools Reference](/docs/notes/mcp-tools-reference) for the complete tool specifications with input/output schemas.

## Alternatives Considered

- **REST API endpoints**: Would require running a server process. MCP's stdio transport is simpler for agent integration — no port management, no auth, no CORS.

- **Shared `platform/mcp/` package**: The MCP wiring is ~5 lines per CLI. Abstracting it would create a dependency for minimal code reuse and violate the principle that platform should not import the MCP SDK.

- **Single combined server**: Would mix project-aware and standalone tools, requiring agents to handle "no project found" errors for file processing tasks. Separate servers give agents clear scoping.

- **Custom JSON-RPC protocol**: Would require custom client implementations. MCP is an established standard with broad agent support.

## Consequences

- **AI agents gain structured access** to neokapi's format parsing, content extraction, flow execution, and project management capabilities.

- **Tool discovery is automatic** — agents call `tools/list` to see available tools with schemas, enabling self-directed workflows.

- **No new dependencies beyond the MCP SDK** — tools reuse existing CLI infrastructure (FormatRegistry, FlowExecutor, project connectors).

- **Stdio transport is zero-config** — no ports, no auth tokens, no network exposure. The server runs as a child process of the MCP client.

- **Module isolation preserved** — kapi and bowrain add the SDK independently, no shared platform abstraction needed.

- **Extensible** — new tools can be registered by adding handlers to `registerKapiTools()` or `registerBowrainTools()` without protocol changes.
