---
id: m-07-metadata-i18n
sidebar_position: 7
title: "M-07: Metadata in other languages"
description: "The English literal is the lookup key, JSON generated from the Go registries is the extraction source, a committed JSON catalog is the review surface compiled to an embedded MO at build time, and translation happens in one pass at the boundary where metadata leaves the process."
keywords: [neokapi, architecture decision, metadata, translated interface, gettext, MO catalog, locale resolution, tool schema]
---

import { CycleDiagram } from "@neokapi/docs-shared";

# M-07: Metadata in other languages

## The surface

kapi's frontends are translated through the ordinary content pipeline: extract
the translatable blocks from source, run them through the loop, compile
per-locale runtime catalogs. The Go backends serving those frontends emit their
own text (a tool's, format's or plugin's display name and description, a
parameter's title and description, enum labels and descriptions, group labels)
and so does the CLI itself, in command help and in the fixed chrome of its
table output.

Left in English, the backend-sourced half of every screen would sit beside a
translated frontend. So the same pipeline covers three surfaces:

| Surface | Source of truth | Scope prefix |
| --- | --- | --- |
| tool / format / plugin metadata | Go registry structs | `tools.` / `formats.` |
| command help (short, long, examples) | the cobra command tree | `cli.commands.` |
| output chrome (table headers, fixed messages) | a string table in the output package | `cli.output.` |

Everything else stays English by design: ad-hoc error strings are not part of the
translated surface.

The same catalogs carry the reference pages of the documentation site. The
generated dataset behind `/tools`, `/formats`, `/commands` and `/models` is
written in English from the registries, and beside it one variant per locale
that has a catalog: the tool and format names and descriptions, the command
help and the model notes are looked up under the same scopes, and the authored
dossiers are overlaid from their translations. A string the catalog lacks keeps
its English. The site swaps the variant in at build time for the locale it is
building, so the reference reads in a locale as soon as the loop has written
one, with no second extraction of the same prose.

## Four sequenced decisions

### The English text is the lookup key

Registry structs keep their English literals. Translation is a read-side
projection keyed by `(scope, source)`, where the scope disambiguates collisions
and the source *is* the English text. That is the same convention the frontend
uses, so a translator sees exactly one source artefact across the whole stack
rather than a message id on one side and a sentence on the other.

`Scope` is the dot-separated full key path of the value in the canonical
document: `tools.translate.displayName`,
`tools.translate.properties.provider.title`,
`cli.commands.kapi.extract.short`. Homonyms stay isolated: "Description" across
many tools is many entries, not one. An enum value's description resolves under
its own container, `<base>.enumDescriptions.<value>`, so the sentence beside an
enum label is translated as well as the label.

The first path segment of a command scope is always `kapi`, regardless of the
root command's name, so a catalog stays valid across every binary built on the
shared CLI base.

### JSON is the extraction source

The JSON format supports regex-matched key extraction, full-key-path block
names, and round-trips through the tools the framework ships
([E-02](../engine/e-02-format-system.md)). Plugins already place their metadata
on disk as a manifest plus schema files, so the JSON reader reads them directly.

For builtins, whose metadata lives in Go structs rather than on disk, generators
close the gap. One emits an object-keyed metadata document from the in-process
registries; another reconstructs the full command set from the exported command
factories and emits one entry per translatable help string. Both are committed,
and CI gates their freshness with a clean-diff check.

The generated documents are **object-keyed** (`tools.<id>`, `formats.<id>`,
`cli.commands.<path>`), not array-keyed, so the block names the JSON reader
derives stay stable when a tool or a command is added or removed.

### JSON is the committed catalog; MO is the runtime format

The gettext MO format is the right runtime shape: a binary indexed catalog whose
message-context field maps directly onto the `(scope, source)` lookup, with a
mature pure-Go loader. But a compiled catalog is build output, and a binary in
version control is a diff nobody can read.

So the repository carries the **translated catalog as JSON in the shape of its
source document**, the review surface written by the loop, and a build-time
compiler turns each into a sibling MO before anything that embeds it compiles.

<CycleDiagram
  steps={[
    { label: "Go registries", sub: "English literals" },
    { label: "generated JSON", sub: "committed, CI-gated" },
    { label: "the loop", sub: "per-locale catalog JSON" },
    { label: "compiled MO", sub: "gitignored build output" },
    { label: "embedded catalog", sub: "//go:embed" },
  ]}
  caption="The committed artefacts are the two readable ones; the binary catalog is derived on every build."
/>

The compiler reads its extraction configuration from the recipe rather than
restating it, so the message context it writes is *by construction* the key path
the catalog JSON was written under and the scope the runtime looks a string up
by. It pairs source and translation by that key path: the message id is the
English source, the message string is the translation. A string the locale has
not translated, or has copied verbatim, is left out; the runtime then returns
the source, which is exactly what target-language drift should look like.

The compiler imports neither package it writes for, so it builds and runs
against an empty catalog directory: no bootstrap cycle, and no kapi binary in
the loop. It rewrites a catalog only when the bytes change, so a repeat run
invalidates nothing.

Compilation is a **build prerequisite**, not a step someone remembers: the embed
directive resolves at compile time, so the catalog target runs ahead of every
target that builds, tests, vets or lints the affected packages, and a build that
skipped it fails on the embed rather than silently shipping English.

### Translate at the boundary

One pass where metadata leaves the process centralizes translation instead of
scattering lookups through tool constructors. Tool and format metadata both
serialize as the same component-schema type, so a single pass over that type
covers both. Command help is translated by one walk of the command tree at
startup; output chrome goes through the output package's own table lookup.

Two details of that walk matter. A command whose name contains a path
separator is left untouched, because it would corrupt the scope. And a
multi-line help string whose translation came back without line breaks is
rejected in favour of the English source: the memory's plain-text path normalizes
whitespace, so a leveraged catalog can carry a long help text whose structure was
collapsed into one unreadable paragraph. Preferring the source there is the same
principle as leaving an untranslated string out of the catalog.

## Runtime lookup

The translator is built at startup from the first locale that resolves: the
`--lang` flag, then `KAPI_LANG`, then the language in the user's config, then
`LC_ALL`, `LC_MESSAGES`, `LANG`, falling back to English. Every value, whichever
source supplied it, passes through the one canonicalization the framework
applies at every ingress (`core/locale.Canonical`), so `en_US.UTF-8` arrives as
`en-US`, `fr_CA@euro` as `fr-CA`, and `--lang nb_NO` as `nb-NO`. `C` and
`POSIX` are not locales; they fall back to the English source rather than
being refused, because asking for a catalog is not the place to reject a bad
locale.

English needs no catalog, because the message ids *are* the English source. The
comparison is made in minimal form, so `en-US` short-circuits too while a
genuinely distinct `en-GB` still gets a lookup.

Catalog lookup follows the locale's fallback chain (the exact tag, then the
CLDR-minimal form, then the bare language), so a config that still says `nb-NO`,
or a `LANG` of `nb_NO.UTF-8`, finds the `nb` catalog instead of silently
degrading to English.

Catalogs merge rather than compete. The framework's builtin catalog is embedded
in the framework module and the CLI's is embedded in the CLI module; their
scopes are disjoint, so merge order is immaterial and a miss in either still
falls back to the English source.

## Plugin catalogs

Plugin archives may carry an optional `i18n/` directory beside their schemas,
holding one MO per locale:

```
plugin-dir/
├── manifest.json
├── schemas/
└── i18n/
    ├── fr.mo
    └── ja.mo
```

The conventional path and the loader helpers exist in the framework, and the
resolver takes a list of plugin catalogs to merge. **Not yet built:** nothing
populates that list. Plugin discovery does not feed catalogs into the
translator, so a plugin's `i18n/` directory is currently inert. A plugin without
one works unchanged: the absence of a translation is silent, never an error.

## Consequences

- **One authoring workflow across the stack.** Both halves ship catalogs in the
  same English-source convention, so a translator sees one source artefact
  whether the string came from a React component or a Go struct.
- **Adding a locale is a loop run and a commit.** No tool registration changes,
  no schema edits.
- **English source in the registry structs stays authoritative.** Translation is
  strictly additive: a missing translation falls back to the English source,
  never to a placeholder or an error.
- **The CLI surface is minimally extended.** One persistent flag, no new
  subcommands.

## Scope boundaries

The MO writer does not flatten placeholder runs. Metadata strings are plain
text, so placeholder handling would be dead code; revisit if a metadata surface
grows interpolation.

The desktop backend's raw schema accessor, which returns unprocessed JSON to
preserve schema extensions, is not translated at that path; the tool palette
reads the listing endpoint, which is.

## Related

- [E-02: The format system](../engine/e-02-format-system.md): the JSON reader that extracts and the MO writer that compiles
- [E-03: The tool system](../engine/e-03-tool-system.md): the component-schema surface being translated
- [E-05: The plugin system](../engine/e-05-plugin-system.md): the plugin manifest and the `i18n/` bundle layout
- [S-01: The kapi CLI](../surfaces/s-01-kapi-cli.md): the `--lang` flag, where the translator is installed, and the locale canonicalization every ingress shares
- [S-02: Kapi Desktop](../surfaces/s-02-kapi-desktop.md): the frontend this aligns with
- [S-05: The i18n runtime](../surfaces/s-05-i18n-runtime.md): the runtime catalogs the frontend half compiles to
