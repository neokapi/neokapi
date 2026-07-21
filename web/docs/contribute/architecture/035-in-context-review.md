---
id: 035-in-context-review
sidebar_position: 35
title: "AD-035: In-Context Review"
description: "Architecture decision: review translations on the running app — build-time DOM stamping maps pixels to block hashes, a dev-server middleware serves and writes back the local KLF tree, and a framework-free overlay paints terminology and QA findings with the CSS Custom Highlight API."
keywords: [in-context review, live editing, DOM stamping, CSS Custom Highlight API, KLF write-back, terminology, QA, SSE, neokapi-i18n, architecture decision, neokapi]
---

# AD-035: In-Context Review

## Summary

Translations are reviewed **on the running app**, not in a file. A
build-time transform stamps each extracted element with its block hash;
a dev-server middleware serves and writes the local KLF tree; a
framework-free browser overlay maps a click back to a block, edits its
target, and writes it into the `.klf` on disk. Terminology and QA
findings from stand-off `*.klfl` files are painted onto the live text
with the CSS Custom Highlight API — no DOM mutation.

## Context

The file format throws context away. A reviewer looking at a
spreadsheet row cannot see that "Save" landed on a 40-pixel button,
that the German wraps to two lines, or that the register is wrong for a
destructive-action dialog. Everything that makes a translation right or
wrong is a property of *where it renders*, and that is exactly what the
extraction step discards.

In-context editing is an established idea. The open question is how a
rendered pixel maps back to a source string.

Two mechanisms were considered and not taken:

- **Fiber inspection.** Reading React's internal tree for debug source
  information is no longer viable — React 19 removed `_debugSource`.
- **In-band markers.** Encoding the identifier into the rendered *text*
  (invisible characters, private-use code points) needs no build step,
  but the rendered string is then not the string the app computed:
  equality comparisons, measurement, and copy/paste all see the marker,
  and an attribute value has no text node to carry one at all. Review
  must be observation-free — the app under review has to behave exactly
  as it does in production.

What we want is an out-of-band channel the build already has an
opportunity to write, and that the rendered text never sees.

## Decision

### Stamp the DOM at build time

Under `review: true` (or `KAPI_REVIEW=1`), the transform adds to each
extracted element:

| Attribute        | Carries                                             |
| ---------------- | --------------------------------------------------- |
| `data-kapi-id`   | the block hash                                      |
| `data-kapi-loc`  | `file:line` — jump-to-source                        |
| `data-kapi-attr` | hashes of the element's translatable **attributes** |

The identifier goes in an *attribute*, never in the text. The rendered
string stays byte-identical to production, `===` still works, copy/paste
is clean, and attribute blocks (which have no text node to hide a
marker in) are addressable on the same footing as children. The plugin
already visits every extracted element to rewrite it, so the stamp is
free — no separate traversal, and no runtime identification machinery.

This is dev/staging only, and gated behind an explicit flag: it puts
source paths in the DOM and mounts a middleware that writes to disk.

### Head strings have no DOM node — a runtime registry

The stamp needs an element to attach to. The document head has none: a
`<title>`, a `<meta name="description">`, an Open Graph card title are
page metadata, not rendered content, so there is nothing to hover and
nothing to outline — and these are frequently the highest-SEO-value
strings on the page.

They are reached through the opt-in `@neokapi/i18n-react/head` hooks
(`useTranslatedTitle`, `useTranslatedDescription`, `useTranslatedMeta`).
A head string never gets a DOM text node, so the build transform cannot
rewrite it the way it rewrites a `t()` call. Instead the hook
**self-hashes** its source at render time, using a browser port of the
build hasher over a dedicated `head` descriptor channel, and looks the
translation up in the runtime dictionary. The extractor recognises the
same hook calls and emits a matching block, so the hash the hook
computes and the `Block.hash` the extractor writes are identical by
construction — the [hash-parity](019-i18n-react.md) invariant, extended
to the head. (A `hash-parity` test asserts the two hashers agree.)

As it applies the string, the hook registers `{ kind, hash, source }`
in a small framework-free registry the overlay reads. The overlay lists
the page's head translatables in a panel and edits them through the
identical `PUT /{hash}` write-back a body string uses — the block exists
in the KLF tree, so the store keys on it with no new protocol. Nothing
scrapes arbitrary head markup: only strings the pipeline translated
register, and a page with none shows no panel.

### The dev server serves the KLF tree

`/__kapi/review` (mounted by the Vite plugin, plain Node `http` so it
ports to any dev server) over the local KLF directory:

| Route              | Purpose                                       |
| ------------------ | --------------------------------------------- |
| `GET /{hash}`      | block payload — source runs, targets, notes   |
| `PUT /{hash}`      | write a target back into the `.klf`           |
| `GET /annotations` | stand-off findings, keyed by block hash       |
| `GET /events`      | SSE — updates broadcast to every open tab     |

The index is rebuilt whenever any `.klf` / `.klfl` mtime drifts, so an
out-of-band `kapi translate` or `kapi exec qa` shows up without a
restart.

### Write back to the `.klf`, not to a database

A reviewed target is written into the block's `.klf` file as a target
run. The obvious alternative — a review database with comments,
states, and an approve button — was rejected.

The `.klf` tree is *already* the contract between developers and
translators: `extract` produces it, `kapi translate` fills it, QA reads
it, `compile` ships it. Writing a reviewed target into it makes the
review a **git diff** — reviewable in a PR, revertable, attributable,
and consumed by the very next `compile` with no synchronisation step. A
reviewer's fix travels the identical path as a translator's, because it
is the identical artifact. It also means review needs no server, no
account, and no network: `git clone && vp dev`.

The cost is that local review has no workflow state (assignment,
approval, threaded comments). That is deliberate — those belong to the
platform tier (see [Related](#related)), and the same stamping and the
same hashes carry the overlay there unchanged.

### The identifier is content-addressed — review runs on any render mode

`data-kapi-id` is the block's **content hash** —
`hashKey(source, descriptor)` — computed identically whether the build
**inlines** the translation (zero-runtime, SSG/SSR) or emits a
**runtime** `t()` call, by the same [hash-parity](019-i18n-react.md)
invariant. Nothing about the stamp depends on a runtime dictionary
being present, so review is **render-mode-independent**: an inline/SSG
build stamps the same id an OTA build does, and one manifest resolves
both.

To make a statically-rendered page self-sufficient, the plugin emits a
read-only `translations/review.json` (source, per-locale target,
`file:line`, element) whenever `review: true` — gated on `review`, not
on mode, so an inline build emits it as much as a runtime one. A
**hosted overlay** (`@neokapi/i18n-react/review/hosted`) fetches that
file, finds elements by `data-kapi-id`, and needs neither a dev server
nor the app's i18n runtime: deep-link `?kapi-focus=<hash>`, a
whole-page index, and read-only inspection all work on a plain static
deploy. With `edit: true` it edits through the identical `PUT /{hash}`
protocol and repaints by patching the element's text **directly** —
text-only, so blocks with inline codes or ICU plurals save to the store
but are not repainted live — giving live in-context editing even on a
page with no runtime dictionary to repaint through. Richer manifests
(every locale, term/QA annotations) still come from
`neokapi-i18n compile --review` over the whole KLF tree.

### Paint findings with the CSS Custom Highlight API

Terminology and QA results are stand-off annotations (`*.klfl`)
anchored to run-index ranges — the framework's existing overlay model
([AD-002](002-content-model.md)). The browser renders them via
[`CSS.highlights`](https://developer.mozilla.org/en-US/docs/Web/API/CSS_Custom_Highlight_API):
`Range` objects are registered in a named `Highlight` and styled with
`::highlight(kapi-term)`.

Nothing is inserted into the DOM. The classic approach — wrapping
matched text in `<span class="highlight">` — mutates the tree React
owns, which means layout shifts, hydration mismatches, and a React
re-render silently discarding the highlights. Custom Highlight paints
in the text renderer, *beside* the DOM: React's tree is untouched, so
what the reviewer sees is the app, not the app plus the instrument.

The mapping from a stand-off range to a `Range` is direct, because both
are offsets into the same flat text the block already carries.

## Consequences

- **The reviewer sees the app.** Every property that only exists at
  render time — truncation, wrapping, tone-in-place, a term that reads
  wrong next to the button beside it — is visible at review time.
- **Review is a diff.** No import/export, no sync step, no platform
  dependency for the local tier.
- **QA and terminology become visual.** The checks kapi already runs
  stop being a list of line numbers and become marks on the words they
  are about — which is the only form in which a reviewer will act on
  them.
- **The stamp is the seam.** The same `data-kapi-id` that lets the
  local overlay find a `.klf` block lets a staging overlay find a
  platform block. The Tier-2 (Bowrain) surface is a different backend
  behind the same client contract, not a different feature.
- **Render-mode-independent.** Because the id is a content hash, the
  same stamp and the same `review.json` drive review on an inline/SSG/SSR
  page, a runtime/OTA page, and the platform tier — one mechanism,
  every render mode.
- **Enable deliberately.** Review is opt-in per build. Stamping puts
  `file:line` in the DOM, the dev server mounts a writer, and inline
  review bakes a `review.json` — all appropriate for dev and
  preview/staging deploys, but turn review off for the production build
  end users get.

## Related

- [AD-019: neokapi-i18n extraction model](019-i18n-react.md) — the block
  hashes, the transform, and the KLF the review tier reads and writes
- [AD-002: Content Model](002-content-model.md) — stand-off overlays
  and run-anchored ranges, the shape the annotations arrive in
- [AD-008: Project Model](008-project-model.md) — the `kapi.yaml` recipe and
  the KLF interchange the review writes into
