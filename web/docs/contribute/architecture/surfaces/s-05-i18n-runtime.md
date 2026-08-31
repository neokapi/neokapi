---
id: s-05-i18n-runtime
sidebar_position: 5
title: "S-05: The i18n runtime for React"
description: "@neokapi/i18n-react extracts translatable content from JSX and TypeScript at build time into the framework's own Run sequences, keys it on the text rather than the tree, renders translations back through React elements, and reviews them on the running app via DOM stamps and a dev-server write-back."
keywords: [neokapi, architecture decision, i18n-react, JSX extraction, Run, content hash, ICU, in-context review, DOM stamping, CSS Custom Highlight]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# S-05: The i18n runtime for React

## Summary

`@neokapi/i18n-react` extracts translatable content directly from React and JSX
source at build time, producing `Block` records whose `Source` is a typed
`Run[]`, the framework's canonical inline-content model
([F-02](../foundations/f-02-content-model.md)). An inline JSX element with
children becomes a paired open/close run triple in its parent's sequence, so
`<p>Click <a href="/docs">here</a> to read.</p>` extracts as **one** block whose
translator keeps the link wrapped around the right word in every language. A
small runtime interleaves the original React elements back at marker positions.
A lint package (`@neokapi/i18n-react-lint`) keeps the source extractable, and a
review layer lets a person fix the translation **on the running app**, writing
back into the same interchange files the pipeline already uses.

## Context

A React-native multilingual story has two hard parts and one that only appears
later.

**Authoring.** Developers write JSX. Translators want sentences with their
inline structure intact (links, emphasis, variables, icons), not opaque
placeholders or fragmented sub-strings. Forcing every string into a
`t("hello-key")` call breaks a component's reading order and pushes inline
structure into separate sub-keys.

**Runtime.** The translated string has to compose back with the original React
elements, preserving event handlers, refs, and props. An HTML-string round-trip
loses handlers and bypasses reconciliation.

**Review.** The file format throws context away. A reviewer looking at a
spreadsheet row cannot see that a word landed on a forty-pixel button, that the
German wraps to two lines, or that the register is wrong for a destructive-action
dialog. Everything that makes a translation right or wrong is a property of
*where it renders*, exactly what extraction discards.

## Decision

### Build-time extraction into `Run[]`

An SWC-AST walker (`src/extract/walker.ts`) descends each module looking for
translatable JSX. Translatability comes from element vocabulary plus
user-supplied `componentMap` and `rules`. For each translatable element the
walker emits a block whose source run sequence is built by `extract/runs.ts`.

The element vocabulary has one definition. `plugin/defaults.ts` holds the W3C
HTML5 translatability table: which elements' text belongs to the translator,
which must survive byte for byte, and which attributes carry visible copy.
`scripts/gen-translatability.ts` emits it as `core/translatability/data/w3c.json`
for the markdown and MDX readers, so the Go side classifies the same elements
the same way, and `make check-translatability` fails on drift between the two.

TypeScript modules extract too. A `.ts` file is parsed for `t()` calls with the
syntax decided by its extension, in one function the extractor and the
transform share, so the two cannot disagree; `tsx` is enabled only for `.tsx`,
because in a `.ts` file `<Foo>x` is a type assertion. A file the parser cannot
read is reported as a failure, never as a file with nothing to translate.

The defining rule: **an inline JSX element with at least one child becomes a
paired inline code in its parent's run sequence.**

```tsx
<p>
  Click <a href="/docs">here</a> to read the docs.
</p>
```

extracts as one block whose source is a `TextRun`, a `PcOpenRun`, a `TextRun`, a
`PcCloseRun`, and a `TextRun`. Type and subtype follow the JSX vocabulary in
`@neokapi/kapi-format`: every JSX element uses `type: "jsx:element"` with the
resolved HTML tag, or the unmapped React component name, in `subType`, and the
vocabulary entry drives editor rendering, chip labels, and editing constraints
([S-06](s-06-visual-editor.md)).

Pairs nest last-in-first-out, and inner content may hold text, expressions,
placeholders, or further pairs:

| Source | Runs |
| --- | --- |
| `<a>here</a>` | open + text + close |
| `<a><Icon/></a>` | open + placeholder + close |
| `<strong>{count}</strong>` | open + placeholder + close |
| `<a>read <em>the</em> docs</a>` | open + text + open + text + close + text + close |

This makes the translator the unit of decision: a German translation can wrap
the link around a different word, and a French one can move it elsewhere in the
sentence.

Constructs without children (self-closing icons, line breaks, zero-child
unmapped components, expression containers, conditional nodes) become a single
placeholder run instead, typed `jsx:element`, `jsx:var`, or `jsx:node`.
Variable containers keep the JavaScript identifier as their equivalence text, so
the flat form reads naturally to a translator and substitutes through the
standard named-parameter path at runtime.

Container elements with at least one direct non-whitespace text child and only
inline children are auto-promoted to translatable. Promotion is silent (the
pattern is too common in real interfaces to warn on every occurrence), and the
opt-outs are `translate="no"` or a `rules` entry. Fragment roots extract on the
same terms, under a reserved descriptor standing in for the tag they do not
have: a fragment is a real authoring idiom for "inline content with no wrapper I
am allowed to add", and skipping it would push authors toward a `<span>` they do
not want in the DOM.

Translatable attributes come from two vocabularies with different scopes. The
HTML and ARIA names (`alt`, `title`, `placeholder`, `aria-label`, …) are
standardised as user-visible text, so they extract on **any** element. React's
prop-name conventions (`label`, `description`, `heading`, `helpText`, …) extract
on **PascalCase components only**. The scoping exists because the convention
names are not reserved: on a plain HTML element, `label` or `heading` is far more
often a DOM prop, an enum key, or a data-binding field than copy, and extracting
those swept strings nobody wanted translated into the catalog. One predicate
decides, and both the walker and the transform call it.

### The key is the text

**The key is FNV-1a 64 over `JSON.stringify(flatText) + "|" + desc`,
base62-encoded**, where `flatText` is the block's flat template (text verbatim,
expressions as `{name}`, inline elements as `{=mN}`) and `desc` is a structural
descriptor.

The descriptor is the element's **own resolved tag**: `"p"`, `"button"`,
`"fragment"`, or a context-qualified form for an explicit `t()` call. Ancestors
are deliberately excluded.

The key *is* the translator's contract. The property we need is that **a key
changes when, and only when, a translator should look at the string again.**
Rewording changes the key. Moving a paragraph into a new wrapper does not: that
is a layout refactor, and the German is still the German.

Two alternatives were rejected for concrete reasons. A **full ancestor path**
makes every layout refactor a silent mass-orphaning event: wrap a section in one
new `<div>` and every string beneath it gets a new key, loses its translation,
and quietly falls back to source. Structure is the least stable thing about a
React codebase, so keying on it inverts the stability we want. **Flat text
alone** collapses `<p>Save</p>` and `<button>Save</button>` into one key, and a
translator can then never give the noun and the verb different words; the
immediate tag is the cheapest disambiguator that survives refactoring and maps to
a real distinction.

Where the tag is not enough (two buttons both reading "Open", one a verb and one
a state) the disambiguator is *explicit*: a `data-i18n-note` attribute or a
context argument, folded into the descriptor. That puts the decision with the
author who knows the two strings differ, rather than with an incidental fact
about the DOM tree.

The hash is 64 bits. A 32-bit hash reaches roughly even odds of a birthday
collision around eighty thousand strings, well inside what a large application
reaches, so 64 gives headroom for million-string corpora.

Because `componentMap` and `rules` feed the hash, the extract CLI and the build
plugin **must** be configured identically. A desync is silent: both sides run
without error and compute different keys, so every affected string falls
back to source. Projects should keep one configuration file that both sides read.

### One template builder, two consumers

Extract and transform must agree on the flat template **byte for byte**: extract
hashes it into the interchange file, transform hashes it to look the translation
back up. As two independent walks over the same AST they would drift silently
(a paired inline element leaking a literal close token into the DOM, a plural
emitting unparseable JSX), because nothing compares them.

So the decision is structural rather than procedural. `extract/runs.ts` owns the
single `buildRuns()` builder and the transform **consumes** it, taking both the
flat text and a list of per-appearance occurrence spans that it maps back onto
the source to rewrite the JSX. Parity is a property of the code shape, not of two
authors remembering to keep two walks in step. Any future front end plugs in at
the same seam.

### Runtime rendering

<PipelineDiagram
  stages={[
    { label: "JSX source", sub: "SWC AST", role: "io" },
    { label: "buildRuns", sub: "extract/runs.ts", note: "one builder" },
    { label: "Block + hash", sub: "Run[] → interchange", role: "annotate" },
    { label: "Translate", sub: "the kapi pipeline", role: "translate" },
    { label: "Dictionary", sub: "hash → flat template" },
    { label: "Render", sub: "elements re-interleaved", role: "io" },
  ]}
  channelLabel=""
  caption="Extraction and rendering meet at the flat template: the same builder produces the text that is hashed, translated, and parsed back."
/>

At extract time the transform replaces the original JSX with a runtime call site
and bundles a per-locale dictionary mapping each hash to a translation in the
**runtime textual projection**: a flattened string where variables are
`{identifier}`, standalone JSX placeholders are `{=mN}` with no matching close,
and paired elements are `{=mN}` … `{/=mN}` around their inner content. This is
the only textual form the runtime parses; every other consumer uses one of the
framework's other projections.

Compiling a dictionary also checks each target against its source on that
flattened form. A translation whose placeholder and marker tokens do not match
the source's, counted rather than merely present (one token where the source
had two still loses a value; a token the source never had renders as a literal
brace), is left out of the dictionary and named in the compile output. The
runtime then falls back to source for that key, which reads as pending
target-language work rather than as a sentence missing its count, its name or
its link.

The runtime resolves the hash, substitutes named-variable tokens, and walks the
remaining markers, scanning the resolved text once to identify pair scopes. An
open token with a matching close in the same scope forms a **paired** range; one
without is **standalone**. A paired range renders the slice between the tokens as
children and clones the original element with those children, preserving its
handlers and props; a standalone token substitutes its element directly. The
output is a fragment of interleaved strings and elements, with no wrapping
`<span>`, so layouts that rely on spacing between direct children are not
disrupted.

ICU lives inside the translated string: plural and select forms resolve through
`Intl.PluralRules`, and number, date, and time skeletons through the
corresponding `Intl` formatters. Because the format is in the *string*, a
translator can add one to a target (German wants a grouped number where English
did not) without a source change.

**Inline mode** bakes the translation into the JSX at build time. For rich text
that means *rebuilding* the element: the translated template's pairs are matched
last-in-first-out and each range is re-wrapped in the original child's opening
and closing tags, props and handlers preserved verbatim, while standalone tokens
splice the child's source in. The translator may reorder and renest; the JSX
follows. Anything the builder cannot account for is escaped as text rather than
emitted raw: a leaked interchange token in the DOM is the one outcome that must
be impossible.

ICU is the documented exception. A plural's pivot is a runtime value, so no build
step can choose the form. Blocks carrying ICU therefore keep a runtime call
**even in inline mode**, with the translated template baked in as the call's
fallback: the dictionary fetch disappears, the small resolver stays. Emitting the
source-locale form, or trying to inline a runtime choice, is either silently
wrong or invalid JSX.

### Entry points

The package exposes one build adapter per bundler (`./vite`, `./webpack`,
`./rollup`, `./esbuild`), the runtime (`./runtime`, with `./runtime/pseudo` for
the pseudo locale), the head hooks (`./head`), the review layer (`./review`,
`./review/hosted`, `./review/manifest`), the ship-status picker (`./ship`), and
two more that exist for hosts the adapters cannot reach:

- `./loader` is a plain webpack loader, named by a path string in
  `module.rules`, for a host that evaluates its config through a transpiler
  that cannot load the unplugin adapter. Docusaurus is the case in hand. It
  carries only the per-file transform, so inline mode works exactly as through
  the adapter, while runtime mode's chunk manifest and the review overlay's
  script injection, both bundle-level, are given up.
- `./storybook` supplies a locale toolbar and a decorator that applies a
  dictionary to a story, so a published Storybook renders in each locale.

### Lint keeps the source extractable

`@neokapi/i18n-react-lint` is a source-authoring plugin that works unchanged for
both ESLint flat config and oxlint (oxlint's plugin API is a strict subset of
ESLint's, so no adapter is needed) and ships shareable `recommended` and
`recommended-strict` configurations.

Its rules flag patterns that would fragment or escape extraction: a non-literal
first argument to `t()`, concatenation or interpolation inside `t()`,
concatenation or a ternary in a translatable attribute, a bare string literal in
a JSX expression container, a ternary with string-literal branches as a child,
and label props or expressions that should be wrapped. It validates the *source*,
not translated output: translation QA routes through kapi, and the compile step
only flattens target runs into per-locale dictionaries.

### Head strings have no DOM node

A `<title>`, a meta description, or a card title is page metadata, not rendered
content, so there is nothing to hover and nothing to outline, and these are
frequently the highest-value strings on the page. They are reached through the
opt-in head hooks, and because a head string never gets a DOM text node, the
build transform cannot rewrite it the way it rewrites a call site.

Instead the hook **self-hashes** its source at render time, using a browser port
of the build hasher over a dedicated head descriptor channel, and looks the
translation up in the runtime dictionary. The extractor recognises the same hook
calls and emits a matching block, so the hash the hook computes and the block
hash the extractor writes are identical by construction: the hash-parity
invariant, extended to the head, and asserted by a test.

As it applies the string, the hook registers its kind, hash, and source in a
small framework-free registry the review overlay reads, so head translatables are
listed and edited through the identical write-back a body string uses. Nothing
scrapes arbitrary head markup: only strings the pipeline translated register, and
a page with none shows no panel.

## Review on the running app

### Stamp the DOM at build time

Under `review: true` (or `KAPI_REVIEW=1`), the transform adds three attributes to
each extracted element:

| Attribute | Carries |
| --- | --- |
| `data-kapi-id` | the block hash |
| `data-kapi-loc` | `file:line`, for jump-to-source |
| `data-kapi-attr` | the hashes of the element's translatable attributes |

The identifier goes in an *attribute*, never in the text. Two mechanisms were
considered and not taken. **Fiber inspection**, reading React's internal tree
for debug source information, is not viable, because React 19 exposes none.
**In-band markers**, invisible characters or private-use code points inside the
rendered text, need no build step, but then the rendered string is not the
string the application computed: equality comparisons, measurement, and
copy/paste all see the marker, and an attribute value has no text node to carry
one at all. Review must be observation-free; the application under review has to
behave exactly as it does in production.

An attribute is the out-of-band channel the build already has an opportunity to
write. The rendered string stays byte-identical, `===` still works, and attribute
blocks are addressable on the same footing as children. The plugin already visits
every extracted element to rewrite it, so the stamp costs no extra traversal and
no runtime identification machinery.

This is a dev and staging facility, gated behind an explicit flag: it puts source
paths in the DOM and mounts a middleware that writes to disk.

### The dev server serves the interchange tree

The Vite plugin mounts a middleware at `/__kapi/review`, written against plain
Node HTTP so it ports to any dev server, over the local interchange directory:

| Route | Purpose |
| --- | --- |
| `GET {base}/{hash}` | the block payload: source runs, targets, notes |
| `PUT {base}/{hash}` | write a target back into the block's file |
| `GET {base}/annotations` | stand-off findings, keyed by block hash |
| `GET {base}/events` | server-sent events, broadcast to every open tab |

The index rebuilds whenever a block file or an overlay sidecar changes on disk,
so an out-of-band translation or QA pass shows up without a restart.

### A review is a diff

A reviewed target is written into the block's own file as a target run. The
obvious alternative, a review database with comments, states, and an approve
button, was rejected.

That file tree is *already* the contract between developers and translators:
extraction produces it, translation fills it, QA reads it, compilation ships it.
Writing a reviewed target into it makes the review a **git diff**: reviewable in
a pull request, revertable, attributable, and consumed by the very next compile
with no synchronisation step. A reviewer's fix travels the identical path as a
translator's, because it is the identical artifact. It also means review needs no
server, no account, and no network.

The cost is that local review has no workflow state (assignment, approval,
threaded comments). That is deliberate: those belong to a platform tier, and the
same stamping and the same hashes carry the overlay there unchanged.

### Reading a string inside the component that ships it

A catalog tells a reviewer what a string says; the component can show what it
looks like in the button. For a JSX project the component is usually already
published as a story. The join is by convention, resolved when someone looks:
a bundle records the component file each block came from, and a Storybook's
index records the file each story was written in. Nothing bakes a story id into
the catalog, which would couple the extractor to the consuming Storybook's id
derivation and freeze a stale id into every bundle.

The translation is posted in rather than read from the published build. A story
renders the dictionary it was built with, so the review surface sends what the
reviewer is holding, drafts included, and the `./storybook` decorator merges it
over what the story already had: the strings under review are the reviewer's,
and everything else stays translated instead of falling back to source. The key
the runtime looks a string up by is the block hash, which the KBF reader carries
into block properties so a block in a bundle has the same address inside a
running page.

### Content addressing makes review render-mode-independent

`data-kapi-id` is the block's content hash, computed identically whether the
build **inlines** the translation for a static or server-rendered page or emits a
**runtime** call, the same hash-parity invariant. Nothing about the stamp
depends on a runtime dictionary being present.

To make a statically rendered page self-sufficient, the plugin emits a read-only
review manifest whenever review is on, gated on review rather than on mode, so
an inline build emits it as much as a runtime one. A **hosted overlay** fetches
that manifest, finds elements by `data-kapi-id`, and needs neither a dev server
nor the application's i18n runtime: deep-linking to a single block, a whole-page
index, and read-only inspection all work on a plain static deploy. With editing
enabled it writes through the identical `PUT` protocol and repaints by patching
the element's text directly; text-only, so blocks with inline codes or ICU
plurals save to the store but are not repainted live. Richer manifests, covering
every locale and the term and QA annotations, still come from the compile step
over the whole tree.

### Findings are painted beside the DOM

Terminology and QA results are stand-off annotations anchored to run-index
ranges, the framework's existing overlay model
([F-02](../foundations/f-02-content-model.md)). The browser renders them through
the CSS Custom Highlight API: `Range` objects are registered in named highlights
and styled with `::highlight(kapi-term)` and `::highlight(kapi-qa)`.

Nothing is inserted into the DOM. The classic approach, wrapping matched text in
a styled `<span>`, mutates the tree React owns, which means layout shifts,
hydration mismatches, and a re-render silently discarding the highlights. Custom
Highlight paints in the text renderer, *beside* the DOM: React's tree is
untouched, so what the reviewer sees is the application, not the application plus
the instrument. Mapping a stand-off range to a `Range` is direct, because both are
offsets into the same flat text the block already carries.

### Shipping a locale

A separate subpath turns the project's ship status into a language picker's
render model. A minimal manifest, emitted by the CLI, keys each locale to two
gates: *shippable* (cleared the ship gate, safe to offer) and *verified*
(a person reviewed or signed off). A locale that ships but is not verified is
AI-only work and is the only case this layer badges. Display labels derive from
each locale code as its endonym through `Intl.DisplayNames`, so no per-locale
label table is needed; an explicit label still wins, for the codes `Intl` cannot
name. See [C-04](../context/c-04-unit-state-and-decisions.md) for what the gates
mean.

## Consequences

- **One block per translatable element.** Inline structure stays inside the
  block, so content memory keys on whole sentences and translators see sentences
  with their context. Model and machine-translation quality is better for
  sentences with inline links and emphasis than for the fragments a sub-key
  scheme produces.
- **A single emit path.** Inline-with-children becomes a pair, whatever the inner
  content is. There is no special case for "only an icon inside" or "only one
  variable inside".
- **A single textual grammar.** Named variables, an element token, and its close
  half are the whole vocabulary; the runtime decides standalone versus paired by
  looking for a matching close in scope, so no separate marker prefix is needed.
- **Refactor-stable keys.** Rewording orphans a translation, restructuring does
  not, which is the property that makes a large React codebase survivable.
- **One translatability table.** The JSX transform and the Go readers classify
  the same elements the same way, and a change on either side fails a gate.
- **A hole never ships silently.** A target that lost a placeholder is left out
  and named; the reader sees the source sentence, not a broken one.
- **The framework convention extends to JSX.** The same `Run[]` model as the HTML
  reader, the same paired-code semantics, the same projections at boundaries, so
  the visual editor, interchange round-trip, memory matching, and translation
  tools work on this output with no special cases.
- **The reviewer sees the application.** Truncation, wrapping, tone in place, a
  term that reads wrong next to the button beside it: all visible at review time.
- **Review is a diff.** No import, no export, no sync step, no platform
  dependency for the local tier.
- **QA and terminology become visual.** The checks kapi already runs stop being a
  list of line numbers and become marks on the words they are about, which is the
  only form in which a reviewer acts on them.
- **The stamp is the seam.** The same identifier that lets a local overlay find a
  block lets a hosted overlay find one, and a platform tier is a different backend
  behind the same client contract rather than a different feature.
- **Enable review deliberately.** It puts `file:line` in the DOM, mounts a
  writer, and bakes a manifest: appropriate for development and preview deploys,
  and off for the production build end users get.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): run sequences, inline codes, stand-off overlays, projections at boundaries
- [F-03: Identity](../foundations/f-03-identity.md): content-addressed block identity
- [E-02: The format system](../engine/e-02-format-system.md): how an extractor plugs into the pipeline, and the readers that share the translatability table
- [E-03: The tool system](../engine/e-03-tool-system.md): the tools that consume extracted blocks
- [C-04: Unit state and decisions](../context/c-04-unit-state-and-decisions.md): the ship and verified gates the picker reads
- [M-06: Content packages](../multilingual/m-06-content-packages.md): the interchange files review reads and writes
- [S-06: The visual editor data model](s-06-visual-editor.md): the vocabulary that styles these runs
- [React i18n guide](/react/introduction): the user-facing documentation
- [In-context review](/react/in-context-review): the review guide
