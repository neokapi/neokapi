---
id: 007-plugin-system
sidebar_position: 7
title: "AD-007: Plugin System and Okapi Bridge"
---
# AD-007: Plugin System and Okapi Bridge

## Context

Plugins enable third-party formats, tools, connectors, and providers that evolve
independently of the core framework and of each other. The key requirements are:

- **Crash isolation** — a buggy plugin must not crash the host process
- **Language agnosticism** — plugins could be written in Go, Java, Python, Rust
- **Versioned protocol** — host and plugin can negotiate capabilities
- **Multi-version support** — different projects may need different plugin versions
- **Simple discovery** — scan a directory for executables matching naming conventions

Okapi provides 40+ production-proven format filters (DOCX, XLSX, EPUB, IDML, PDF,
etc.) that represent years of development. Rewriting them all in Go is impractical
in the near term. These filters need to be accessible from Go with minimal overhead.

Go's standard `plugin` package loads shared objects into the same process, offering
no crash isolation and limited platform support (Linux/macOS only).

## Decision

### Out-of-Process Plugins via go-plugin

Use [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) for Go binary
plugins and direct gRPC for bridge plugins. Go binary plugins communicate via
`net/rpc` over stdin/stdout with a magic cookie handshake. Bridge plugins (like
the Okapi bridge) run a gRPC server and the Go side connects as a client;
protocol buffers define the service contract.

Plugin discovery uses a two-phase approach that avoids launching external
processes (JVMs, binaries) until they are actually needed:

1. **`ScanMetadata`** — scans versioned plugin directories and reads
   `manifest.json` files and parameter schemas from disk. Format metadata
   (names, MIME types, extensions, capabilities) is registered into the
   format registry so that `kapi formats list` works without starting any
   external process.
2. **`LoadBridges`** — called lazily when a flow actually needs a bridge
   format. Creates the shared `BridgePool` and registers reader/writer
   factories that acquire a bridge instance on demand.

This design means listing installed plugins and formats is instant, and
JVM startup cost is only paid when processing files.

### Plugin Types

| Type | What it adds | Example |
|------|-------------|---------|
| **Bundle** | Collection of formats and/or tools | Okapi bridge (40+ formats) |
| **Format** | Reader + Writer for a file format | `gokapi-plugin-docx` |
| **Tool** | Processing step in a flow | `gokapi-plugin-terminology` |
| **Connector** | Bidirectional system integration | `gokapi-plugin-contentful` |
| **Provider** | AI/LLM or MT backend | `gokapi-plugin-deepl-v2` |

Go binary plugins are discovered by scanning for executables named
`gokapi-plugin-*` in versioned directories. Bridge plugins are discovered
via `manifest.json` files in their version directories.

### Bundles

A **bundle** is a plugin that packages multiple formats and/or tools into a
single distributable unit. The Okapi bridge is the canonical bundle — it provides
40+ format filters and several processing tools in one JAR. Bundles are installed
and versioned as a single unit, but their individual capabilities (formats, tools)
are registered separately into the core registries.

Bundles are declared with `PluginType: "bundle"` in the registry manifest.
Each bundle lists its capabilities explicitly, allowing the CLI to search and
filter by capability type:

```bash
kapi plugins search --bundle         # list all bundles
kapi plugins search --format         # list formats (including those in bundles)
kapi plugins search --tool           # list tools (including those in bundles)
kapi plugins search --bundle --tool  # bundles that contain tools
```

The `--bundle`, `--format`, and `--tool` flags are combined with AND logic
alongside existing `--type`, `--mime`, and `--ext` filters.

The **connector** type enables community-built connectors for CMS platforms,
design tools, and other systems that integrate with gokapi's connector-first
architecture ([AD-005](./005-connector-system.md)). Connector plugins implement
the same bidirectional sync interface as built-in connectors, pulling content into
the store and pushing translations back.

### Multi-Version Support

Multiple versions of the same plugin can be installed side-by-side using a
versioned directory layout:

```
~/.config/kapi/plugins/
  okapi/
    1.46.0/
      version.json
      manifest.json
      schemas/
      gokapi-okapi-bridge.jar
    1.47.0/
      version.json
      manifest.json
      schemas/
      gokapi-okapi-bridge.jar
```

Each version directory contains a `version.json` with name, version, and
install type, and a `manifest.json` (`BundledManifest`) listing capabilities,
command configuration, and plugin type. Bridge plugins also ship a `schemas/`
directory with filter parameter schemas that drive configuration, preset
extraction, and format metadata — all readable without starting the JVM. The plugin loader scans all version directories and registers
capabilities with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only
if no explicit bare-name registration exists, preventing conflicts.

### Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- **`ScanMetadata`**: Scans versioned subdirectories, reads `manifest.json`
  and `schemas/` from disk, extracts presets, registers format metadata into
  registries — all without starting any external process
- **`LoadBridges`**: Starts the shared `BridgePool`, launches Go binary plugins
  via `host.PluginManager`, and registers reader/writer factories for bridge
  formats
- **`LoadAll`**: Convenience method that calls `ScanMetadata` then `LoadBridges`
- Manages the shared bridge pool and plugin lifecycle (`Shutdown`, `WarmupBridges`)

### Okapi Bridge

The **Okapi bridge** is a gRPC-based subprocess that hosts Okapi Framework filters.
The Go side launches a JVM (or any process implementing the `BridgeService` gRPC
contract), connects as a gRPC client, and translates between gokapi's Part model
and Okapi's Event model. The adapter layer wraps the bridge behind standard
`DataFormatReader` / `DataFormatWriter` interfaces — bridge-backed formats are
indistinguishable from native Go formats at the registry level.

The bridge protocol (`core/plugin/proto/v2/gokapi_bridge.proto`) defines eight RPCs:

| RPC | Direction | Purpose |
|-----|-----------|---------|
| `ListFilters` | Unary | Discover available filters (runtime fallback; metadata preferred) |
| `Info` | Unary | Get metadata for a specific filter |
| `Open` | Unary | Open a document with a filter for reading |
| `Read` | Server-streaming | Stream extracted Parts from the document |
| `Write` | Client-streaming | Send Parts to reconstruct the document |
| `Close` | Unary | Release filter resources |
| `RoundTrip` | Bidirectional-streaming | Complete read→process→write cycle in a single RPC |
| `Shutdown` | Unary | Gracefully stop the bridge process |

The `Read` and `Write` RPCs use gRPC streaming, so content flows incrementally
rather than requiring the entire document to be buffered in memory.

`RoundTrip` combines the Open/Read/Write/Close lifecycle into a single
bidirectional stream: the server reads the document and streams Parts to the
client, the client processes each Part and sends it back, and the server writes
the output. This eliminates multiple RPC round-trips and ensures only one bridge
instance is needed for the entire operation. Multiple `RoundTrip` calls can
share the same JVM via concurrent gRPC streams.

Note: `ListFilters` and `Info` exist in the protocol but are not the primary
discovery mechanism. The `PluginLoader` reads format metadata from
`manifest.json` and schema files on disk during `ScanMetadata`, avoiding JVM
startup for format listing and configuration. The RPCs serve as a runtime
fallback when schemas are unavailable.

### Global Bridge Pool

A single process-wide `BridgePool` manages all bridge subprocess instances, keyed
by process configuration (command + args), with a global capacity limit of
`runtime.NumCPU()`. The pool uses LIFO reuse, cross-key eviction to prevent
deadlocks, and `sync.Cond` with `Broadcast()` for fair waiting.

See [Bridge Protocol](/docs/notes/plugin-bridge-protocol) for the gRPC service
definition, BridgePool acquire algorithm, and bridge descriptor format.

### Plugin Configuration

Plugins declare their configuration parameters via JSON schema files shipped in
the `schemas/` directory of each plugin version. Schemas define parameter groups,
defaults, and validation rules. The `SchemaRegistry` loads these from disk
during `ScanMetadata` — no external process needed. Configuration is namespaced
by plugin type and name in the project config file, integrated with the
Viper-based layered configuration system ([AD-001](./001-vision.md)). On the
CLI, plugin parameters become namespaced flags.

See [Plugin Bridge Protocol](/docs/notes/plugin-bridge-protocol) for the schema
format and configuration examples.

### Presets

**Presets** are named configuration bundles that simplify format and project setup.
They provide reusable parameter sets without requiring users to manually specify
every format option.

#### Format Presets

A **format preset** is a named parameter configuration for a specific format. Three
sources supply format presets:

- **Bridge configurations**: Auto-surfaced from `x-filter.configurations` in
  composite schemas. For example, `okf_html-wellFormed` becomes preset `wellFormed`
  for format `okf_html`.
- **Plugin presets**: Defined in `presets.yaml` within plugin version directories.
  Plugins can ship presets for any format they provide.
- **Local presets**: Defined in `.bowrain/config.yaml` under `format_presets:`.
  Users create project-specific configurations.

#### Framework Presets

A **framework preset** is a project setup template providing mappings, format
configs, exclude patterns, and flow defaults. Built-in templates (Next.js,
react-intl, Angular) ship with kapi; plugins can add more.

Framework presets are applied via `bowrain init --preset nextjs` and set the `preset:`
field in `.bowrain/config.yaml` for documentation.

#### Configuration Layering

Configuration resolution follows a three-layer model:

```
Format defaults → Preset config → Local overrides
```

Deep map merge at each layer: maps merge recursively; scalar values replace.
This lets presets provide comprehensive defaults while users override specific
values per mapping.

#### Format Reference Syntax

The format reference syntax `name[@version][:preset]` uses separate delimiters
for version and preset:

- `@` denotes a **version pin** (`okf_html@1.46.0`)
- `:` denotes a **preset reference** (`okf_html:wellFormed`)
- Both can be combined: `okf_openxml@0.38:wellFormed`

| Reference | Version | Preset |
|---|---|---|
| `okf_openxml` | latest | default |
| `okf_openxml@0.38` | 0.38 | default |
| `okf_openxml:wellFormed` | latest | wellFormed |
| `okf_openxml@0.38:wellFormed` | 0.38 | wellFormed |

#### Design Principle

Presets are a **capability**, not a plugin type. Existing plugin types (bundles,
formats) can declare preset capabilities via `presets.yaml` files. No additional
plugin type is needed.

### Plugin Registries

Plugins are distributed via **registries** — JSON endpoints listing available
plugins with versions, capabilities, and download URLs. Multiple registries can
be configured, enabling organizations to host internal plugins alongside the
official registry.

**Registry configuration** follows a three-level resolution:

1. **Project config** (`.bowrain/config.yaml`) — `registries:` list. When present,
   overrides global config entirely (no merging).
2. **Global config** (`~/.config/kapi/kapi.yaml`) — `registries:` list. Managed
   via `kapi registry add/remove/list`.
3. **Fallback** — `plugins.registry` single URL, or the hardcoded official URL.

```yaml
# ~/.config/kapi/kapi.yaml or .bowrain/config.yaml
registries:
  - name: official
    url: https://gokapi.github.io/registry/plugins.json
    channels: [default, snapshot]
  - name: company
    url: https://registry.example.com/plugins.json
```

Each registry entry can declare its available `channels` — named release tracks
(e.g., `snapshot` for pre-release builds). The `channels` field is informational,
helping teams document which channels a registry provides.

**Resolution behavior:**
- **Install/update**: iterate registries in order, first match wins
- **Search/list**: merge results from all registries, deduplicating by name+version
- `--registry <name>` flag pins to a specific named registry
- `--channel <name>` flag derives channel-specific URLs (orthogonal to registry selection)

**Registry management:**
```bash
kapi registry list                                          # List configured registries
kapi registry add <name> <url>                              # Add to global config
kapi registry add <name> <url> --channels default,snapshot  # Add with channel declarations
kapi registry remove <name>                                 # Remove from global config
```

### Plugin Governance

**Protocol stability.** The v1 protocol (defined in `plugin/proto/v1/gokapi.proto`)
is frozen. New fields can be added (protobuf is forward-compatible), but existing
fields cannot be changed or removed. New capabilities require a new protocol
version (v2, v3, etc.). The host supports at least two protocol versions
simultaneously, giving plugin authors time to migrate.

**Quality tiers.** Not all plugins carry the same trust level:

| Tier | Source | Trust | Installation |
|------|--------|-------|-------------|
| **Built-in** | `formats/`, `tools/` | Full | Ships with binary |
| **Official** | gokapi org registry | High | `kapi plugins install` |
| **Community** | Third-party registry | Medium | `kapi plugins install --registry <name>` |
| **Local** | User-built | User's risk | Copy to plugin directory |

**Built-in vs plugin split.** Formats with broad usage (HTML, XML, JSON, YAML,
XLIFF, PO, Markdown, CSV, SRT, VTT) are built-in. Specialized or proprietary
formats (DOCX, IDML, InDesign, MIF, DITA) are plugins. Core tools (pseudo-translate,
QA check, word count, segmentation, TM leverage) are built-in. Integration tools
(specific TMS connectors, custom MT engines) are plugins. AI providers (Anthropic,
OpenAI, Ollama) are built-in because AI translation is a core value proposition.

The test: if removing it makes gokapi feel incomplete for the common case, it
should be built-in.

## Alternatives Considered

- **Go `plugin` package** — in-process; no crash isolation; Linux/macOS only;
  version skew issues with Go runtime.
- **HTTP/REST plugins** — heavier protocol; requires port allocation; less
  natural for streaming.
- **WASM plugins** — promising but immature Go WASM host support; limited
  system access.
- **JNI/CGo for Java bridge** — tight coupling; crash propagation; complex build.
- **NDJSON over stdin/stdout for bridge** — the original v1 bridge protocol.
  Replaced by gRPC streaming (v2) to support incremental content transfer,
  eliminate OOM for large files, and enable richer error handling.
- **Per-descriptor bridge pools** — simpler but allows JVM count to scale
  linearly with plugin versions (e.g., 2 versions x NumCPU = 2x JVMs).
- **Docker-based isolation** — heavyweight; doesn't work for desktop app.

## Consequences

- Plugins run out-of-process: crashes do not affect the host
- Any language that can implement a gRPC server works as a plugin
- Protocol versioning in the handshake prevents incompatible plugins from loading
- Multiple versions of the same plugin coexist without conflict
- All 40+ Okapi filters are accessible without Go rewrites via the Okapi bridge
- JVM count is bounded by one `maxSize` regardless of plugin versions; different
  JARs share capacity fairly via eviction
- Bundles package multiple formats and tools as a single installable unit,
  simplifying distribution while allowing individual capability registration
- Multiple registries can be configured at project or global level, enabling
  organizations to host internal plugins alongside the official registry
- The `kapi registry` command provides add/remove/list management of global
  registries; project-level registries override global ones entirely
- The CLI searches both standalone plugins and bundles; `--bundle`, `--format`,
  and `--tool` flags allow users to narrow results by plugin kind
- Connector plugins extend gokapi's integration capabilities, enabling
  community-built connectors for CMS platforms and design tools
  ([AD-005](./005-connector-system.md))
- Plugin configuration integrates cleanly with the Viper config system
  ([AD-001](./001-vision.md)); namespaced flags prevent collisions
- Plugin scope is enforced by the RPC boundary -- plugins cannot modify core
  behavior
- The built-in vs plugin split keeps the out-of-box experience complete while
  allowing specialization through plugins
