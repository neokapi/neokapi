---
sidebar_position: 2
title: Markdown in the UI
description: Implementation note — which tool/format/flow/plugin metadata fields carry markdown, and the single shared Markdown typeset primitive that every neokapi UI uses to render them.
keywords: [markdown, typeset, prose, react-markdown, remark-gfm, tool description, format overview, ui copy, shadcn]
---

# Markdown in the UI

Several metadata fields that flow from the framework to the UIs (tool and
format descriptions, format overviews, parameter help, example descriptions,
long-form docs) are authored as **markdown** rather than plain text. This note is the
canonical list of which fields carry markdown and the single component every UI
must use to render them.

> **Two senses of "markdown".** This note is about markdown used as *UI copy*
> (a `description` that contains `**bold**`, `` `code` ``, or a link). It is
> unrelated to the **Markdown data format** (`core/formats/markdown/`,
> `core/formats/mdx/`), the content neokapi *reads and writes*. Keep the two
> separate.

## The rule

**Never drop a markdown-bearing field straight into JSX as a string.** Rendering
`{tool.description}` shows literal `**` and backticks and mangles links. Render
it through the shared typeset primitive instead:

```tsx
import { Markdown } from "@neokapi/ui-primitives"; // or "@neokapi/ui" in bowrain apps

// Dedicated detail / doc view — block prose:
<Markdown>{tool.description}</Markdown>

// Compact context (clamped list row, table cell, tooltip, chip,
// CardDescription) — inline flow, no block margins:
<Markdown inline>{tool.description}</Markdown>
```

The primitive lives at
[`packages/ui/src/components/ui/markdown.tsx`](https://github.com/neokapi/neokapi/blob/main/packages/ui/src/components/ui/markdown.tsx)
and is exported from `@neokapi/ui-primitives`. `@neokapi/ui` (bowrain's shared
package) re-exports it, so bowrain apps import it from `@neokapi/ui`.

- It wraps `react-markdown` + `remark-gfm` for rendering (GitHub-flavoured
  markdown: tables, strikethrough, autolinks, task lists) and renders
  react-markdown's default HTML.
- Block styling is the shared **`.typeset`** layer
  ([`packages/ui/src/styles/typeset.css`](https://github.com/neokapi/neokapi/blob/main/packages/ui/src/styles/typeset.css)),
  our implementation of the [shadcn/typeset](https://ui.shadcn.com/docs/changelog/2026-07-typeset)
  contract: one CSS file, three controls (`--typeset-size` / `--typeset-leading`
  / `--typeset-flow`) that everything else derives from, zero-specificity
  `:where()` rules in `@layer components` (so Tailwind utilities still override),
  `margin-block-start`-only flow (stream-stable), and colours mapped to our theme
  tokens (themes light/dark for free). It is **not** `@tailwindcss/typography`
  (which the repo does not ship). Variants: `.typeset-docs` (default) and
  `.typeset-chat` (the `variant` prop). Opt a subtree out with `not-typeset` /
  `data-not-typeset`.
- The CSS is `@import`ed from `styles/theme-tokens.css`, which every app already
  imports, so there is **no per-app CSS wiring**.
- `inline` mode is a small bespoke path (typeset is block-oriented): it collapses
  paragraphs to inline flow and degrades block constructs (headings, lists,
  rules) to lightweight inline equivalents, so a one-line clamped row never shows
  literal markup.
- Empty/whitespace input renders nothing.

Do **not** hand-roll another renderer (a local `react-markdown` wrapper, a regex
`markdownToHtml` + `dangerouslySetInnerHTML`, or a bespoke `components` map).
Those drift apart and are how the "markdown shown as a raw string" bugs crept in.
There is one renderer.

## Fields that carry markdown

These are the fields the UIs treat (or should treat) as markdown. The Go structs
and the mirroring TypeScript types are annotated with a `Markdown.` / `Markdown:`
comment so the contract is discoverable at the definition.

| Concern | Field | Go definition | TS mirror |
| --- | --- | --- | --- |
| Tool | `Description` | `core/registry/tool.go` `ToolInfo.Description`; `core/schema/schema.go` `ToolMeta.Description` | `packages/contract-types` `ComponentSchema`; desktop `api.ts` tool DTOs |
| Tool config | `Description` (per property) | `core/schema/schema.go` `PropertySchema.Description`, `ComponentSchema.Description` | `contract-types` `PropertySchema.description`, `ParameterGroup.description` |
| Format | `Description`, `Overview` | `core/format/spec/spec.go` `Spec.Description`, `Variant.Description`, `ConfigKey.Description`, `Feature.Description`, `Example.Description` | reference-data `ReferenceDoc.overview`; desktop `FilterDoc.overview` |
| Flow | `Description` | `core/flow/definition.go` `FlowDefinition.Description` | desktop/bowrain flow DTOs |
| Plugin | `Description` (one line of inline markdown) | `core/plugin/manifest/manifest.go` `Manifest.Description` | plugin DTOs |
| Reference docs | `Overview`, `Limitations`, `ProcessingNotes`, param `Help`/`Description`, example `Description` | `scripts/gen-refs/model.go` `Doc.*`, `DocParam.*`, `DocExample.Description` | `packages/reference-data/src/types.ts` `ReferenceDoc.*` |
| Command | `Long` (long help) | (CLI help catalogs) | `packages/reference-data` `CommandEntry.long` |
| Provider | `Description` (model/provider) | `providers/ai/provider.go` `Description` | provider DTOs |

Short **`display_name`**, single-word **`source`/`type`** badges, tab labels, and
`translate="no"` document-content previews carry **no** markdown, so leave them
plain.

## Where it renders today

The primitive is used across every React UI:

- **Kapi Desktop** (`apps/kapi-desktop/frontend`): tool detail + list
  (`ToolRunnerPage`), flow descriptions (`FlowsPage`), plugin descriptions
  (`PluginManager`), format overview + presets (`FormatsPage`), and the docs
  panel (`DocsPanel`).
- **Flow editor** (`packages/flow-editor`): tool palette entries, step example
  and parameter descriptions, template cards.
- **Bowrain UI** (`bowrain/packages/ui`): full-page tool/format docs
  (`ToolDocViewer`), tool/filter config field descriptions, and the bowrain
  desktop settings page.

The **docs site** (`web/`, Docusaurus) has its own `Markdown.tsx` wrapper for the
generated reference dataset. It is a separate build (its own CSS module, no
`@neokapi/ui-primitives` dependency) and renders the same set of fields listed
above through `react-markdown` + `remark-gfm`.

## When you add a field

If you add a metadata field that authors will write markdown into:

1. Annotate the Go struct field and its TypeScript mirror with a `Markdown.`
   comment.
2. Add it to the table above.
3. Render it with `<Markdown>` / `<Markdown inline>`, never as a raw string.

Keep the markdown light. These are descriptions and help text, not documents:
inline emphasis, code spans, links, short lists. Follow the register in
[brand-communication](https://github.com/neokapi/neokapi/blob/main/docs/internals/brand-communication.md).
