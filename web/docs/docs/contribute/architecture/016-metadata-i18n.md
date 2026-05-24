---
id: 016-metadata-i18n
sidebar_position: 16
title: "AD-016: Metadata i18n for Go Surfaces"
description: "Architecture decision: format and tool metadata (names, descriptions, parameter labels) is i18n-aware — stored in Go structs with locale fallback so CLI output, schema forms, and docs can be localized without runtime reflection."
keywords: [metadata i18n, Go surfaces, localization, format metadata, tool schema, architecture decision, neokapi]
---

# AD-016: Metadata i18n for Go Surfaces

## Context

The frontend packages ([AD-014](014-kapi-desktop.md)) are localized
through the KLF pipeline: extract translatable blocks from source, run
them through `kapi pseudo-translate` / `kapi ai-translate`, and compile
per-locale runtime catalogs. The Go backends serving those frontends emit
a metadata surface (tool / format / plugin `displayName`, `description`,
parameter `title` / `description` / enum labels / group labels) that also
needs matching localization, so the backend-sourced half of every screen
lines up with the frontend.

This AD describes a **metadata Translator** that localizes tool,
format, and plugin metadata at API egress, fed by the same extraction
pipeline as the frontend.

## Decision

Metadata i18n uses four sequenced ideas, each chosen for its fit with
the surrounding architecture:

### 1. English text is the lookup key (no artificial message IDs)

Registry structs keep their English literals (`tool.DisplayName = "AI
Translate"`). Translation is a read-side projection keyed by
`(scope, source)` where `scope` disambiguates collisions and `source`
is the English text itself. Same convention the frontend LinguiJS
setup uses — `<Trans>AI Translate</Trans>` — so translators see
exactly one source artifact across the stack.

### 2. JSON is the extraction source

`core/formats/json/` ([AD-005](005-format-system.md)) supports
regex-matched key extraction (`extractionRules`), full-key-path block
names (`useKeyAsName`), and round-trips through every SessionTool the
framework ships. Plugins place their metadata on disk as
`manifest.json` + `schemas/*.json` — the JSON filter reads them
directly.

For builtin tools and formats (whose metadata lives in Go structs, not
on disk), a `//go:generate` step emits an object-keyed
`core/i18n/builtins/metadata.json` from the in-process registries,
committed to the repo. CI gates freshness with `git diff --exit-code`.
The generated document is object-keyed (`tools.<id>`, `formats.<id>`),
not capability-array-keyed, so block names produced by the JSON filter
stay stable when tools are added or removed.

### 3. KLF is the authoring format; gettext MO is the runtime format

KLF is the platform's authoring / exchange format — rich, placeholder-
aware, and segment-oriented. It is the wrong shape for runtime lookup.
MO's binary hash-indexed catalog with `msgctxt` for disambiguation
maps directly onto the `(scope, source)` lookup, and
[`github.com/leonelquinteros/gotext`](https://github.com/leonelquinteros/gotext)
is a mature pure-Go loader. The `core/formats/mo/` format writer
consumes `klf.Block` streams and emits MO; `DetectByExtension(".mo")`
picks it up when the output path's extension says `.mo`.

### 4. Localize at the API boundary, not per-call-site

One pass at metadata egress — `i18n.LocalizeComponentSchema(s, t)` and
`i18n.LocalizeCapability(c, t)` — centralizes translation instead of
scattering `T(...)` calls through tool constructors. The surface is
finite and centralized (CLI `tools` / `formats` / `plugins` listings and
the Wails metadata readers), so one localization pass at egress covers it.

## End state

### Package layout

```
core/i18n/
├── doc.go                    # //go:generate directive
├── catalog.go                # Scope, Translator, gotext-backed lookup
├── resolve.go                # --lang / KAPI_LANG / config / LC_ALL / LANG chain
├── schema.go                 # LocalizeComponentSchema, LocalizeCapability
├── embed.go                  # //go:embed all:catalogs → builtin MO files
├── catalogs/                 # Compiled MO per locale (committed)
├── builtins/
│   └── metadata.json         # Generated, committed; extraction input
├── gen/
│   ├── gen.go                # Generator library
│   └── cmd/main.go           # //go:generate entry point
└── i18n.kapi                 # Project file documenting the pipeline

core/formats/mo/              # Writer + stub reader (for DetectByExtension)
```

### Runtime lookup

Every CLI / Wails / REST handler that emits tool, format, or plugin
metadata passes the result through `App.T()` (CLI) or `app.T()`
(kapi-desktop backend) before handing to the client. The `Translator`
is built at startup from the locale precedence chain:

```
--lang flag  >  KAPI_LANG  >  config.language  >  LC_ALL / LC_MESSAGES / LANG  >  "en"
```

Builtin MO catalogs are embedded via `//go:embed`; plugin catalogs are
loaded from `<pluginDir>/<name>/<version>/i18n/<locale>.mo` at
plugin-discovery time (`cli/pluginhost`).

### Pipeline

```bash
go generate ./core/i18n/...               # Go registries → builtins/metadata.json
kapi pseudo-translate builtins/metadata.json \
    --target-lang qps -f json \
    -o core/i18n/catalogs/qps.mo          # JSON reader → pseudo-translate → MO writer
```

One conversion, no KLF intermediate on disk — `core/klf/` blocks flow
through the in-process pipeline and the MO writer flattens them at the
sink.

### Plugin bundles

Plugin archives gain an optional `i18n/` directory sibling to
`schemas/`:

```
plugin-dir/
├── manifest.json
├── schemas/
└── i18n/
    ├── fr-FR.mo
    └── ja-JP.mo
```

`PluginManifest.I18nDir` overrides the default `"i18n"` path when
plugins want a different layout. Plugins without an `i18n/` directory
work unchanged — absence of a translation is silent, not an error.

### Scope format

`Scope` is the dot-separated full key path of the value in the
canonical metadata document:

- `tools.ai-translate.displayName`
- `formats.okf_html.description`
- `tools.ai-translate.properties.target-lang.title`
- `tools.ai-translate.groups.advanced.label`

The MO file stores this string as `msgctxt`; the English source is
`msgid`; the translation is `msgstr`. Homonyms ("Description" across
many tools) stay isolated.

## Consequences

- **Same authoring workflow for frontend and backend translators.**
  Both sides ship `.po`-editable catalogs (via `msgunfmt` / Poedit) or
  the same English-source convention, so translators see one source
  artifact across the stack.
- **Adding a locale is one `make kapi-i18n-translations` run + commit.**
  No tool registration changes, no schema edits.
- **Plugins contribute their own localizations.** The platform does
  not need a centralized plugin-translation database — each plugin
  release ships its own `i18n/` directory.
- **English source text in registry structs stays authoritative.**
  Translation is strictly additive — missing translations fall back to
  the English source, never to a placeholder or error.
- **CLI surface is minimally extended.** One new persistent flag
  (`--lang`); no new subcommands.

## Scope

Cobra command `Short` / `Long` / `Example`, `fmt.Fprintf` table
headers (`"FORMAT"`, `"SOURCE"`, …), and ad-hoc `errors.New` strings
stay English — the metadata Translator targets the registry surface,
not general CLI chrome.

The MO writer does not flatten placeholder runs — metadata strings
are plain text, so placeholder handling would be dead code. Revisit if
a metadata surface grows interpolation.

Schema deep-walk in `kapi-desktop`'s `GetToolSchema` (which returns
raw JSON to preserve `x-*` extensions) is not localized at that path;
the tool palette uses `ListTools`, which is.

## Related

- [AD-005: Format System](005-format-system.md) — JSON reader and MO writer.
- [AD-006: Tool System](006-tool-system.md) — the ComponentSchema surface being localized.
- [AD-007: Plugin System](007-plugin-system.md) — plugin manifest and `i18n/` bundle layout.
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — frontend i18n this AD aligns with.
