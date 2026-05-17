---
id: 007-plugin-system
sidebar_position: 7
title: "AD-007: Plugin System and Okapi Bridge"
---

# AD-007: Plugin System and Okapi Bridge

## Summary

Plugins are manifest-driven, signed, out-of-process executables.
Every plugin ships a `manifest.json` declaring everything it provides
— commands, MCP tools, format readers/writers, flow tools, source
connectors, and recipe schema extensions. kapi reads all manifests at
startup and builds dispatch tables from them; there is no name
fall-through. Plugins are discovered structurally by location
(`$KAPI_PLUGINS_DIR` > `$XDG_DATA_HOME/kapi/plugins/` > system roots),
not by `$PATH`. Each capability picks its transport:

- **Mode A** — one-shot subprocess (commands)
- **Mode B** — long-lived stdio subprocess (MCP tools)
- **Mode C** — long-lived daemon over Unix socket + gRPC
  (formats, tools, source connectors)

Plugin tarballs are cosign-signed via Sigstore keyless OIDC; `kapi
plugin install` verifies SHA-256 + Sigstore JSON bundle against a
registry-pinned cert identity before unpacking. Bowrain, the Okapi
bridge, and any third-party plugin all use the same model. The default
`kapi` binary is Apache-2.0 and ships zero vendor-plugin code.

## Context

Plugins enable third-party formats, tools, connectors, and providers
to evolve independently of the framework. Key requirements:

- **License clarity.** kapi is Apache-2.0. Bundling AGPL plugins
  forces the binary distribution to AGPL. The plugin model must let
  vendors ship their own binaries on their own license terms without
  re-licensing kapi.
- **Discoverability and consent.** A teammate's recipe declaring
  `requires: { bowrain: "^1.0" }` should produce a clear, one-step
  path to install — not a cryptic "extension group not registered"
  error.
- **Security.** Plugins run with full user privileges; signature
  verification raises the bar against tampering and supply-chain
  attacks.
- **Performance for format-heavy workloads.** Okapi bridge processes
  large IDML / TMX / Word files at high throughput. JVM startup is
  hundreds of ms; the model must support long-lived daemons with
  multiplexed concurrent requests so the JVM only starts once per
  kapi session.
- **Polyglot from day one.** kapi publishes a language-neutral
  protocol spec; plugin authors implement against it in any language.
  A minimal Go reference plugin ships in `examples/plugins/hello/`.

## Decision

### Manifest

Every plugin's directory contains a `manifest.json` declaring its
identity (`plugin`, `version`, `binary`, `license`, `author`,
`homepage`, `min_kapi_version`, `group`) and the capabilities it
provides under one or more sections:

```json
{
  "manifest_version": "1",
  "plugin": "bowrain",
  "version": "1.4.0",
  "binary": "kapi-bowrain",
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

The `daemon` block is present only for plugins that declare any
formats, tools, or source connectors (Mode C). The full schema is
embedded at `core/plugin/manifest/schema.json`; canonical Go types
live in `core/plugin/manifest/manifest.go`. The protocol is described
in detail at [`docs/internals/plugin-protocol-v1.md`](https://github.com/neokapi/neokapi/blob/main/docs/internals/plugin-protocol-v1.md).

### Discovery

kapi scans this fixed list of locations in precedence order:

| Order       | Location                                                                | Purpose                      |
| ----------- | ----------------------------------------------------------------------- | ---------------------------- |
| 1 (highest) | `$KAPI_PLUGINS_DIR` (`:`-separated; `;` on Windows)                     | Dev / CI / sandbox           |
| 2           | `$XDG_DATA_HOME/kapi/plugins/` (default `~/.local/share/kapi/plugins/`) | `kapi plugin install` target |
| 3           | `/opt/homebrew/share/kapi/plugins/` (macOS Homebrew)                    | OS package manager           |
| 3           | `/usr/local/share/kapi/plugins/` (Linux `/usr/local`)                   | OS package manager           |
| 3           | `/usr/share/kapi/plugins/` (distro)                                     | OS package manager           |

Within each location, every direct subdirectory containing a
`manifest.json` is a plugin. First-match-wins on plugin name.
Conflicting capabilities between two different plugins are an error
— kapi prints both manifests and refuses to dispatch the conflicting
capability.

A consolidated dispatch cache at `$XDG_CACHE_HOME/kapi/plugins-cache.json`
holds parsed manifests + pre-compiled JSON Schema validators. The
cache is invalidated by an mtime check on each discovery root: if
none of the roots changed since the last write, kapi loads the cache
and skips manifest parsing entirely.

### Three transport modes

A plugin declares one or more capability sections in its manifest.
kapi picks the right transport per capability type.

#### Mode A — one-shot subprocess

Used for `commands`. kapi forks and execs the plugin once per
invocation:

```
<binary> command <name> [extra args/flags]
```

stdin / stdout / stderr inherited; env block carries
`KAPI_PLUGIN_DIR`, `KAPI_PLUGIN_NAME`, `KAPI_PLUGIN_VERSION`. Exit
code propagated. The plugin doesn't keep state across calls.

#### Mode B — session subprocess

Used for `mcp_tools`. kapi spawns one plugin process per `kapi mcp`
session and proxies tool calls over MCP-over-stdio:

```
<binary> mcp-server
```

#### Mode C — daemon over Unix socket

Used for `formats`, `tools`, `source_connectors`. kapi spawns a
long-lived plugin process; the plugin binds a Unix-domain socket,
prints one JSON line on stdout (the canonical handshake), then
serves gRPC on the socket:

```
<binary> daemon
   ↓
{"socket":"/tmp/kapi-daemon-bowrain-12345.sock","version":"1.4.0"}
```

kapi opens a gRPC client to that socket and dispatches concurrent
requests. The daemon stays alive until kapi exits or hits its
idle timeout (per-manifest, default 5 min). Concurrent daemons are
capped via `KAPI_MAX_DAEMONS` (default 8) with LRU eviction. Java's
native `UnixDomainSocketAddress` (JDK 16+) makes this work on
Windows 10+ as well as POSIX.

### Lifecycle commands

```
kapi plugin list                              # show installed plugins
kapi plugin install <name>                    # download + verify + register
kapi plugin install <name>@<version>          # pin a specific version
kapi plugin install <name> --channel beta     # pick a channel; persists for updates
kapi plugin update <name>                     # upgrade to latest matching constraint
kapi plugin update-index                      # explicit registry-index refresh
kapi plugin remove <name>                     # uninstall
kapi plugin info <name>                       # show manifest details
kapi plugin search <query>                    # list registry candidates
kapi plugin verify <name>                     # re-check sha256 + signature
kapi plugin rebuild-cache                     # force regenerate the dispatch cache
```

### Recipe `requires:` syntax

A `.kapi` recipe declares plugin dependencies as a map of plugin
name to semver constraint:

```yaml
version: v1
name: my-app
requires:
  bowrain: "^1.0"
  okapi-bridge: ">=1.47.0"
```

Validation fails if any named plugin is not registered. On a TTY,
kapi prompts to install the missing plugin and retries the command;
in CI it prints an actionable error pointing at `kapi plugin install`.
The bare-list form (`requires: [bowrain]`) is rejected with an
actionable migration hint.

### Registry and signing

A registry is a JSON index served over HTTPS. The default registry is
`https://neokapi.github.io/registry/manifest-plugins.json`. The schema
maps plugin name → versions → per-platform tarball URL + SHA-256 +
cosign cert identity:

```json
{
  "plugins": {
    "okapi-bridge": {
      "versions": {
        "1.47.0": {
          "channel": "stable",
          "min_kapi_version": "0.1.0",
          "platforms": {
            "darwin/arm64": {
              "url": "https://github.com/.../kapi-okapi-bridge_1.47.0_darwin_arm64.tar.gz",
              "sha256": "...",
              "signature": "https://.../kapi-okapi-bridge_1.47.0_darwin_arm64.tar.gz.sigstore.json",
              "cert_identity": "https://github.com/neokapi/okapi-bridge/.github/workflows/release.yml@refs/tags/v2.46.0",
              "cert_oidc_issuer": "https://token.actions.githubusercontent.com"
            }
          }
        }
      }
    }
  }
}
```

`kapi plugin install` downloads the tarball + Sigstore JSON bundle,
verifies SHA-256 against the registry-pinned hash, then verifies the
bundle's signing cert against the pinned identity + OIDC issuer using
[`sigstore-go`](https://github.com/sigstore/sigstore-go). Unsigned
plugins refuse to install unless `--unsafe` is passed.

The 1-hour cache at `$XDG_CACHE_HOME/kapi/registry-index.json` keeps
auto-install prompts cheap; explicit `kapi plugin install / search /
update-index` always fetches fresh.

### JSON Schema validation for `schema_extensions`

A plugin can declare recipe schema keys it owns:

```json
{
  "schema_extensions": [
    { "name": "server", "scope": "project", "json_schema": "schemas/server.json" }
  ]
}
```

At plugin-register time, kapi loads `<plugin-dir>/schemas/server.json`,
compiles it via `github.com/google/jsonschema-go`, and registers an
extension decoder with `core/project`. When a recipe is loaded, the
decoder validates the YAML payload against the compiled schema.
Failures render with the recipe path prefix and the JSON Schema
constraint that failed.

### Standard plugins

Two reference plugins ship with the project:

- **bowrain** — cloud-server sync (push/pull/auth), the AGPL plugin
  that proves the model. Ships as `bowrain-cli` brew formula
  (depends on `kapi`, drops `kapi-bowrain` into
  `share/kapi/plugins/bowrain/`).
- **okapi-bridge** — Java bridge exposing 57+ Okapi Framework filters.
  Built with `jpackage` (no Go shim): produces a native launcher
  - bundled JRE per platform. Cosign-signed via GitHub Actions
    keyless OIDC.

A minimal Go reference plugin in `examples/plugins/hello/` covers
Mode A + B with no third-party deps.

## Status

Implemented and merged in #438 (phases 1-9). The legacy v1 plugin
runtime — `core/plugin/{loader,host,server,shared,registry,cache}/`
plus the `kapi plugins` (plural) command tree — has been deleted.
`core/plugin/{bridge,manifest,proto}/` are kept: `bridge` for the
in-process Java filter calls used by `core/flow/bridgerunner`,
`manifest` for the v1 manifest types, `proto` for the gRPC service
definitions consumed by Mode-C daemons.

Native binaries ship for `linux/amd64`, `linux/arm64`,
`darwin/arm64`, and `windows/amd64`. `darwin/amd64` (Intel Mac) is
intentionally not in the release matrix — Apple has dropped Intel
from new product lines and macos-13 runners on GitHub Actions are
scarce. Intel users can run the JAR directly with their own JRE 17+
or use Rosetta on the arm64 binary.

## References

- Issue [#438](https://github.com/neokapi/neokapi/issues/438) —
  unified plugin model design + delivery
- [`docs/internals/plugin-protocol-v1.md`](https://github.com/neokapi/neokapi/blob/main/docs/internals/plugin-protocol-v1.md) — language-neutral protocol spec
- [`core/plugin/manifest/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/manifest) — Go types and embedded JSON Schema
- [`cli/pluginhost/`](https://github.com/neokapi/neokapi/tree/main/cli/pluginhost) — host-side runtime (discovery, dispatch, daemon pool, registry, cosign)
- [`examples/plugins/hello/`](https://github.com/neokapi/neokapi/tree/main/examples/plugins/hello) — minimal Go reference plugin
- [neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge) — Java filter bridge
- [neokapi/registry](https://github.com/neokapi/registry) — published `manifest-plugins.json`
