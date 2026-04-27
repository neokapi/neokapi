# Plugin protocol v1

This is the language-neutral specification for plugins targeting kapi's
unified plugin model (issue #438). Plugin authors implement against this
spec in any language. A worked Go example lives in
[`examples/plugins/hello/`](../../examples/plugins/hello/).

## Manifest

Every plugin ships a `manifest.json` at the root of its install directory.
The schema is embedded in `core/plugin/manifest/schema.json` and described
by the Go types in `core/plugin/manifest/manifest.go`.

```json
{
  "manifest_version": "1",
  "plugin": "<name>",
  "version": "<semver>",
  "binary": "<executable-name>",
  "license": "<SPDX>",
  "min_kapi_version": "<semver>",
  "capabilities": {
    "commands": [...],
    "mcp_tools": [...],
    "formats": [...],
    "tools": [...],
    "source_connectors": [...],
    "schema_extensions": [...]
  },
  "daemon": {
    "idle_timeout_seconds": 300,
    "handshake": { "type": "stdio-handshake", "fields": ["socket", "version"] }
  }
}
```

The `daemon` block is **only** present for plugins that declare any
`formats`, `tools`, or `source_connectors` (Mode-C transport).

## Plugin discovery

Kapi scans this list of directories at startup, in precedence order:

| Order | Location | Purpose |
|---|---|---|
| 1 (highest) | `$KAPI_PLUGINS_DIR` (`:` separated; `;` on Windows) | Dev / CI / sandbox overrides |
| 2 | `$XDG_DATA_HOME/kapi/plugins/` (defaults to `~/.local/share/kapi/plugins/`) | `kapi plugin install` target |
| 3 | `/opt/homebrew/share/kapi/plugins/` (Homebrew on macOS) | OS package manager |
| 3 | `/usr/local/share/kapi/plugins/` (Linux `/usr/local`) | OS package manager |
| 3 | `/usr/share/kapi/plugins/` (distro packages) | Distro package manager |

Within each location, every direct subdirectory is a plugin candidate —
kapi treats `<location>/<plugin>/manifest.json` as the manifest and
ignores subdirectories without one. Kapi never consults `$PATH` for
discovery.

First-match-wins on plugin name. Conflicting capabilities between two
*different* plugin names are an error: kapi prints both manifests and
refuses to dispatch the conflicting capability until the user removes one.

## Three transport modes

A plugin's `manifest.json` declares one or more capability sections.
Kapi picks the right transport per capability type.

### Mode A — One-shot subprocess (commands)

Used for `capabilities.commands`.

```
<binary> command <name> [extra args/flags]
```

- stdin / stdout / stderr inherited from kapi
- Env block includes `KAPI_RECIPE_PATH`, `KAPI_PROJECT_ROOT`, `KAPI_PLUGIN_DIR`
- Exit code propagated to the kapi caller
- Plugin process exits after each command — no state carried over

### Mode B — Session subprocess (MCP)

Used for `capabilities.mcp_tools`.

```
<binary> mcp-server
```

- Long-lived; one process per `kapi mcp` session
- Speaks MCP-over-stdio (JSON-RPC framed by the MCP go-sdk)
- Kapi proxies tool calls from its own MCP server to this subprocess
- Process exits when stdio closes

### Mode C — Daemon over local socket (formats / tools / connectors)

Used for `capabilities.formats`, `capabilities.tools`, and
`capabilities.source_connectors`.

```
<binary> daemon
```

- Long-lived; one daemon per kapi session
- The daemon prints a single JSON line on stdout — the **handshake** —
  containing fields declared in `manifest.daemon.handshake.fields`.
  At minimum:
  ```json
  {"socket": "/tmp/kapi-daemon-bowrain-12345.sock", "version": "1.4.0"}
  ```
- Kapi opens a gRPC client to that socket and dispatches format-read /
  tool-process / connector calls
- Daemon stays alive until kapi exits or hits its idle timeout
- Kapi caps concurrent daemons via `KAPI_MAX_DAEMONS` (default 8) with
  LRU eviction

The gRPC service definitions are shared with the existing
`core/plugin/proto/v2/neokapi_bridge.proto` — a daemon-mode plugin
implements that service.

## Subprocess invocation reference

```
# Mode A — commands
<binary> command <name> [args...] [flags...]

# Mode B — MCP server
<binary> mcp-server

# Mode C — daemon
<binary> daemon

# Utility
<binary> version
```

Every plugin must support `version` — it prints the plugin's semver
to stdout and exits 0. `kapi plugin verify` uses this to confirm the
binary matches the manifest.

## Schema validation

Schema validation does **not** cross any subprocess boundary. The
manifest ships JSON Schema files (e.g., `schemas/server.json`); kapi
loads them at plugin-register time and validates recipe Extras with a
JSON Schema library at recipe parse time.

## Recipe `requires:` syntax

`requires:` in a `.kapi` recipe is **always a map** of plugin name →
version constraint:

```yaml
version: v1
name: my-app
requires:
  bowrain: "^1.0"
  okapi-bridge: ">=1.47.0"
```

The bare-list form (`requires: [bowrain]`) is rejected with an
actionable migration hint.

Constraints follow semver: `^1.0`, `~1.4.2`, `>=1.47.0`, `1.4.0`
(exact), `*` (any).

## Tarball layout

Release tarballs contain a top-level `<plugin-name>/` directory:

```
kapi-bowrain-1.4.0-darwin-arm64.tar.gz
└── bowrain/
    ├── manifest.json
    ├── kapi-bowrain
    └── schemas/
        └── server.json
```

This lets users install manually without touching kapi:

```
tar -xzf kapi-bowrain-*.tar.gz -C ~/.local/share/kapi/plugins/
```

## Signing

Plugin tarballs are signed with [cosign](https://sigstore.dev/) keyless
(OIDC, GitHub Actions). `kapi plugin install` verifies:

1. SHA-256 against the registry-pinned hash
2. Cosign signature against the registry-pinned cert identity and
   OIDC issuer

`--unsafe` skips signature verification (used for local development of
unsigned plugins under `$KAPI_PLUGINS_DIR`).

## Reference plugin

A minimal Go reference plugin lives in
[`examples/plugins/hello/`](../../examples/plugins/hello/). It declares
one Mode-A command and one Mode-B MCP tool. Run it as:

```
go build -o hello/kapi-hello ./examples/plugins/hello
KAPI_PLUGINS_DIR=$PWD/examples/plugins kapi --help
```

## Reference table

| Capability key | Transport | Subcommand |
|---|---|---|
| `commands` | Mode A | `<binary> command <name> [args]` |
| `mcp_tools` | Mode B | `<binary> mcp-server` |
| `formats` | Mode C | `<binary> daemon` |
| `tools` | Mode C | `<binary> daemon` |
| `source_connectors` | Mode C | `<binary> daemon` |
| `schema_extensions` | n/a | validated in-process by kapi |
