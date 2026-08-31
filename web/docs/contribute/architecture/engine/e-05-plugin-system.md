---
id: e-05-plugin-system
sidebar_position: 5
title: "E-05: The plugin system"
description: "Plugins are manifest-driven, signed, out-of-process executables discovered by location and dispatched as subprocesses over gRPC, against a versioned protocol with a conformance suite an out-of-tree repository can run."
keywords: [neokapi, architecture decision, plugin system, manifest, gRPC, protocol, conformance, out-of-process, cosign, sigstore, daemon]
---

# E-05: The plugin system

## Summary

Plugins are manifest-driven, signed, out-of-process executables. Every plugin
ships a `manifest.json` declaring everything it provides: commands, MCP tools,
format readers and writers, flow tools, segmenters, source connectors, recipe
schema extensions, config namespaces, command contributions, and whether it
answers the standard self-check. kapi reads all
manifests at startup and builds dispatch tables from them; there is no name
fall-through. Plugins are discovered structurally by location
(`$KAPI_PLUGINS_DIR` > `$XDG_DATA_HOME/kapi/plugins/` > system roots); `$PATH`
is never consulted. Each capability picks its transport:

- **Mode A**: one-shot subprocess (commands)
- **Mode B**: long-lived stdio subprocess (MCP tools)
- **Mode C**: long-lived daemon over a Unix socket + gRPC (formats, tools,
  segmenters, source connectors)

Plugin tarballs are cosign-signed via Sigstore keyless OIDC; `kapi plugin
install` verifies SHA-256 plus the Sigstore JSON bundle against a
registry-pinned certificate identity before unpacking. First-party and
third-party plugins all use the same model. The default `kapi` binary is
Apache-2.0 and contains no plugin's code: a plugin is a separate executable,
reached through its manifest and run as a subprocess, so no plugin's licence or
dependency set reaches the binary.

The contract is **versioned** and **independently verifiable**: the protocol is
specified in [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1), and
`core/plugin/conformance` is that specification in executable form: a consumable
Go package a plugin repository imports from a released kapi to self-report
conformance in its own CI.

## Context

Plugins let third-party formats, tools, connectors, and providers evolve
independently of the framework. Key requirements:

- **License clarity.** kapi is Apache-2.0. Bundling a plugin under a more
  restrictive licence would force the combined binary distribution onto those
  terms. The plugin model must let vendors ship their own binaries on their own
  licence terms without re-licensing kapi.
- **Discoverability and consent.** A teammate's recipe declaring `requires: {
  myplugin: "^1.0" }` should produce a clear, one-step path to install rather than a
  cryptic "extension group not registered" error.
- **Security.** Plugins run with full user privileges; signature verification
  raises the bar against tampering and supply-chain attacks. This is
  supply-chain signing (cosign / Sigstore), distinct from OS code signing and
  notarization; see
  [Plugin signing vs. OS notarization](#plugin-signing-vs-os-notarization).
- **Performance for format-heavy workloads.** A format plugin may process large
  binary documents at high throughput while paying a runtime startup cost of
  hundreds of milliseconds (a JVM, a Python interpreter, a model load). The model
  must support long-lived daemons with multiplexed concurrent requests, so that
  cost is paid once per kapi session rather than once per document.
- **Polyglot from day one.** kapi publishes a language-neutral, versioned
  protocol spec; plugin authors implement against it in any language. A minimal
  Go reference plugin ships in `examples/plugins/hello/`.
- **Verifiable from outside the repository.** A plugin is not obliged to live
  here to be trusted. The contract must be checkable by a plugin repository that
  depends only on a *released* kapi; otherwise "does this plugin still work?" is
  answerable only by tests inside this repository, which re-couples every plugin
  to this repository's release cycle.

## Decision

### Manifest

Every plugin's directory contains a `manifest.json` declaring its identity
(`plugin`, `version`, `binary`, `license`, `author`, `homepage`,
`min_kapi_version`, `group`, `models`) and the capabilities it provides:

```json
{
  "manifest_version": "1",
  "plugin": "myplugin",
  "version": "1.4.0",
  "binary": "kapi-myplugin",
  "license": "Apache-2.0",
  "min_kapi_version": "1.0.0",
  "capabilities": {
    "commands": [],
    "command_contributions": [],
    "mcp_tools": [],
    "formats": [],
    "tools": [],
    "segmenters": [],
    "source_connectors": [],
    "schema_extensions": [],
    "config_namespaces": [],
    "selfcheck": true
  },
  "daemon": {
    "idle_timeout_seconds": 300,
    "handshake": { "type": "stdio-handshake", "fields": ["socket", "version"] }
  }
}
```

`manifest_version`, `plugin`, `version`, and `binary` are the required fields;
the `daemon` block is present only for plugins that declare any formats, tools,
segmenters, or source connectors (Mode C). `manifest.SupportedVersions` names the
manifest-document revisions a kapi binary accepts. The full schema is embedded at
`core/plugin/manifest/schema.json`; canonical Go types live in
`core/plugin/manifest/manifest.go`. The wire contract (every manifest rule, all
three transports, the Mode-C gRPC surface, and the conformance suite) is
specified in [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1).

### Discovery

kapi scans this fixed list of locations in precedence order:

| Order       | Location                                                                | Purpose                      |
| ----------- | ----------------------------------------------------------------------- | ---------------------------- |
| 1 (highest) | `$KAPI_PLUGINS_DIR` (`:`-separated; `;` on Windows)                     | Dev / CI / sandbox           |
| 2           | `$XDG_DATA_HOME/kapi/plugins/` (default `~/.local/share/kapi/plugins/`) | `kapi plugin install` target |
| 3           | `/opt/homebrew/share/kapi/plugins/` (macOS, Homebrew)                   | OS package manager           |
| 3           | `/usr/local/share/kapi/plugins/` (macOS and Linux)                      | OS package manager           |
| 3           | `/usr/share/kapi/plugins/` (Linux distributions)                        | OS package manager           |

Within each location, every direct entry that resolves to a directory containing
a `manifest.json` is a plugin. Symlinks are followed: a package manager installs
the plugin inside its own package prefix and links it into the shared root: a
Homebrew formula stages `share/kapi/plugins/<plugin>` in its keg, and `brew link`
publishes it at `/opt/homebrew/share/kapi/plugins/<plugin>`, so a name-only or
link-skipping scan would find nothing there. First match wins on plugin name.
Conflicting capabilities between two different plugins are an error: kapi prints
both manifests and refuses to dispatch the conflicting capability.
`KAPI_PLUGINS_DIR_ONLY` restricts discovery to the first root, which is how an
in-repo kapi stays isolated from the developer's installed plugins.

**Precedence over built-ins.** A plugin capability that collides with a
*built-in* one (a plugin reader for a format the framework also ships natively)
**overrides the built-in**, because installing a plugin for a format is an
explicit signal to prefer it. Built-ins remain the fallback when the plugin is
absent, so behaviour degrades gracefully. Plugin-versus-plugin collisions still
error; plugin-versus-built-in is resolved in the plugin's favour via the format
registry's source and priority (`SetFormatSource` assigns
`format.DefaultPluginPriority` = 100 over `format.DefaultBuiltInPriority` = 50).

A consolidated dispatch cache at `$XDG_CACHE_HOME/kapi/plugins-cache.json`
(`KAPI_PLUGIN_CACHE` overrides the path) holds
parsed manifests plus pre-compiled JSON Schema validators. The cache is
invalidated by an mtime check on each discovery root: if none of the roots
changed since the last write, kapi loads the cache and skips manifest parsing
entirely.

### Three transport modes

A plugin declares one or more capability sections in its manifest. kapi picks the
right transport per capability type.

#### Mode A: one-shot subprocess

Used for `commands`. kapi forks and execs the plugin once per invocation:

```
<binary> command <name> [extra args/flags]
```

stdin, stdout, and stderr are inherited; the env block carries
`KAPI_PLUGIN_DIR`, `KAPI_PLUGIN_NAME`, and `KAPI_PLUGIN_VERSION`, and inherits
kapi's environment **except** the provider API-key variables: installing a
plugin is not a decision to hand it the user's model credentials. The exit code
is propagated. The plugin keeps no state across calls.

Command attachment follows the **no-shadowing rule**: installing a plugin never
changes what an existing kapi verb means. Plugins extend kapi the way `gh`
extends `git`. Concretely:

- Every plugin command also attaches under a per-plugin group command,
  `kapi <group> <verb>`, where `group` is the manifest's plugin-level `group`
  field, falling back to the plugin name.
- A plugin command whose name collides with a built-in attaches under the group
  **only**; the built-in keeps the top-level verb. This is the supported layout
  for verbs whose plugin semantics differ from the core meaning.
- A command marked `"hidden": true` is dispatch **plumbing** consumed by a
  built-in rather than typed by users. Hidden commands stay routable but are
  omitted from `--help` and completion.
- A plugin that needs to participate in a core verb uses a `command_contribution`
  or hidden plumbing the built-in dispatches, never a same-name command.

#### Mode B: session subprocess

Used for `mcp_tools`. kapi spawns one plugin process per `kapi mcp` session and
proxies tool calls over MCP-over-stdio:

```
<binary> mcp-server
```

#### Mode C: daemon over a Unix socket

Used for `formats`, `tools`, `segmenters`, and `source_connectors`. kapi spawns a
long-lived plugin process; the plugin binds a Unix-domain socket, prints one JSON
line on stdout (the canonical handshake), then serves gRPC on the socket:

```
<binary> daemon
   ↓
{"socket":"/tmp/kapi-daemon-myplugin-12345.sock","version":"1.4.0"}
```

kapi opens a gRPC client to that socket and dispatches concurrent requests. The
daemon stays alive until kapi exits or hits its idle timeout (per-manifest,
default 5 minutes). Concurrent daemons are capped via `KAPI_MAX_DAEMONS`
(default 8) with LRU eviction. The transport is a Unix-domain socket, dialed by
kapi as a gRPC client over `unix`, and is POSIX-only today. Each plugin supplies
its own socket server.

### Lifecycle commands

```
kapi plugin list                              # show installed plugins
kapi plugin install <name>                    # download + verify + register
kapi plugin install <name>@<version>          # pin a specific version
kapi plugin install <name> --channel beta     # pick a channel; persists for updates
kapi plugin update <name>                     # upgrade to latest matching constraint
kapi plugin update-index                      # explicit registry-index refresh
kapi plugin remove <name>                     # uninstall
kapi plugin prune                             # drop superseded installs
kapi plugin info <name>                       # show manifest details
kapi plugin search <query>                    # list registry candidates
kapi plugin verify <name>                     # re-check sha256 + signature
kapi plugin doctor                            # diagnose discovery and dispatch
kapi plugin registry add|list|remove          # manage registry indexes
kapi plugin rebuild-cache                     # force regenerate the dispatch cache
```

The generated [command reference](/reference/commands/plugin) carries each
command's flags.

### Recipe `requires:` syntax

A `kapi.yaml` recipe declares plugin dependencies as a map of plugin name to
semver constraint:

```yaml
version: v1
name: my-app
requires:
  myplugin: "^1.0"
  okapi-bridge: ">=1.47.0"
```

Validation fails if any named plugin is not registered. On a TTY, kapi prompts to
install the missing plugin and retries the command; in CI it prints an actionable
error pointing at `kapi plugin install`. The bare-list form
(`requires: [myplugin]`) is rejected with an actionable hint.

### The venue extension

A `schema_extensions` entry may set `"venue": true`. That marks the key as the
recipe's binding to a remote convergence venue (a server that holds the content
memory, runs the loop on organization keys, and carries a review queue), and it
is how kapi finds the binding without knowing the key's name. The framework reads
exactly two fields out of the block, `url:` and `converge:`, to decide where
`kapi up` runs; everything else under it is the plugin's own schema. At most one
key across all installed plugins should claim the flag.

```json
{"name": "myvenue", "scope": "project", "group": "myvenue", "venue": true}
```

`kapi` links no platform, so an unregistered key of the same name, in a binary
without the plugin, reports no venue.

### Missing-plugin verbs

A recipe need not spell out `requires:` to imply a plugin. Declaring a top-level
key that a plugin's `schema_extensions` own is itself the declaration, and the
key survives untouched in the recipe when the plugin that would decode it is
absent.

That leaves one gap the manifest model cannot close on its own: a verb the plugin
provides is not in the command tree when the plugin is not installed, so cobra
rejects it as an unknown command and names neither the cause nor the fix. kapi
closes it with the same treatment `requires:` gets. When the typed verb belongs
to a plugin, no installed plugin routes it, and the recipe declares that plugin's
key, kapi explains the situation and offers the install: prompting on a
terminal, installing under `--yes`, then running the verb the user typed. In CI,
behind a pipe, or under `--quiet` it prints both install routes (`kapi plugin
install <name>` and the Homebrew formula) and exits `ExitUsage`. A mistyped verb
that no plugin claims keeps cobra's error and its suggestions.

Deciding this needs one thing kapi cannot read from a manifest it does not have:
which verbs the plugin provides. A compiled-in `host.PluginHint` table carries
them: the plugin name, the recipe key it owns, its non-colliding top-level
verbs, and its Homebrew formula. It is consulted **only** to compose this
message, never for dispatch, which stays manifest-driven; and a drift test in the
plugin's own module pins each entry to that plugin's real manifest, so adding or
removing a verb fails the build until the table follows.

### Missing-plugin formats

A collection can name a format that a plugin supplies. The same recipe therefore
reads on a machine with the plugin installed and fails to read on one without it,
which forces an answer for every project-wide command: `kapi status`,
`kapi check --ship`, and the source settle inside `kapi up`. Each may find that
one collection out of twenty cannot be opened.

They report that collection as unread and measure the rest. Aborting would make
an entire project unreportable because of one optional dependency, and the
collections that do read are almost always the ones the command was asked about.

The skip is reported, never silent. Coverage computed over content that was
never opened looks identical to coverage over content that was read and found
complete, so the unreadable format names travel back out of the rollup. Three
consumers surface them: `source.unreadable` in the JSON output, a warning naming
the plugin to install, and an event on the convergence stream.

Only a missing reader survives. `registry.ErrUnknownFormat` is a sentinel, so
callers match it with `errors.Is` rather than on message text. An unknown format
means the file was never opened. Any other read error means it was opened and is
broken, and that still fails the command.

A gate written for content in a plugin format installs that plugin
(`make check-governed-prose` stages it). Degrading applies to the project-wide
sweep, not to the gate built for the collection.

### Registry and signing

A registry is a JSON index served over HTTPS. The default registry is
`https://neokapi.github.io/registry/manifest-plugins.json`. The schema maps
plugin name → versions → per-platform tarball URL, SHA-256, and cosign
certificate identity:

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

`kapi plugin install` downloads the tarball plus the Sigstore JSON bundle,
verifies SHA-256 against the registry-pinned hash, then verifies the bundle's
signing certificate against the pinned identity and OIDC issuer using
[`sigstore-go`](https://github.com/sigstore/sigstore-go). A registry entry
missing a signature, certificate identity, or OIDC issuer refuses to install
unless `--unsafe` is passed, which skips both the hash and the signature check
and says so.

A one-hour cache at `$XDG_CACHE_HOME/kapi/registry-index.json` keeps auto-install
prompts cheap; explicit `kapi plugin install / search / update-index` always
fetches fresh.

### Plugin signing vs. OS notarization

Plugin signing sits on a different trust layer than the OS code signing applied
to the kapi CLI and desktop apps. The two are independent and answer different
questions:

| Layer | Question | Mechanism | Triggered by |
| ----- | -------- | --------- | ------------ |
| **Supply chain** | Is this the genuine, untampered plugin? | cosign / Sigstore bundle + SHA-256, verified at install | every `kapi plugin install` |
| **OS Gatekeeper / SmartScreen** | Will the OS let the binary run without a warning? | Apple Developer ID + notarization (macOS); Authenticode (Windows) | the `com.apple.quarantine` xattr, set only by browser and mail downloads |

Plugins rely on the **supply-chain** layer only. `kapi plugin install` fetches
tarballs over HTTPS using Go's HTTP client, which does **not** set the quarantine
attribute, and unpacks them under the data directory. The extracted binary is
therefore never quarantined, so macOS Gatekeeper and Windows SmartScreen never
engage on it; no Apple notarization or Authenticode signature is required for a
plugin to run. The cosign signature plus the SHA-256 check is the meaningful
integrity guarantee, and it is enforced on every platform.

This is the inverse of the kapi CLI and desktop apps, which **are**
Developer-ID-signed and notarized (macOS) and Authenticode-signed (Windows):
users fetch those through a browser, so they arrive quarantined and must clear
Gatekeeper or SmartScreen on first launch.

The reasoning holds even for a plugin that is genuine native code, a JVM
app-image with a bundled runtime, say. Installed unquarantined via `kapi plugin
install` and verified by cosign plus SHA-256, it runs without OS-level signing.
Deep-signing and notarizing such a plugin (the launcher plus every bundled
dynamic library, across each version × OS × architecture) is not done, because
it buys nothing for the programmatic install path. A plugin would
need OS code signing only if it were also distributed as a **direct browser
download**, which is out of scope for the registry-driven model.

### JSON Schema validation for `schema_extensions`

A plugin can declare recipe schema keys it owns:

```json
{
  "schema_extensions": [
    { "name": "myplugin", "scope": "project", "json_schema": "schemas/myplugin.json" }
  ]
}
```

At plugin-register time, kapi loads `<plugin-dir>/schemas/myplugin.json`,
compiles it, and registers an extension decoder with `core/project`. When a
recipe is loaded, the decoder validates the YAML payload against the compiled
schema. Failures render with the recipe path prefix and the JSON Schema
constraint that failed.

### Protocol versioning and conformance

The protocol is versioned as a whole, independently of any kapi release.
`conformance.ProtocolVersion` names the revision this repository implements.
Within a version the rules are **additive**: capabilities, manifest fields, and
conformance checks may be added, but an existing rule does not change and a check
ID is never renamed or repurposed. A change that would break a conforming plugin
is a new protocol version.

There is **no protocol-version handshake on the wire**. The manifest
is read from disk before any process is launched, so incompatibility is detected
structurally rather than negotiated at runtime, which is also what keeps
discovery free of subprocess launches.

`core/plugin/conformance` is the specification in executable form: a black-box
driver that exercises the manifest rules, the standard verbs, and whichever
transports a manifest declares, against an installed plugin directory. Three
properties make it usable from outside this repository, and each one is a
constraint on where it may live:

- **Framework-layer, so it is cheap to import.** It sits in the framework module
  and depends only on `core/plugin/manifest`, `core/plugin/proto`, and gRPC: no
  host module, no CLI, no cobra tree, nothing under a copyleft licence. A plugin
  repository adds one `require` on a released `github.com/neokapi/neokapi`.
- **Black-box, so it is language-neutral.** It reads no plugin source and spawns
  the plugin exactly as the host does, so a JVM plugin and a Go plugin are
  checked by the same code.
- **Side-effect-free by default.** It runs only the read-only standard verbs plus
  deliberately invalid arguments that prove a plugin rejects them. Invoking a
  declared command, reading a document through a declared format, or calling a
  segmenter is opt-in, because only the plugin author knows which capabilities
  are safe to exercise unattended.

Checks are `required` or `advisory`: an advisory failure is reported but does not
clear conformance, naming something the host tolerates today that a plugin should
still fix. A check that does not apply is skipped, never failed. The
authoritative check list is `conformance.Checks()`.

This is what lets a plugin repository own its own tests. A plugin repository runs
the suite against released kapi versions on its own schedule and reports its own
conformance; nothing about it builds, tests, or gates anything here.

### First-party plugins

The first-party plugins live under `plugins/`, one module each, and are indexed
in the [registry](https://github.com/neokapi/registry) like any third-party
plugin. They cover the capability kinds the framework keeps out of its own
binary: native-library formats (PDF through PDFium, source code through
tree-sitter), ML segmentation, speech and media recognition, vision layout, and
the embedding model behind the similarity checks. Two are described here because
they shape the format system; the others are described where their capability
is ([M-02](../multilingual/m-02-segmentation.md),
[M-03](../multilingual/m-03-multimodal-content.md),
[E-08](e-08-document-structure-tiers.md)).

**kapi-pdfium** (`plugins/pdfium/`) is a first-party Mode-C format plugin
providing a high-fidelity `pdf` reader backed by Google's PDFium (via cgo). It
extracts correct text, including CID/Type0 fonts and CJK, per-block and
per-glyph geometry, and document structure, and it runs as an isolated daemon so
a malformed-PDF crash dies with the subprocess rather than with kapi. The CLI's
Homebrew formula depends on the plugin's, so a `brew install` brings it along;
any other install adds it with `kapi plugin install pdfium`, and the desktop app
installs it on demand the first time a PDF is opened. Both host it over the same
discovery and daemon pool, so there is one engine rather than one per host. The
full PDF subsystem is described in [E-08](e-08-document-structure-tiers.md).

**kapi-sourcecode** (`plugins/sourcecode/`) is a first-party Mode-C format
plugin providing a `sourcecode` reader over tree-sitter grammars. It answers a
question no other reader can: which strings in a program are prose. A Homebrew
cask spells both `desc "Desktop workbench…"` and `zap trash:
["~/Library/Caches/Kapi"]` as string literals. A pattern cannot separate them.
The syntax tree can, because it knows the first is an argument to `desc` and the
second an element of an array. A recipe names the prose-bearing calls with
`nodePathPatterns`, the way it already names prose-bearing keys in YAML or JSON.

It is **read-only**, declared as `capabilities: ["read"]` and backed by the
absence of a writer on either side of the boundary. A round-trip error in a
document mangles a paragraph. In a program it produces something that does not
compile, or something that does compile with a changed string escape. Writing
into source is a codemod, a different discipline with different correctness
conditions, and it stays out of scope.

Like the PDF reader it runs as a daemon, so a parser fault stays in the
subprocess. Its config also lives in core (`core/formats/sourcecode`) while the
cgo stays in the plugin, giving the format one config definition and keeping
grammars out of the framework.

A **separately-licensed platform plugin** demonstrates the licence boundary the
model exists for: it attaches over the manifest model, is distributed on its own
terms through its own Homebrew formula (which depends on `kapi` and drops its
binary into `share/kapi/plugins/<plugin>/`), and requires no re-licensing of
`kapi`.

A minimal Go reference plugin in `examples/plugins/hello/` covers Mode A and Mode
B with no third-party dependencies.

## Code layout and distribution

The framework module keeps the parts a plugin author needs:
`core/plugin/manifest` (manifest types plus the embedded JSON Schema),
`core/plugin/proto` (the gRPC service definitions a Mode-C daemon implements),
`core/plugin/protoconvert` (Part ↔ proto translation), and
`core/plugin/conformance` (the protocol conformance suite). Those four are
importable on their own: an out-of-tree plugin repository depends on
a released framework module and nothing else.

The host-side runtime (discovery, dispatch, the daemon pool, the registry
client, cosign verification, and the Mode-C format client) lives in
`host/pluginhost/`.

Native binaries ship for `linux/amd64`, `linux/arm64`, `darwin/arm64`, and
`windows/amd64`. `darwin/amd64` is not in the release matrix:
Apple has dropped Intel from new product lines and macos-13 runners on GitHub
Actions are scarce. Intel users can run the arm64 binary under Rosetta.

### Release shape

A plugin releases on its own tag, `<plugin>-vX.Y.Z`, through its own workflow
(`.github/workflows/release-<plugin>.yml`). The release builds one tarball per
platform with the plugin directory at its top level (`manifest.json`, the
binary, any schemas and bundled libraries) and the plugin's `LICENSE` text
beside them, so a tarball carries the terms of the work it contains; a plugin
that bundles a third-party runtime ships that runtime's licence text too. The
GitHub release is published as not-latest, so a tool resolving the repository's
latest release still finds kapi, and the same run writes the plugin's registry
entry and renders its Homebrew formula from one set of checksums, so the two
install channels move on one tag.

## Related

- [E-02: The format system](e-02-format-system.md): how plugin and bridge formats register into the one registry
- [E-03: The tool system](e-03-tool-system.md): plugin tools and plugin-contributed tool-group members
- [E-04: Flows and I/O binding](e-04-flows-and-io-binding.md): the `source_connectors` capability as a provider binding
- [E-08: Document structure tiers](e-08-document-structure-tiers.md): the first-party PDF plugin in detail
- [C-01: The project model](../context/c-01-project-model.md): `requires:` and the recipe schema extensions plugins own
- [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1): the versioned, language-neutral specification
- [`core/plugin/manifest/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/manifest): Go types and embedded JSON Schema
- [`host/pluginhost/`](https://github.com/neokapi/neokapi/tree/main/host/pluginhost): host-side runtime (discovery, dispatch, daemon pool, registry, cosign)
- [`examples/plugins/hello/`](https://github.com/neokapi/neokapi/tree/main/examples/plugins/hello): minimal Go reference plugin
- [`core/plugin/conformance/`](https://github.com/neokapi/neokapi/tree/main/core/plugin/conformance): the protocol conformance suite, importable from a released kapi
- [neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge): a third-party plugin in a non-Go language (a JVM Mode-C daemon exposing the Okapi Framework's filters); it consumes released kapi versions and reports conformance on its own schedule
- [neokapi/registry](https://github.com/neokapi/registry): the published plugin index
