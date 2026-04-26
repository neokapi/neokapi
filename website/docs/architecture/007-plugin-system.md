---
id: 007-plugin-system
sidebar_position: 7
title: "AD-007: Plugin System and Okapi Bridge"
---

# AD-007: Plugin System and Okapi Bridge

## Summary

Plugins are out-of-process executables communicated with via gRPC, giving
crash isolation and language agnosticism. Go binary plugins use HashiCorp
`go-plugin`'s magic-cookie handshake; bridge plugins (like the Okapi
bridge) run a long-lived gRPC server and the framework connects as a
client. Plugins come in five kinds — Bundle, Format, Tool, Connector,
Provider — and are discovered in two phases: `ScanMetadata` reads
manifests and schemas from disk so `kapi formats list` never launches a
subprocess, and `LoadBridges` starts processes lazily when a flow
actually needs them. Multiple versions of the same plugin coexist in
versioned directories; format references use the syntax
`name[@version][:preset]`. Plugin registries serve installable plugin
archives; multiple registries can be configured at project and global
level.

## Context

Plugins enable third-party formats, tools, connectors, and providers to
evolve independently of the framework. Key requirements:

- **Crash isolation** — a buggy plugin must not crash the host process.
- **Language agnosticism** — plugins may be written in Go, Java, Python,
  or Rust.
- **Versioned protocol** — host and plugin negotiate capabilities
  explicitly.
- **Multi-version support** — different projects may need different
  plugin versions.
- **Simple discovery** — scan a directory for executables matching
  naming conventions.

Okapi provides 40+ production-proven format filters (DOCX, XLSX, EPUB,
IDML, PDF, and more) that represent years of development. Rewriting them
all in Go is not practical; they need to be accessible from Go with
minimal overhead.

Go's standard `plugin` package loads shared objects into the same
process, offering no crash isolation and limited platform support. It is
unsuitable.

## Decision

### Out-of-process plugins

The framework uses [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)
for Go binary plugins and direct gRPC for bridge plugins. Go binary
plugins communicate over stdin/stdout with a magic-cookie handshake.
Bridge plugins run a gRPC server and the Go side connects as a client;
protocol-buffer definitions specify the service contract.

Plugins run as separate processes. A plugin crash does not affect the
host; credentials are passed via environment variables, not shared
memory.

### Plugin types

| Type          | What it adds                       | Example                      |
| ------------- | ---------------------------------- | ---------------------------- |
| **Bundle**    | Collection of formats and/or tools | Okapi bridge (40+ formats)   |
| **Format**    | Reader + Writer for a file format  | `neokapi-plugin-docx`        |
| **Tool**      | Processing step in a flow          | `neokapi-plugin-terminology` |
| **Connector** | Bidirectional system integration   | `neokapi-plugin-contentful`  |
| **Provider**  | AI/LLM or MT backend               | `neokapi-plugin-deepl-v2`    |

Go binary plugins are discovered by scanning for executables named
`neokapi-plugin-*` in versioned directories. Bridge plugins are
discovered via `manifest.json` files in their version directories.

A **bundle** packages multiple formats and/or tools into a single
distributable unit. The Okapi bridge is the canonical bundle — one JAR,
40+ format filters, several processing tools. Bundles install and version
as a unit, but their individual capabilities register separately into the
core registries, so callers of the FormatRegistry cannot tell that two
formats share a JAR.

### Two-phase discovery

Plugin discovery avoids launching external processes until they are
actually needed:

1. **`ScanMetadata`** — walks versioned plugin directories and reads
   `manifest.json` files and parameter schemas from disk. Format
   metadata (names, MIME types, extensions, capabilities) is registered
   into the format registry so that `kapi formats list` works without
   starting any external process.
2. **`LoadBridges`** — called lazily when a flow actually needs a bridge
   format. Creates the shared `BridgePool` and registers reader/writer
   factories that acquire a bridge instance on demand.

Listing installed plugins and formats is instant; JVM startup cost is
only paid when processing files.

### Multi-version support

Multiple versions of the same plugin install side-by-side using a
versioned directory layout:

```
~/.config/kapi/plugins/
  okapi/
    1.46.0/
      version.json
      manifest.json
      schemas/
      neokapi-okapi-bridge.jar
    1.47.0/
      version.json
      manifest.json
      schemas/
      neokapi-okapi-bridge.jar
```

Each version directory contains:

- `version.json` — name, version, install type
- `manifest.json` — capabilities, command configuration, plugin type
- `schemas/` — filter parameter schemas (bridge plugins), readable without
  starting the JVM

The plugin loader scans all version directories and registers capabilities
with versioned names:

- `okapi-html@1.46.0` — specific version
- `okapi-html` — bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases register only if
no explicit bare-name registration exists, preventing conflicts.

### Format reference syntax

Format references use separate delimiters for version and preset:

- `@` denotes a **version pin** (`okf_html@1.46.0`)
- `:` denotes a **preset reference** (`okf_html:wellFormed`)
- Both combine: `okf_openxml@0.38:wellFormed`

| Reference                     | Version | Preset     |
| ----------------------------- | ------- | ---------- |
| `okf_openxml`                 | latest  | default    |
| `okf_openxml@0.38`            | 0.38    | default    |
| `okf_openxml:wellFormed`      | latest  | wellFormed |
| `okf_openxml@0.38:wellFormed` | 0.38    | wellFormed |

### Presets as a capability

A **preset** is a named parameter configuration. Presets are a capability,
not a plugin type — existing plugin types (bundles, formats) declare
preset capabilities via `presets.yaml` files. Three sources supply format
presets:

- **Bridge configurations** — auto-surfaced from `x-filter.configurations`
  in composite schemas. For example, `okf_html-wellFormed` becomes preset
  `wellFormed` for format `okf_html`.
- **Plugin presets** — defined in `presets.yaml` within plugin version
  directories.
- **Local presets** — defined in project or user config under
  `format_presets:`.

Configuration resolution follows a three-layer model:

```
Format defaults → Preset config → Local overrides
```

Deep map merge at each layer: maps merge recursively, scalar values
replace. Presets provide comprehensive defaults while users override
specific values per mapping.

### Plugin loader

`PluginLoader` in `core/plugin/loader/` ties discovery together:

- **`ScanMetadata`** — scans versioned subdirectories, reads
  `manifest.json` and `schemas/` from disk, extracts presets, registers
  format metadata into registries — all without starting any external
  process.
- **`LoadBridges`** — starts the shared `BridgePool`, launches Go binary
  plugins via `host.PluginManager`, and registers reader/writer factories
  for bridge formats.
- **`LoadAll`** — convenience method that calls `ScanMetadata` then
  `LoadBridges`.
- Manages the shared bridge pool and plugin lifecycle (`Shutdown`,
  `WarmupBridges`).

### Okapi bridge

The **Okapi bridge** is a gRPC-based subprocess that hosts Okapi
Framework filters. The Go side launches a JVM (or any process
implementing the `BridgeService` gRPC contract), connects as a gRPC
client, and translates between neokapi's Part model and Okapi's Event
model. An adapter layer wraps the bridge behind standard
`DataFormatReader` / `DataFormatWriter` interfaces — bridge-backed
formats are indistinguishable from native Go formats at the registry
level.

The bridge protocol (`core/plugin/proto/v2/neokapi_bridge.proto`) defines
a minimal unified streaming API:

| RPC        | Direction               | Purpose                            |
| ---------- | ----------------------- | ---------------------------------- |
| `Process`  | Bidirectional-streaming | Complete read→process→write cycle  |
| `Shutdown` | Unary                   | Gracefully stop the bridge process |

`Process` combines the entire document lifecycle into a single
bidirectional stream:

- The Java side reads events from the Okapi filter, converts subscribed
  events to lightweight `ContentBlock` messages (no skeleton — ~10x
  smaller than full `BlockMessage` for typical XLSX cells), batches them
  into `ContentBlockBatch` messages (up to 1024 blocks), and streams
  them to Go.
- Go processes the blocks through its tool chain and sends them back
  individually.
- The Java side applies translations and writes output in a two-thread,
  single-pass pipeline (one filter read, no double I/O):
  - **Reader thread** — reads filter events, sends subscribed parts to
    Go via gRPC, enqueues events into a bounded queue for the writer
    thread.
  - **Writer thread** — dequeues events, applies translations from a
    translation queue (fed by gRPC responses from Go), writes to the
    filter writer.

The `ProcessHeader.subscribe_parts` field controls which event types
cross the gRPC boundary. Subscribing only to Block events (`[4]`) means
structural events (Layer, Data, Group) are written directly by Java
without gRPC round-trips — reducing message count from ~570K to ~157K on
large XLSX files.

Format metadata is not discovered via gRPC — the `PluginLoader` reads it
from `manifest.json` and schema files on disk during `ScanMetadata`,
avoiding JVM startup for format listing.

See [Plugin Bridge Protocol](/notes-internal/plugin-bridge-protocol) for the
full gRPC service definition, wire format, protobuf messages, and
performance tuning notes.

### Bridge registry and concurrency

A single process-wide `BridgeRegistry` manages bridge instances with
semaphore-based concurrency control:

- **Global semaphore** bounds total concurrent streams across all JVMs.
- **Per-JVM semaphore** bounds concurrent streams on each JVM.
- **Daemon mode** (`KAPI_BRIDGE_DAEMON=1`): JVMs persist across kapi
  invocations, discovered via address files on disk.
- **Pipeline semaphore** on the Java side rejects excess streams with
  `RESOURCE_EXHAUSTED`.

JVM count is bounded regardless of how many plugin versions are
installed; different JARs share capacity fairly via eviction.

### Plugin configuration

Plugins declare their configuration parameters via JSON schema files
shipped in the `schemas/` directory of each plugin version. Schemas
define parameter groups, defaults, and validation rules. The
`SchemaRegistry` loads these from disk during `ScanMetadata` — no
external process needed. Configuration is namespaced by plugin type and
name in the project config file, integrated with the Viper-based layered
configuration system ([AD-001: Vision and Module Architecture](001-vision-and-modules.md)).
On the CLI, plugin parameters become namespaced flags.

### Plugin registries

Plugins are distributed via **registries** — JSON endpoints listing
available plugins with versions, capabilities, and download URLs.
Multiple registries can be configured, letting organizations host
internal plugins alongside the official registry.

Registry configuration follows a three-level resolution:

1. **Project config** — `registries:` list. When present, overrides
   global config entirely (no merging).
2. **Global config** (`~/.config/kapi/kapi.yaml`) — `registries:` list.
   Managed via `kapi registry add/remove/list`.
3. **Fallback** — `plugins.registry` single URL, or the hardcoded
   official URL.

```yaml
registries:
  - name: official
    url: https://neokapi.github.io/registry/plugins.json
    channels: [default, snapshot]
  - name: company
    url: https://registry.example.com/plugins.json
```

Each registry entry declares its available `channels` — named release
tracks (e.g., `snapshot` for pre-release builds). The `channels` field
is informational, helping teams document which channels a registry
provides.

Resolution behavior:

- **Install/update** — iterate registries in order; first match wins.
- **Search/list** — merge results from all registries, deduplicating by
  name+version.
- `--registry <name>` pins to a specific named registry.
- `--channel <name>` derives channel-specific URLs (orthogonal to
  registry selection).

### Quality tiers

Not all plugins carry the same trust level:

| Tier          | Source               | Trust       | Installation                             |
| ------------- | -------------------- | ----------- | ---------------------------------------- |
| **Built-in**  | `formats/`, `tools/` | Full        | Ships with binary                        |
| **Official**  | neokapi org registry | High        | `kapi plugins install`                   |
| **Community** | Third-party registry | Medium      | `kapi plugins install --registry <name>` |
| **Local**     | User-built           | User's risk | Copy to plugin directory                 |

Formats with broad usage (HTML, XML, JSON, YAML, XLIFF, PO, Markdown,
CSV, SRT, VTT) are built-in. Specialized or proprietary formats (DOCX,
IDML, InDesign, MIF, DITA) are plugins. Core tools (pseudo-translate, QA
check, word count, segmentation, TM leverage) are built-in; integration
tools (specific TMS connectors, custom MT engines) are plugins. AI
providers (Anthropic, OpenAI, Ollama, Gemini) are built-in because AI
translation is a core value proposition.

The test: if removing it makes neokapi feel incomplete for the common
case, it is built-in.

### Protocol stability

The v1 protocol (defined in `plugin/proto/v1/neokapi.proto`) is frozen.
New fields can be added (protobuf is forward-compatible), but existing
fields cannot change or be removed. New capabilities require a new
protocol version (v2, v3, etc.). The host supports at least two protocol
versions simultaneously, giving plugin authors time to migrate.

### Security

- **Plugins run out of process.** A plugin crash does not affect the
  host.
- **Credentials are passed via environment variables**, not shared
  memory or files readable by other processes.
- **Plugin scope is enforced by the RPC boundary** — plugins cannot
  modify core behavior; they can only respond to framework-defined RPC
  calls.

## Consequences

- Plugins run out-of-process: crashes do not affect the host.
- Any language that can implement a gRPC server works as a plugin.
- Protocol versioning in the handshake prevents incompatible plugins from
  loading.
- Multiple versions of the same plugin coexist without conflict.
- All 40+ Okapi filters are accessible without Go rewrites via the Okapi
  bridge.
- JVM count is bounded regardless of plugin-version count; different JARs
  share capacity fairly.
- Bundles package multiple formats and tools as a single installable
  unit, simplifying distribution while allowing individual capability
  registration.
- Multiple registries can be configured at project or global level,
  enabling organizations to host internal plugins alongside the official
  registry.
- The CLI searches both standalone plugins and bundles; `--bundle`,
  `--format`, and `--tool` flags narrow results by plugin kind.
- Plugin configuration integrates cleanly with the Viper config system
  ([AD-001: Vision and Module Architecture](001-vision-and-modules.md));
  namespaced flags prevent collisions.
- Plugin scope is enforced by the RPC boundary — plugins cannot modify
  core behavior.
- The built-in vs plugin split keeps the out-of-box experience complete
  while allowing specialization through plugins.
- Two-phase discovery makes `kapi formats list` and `kapi plugins list`
  instant; JVM startup cost is paid only when files are actually
  processed.

## Related

- [AD-001: Vision and Module Architecture](001-vision-and-modules.md) — config system, module layout
- [AD-005: Format System](005-format-system.md) — how plugin formats register alongside native formats
- [AD-006: Tool System](006-tool-system.md) — plugin tools use the same Tool interface
- [Plugin Bridge Protocol](/notes-internal/plugin-bridge-protocol) — full gRPC service definition, wire format, and performance tuning
