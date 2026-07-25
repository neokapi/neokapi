---
id: 007-plugin-system
sidebar_position: 7
title: "AD-007: Plugin System"
description: "Architecture decision: plugins are manifest-driven, signed, out-of-process executables — discovered via manifest.json, dispatched as subprocesses over gRPC. The contract is a versioned protocol with a conformance suite an out-of-tree plugin repository can run against a released kapi."
keywords: [plugin system, manifest, gRPC, protocol v1, conformance, out-of-process, architecture decision, neokapi]
---

# AD-007: Plugin System

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
registry-pinned cert identity before unpacking. First-party and
third-party plugins all use the same model. The default `kapi` binary is
Apache-2.0 and ships zero vendor-plugin code.

The contract is **versioned** and **independently verifiable**: protocol
v1 is specified in
[Plugin protocol v1](../implementation/plugin-protocol-v1.md), and
`core/plugin/conformance` is that specification in executable form — a
consumable Go package a plugin repository imports from a released kapi to
self-report conformance in its own CI.

## Context

Plugins enable third-party formats, tools, connectors, and providers
to evolve independently of the framework. Key requirements:

- **License clarity.** kapi is Apache-2.0. Bundling a plugin under a
  more restrictive (e.g. copyleft) license would force the combined
  binary distribution onto those terms. The plugin model must let
  vendors ship their own binaries on their own license terms without
  re-licensing kapi.
- **Discoverability and consent.** A teammate's recipe declaring
  `requires: { myplugin: "^1.0" }` should produce a clear, one-step
  path to install — not a cryptic "extension group not registered"
  error.
- **Security.** Plugins run with full user privileges; signature
  verification raises the bar against tampering and supply-chain
  attacks. This is supply-chain signing (cosign / Sigstore), distinct
  from OS code signing / notarization — see
  [Plugin signing vs. OS notarization](#plugin-signing-vs-os-notarization).
- **Performance for format-heavy workloads.** A format plugin may
  process large binary documents at high throughput while paying a
  runtime startup cost of hundreds of milliseconds (a JVM, a Python
  interpreter, a model load). The model must support long-lived daemons
  with multiplexed concurrent requests, so that cost is paid once per
  kapi session rather than once per document.
- **Polyglot from day one.** kapi publishes a language-neutral,
  versioned protocol spec; plugin authors implement against it in any
  language. A minimal Go reference plugin ships in
  `examples/plugins/hello/`.
- **Verifiable from outside the repository.** A plugin is not obliged to
  live here to be trusted. The contract must be checkable by a plugin
  repository that only depends on a *released* kapi — otherwise "does
  this plugin still work?" is answerable only by tests inside this
  repository, which re-couples every plugin to this repository's release
  cycle.

## Decision

### Manifest

Every plugin's directory contains a `manifest.json` declaring its
identity (`plugin`, `version`, `binary`, `license`, `author`,
`homepage`, `min_kapi_version`, `group`) and the capabilities it
provides under one or more sections:

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

The `daemon` block is present only for plugins that declare any
formats, tools, segmenters, or source connectors (Mode C). The full
schema is embedded at `core/plugin/manifest/schema.json`; canonical Go
types live in `core/plugin/manifest/manifest.go`. The wire contract —
every manifest rule, all three transports, the Mode-C gRPC surface, and
the conformance suite — is specified in
[Plugin protocol v1](../implementation/plugin-protocol-v1.md).

### Discovery

kapi scans this fixed list of locations in precedence order:

| Order       | Location                                                                | Purpose                      |
| ----------- | ----------------------------------------------------------------------- | ---------------------------- |
| 1 (highest) | `$KAPI_PLUGINS_DIR` (`:`-separated; `;` on Windows)                     | Dev / CI / sandbox           |
| 2           | `$XDG_DATA_HOME/kapi/plugins/` (default `~/.local/share/kapi/plugins/`) | `kapi plugin install` target |
| 3           | `/opt/homebrew/share/kapi/plugins/` (macOS Homebrew)                    | OS package manager           |
| 3           | `/usr/local/share/kapi/plugins/` (Linux `/usr/local`)                   | OS package manager           |
| 3           | `/usr/share/kapi/plugins/` (distro)                                     | OS package manager           |

Within each location, every direct entry that resolves to a directory
containing a `manifest.json` is a plugin. Symlinks are followed: a package
manager installs the plugin inside its own package prefix and links it into
the shared root (a Homebrew formula stages
`share/kapi/plugins/<plugin>` in its keg, and `brew link` publishes it at
`/opt/homebrew/share/kapi/plugins/<plugin>`), so a name-only or
link-skipping scan would find nothing there. First-match-wins on plugin name.
Conflicting capabilities between two different plugins are an error
— kapi prints both manifests and refuses to dispatch the conflicting
capability.

**Precedence over built-ins.** A plugin capability that collides with a
*built-in* one (e.g. a plugin format reader for `pdf` when the framework
ships a pure-Go one) **overrides the built-in** — installing a plugin for a
format is an explicit signal to prefer it. Built-ins remain the fallback when
the plugin is absent, so behaviour degrades gracefully. Plugin-vs-plugin
collisions still error (above); plugin-vs-built-in is resolved in the plugin's
favour via the format registry's source/priority (`SetFormatSource` assigns
`DefaultPluginPriority` > `DefaultBuiltInPriority`).

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

Command attachment follows the **no-shadowing rule**: installing a plugin
never changes what an existing kapi verb means (plugins extend kapi the way
`gh` extends `git`). Concretely:

- Every plugin command also attaches under a per-plugin group command —
  `kapi <group> <verb>` (e.g. `kapi bowrain ls`), where `group` is the
  manifest's plugin-level `group` field, falling back to the plugin name.
- A plugin command whose name collides with a built-in attaches under the
  group **only**; the built-in keeps the top-level verb. This is a supported
  layout for verbs whose plugin semantics differ from core (bowrain's
  `config`, whose recipe keys the built-in also covers positionally).
- A command with `"hidden": true` is dispatch **plumbing** consumed by a
  built-in rather than typed by users — bowrain's `server-status` (merged
  into `kapi status`), `server-up` (the server venue behind `kapi up`),
  and `server-ls` (the sync column on `kapi ls`). Hidden commands stay
  routable but are omitted from `--help` and completion.
- A plugin that needs to participate in a core verb uses a
  `command_contribution` (bowrain's `init --server`) or hidden plumbing the
  built-in dispatches — never a same-name command.

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
{"socket":"/tmp/kapi-daemon-myplugin-12345.sock","version":"1.4.0"}
```

kapi opens a gRPC client to that socket and dispatches concurrent
requests. The daemon stays alive until kapi exits or hits its
idle timeout (per-manifest, default 5 min). Concurrent daemons are
capped via `KAPI_MAX_DAEMONS` (default 8) with LRU eviction. The
daemon transport is a Unix-domain socket, dialed by kapi as a gRPC
client over `unix`, and is POSIX-only today. Each plugin supplies its
own socket server — the out-of-tree okapi-bridge, for instance, serves
it with Netty's native transports (kqueue on macOS, epoll on Linux).

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

A `kapi.yaml` recipe declares plugin dependencies as a map of plugin
name to semver constraint:

```yaml
version: v1
name: my-app
requires:
  myplugin: "^1.0"
  okapi-bridge: ">=1.47.0"
```

Validation fails if any named plugin is not registered. On a TTY,
kapi prompts to install the missing plugin and retries the command;
in CI it prints an actionable error pointing at `kapi plugin install`.
The bare-list form (`requires: [myplugin]`) is rejected with an
actionable migration hint.

### Missing-plugin verbs

A recipe need not spell out `requires:` to imply a plugin. Declaring a
top-level key that a plugin's `schema_extensions` own — `server:`, owned by the
platform plugin — is itself the declaration, and the key survives untouched in
the recipe when the plugin that would decode it is absent.

That leaves one gap the manifest model cannot close on its own: a verb the
plugin provides is not in the command tree when the plugin is not installed, so
cobra rejects it as an unknown command and names neither the cause nor the fix.
kapi closes it with the same treatment `requires:` gets. When the typed verb
belongs to a plugin, no installed plugin routes it, and the recipe declares that
plugin's key, kapi explains the situation and offers the install — prompting on
a terminal, installing under `--yes`, then running the verb the user typed. In
CI, behind a pipe, or under `--quiet` it prints both install routes
(`kapi plugin install <name>`, `brew install <formula>`) and exits `ExitUsage`.
A verb that is simply mistyped keeps cobra's error and its suggestions.

Deciding this needs one thing kapi cannot read from a manifest it does not have:
which verbs the plugin provides. A compiled-in hint table carries them —
consulted only to compose this message, never for dispatch, which stays
manifest-driven — and a drift test in the plugin's own module pins the table to
the plugin's real manifest, so adding or removing a verb fails the build until
the table follows.

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

### Plugin signing vs. OS notarization

Plugin signing sits on a different trust layer than the OS code
signing applied to the kapi CLI and desktop apps. The two are
independent and answer different questions:

| Layer | Question | Mechanism | Triggered by |
| ----- | -------- | --------- | ------------ |
| **Supply chain** | Is this the genuine, untampered plugin? | cosign / Sigstore bundle + SHA-256, verified at install (above) | every `kapi plugin install` |
| **OS Gatekeeper / SmartScreen** | Will the OS let the binary run without a warning? | Apple Developer ID + notarization (macOS); Authenticode (Windows) | the `com.apple.quarantine` xattr — set only by browser / mail downloads |

Plugins rely on the **supply-chain** layer only. `kapi plugin install`
fetches tarballs over HTTPS using Go's HTTP client, which does **not**
set the quarantine attribute, and unpacks them under the data dir. The
extracted binary is therefore never quarantined, so macOS Gatekeeper
and Windows SmartScreen never engage on it — no Apple notarization or
Authenticode signature is required for a plugin to run. The cosign
signature + SHA-256 check is the meaningful integrity guarantee, and it
is enforced on every platform.

This is the inverse of the kapi CLI and desktop apps, which **are**
Developer-ID-signed + notarized (macOS) and Authenticode-signed
(Windows): users fetch those through a browser (DMG, release archive),
so they arrive quarantined and must clear Gatekeeper / SmartScreen on
first launch.

The out-of-tree okapi-bridge plugin is a `jpackage` app-image — a native
launcher plus a bundled JRE, i.e. genuine native code — yet the same
reasoning holds:
installed unquarantined via `kapi plugin install` and verified by
cosign + SHA-256, it runs without OS-level signing. Deep-signing and
notarizing it (the launcher plus every bundled-JRE dylib, across each
Okapi version × OS × arch) is deliberately **not** done, because it
buys nothing for the programmatic install path. A plugin would need OS
code signing only if it were also distributed as a **direct browser
download** — out of scope for the registry-driven model.

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

### Protocol versioning and conformance

The protocol is versioned as a whole, independently of any kapi release.
`conformance.ProtocolVersion` names the revision this repository
implements; `manifest.SupportedVersions` names the manifest-document
revisions a kapi binary accepts. Within a version the rules are
**additive**: capabilities, manifest fields, and conformance checks may
be added, but an existing rule does not change and a check ID is never
renamed or repurposed. A change that would break a conforming plugin is
a new protocol version.

There is deliberately **no protocol-version handshake on the wire**. The
manifest is read from disk before any process is launched, so
incompatibility is detected structurally rather than negotiated at
runtime — which is also what keeps discovery free of subprocess launches.

`core/plugin/conformance` is the specification in executable form: a
black-box driver that exercises the manifest rules, the standard verbs,
and whichever transports a manifest declares, against an installed plugin
directory. Three properties make it usable from outside this repository,
and each one is a constraint on where it may live:

- **Framework-layer, so it is cheap to import.** It sits in the
  framework module and depends only on `core/plugin/manifest`,
  `core/plugin/proto/v2`, and gRPC — no host module, no CLI, no cobra
  tree, nothing under a copyleft licence. A plugin repository adds one
  `require` on a released `github.com/neokapi/neokapi`.
- **Black-box, so it is language-neutral.** It reads no plugin source and
  spawns the plugin exactly as the host does, so the JVM bridge and a Go
  plugin are checked by the same code.
- **Side-effect-free by default.** It runs only the read-only standard
  verbs plus deliberately invalid arguments that prove a plugin rejects
  them. Invoking a declared command, reading a document through a
  declared format, or calling a segmenter is opt-in, because only the
  plugin author knows which capabilities are safe to exercise unattended.

Checks are `required` or `advisory`: an advisory failure is reported but
does not clear conformance, naming something the host tolerates today
that a plugin should still fix. A check that does not apply is skipped,
never failed. The authoritative check list is `conformance.Checks()`.

This is what lets a plugin repository own its own tests. The bridge repo
runs the suite against released kapi versions on a schedule and reports
its own conformance; nothing about it builds, tests, or gates anything
here.

### Standard plugins

- **A platform plugin** — cloud-server sync (push/pull/auth),
  distributed separately on its own license terms. It demonstrates how
  a separately-licensed plugin attaches over the manifest model without
  re-licensing `kapi`: installed via its own brew formula (depends on
  `kapi`, drops its binary into `share/kapi/plugins/<plugin>/`).
- **kapi-pdfium** (`plugins/pdfium/`) — first-party Mode-C format plugin
  providing a high-fidelity `pdf` reader backed by Google's PDFium
  (go-pdfium, cgo). It extracts correct text (including CID/Type0 fonts and
  CJK), per-block and per-glyph geometry, and document structure, and runs as
  an isolated daemon so a malformed-PDF crash dies with the subprocess, not
  kapi. There is no in-core PDF reader on native builds, so the plugin supplies
  the `pdf` format outright; the browser uses PDFium compiled to WebAssembly
  instead. **Bundled with both the kapi-cli distribution and the kapi-desktop
  app**: the CLI installs it into the shared `share/kapi/plugins/pdfium/` root,
  and the desktop installs it on demand the first time a PDF is opened, both
  hosting it over the same `host/pluginhost` discovery + daemon pool — one
  engine, not one per host. PDFium ships as a bundled shared library beside the
  binary (found via rpath), not statically linked. The full PDF subsystem —
  extraction modes, the geometry model, and the tagged/geometric structure
  tiers — is described in [AD-028](028-pdf-reader-plugin.md).

A minimal Go reference plugin in `examples/plugins/hello/` covers
Mode A + B with no third-party deps.

## Status

Implemented and merged in #438 (phases 1-9); protocol v1 was tagged and
the conformance suite extracted in #1073. The legacy v1 plugin runtime —
`core/plugin/{loader,host,server,shared,registry,cache}/` plus the `kapi
plugins` (plural) command tree — has been deleted.

The framework module keeps the parts a plugin author needs:
`core/plugin/manifest` (manifest types + embedded JSON Schema),
`core/plugin/proto` (the gRPC service definitions a Mode-C daemon
implements), `core/plugin/protoconvert` (Part↔proto translation), and
`core/plugin/conformance` (the protocol conformance suite). Those four
are deliberately importable on their own: an out-of-tree plugin
repository depends on a released framework module and nothing else.

The host-side runtime — discovery, dispatch, the daemon pool, the
registry client, cosign verification, and the Mode-C format client —
lives in `host/pluginhost/`.

Native binaries ship for `linux/amd64`, `linux/arm64`,
`darwin/arm64`, and `windows/amd64`. `darwin/amd64` (Intel Mac) is
intentionally not in the release matrix — Apple has dropped Intel
from new product lines and macos-13 runners on GitHub Actions are
scarce. Intel users can use Rosetta on the arm64 binary.

## References

- Issue [#438](https://github.com/neokapi/neokapi/issues/438) —
  unified plugin model design + delivery
- Issue [#1073](https://github.com/neokapi/neokapi/issues/1073) —
  decoupling okapi-bridge; protocol v1 + the conformance suite
- [Plugin protocol v1](../implementation/plugin-protocol-v1.md) — the
  versioned, language-neutral specification
- [`core/plugin/manifest/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/manifest) — Go types and embedded JSON Schema
- [`host/pluginhost/`](https://github.com/neokapi/neokapi/tree/main/host/pluginhost) — host-side runtime (discovery, dispatch, daemon pool, registry, cosign)
- [`examples/plugins/hello/`](https://github.com/neokapi/neokapi/tree/main/examples/plugins/hello) — minimal Go reference plugin
- [`core/plugin/conformance/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/conformance) — the protocol conformance suite, importable from a released kapi
- [neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge) — the reference implementation of a third-party plugin in a non-Go language (a JVM Mode-C daemon exposing the Okapi Framework's filters); it consumes released kapi versions and reports conformance on its own schedule
- [neokapi/registry](https://github.com/neokapi/registry) — published `manifest-plugins.json`
