---
sidebar_position: 5
title: Plugin System
description: neokapi plugins are manifest-driven, signed, out-of-process executables. Each plugin ships a manifest.json declaring the commands, MCP tools, formats, tools, source connectors, and recipe schema it provides; kapi discovers them by location and dispatches over one of three transport modes.
keywords: [plugin system, manifest.json, gRPC, daemon, Mode A B C, Okapi bridge, kapi plugin install, recipe requires, neokapi plugins]
---

# Plugin System

neokapi plugins are **manifest-driven, signed, out-of-process executables**. The
default `kapi` binary is Apache-2.0 and links zero vendor-plugin code; everything
beyond the open-source core — cloud sync, the Okapi filter bridge, third-party
formats — ships as a separate binary that kapi discovers on disk and dispatches
to at runtime.

This page is the developer-facing overview. [AD-007: Plugin System](/contribute/architecture/007-plugin-system)
holds the full design rationale; the [Plugin model note](/contribute/notes-internal/plugin-model)
covers the complementary in-process side — how the Go code _inside_ a plugin
binary wires its features into the shared `cli.App`.

## The manifest

Every plugin's directory contains a `manifest.json` declaring its identity
(`plugin`, `version`, `binary`, `license`, `min_kapi_version`, `group`) and the
capabilities it provides:

```json
{
  "manifest_version": "1",
  "plugin": "myplugin",
  "version": "1.4.0",
  "binary": "kapi-myplugin",
  "license": "Apache-2.0",
  "min_kapi_version": "1.0.0",
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

The `daemon` block appears only when a plugin declares formats, tools, or source
connectors (the Mode-C transport, below). The canonical Go types live in
[`core/plugin/manifest/manifest.go`](https://github.com/neokapi/neokapi/blob/main/core/plugin/manifest/manifest.go),
and the embedded JSON Schema at `core/plugin/manifest/schema.json`.

kapi reads every manifest at startup and builds dispatch tables from them. There
is no name fall-through and no `$PATH` lookup — a capability dispatches only if a
manifest declares it.

## Discovery

Plugins are discovered structurally by location, in precedence order:

| Order       | Location                                                       | Purpose                       |
| ----------- | -------------------------------------------------------------- | ----------------------------- |
| 1 (highest) | `$KAPI_PLUGINS_DIR` (`:`-separated; `;` on Windows)            | Dev / CI / sandbox            |
| 2           | `$XDG_DATA_HOME/kapi/plugins/` (`~/.local/share/kapi/plugins/`) | `kapi plugin install` target  |
| 3           | system roots (`/opt/homebrew/share/kapi/plugins/`, `/usr/local/share/kapi/plugins/`, `/usr/share/kapi/plugins/`) | OS package managers |

Within each location, every direct subdirectory containing a `manifest.json` is a
plugin. First-match-wins on plugin name. Two different plugins declaring the same
capability is an error — kapi prints both manifests and refuses to dispatch the
conflicting capability. A consolidated dispatch cache at
`$XDG_CACHE_HOME/kapi/plugins-cache.json` skips manifest parsing when no
discovery root has changed.

## Three transport modes

A plugin declares one or more capability sections; kapi picks the transport per
capability type.

- **Mode A — one-shot subprocess** (`commands`). kapi forks `<binary> command <name> [args]`
  once per invocation, inheriting stdio and propagating the exit code. No state
  survives across calls.
- **Mode B — session subprocess** (`mcp_tools`). kapi spawns `<binary> mcp-server`
  once per `kapi mcp` session and proxies tool calls over MCP-over-stdio.
- **Mode C — daemon over Unix socket** (`formats`, `tools`, `source_connectors`).
  kapi spawns `<binary> daemon`; the plugin binds a Unix-domain socket and prints
  one JSON handshake line on stdout, then serves gRPC on the socket:

  ```
  {"socket":"/tmp/kapi-daemon-myplugin-12345.sock","version":"1.4.0"}
  ```

  kapi opens a gRPC client to that socket and dispatches concurrent requests. The
  daemon stays alive until kapi exits or hits its idle timeout (per-manifest,
  default 5 min). Concurrent daemons are capped via `KAPI_MAX_DAEMONS` (default 8)
  with LRU eviction. Format and tool capabilities register into the standard
  `FormatRegistry` / `ToolRegistry` and are indistinguishable from native ones at
  the API level. The Okapi bridge is the canonical Mode-C plugin — see
  [Okapi Bridge](/contribute/java-bridge).

The host-side runtime — discovery, dispatch, the daemon pool, the registry
client, and signature verification — lives in
[`cli/pluginhost/`](https://github.com/neokapi/neokapi/tree/main/cli/pluginhost).

## Declaring a plugin dependency

A `.kapi` recipe declares the plugins it needs as a map of name → semver
constraint:

```yaml
version: v1
name: my-app
requires:
  myplugin: "^1.0"
  okapi-bridge: ">=1.47.0"
```

Loading the recipe fails if a named plugin is not registered. On a TTY, kapi
offers to install it and retries; in CI it prints an actionable error pointing at
`kapi plugin install`.

## Lifecycle commands

```bash
kapi plugin list                       # show installed plugins
kapi plugin install <name>             # download + verify signature + register
kapi plugin install <name>@<version>   # pin a specific version
kapi plugin update <name>              # upgrade to latest matching constraint
kapi plugin remove <name>              # uninstall
kapi plugin info <name>                # show manifest details
kapi plugin search <query>             # list registry candidates
kapi plugin verify <name>              # re-check sha256 + signature
kapi plugin update-index               # refresh the cached registry index
kapi plugin rebuild-cache              # force a rebuild of the plugin dispatch cache
```

`kapi plugin install` resolves the plugin from a registry — a JSON index served
over HTTPS that maps plugin → versions → per-platform tarball URL, SHA-256, and a
cosign certificate identity. Tarballs are cosign-signed via Sigstore keyless
OIDC; install verifies the SHA-256 and the signing certificate against the
registry-pinned identity before unpacking. Unsigned plugins refuse to install
without `--unsafe`.

## Standard plugins

- **A platform plugin** — cloud-server sync (`push` / `pull` / `auth`),
  distributed separately on its own license terms. It demonstrates how a
  separately-licensed plugin attaches over the manifest model without
  re-licensing `kapi`: installed via its own Homebrew formula, which drops its
  binary into `share/kapi/plugins/<plugin>/`.
- **okapi-bridge** — a JVM-backed Mode-C daemon exposing the Okapi Framework's
  filter library to neokapi. See [Okapi Bridge](/contribute/java-bridge).
- **On-device ML sidecars** — cgo plugins that run native ML in their own
  subprocess so the heavy stack (onnxruntime, whisper.cpp, PDFium, ffmpeg) never
  enters the portable `kapi` binary: `kapi-sat` (segmentation), `kapi-vision`
  (OCR/layout), `kapi-asr` (speech-to-text), `kapi-av` (audio/video), and
  `kapi-pdfium` (PDF).

For on-device **LLM** text generation (translation, chat, QA, brand-voice), kapi
drives a local [Ollama](https://ollama.com) runtime rather than bundling an
inference engine: Ollama already runs GGUF models on the GPU (Metal/CUDA) and is
managed through `kapi ollama` and `--provider ollama` — a free, private
alternative to the paid cloud providers. In the browser, the
[Core Framework lab](/lab) runs a local model via WebGPU instead, since a web
page cannot reach a local daemon.

A minimal Go reference plugin in
[`examples/plugins/hello/`](https://github.com/neokapi/neokapi/tree/main/examples/plugins/hello)
covers Mode A + B with no third-party dependencies.

## Retiring a plugin

When a plugin is superseded (for example, `kapi-llm` was retired once the
built-in Ollama provider replaced the bundled on-device engine), kapi marks it
**retired** rather than silently breaking. Because a plugin binary installed from
a previous version can still be sitting on disk — and the registry may be offline
or pinned — retirement is signalled in two places:

- **A compiled-in tombstone** (`cli/pluginhost/tombstones.go`) is the
  offline-authoritative source. It records the plugin name, the kapi version that
  retired it, the reason, and the replacement. Its mere presence in a build is the
  version gate: an older kapi simply has no entry. This is what makes retirement
  trustworthy with no network and with the binary still installed.
- **A registry `deprecated` field** on the plugin entry is the faster,
  reversible, network-side signal. It lets the registry refuse new installs and
  flag the plugin in search without waiting for a kapi release. When the two
  disagree, the built-in tombstone wins for load-time enforcement.

A retired plugin stays **listed but inert**: kapi registers none of its dispatch
routes (commands, formats, segmenters), so it is never loaded or run, and it
contributes no models to `kapi models`. `kapi plugins list` shows it as `retired`
with the reason and replacement; `kapi plugins install` refuses to (re)install it.
kapi never auto-deletes software on disk — instead, **`kapi plugins prune`**
removes retired user installs after confirmation (`--yes`, `--dry-run`), prints
the OS command for system (Homebrew) installs rather than touching them, and
leaves downloaded model caches and configuration alone.

## See also

- [AD-007: Plugin System](/contribute/architecture/007-plugin-system) — full design rationale and the registry/signing model
- [Plugin model note](/contribute/notes-internal/plugin-model) — the in-process registry contract a plugin binary uses to wire features into `cli.App`
- [Okapi Bridge](/contribute/java-bridge) — the canonical Mode-C bridge plugin
