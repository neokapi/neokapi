---
id: 005-plugin-system
sidebar_position: 5
title: "ADR-005: Plugin System"
---
# ADR-005: Plugin system

## Context

A plugin system is needed for third-party format readers, writers, and tools.
Plugins evolve independently of the core framework and of each other. The key
requirements are:

- Crash isolation: a buggy plugin must not crash the host process
- Language agnosticism: plugins could be written in Go, Java, Python, Rust
- Versioned protocol: host and plugin can negotiate capabilities
- Multi-version support: different projects may need different plugin versions
- Simple discovery: scan a directory for executables

Go's standard `plugin` package loads shared objects into the same process,
offering no crash isolation and limited platform support.

## Decision

### Out-of-Process Plugins via go-plugin

Use [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC
transport. Each plugin is a separate executable that communicates with the host
over stdin/stdout using gRPC. Protocol buffers define the service contract.

Plugin discovery scans a directory for executables matching the naming
convention `gokapi-format-*` or `gokapi-tool-*`. The host launches each
plugin, performs a version handshake, queries capabilities via the `Info()` RPC,
and registers the plugin's formats/tools into the appropriate registry.

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
formats with versioned names:

- `okapi-html@1.46.0` -- specific version
- `okapi-html` -- bare alias pointing to the latest installed version

Semver comparison determines "latest". Bare-name aliases are registered only
if no explicit bare-name registration exists, preventing conflicts.

### Plugin Registry

The plugin registry (`plugin/registry/`) provides:

- `ListAllInstalled(dir)` -- scan for all versions of all plugins
- `CompareSemver(a, b)` -- semantic version comparison
- Version resolution for CLI commands (`kapi plugins list`,
  `kapi plugins install`)

### Plugin Loader

The `PluginLoader` (`plugin/loader/`) ties discovery together:

- Scans the plugin directory for versioned subdirectories
- Loads Go binary plugins via `host.PluginManager`
- Loads Java bridge plugins via bridge descriptors (`*.bridge.json`)
- Registers all discovered formats and tools into the core registries
- Manages the shared bridge pool (see ADR-006) and plugin lifecycle

## Alternatives Considered

- **Go `plugin` package**: in-process; no crash isolation; Linux/macOS only;
  version skew issues with Go runtime.
- **HTTP/REST plugins**: heavier protocol; requires port allocation; less
  natural for streaming.
- **WASM plugins**: promising but immature Go WASM host support; limited
  system access.
- **Single version only**: simpler but forces all projects onto the same
  plugin version; no gradual rollouts.
- **Docker-based isolation**: heavyweight; doesn't work for desktop app.

## How Plugins Affect Applications

Plugins extend gokapi's capabilities across all applications—kapi CLI, Bowrain,
gokapi-server, and CI integrations. Understanding how plugins interact with each
application is critical for maintaining a coherent user experience.

### Plugin Extension Points

Plugins can provide four types of capabilities:

| Type | What it adds | Discovery | Example |
|------|-------------|-----------|---------|
| **Format** | Reader + Writer for a file format | `gokapi-format-*` binary or bridge descriptor | `gokapi-format-docx` adds `.docx` support |
| **Tool** | Processing step in a flow | `gokapi-tool-*` binary | `gokapi-tool-terminology` adds term annotation |
| **Flow** | Named pipeline of tools | Flow definition file (YAML) | `corporate-translate.flow.yaml` |
| **Provider** | AI/LLM or MT backend | `gokapi-provider-*` binary | `gokapi-provider-deepl-v2` |

### kapi CLI

Plugins extend the CLI transparently. On every invocation, `kapi` scans the
plugin directory and registers discovered formats and tools into the same
registries used by built-in components. From the user's perspective, plugin
capabilities appear alongside built-in ones:

```bash
# Built-in format
kapi convert --format html -i page.html -o page.xliff

# Plugin format — same command, same flags, different format reader
kapi convert --format docx -i report.docx -o report.xliff

# Auto-detection works with plugins too — the docx plugin registers .docx
kapi convert -i report.docx -o report.xliff

# Plugin tools appear in flows
kapi flow run corporate-translate -i content.html --target-lang fr

# List everything, built-in and plugin
kapi formats list        # Shows: html, xml, json, ..., docx✱, indd✱
kapi tools list          # Shows: pseudo-translate, ..., terminology✱
kapi flow list           # Shows: ai-translate, ..., corporate-translate✱
# (✱ = provided by plugin)
```

Plugins also extend search and discovery:

```bash
# Find a plugin that handles a file type
kapi plugins search --ext .docx
kapi plugins search --mime application/vnd.openxmlformats

# Install and immediately use
kapi plugins install gokapi-format-docx
kapi convert -i report.docx -o report.xliff  # works now
```

### Bowrain

In Bowrain, plugins affect the set of file formats that can be opened and the
tools available in the processing pipeline. The desktop app loads plugins on
startup using the same `PluginLoader` as the CLI. Plugin-provided formats
appear in the "Open File" dialog's supported format list, and plugin tools
appear in the flow builder UI.

Because Bowrain may connect to a remote gokapi-server (via connection codes,
see [ADR-014](./014-distributed-processing-architecture.md)), the server's
plugin inventory may differ from the local one. Bowrain should display which
capabilities come from the server vs local plugins, and warn when a project
requires a plugin that is available on neither.

### CI/CD Integration

In CI pipelines, plugin management must be deterministic and reproducible.
The recommended approach is to declare plugin dependencies in the project
configuration and install them as a build step:

```yaml
# .gokapi.yaml in project root
plugins:
  required:
    - name: gokapi-format-docx
      version: ">=1.2.0"
    - name: gokapi-tool-terminology
      version: "1.0.x"
```

```bash
# CI build step
kapi plugins install --from-config   # Installs declared plugins
kapi flow run translate -i src/ --target-lang fr,de,ja
```

---

## Project Portability

The central risk of a plugin system is that **projects created by person A
become unusable for person B** if B lacks the required plugins. This is the
"works on my machine" problem applied to localization tooling.

### The Problem

Consider this scenario:

1. Alice installs `gokapi-format-docx@1.2.0` and extracts a Word document
   into a KAZ project
2. Alice shares the KAZ project with Bob
3. Bob runs `kapi merge` to generate the translated `.docx` output
4. Bob's system doesn't have the docx plugin → **merge fails**

Worse: if Bob has `gokapi-format-docx@1.0.0` (an older version), the merge
might silently produce incorrect output due to format differences between
plugin versions.

### Solution: Plugin Dependency Manifest

KAZ project manifests and flow definitions declare their plugin dependencies
explicitly:

```yaml
# manifest.yaml inside project.kaz
project:
  name: "Q4 Marketing Materials"
  source_locale: en
  target_locales: [fr, de, ja]

# Plugin dependencies recorded at extraction time
plugins:
  formats:
    - name: gokapi-format-docx
      version: "1.2.0"        # Exact version used during extraction
      min_version: "1.1.0"    # Minimum compatible version
  tools:
    - name: gokapi-tool-terminology
      version: "1.0.3"
      min_version: "1.0.0"
```

When a project is opened, gokapi validates plugin availability:

```
$ kapi merge -i project.kaz
Error: project requires plugins not available locally:
  ✗ gokapi-format-docx >= 1.1.0 (not installed)
  ✓ gokapi-tool-terminology >= 1.0.0 (installed: 1.0.3)

Install missing plugins with:
  kapi plugins install gokapi-format-docx
```

### Compatibility Rules

Plugin compatibility follows semver conventions:

- **Patch versions** (1.2.0 → 1.2.1): Always compatible. Bug fixes only.
- **Minor versions** (1.2.0 → 1.3.0): Backward compatible. New features may
  be added but existing behavior is preserved. A project created with 1.2.0
  can be processed by 1.3.0.
- **Major versions** (1.x → 2.x): Breaking changes. Projects may require
  migration. The manifest's `min_version` constraint prevents silent
  incompatibility.

### Graceful Degradation

Not all plugin dependencies are hard requirements. The manifest distinguishes
between required and optional plugins:

```yaml
plugins:
  formats:
    - name: gokapi-format-docx
      version: "1.2.0"
      required: true           # Cannot open project without this
  tools:
    - name: gokapi-tool-terminology
      version: "1.0.3"
      required: false          # Flow works without it, just skips term annotation
```

When an optional plugin is missing, the flow skips that tool and logs a warning
rather than failing. This allows projects to degrade gracefully when shared
across environments with different plugin sets.

---

## Plugin Governance and Maintenance

A plugin ecosystem can become a maintenance burden if not carefully managed.
The following practices keep the system healthy as it grows.

### Plugin API Stability

The plugin protocol (defined in `plugin/proto/v1/gokapi.proto`) is the contract
between host and plugins. Breaking this contract breaks all existing plugins.

**Rules:**

1. **The v1 protocol is frozen.** New fields can be added (protobuf is
   forward-compatible), but existing fields cannot be changed or removed.
2. **New capabilities require a new protocol version** (v2, v3, etc.). The
   handshake negotiates the highest mutually supported version.
3. **The host supports at least two protocol versions** simultaneously,
   giving plugin authors time to migrate.
4. **Deprecation cycle:** Announce deprecation in release N, warn at runtime
   in release N+1, remove in release N+3 (minimum).

### Plugin Scope: What Plugins Should and Should Not Do

Plugins should have a clear, bounded scope:

**Good plugin responsibilities:**
- Parse and write a specific file format
- Implement a specific processing tool (terminology lookup, QA check)
- Provide a connector to an external service (MT engine, TMS API)

**Bad plugin responsibilities:**
- Modify core pipeline behavior (reorder tools, skip stages)
- Alter the content model (add new Part types, change Block semantics)
- Intercept or modify other plugins' behavior
- Access the host's internal state outside the RPC contract

This boundary is enforced architecturally: plugins communicate only through the
defined RPC interface (`Info`, `Open`, `Read`, `Write`, `Process`). They
cannot access host memory, registries, or configuration directly.

### Plugin Quality Tiers

Not all plugins carry the same trust level:

| Tier | Source | Trust | Installation |
|------|--------|-------|-------------|
| **Built-in** | `formats/`, `lib/tools/` | Full | Ships with binary |
| **Official** | gokapi org registry | High | `kapi plugins install` |
| **Community** | Third-party registry | Medium | `kapi plugins install --source <url>` |
| **Local** | User-built | User's risk | Copy to plugin directory |

The plugin registry (`kapi plugins search`) shows the tier for each plugin.
CI environments can restrict allowed tiers via configuration:

```yaml
# .gokapi.yaml
plugins:
  allowed_tiers: [builtin, official]  # Reject community plugins in CI
```

### Avoiding Plugin Sprawl

The risk of "plugin everything" is that essential functionality fragments into
dozens of optional installs, degrading the out-of-box experience.

**Guidelines:**

- **Formats with broad usage** (HTML, XML, JSON, YAML, XLIFF, PO, Markdown,
  CSV, SRT, VTT) are **built-in**, not plugins. They ship with the binary.
- **Specialized or proprietary formats** (DOCX, IDML, InDesign, MIF, DITA)
  are plugins, because they have complex dependencies or licensing concerns.
- **Core tools** (pseudo-translate, QA check, word count, segmentation, TM
  leverage) are **built-in**.
- **Integration tools** (specific TMS connectors, custom MT engines,
  proprietary term bases) are plugins.
- **AI providers** (Anthropic, OpenAI, Ollama) are built-in because AI
  translation is a core value proposition. Niche providers are plugins.

The test: if removing it makes gokapi feel incomplete for the common case,
it should be built-in. If it serves a specific audience or has external
dependencies, it should be a plugin.

### Plugin Lifecycle Management

Plugin maintenance across an organization:

```bash
# Audit: what plugins are in use across projects?
kapi plugins audit ./projects/     # Scans KAZ manifests for plugin deps

# Update: upgrade all plugins to latest compatible versions
kapi plugins update

# Pin: lock all plugin versions for reproducible builds
kapi plugins freeze > plugins.lock
kapi plugins install --from-lock plugins.lock

# Prune: remove unused plugin versions
kapi plugins prune --keep-latest 2
```

---

## Consequences

- Plugins run out-of-process: crashes do not affect the host
- Any language that can implement a gRPC server works as a plugin
- Protocol versioning in the handshake prevents incompatible plugins from
  loading
- Multiple versions of the same plugin coexist without conflict
- Users can pin specific versions per project in `gokapi.yaml`
- Bare-name aliases provide a convenient default (latest version)
- Plugin install/upgrade/remove operations are directory-level: copy or
  delete a version directory
- Plugin startup adds latency (~100-500ms per plugin); acceptable for
  long-running pipelines
- Proto definitions in `plugin/proto/` define the contract between host
  and plugins
- KAZ manifests record plugin dependencies for project portability
- Plugin scope is enforced by the RPC boundary—plugins cannot modify core
  behavior
- The built-in vs plugin split keeps the out-of-box experience complete
  while allowing specialization through plugins
