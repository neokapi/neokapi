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

Use [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC
transport. Each plugin is a separate executable that communicates with the host
over stdin/stdout using gRPC. Protocol buffers define the service contract.

Plugin discovery scans a directory for executables matching the naming convention
for each plugin type. The host launches each plugin, performs a version handshake,
queries capabilities via the `Info()` RPC, and registers the plugin's capabilities
into the appropriate registry.

### Plugin Types

| Type | What it adds | Discovery | Example |
|------|-------------|-----------|---------|
| **Bundle** | Collection of formats and/or tools | varies | Okapi bridge (40+ formats) |
| **Format** | Reader + Writer for a file format | `gokapi-format-*` | `gokapi-format-docx` |
| **Tool** | Processing step in a flow | `gokapi-tool-*` | `gokapi-tool-terminology` |
| **Connector** | Bidirectional system integration | `gokapi-connector-*` | `gokapi-connector-contentful` |
| **Provider** | AI/LLM or MT backend | `gokapi-provider-*` | `gokapi-provider-deepl-v2` |

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
~/.config/gokapi/plugins/
  okapi/
    1.46.0/
      version.json
      okapi.bridge.json
      gokapi-okapi-bridge.jar
    1.47.0/
      version.json
      okapi.bridge.json
      gokapi-okapi-bridge.jar
```

Each version directory contains a `version.json` manifest with name, version,
and install type. The plugin loader scans all version directories and registers
capabilities with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only
if no explicit bare-name registration exists, preventing conflicts.

### Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads Java bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats, tools, connectors, and providers into the
  core registries
- Manages the shared bridge pool and plugin lifecycle

### Java Bridge

A Go-managed JVM subprocess hosts an adapter that translates between gokapi's Part model and Okapi's Event model. Communication uses synchronous NDJSON over stdin/stdout with base64-encoded content. The adapter layer wraps the bridge protocol behind standard `DataFormatReader` / `DataFormatWriter` interfaces.

### Global Bridge Pool

A single process-wide `BridgePool` manages all JVM instances, keyed by JARPath, with a global capacity limit of `runtime.NumCPU()`. The pool uses LIFO reuse, cross-JAR eviction to prevent deadlocks, and `sync.Cond` with `Broadcast()` for fair waiting.

See [Plugin Bridge Protocol](/docs/notes/plugin-bridge-protocol) for bridge commands/responses, BridgePool acquire algorithm, and bridge descriptor format.

### Plugin Configuration

Plugins declare their configuration parameters via the `Info()` RPC response using `ParameterDescriptor` structs. Configuration is namespaced by plugin type and name in the project config file, integrated with the Viper-based layered configuration system ([AD-001](./001-vision.md)). On the CLI, plugin parameters become namespaced flags.

See [Plugin Bridge Protocol](/docs/notes/plugin-bridge-protocol) for the ParameterDescriptor struct and configuration examples.

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
- **Local presets**: Defined in `.kapi/config.yaml` under `format_presets:`.
  Users create project-specific configurations.

#### Framework Presets

A **framework preset** is a project setup template providing mappings, format
configs, exclude patterns, and flow defaults. Built-in templates (Next.js,
react-intl, Angular) ship with kapi; plugins can add more.

Framework presets are applied via `kapi init --preset nextjs` and set the `preset:`
field in `config.yaml` for documentation.

#### Configuration Layering

Configuration resolution follows a three-layer model:

```
Format defaults → Preset config → Local overrides
```

Deep map merge at each layer: maps merge recursively; scalar values replace.
This lets presets provide comprehensive defaults while users override specific
values per mapping.

#### @-Notation

The `format@suffix` syntax in mapping format fields serves dual purpose:

- If the suffix matches semver (digits and dots only): **version pin**
  (`okf_html@1.46.0`)
- Otherwise: **preset reference** (`okf_html@wellFormed`)

Preset names must not consist solely of digits and dots, ensuring unambiguous
parsing.

#### Design Principle

Presets are a **capability**, not a plugin type. Existing plugin types (bundles,
formats) can declare preset capabilities via `presets.yaml` files. No additional
plugin type is needed.

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
| **Community** | Third-party registry | Medium | `kapi plugins install --source <url>` |
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
  NDJSON over stdio is simpler for the synchronous command-response pattern of
  filter operations.
- **gRPC for bridge** — would work but NDJSON over stdio is simpler for the
  synchronous command-response pattern of filter operations.
- **Per-descriptor bridge pools** — simpler but allows JVM count to scale
  linearly with plugin versions (e.g., 2 versions x NumCPU = 2x JVMs).
- **Docker-based isolation** — heavyweight; doesn't work for desktop app.

## Consequences

- Plugins run out-of-process: crashes do not affect the host
- Any language that can implement a gRPC server works as a plugin
- Protocol versioning in the handshake prevents incompatible plugins from loading
- Multiple versions of the same plugin coexist without conflict
- All 40+ Okapi filters are accessible without Go rewrites via the Java bridge
- JVM count is bounded by one `maxSize` regardless of plugin versions; different
  JARs share capacity fairly via eviction
- Bundles package multiple formats and tools as a single installable unit,
  simplifying distribution while allowing individual capability registration
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
