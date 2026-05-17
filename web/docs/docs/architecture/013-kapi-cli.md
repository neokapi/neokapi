---
id: 013-kapi-cli
sidebar_position: 13
title: "AD-013: Kapi CLI"
---

# AD-013: Kapi CLI

## Summary

`kapi` is a standalone file-processing CLI that demonstrates the neokapi
framework. Most commands are one-shot and require no project; the `-p` flag
enables project mode with a `.kapi` recipe. Kapi shares a command base
`~/.config/kapi/`, and uses an OS-keychain credential store. A `kapi mcp`
subcommand exposes tools over stdio JSON-RPC for AI agents.

## Context

The framework needs a first-class CLI for three audiences:

1. **Engineers running ad-hoc file processing** — "translate this JSON
   bundle to French and write the output." No project, no state, one
   command.
2. **Teams running reproducible workflows** — a saved `.kapi` recipe with
   flows, plugin pinning, language targets, and defaults. Shared via git.
3. **AI agents invoking tools programmatically** — structured discovery
   and typed input/output over MCP.

These audiences overlap in capability (all use formats, tools, flows) but
differ in invocation style. A single binary with progressive complexity
covers all three: run a tool directly, run a flow from a recipe, or
expose the same tools over MCP.

plugins, presets) but is project-sync-centric. The common surface lives in
a shared CLI base; each CLI selects which commands to register and adds
its own behavior.

## Decision

### Binary and module layout

`kapi` is a Go binary at `kapi/cmd/kapi/`, part of the `kapi` module. It
depends on the framework and the shared CLI base (`cli/`). It has no

```
kapi/
├── go.mod                   # module github.com/neokapi/neokapi/kapi
├── cmd/kapi/                # thin root cmd wiring shared CLI commands
└── apps/
    └── kapi-web/            # web UI embedded by `kapi serve`
```

The shared CLI base provides command factories; kapi registers them and
optionally extends with CLI-specific behavior.

### Command surface

Kapi uses [Cobra](https://github.com/spf13/cobra) for hierarchical
subcommands:

```
kapi
├── <tool>                   # run a tool directly (pseudo-translate, ai-translate, …)
├── run FLOW                 # execute a composed flow
├── extract                  # emit XLIFF 2.x / PO for a translator (AD-017)
├── merge                    # apply a translator's returned XLIFF / PO (AD-017)
├── init                     # scaffold a new .kapi project
├── tools                    # list available tools
├── flows                    # list available flows
├── formats
│   └── list                 # list available formats (built-in + plugin)
├── plugins
│   ├── list                 # list installed plugins
│   ├── install              # install a plugin
│   └── update               # update a plugin
├── presets
│   └── list                 # list presets
├── termbase                 # terminology management
│   ├── list
│   ├── lookup
│   └── stats
├── tm                       # translation memory management
│   ├── list
│   ├── import
│   ├── export
│   ├── lookup
│   ├── search
│   ├── stats                # TU counts, locale breakdown, provenance (AD-017)
│   └── audit                # trace a merge batch's TM impact (AD-017)
├── version                  # version info (set via ldflags)
└── mcp                      # MCP server for AI agent integration
```

Commands fall into categories:

- **Format operations** — `kapi formats`, `kapi extract`, `kapi merge`
- **Tools** — `kapi pseudo-translate`, `kapi word-count`, `kapi ai-translate`,
  `kapi mt-translate`, `kapi run <flow>`
- **Plugins** — `kapi plugins list/install/update`
- **Presets** — `kapi presets list`
- **Terminology and TM** — `kapi termbase`, `kapi tm`

Tools run as top-level commands (`kapi pseudo-translate`); composed
multi-tool flows run under `kapi run` (`kapi run ai-translate-qa`). Both
single tools and composed flows appear in `kapi flows` and `kapi tools`
listings depending on where they were defined.

### One-shot and project modes

Most commands are one-shot by default:

```bash
kapi ai-translate -i file.xliff --target-lang fr
kapi pseudo-translate -i file.json --target-lang qps
kapi word-count file.json
```

The `-p` flag switches a command into project mode, loading a `.kapi`
recipe ([AD-008: Kapi Project Model](008-project-model.md)) for defaults:

```bash
kapi run translate -p myproject.kapi
kapi run translate-and-qa -p myproject.kapi --target-lang de
kapi ai-translate -p myproject.kapi
```

With `-p`:

- The flow name is looked up in the project's `flows` map.
- `sourceLocale` and `targetLocales[0]` provide defaults (CLI flags
  override).
- Project defaults (concurrency, parallel_blocks, encoding) apply unless
  overridden.
- Plugin scoping from `AllowedSources` narrows format detection to the
  project's declared plugins.

Every project-aware command resolves `-p` in this order, matching the
git-style semantics a localization engineer expects when running
commands from inside a project tree:

1. Explicit `-p <path>` flag.
2. `KAPI_PROJECT` env var (CI escape hatch).
3. `project.ResolveLayout(cwd)` — walk upward for the `{name}.kapi`
   recipe + adjacent `.kapi/` state directory.
4. Fall through to one-shot mode (for commands that support it) or
   return a "not a kapi project" error (for commands that require
   one — e.g. `kapi merge`).

`ErrAmbiguousLayout` (multiple `*.kapi` files in the same directory)
surfaces as a CLI error asking for explicit `-p`. The resolution
helper lives in `cli/project.go` once and is reused by `run`,
`extract`, `merge`, and any future project-aware command.

### Output format flags

All commands that produce output support consistent format flags through
the shared `cli/output/` package:

```bash
kapi status --json                 # machine-readable JSON
kapi status --text                 # human-readable (default)
kapi status --output-format=json   # explicit long form
kapi status -o json                # short flag form
```

Flag precedence:

1. `--json` — highest precedence (shorthand for common case).
2. `--text` — explicit text.
3. `--output-format=<fmt>` or `-o <fmt>` — explicit selection.
4. Default: `text`.

Supported formats: `text` (tables, formatted text, colors when the
terminal supports them) and `json` (single JSON object or array per
command).

Each command defines its JSON structure as a concrete Go type with
`json` struct tags. Structures must be:

- **Consistent** — the same command always produces the same structure.
- **Complete** — all relevant data, not just what text mode displays.
- **Typed** — correct JSON types (numbers, booleans, arrays; never
  `map[string]any`).

Types implement a `TextFormatter` interface for human-readable output:

```go
type TextFormatter interface {
    FormatText(w io.Writer) error
}
```

Commands without a `TextFormatter` fall back to formatted JSON.

### Exit codes

| Code | Meaning                                        |
| ---- | ---------------------------------------------- |
| 0    | Success                                        |
| 1    | Error (command failed)                         |
| 2    | Conflict (e.g. sync conflict, overwrite block) |

Exit codes are consistent across formats. In JSON mode, errors are
structured objects:

```json
{
  "error": "failed to connect to server",
  "code": "connection_failed"
}
```

### Credential store

The credential store lives in `cli/credentials/` and is shared by kapi and
`~/.config/kapi/providers.json`; API keys are stored in the OS keychain
under the service name `"kapi"`.

Platform backends:

- **macOS** — Keychain via `/usr/bin/security`.
- **Windows** — Credential Manager via Wincred API.
- **Linux** — `libsecret` (GNOME Keyring, KWallet) via
  `github.com/zalando/go-keyring`.

A file-based fallback encrypts secrets with a machine-derived key for
environments without an OS keychain (containers, headless CI).

### App configuration

App configuration uses [Viper](https://github.com/spf13/viper):

```
~/.config/kapi/
├── kapi.yaml                # global settings
├── providers.json           # AI/MT provider configs (keys in keychain)
└── plugins/                 # installed plugins
```

`kapi.yaml` holds defaults for output format, logging level, plugin
directory, telemetry opt-in, and default provider. CLI flags override
Viper values; environment variables (prefix `KAPI_`) override file
values.

### MCP server

The `kapi mcp` subcommand starts an MCP ([Model Context Protocol](https://modelcontextprotocol.io/))
server over stdio JSON-RPC, exposing kapi's file-processing capabilities
to AI agents (Claude Desktop, Cursor, VS Code, etc.).

Tools exposed:

| Tool               | Description                            |
| ------------------ | -------------------------------------- |
| `list_formats`     | List supported file formats            |
| `detect_format`    | Detect format from file path           |
| `extract_content`  | Parse file, return translatable blocks |
| `word_count`       | Count translatable words               |
| `run_flow`         | Execute a processing flow on a file    |
| `pseudo_translate` | Pseudo-translate a file for QA         |
| `list_flows`       | List available processing flows        |
| `list_tools`       | List available processing tools        |

Each tool has typed `Input` and `Output` structs with `json` and
`jsonschema` struct tags. The MCP SDK generates JSON Schema from these
types automatically:

```go
type ExtractContentInput struct {
    Path       string `json:"path" jsonschema:"File path to extract from"`
    Format     string `json:"format,omitempty" jsonschema:"Override format detection"`
    SourceLang string `json:"source_lang,omitempty" jsonschema:"Source language (default: en)"`
}
```

Wiring pattern:

```go
server := mcp.NewServer(&mcp.Implementation{Name: "kapi", Version: version.Version}, nil)
registerKapiTools(server, app)
return server.Run(cmd.Context(), &mcp.StdioTransport{})
```

`PersistentPreRun` initializes `app.FormatReg` and `app.PluginLoader`
before the MCP server starts. Stdout belongs to the MCP transport; `Init`
only writes to stderr.

Client configuration (Claude Desktop):

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

MCP tools reuse the same infrastructure as CLI commands — `FormatRegistry`
for format detection and reader/writer creation, `Executor` for pipeline
orchestration, built-in tool constructors for flow chains. No parallel
implementation.

### Web UI

`kapi serve` starts a local web UI at `kapi/apps/kapi-web/` for browsing
formats, running tools, and inspecting flows. The frontend is embedded at
build time; `make web-build` produces the assets before `make build`.

## Consequences

- A single binary handles one-shot processing, project-based workflows,
  and AI-agent integration without feature flags or separate builds.
  commands for formats, tools, flows, plugins, presets, termbase,
  version.
- Output format consistency makes `jq`, `yq`, and language-specific
  parsers work uniformly across commands.
- OS keychain storage keeps API keys out of environment variables,
  shell history, and project files.
- The MCP server adds AI-agent support with ~5 lines of wiring; no
  shared MCP abstraction is needed across CLIs.
- The absence of a project dependency in one-shot mode keeps kapi usable
  in CI pipelines with no state beyond the input file.
- Exit-code stability means shell scripts can reliably distinguish
  success, failure, and conflict without parsing output.

## Related

- [AD-001: Vision and Modules](001-vision-and-modules.md) — module
  boundaries, `cli/` module
- [AD-004: Processing Engine](004-processing-engine.md) — flow execution
  behind `kapi run`
- [AD-006: Tool System](006-tool-system.md) — Tool pattern exposed as
  top-level commands
- [AD-007: Plugin System](007-plugin-system.md) — `kapi plugins`
- [AD-008: Kapi Project Model](008-project-model.md) — `.kapi` recipe
  loaded by `-p`
- [AD-011: AI Providers](011-ai-providers.md) — provider credentials
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — GUI companion
  command tree with flags
- [MCP Tools Reference](/notes-internal/mcp-tools-reference) — MCP tool
  input/output schemas
