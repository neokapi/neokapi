---
id: 016-metadata-i18n
sidebar_position: 16
title: "AD-016: Metadata i18n for Go Surfaces"
description: "Architecture decision: format and tool metadata (names, descriptions, parameter labels) is i18n-aware — stored in Go structs with locale fallback so CLI output, schema forms, and docs can be translated without runtime reflection."
keywords: [metadata i18n, Go surfaces, translation, format metadata, tool schema, architecture decision, neokapi]
---

# AD-016: Metadata i18n for Go Surfaces

## Context

The frontend packages ([AD-014](014-kapi-desktop.md)) are translated
through the KBF pipeline: extract translatable blocks from source, run
them through `kapi pseudo-translate` / `kapi translate`, and compile
per-locale runtime catalogs. The Go backends serving those frontends emit
a metadata surface (tool / format / plugin `displayName`, `description`,
parameter `title` / `description` / enum labels / group labels) that needs
the same treatment, so the backend-sourced half of every screen
lines up with the frontend.

This AD describes a **metadata Translator** that translates tool,
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

### 3. KBF is the authoring format; gettext MO is the runtime format

KBF is the platform's authoring / exchange format — rich, placeholder-
aware, and segment-oriented. It is the wrong shape for runtime lookup.
MO's binary hash-indexed catalog with `msgctxt` for disambiguation
maps directly onto the `(scope, source)` lookup, and
[`github.com/leonelquinteros/gotext`](https://github.com/leonelquinteros/gotext)
is a mature pure-Go loader. The `core/formats/mo/` format writer
consumes `kbf.Block` streams and emits MO; `DetectByExtension(".mo")`
picks it up when the output path's extension says `.mo`.

### 4. Translate at the API boundary, not per-call-site

One pass at metadata egress — `i18n.LocalizeComponentSchema(s, t)` —
centralizes translation instead of scattering `T(...)` calls through tool
constructors. Because tool and format metadata both serialize as a
`ComponentSchema`, a single pass over that type covers both. The surface is
finite and centralized (CLI `tools` / `formats` / `plugins` listings and
the Wails metadata readers), so one pass at egress covers it.

## End state

### Package layout

```
core/i18n/
├── doc.go                    # //go:generate directive
├── catalog.go                # Scope, Translator, gotext-backed lookup
├── resolve.go                # --lang / KAPI_LANG / config / LC_ALL / LANG chain
├── schema.go                 # LocalizeComponentSchema
├── embed.go                  # //go:embed all:catalogs → builtin MO files
├── catalogs/                 # Compiled MO per locale (committed)
├── builtins/
│   └── metadata.json         # Generated, committed; extraction input
├── gen/
│   ├── gen.go                # Generator library
│   └── cmd/main.go           # //go:generate entry point
└── kapi.yaml                 # Project file documenting the pipeline

core/formats/mo/              # Writer + stub reader (for DetectByExtension)
```

### Runtime lookup

Every CLI / Wails / REST handler that emits tool, format, or plugin
metadata passes the result through `App.T()` (CLI) or `app.T()`
(kapi-desktop backend) before handing to the client. The `Translator`
is built at startup from the first locale that resolves: the `--lang` flag,
then `KAPI_LANG`, then `config.language`, then `LC_ALL` / `LC_MESSAGES` /
`LANG`, falling back to `en`. The environment values are normalized from POSIX
form on the way through, so `en_US.UTF-8` arrives as `en-US`.

Builtin MO catalogs are embedded via `//go:embed` and merged in
`i18n.Resolve`. The convention for plugin-provided catalogs
(`<pluginDir>/<name>/<version>/<i18nDir>/<locale>.mo`) and the loader
helpers `i18n.PluginCatalogPath` / `i18n.LoadPluginCatalog` exist on
`core/i18n`, but nothing populates `ResolveOptions.PluginCatalogs`:
manifest-driven plugin discovery in `host/pluginhost` does not yet feed
catalogs into the Translator (see the note where the Translator is built,
in `host/app.go`). The "Plugin bundles" and "Plugins contribute their own
catalogs" material below describes the intended design rather than current
behavior.

### Pipeline

```bash
go generate ./core/i18n/...               # Go registries → builtins/metadata.json
kapi pseudo-translate builtins/metadata.json \
    --target-lang qps -f json \
    -o core/i18n/catalogs/qps.mo          # JSON reader → pseudo-translate → MO writer
```

One conversion, no KBF intermediate on disk — `core/kbf/` blocks flow
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
    ├── fr.mo
    └── ja.mo
```

The conventional path is computed by
`i18n.PluginCatalogPath(pluginVersionDir, locale, i18nDir)`
(`core/i18n/resolve.go`), where the `i18nDir` argument defaults to
`"i18n"` when empty; no manifest field currently overrides it. Plugins
without an `i18n/` directory work unchanged — absence of a translation
is silent, not an error.

### Scope format

`Scope` is the dot-separated full key path of the value in the
canonical metadata document:

- `tools.translate.displayName`
- `formats.html.displayName`
- `tools.translate.properties.provider.title`
- `tools.translate.groups.provider.label`

The MO file stores this string as `msgctxt`; the English source is
`msgid`; the translation is `msgstr`. Homonyms ("Description" across
many tools) stay isolated.

## Consequences

- **Same authoring workflow for frontend and backend translators.**
  Both sides ship `.po`-editable catalogs (via `msgunfmt` / Poedit) or
  the same English-source convention, so translators see one source
  artifact across the stack.
- **Adding a locale is one `make l10n` run + commit.**
  No tool registration changes, no schema edits.
- **Plugins contribute their own catalogs.** The platform does
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
raw JSON to preserve `x-*` extensions) is not translated at that path;
the tool palette uses `ListTools`, which is.

## Related

- [AD-005: Format System](005-format-system.md) — JSON reader and MO writer.
- [AD-006: Tool System](006-tool-system.md) — the ComponentSchema surface being translated.
- [AD-007: Plugin System](007-plugin-system.md) — plugin manifest and `i18n/` bundle layout.
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — frontend i18n this AD aligns with.
