---
id: 019-kapi-react
sidebar_position: 19
title: "AD-019: kapi-react extraction model"
description: "Architecture decision: kapi-react extracts translatable content from React/JSX source at Vite build time, producing Block records whose Source is a typed Run[] — the same content model as the rest of the framework."
keywords: [kapi-react, extraction model, JSX, Vite plugin, Block, Run, architecture decision]
---

# AD-019: kapi-react extraction model

## Summary

kapi-react extracts translatable content directly from React/JSX source at
build time, producing `Block` records whose `Source` is a typed
`Run[]` consistent with the framework's canonical inline-content model
([AD-002: Content Model](002-content-model.md)). Inline JSX elements with
children become a paired `PcOpenRun` + inner runs + `PcCloseRun` triple in
their parent's Run sequence, so a sentence like `<p>Click <a
href="/docs">here</a> to read.</p>` extracts as one Block whose translator
keeps the link wrapped around the right word in every target language. A
small runtime (`__t` / `__tx`) interleaves React elements at marker
positions when rendering translations. A lint package (`kapi-react-lint`)
flags i18n anti-patterns in JSX source so strings extract cleanly.

## Context

A React-native localization story has two hard parts:

1. **Authoring.** Developers write JSX. Translators want sentences with
   inline structure (`<a>`, `<strong>`, variables, icons) — not opaque
   placeholders or fragmented sub-strings. Forcing developers to wrap each
   string in a `t("hello-key")` call breaks the natural reading order of a
   component and pushes inline structure into separate sub-keys.
2. **Runtime.** The translated string must compose back with the original
   React elements (preserving event handlers, refs, attributes) at render
   time. A naive HTML-string round-trip loses event handlers and bypasses
   React's reconciliation.

kapi-react addresses both: an SWC-based AST walker extracts translatable
JSX into `Run[]` at build time; a small runtime function re-attaches the
extracted React elements when the runtime resolves the target. The
extracted Block participates in the same neokapi pipeline as any other
format — TM, AI translation, MT, lint — through `Run[]` as the canonical
form.

The framework requires extractors to follow the **structural-canonical
with projections at boundaries** convention from AD-002: emit `Run[]`,
let framework projections (`RunsSemanticHTML`, `RunsPlaceholderText`,
`flattenRuns`, `RenderRunsWithData`) serve every downstream consumer.
kapi-react is the first first-party extractor to apply this convention to
JSX.

## Decision

### Build-time extraction

A SWC-AST walker (`packages/kapi-react/src/extract/walker.ts`) descends
each component module looking for translatable JSX. Translatability is
determined by element vocabulary (`getTranslatability`,
`inlineElements`) plus user-supplied `componentMap` and `rules`
(`packages/kapi-react/src/plugin/defaults.ts`). For each translatable
element the walker emits a Block whose `Source` run sequence is built by the
runs builder (`extract/runs.ts`).

### Inline elements: paired codes

The defining rule of kapi-react extraction:

**An inline JSX element with at least one child becomes a paired
inline code in its parent's Run sequence.**

```tsx
<p>
  Click <a href="/docs">here</a> to read the docs.
</p>
```

extracts as one Block whose `Source` is:

```
TextRun("Click ")
PcOpenRun  { id: "0", type: "jsx:element", subType: "a",
             data: '<a href="/docs">', equiv: "=m0" }
TextRun("here")
PcCloseRun { id: "0", type: "jsx:element", subType: "a",
             data: "</a>", equiv: "=m0" }
TextRun(" to read the docs.")
```

Type and subType follow the JSX vocabulary in `@neokapi/kapi-format`: every JSX element uses `type: "jsx:element"` with the resolved HTML tag (or unmapped React component name) in `subType`. The vocabulary entry handles editor rendering, chip labels, and constraints. Future work may map `<a>` → `link:hyperlink`, `<strong>` → `fmt:bold` etc. so XLIFF exchange uses semantic `<pc type="link">` codes; this AD captures the current end state.

Pairs nest LIFO; the same machinery handles `<a>read <em>the</em>
docs</a>` (two pairs, ids `m0` and `m1`). Inner content may contain text,
expressions (`{userName}`), placeholders (`<Icon/>`), or further paired
elements:

| Source                          | Runs                                                       |
| ------------------------------- | ---------------------------------------------------------- |
| `<a>here</a>`                   | `pcOpen + text + pcClose`                                  |
| `<a><Icon/></a>`                | `pcOpen + ph(icon) + pcClose`                              |
| `<a>{userName}</a>`             | `pcOpen + ph(userName) + pcClose`                          |
| `<strong>{count}</strong>`      | `pcOpen + ph(count) + pcClose`                             |
| `<a>read <em>the</em> docs</a>` | `pcOpen + text + pcOpen + text + pcClose + text + pcClose` |

This makes the translator the unit of decision. A German translation can
write `Klicken Sie {=m0}hier{/=m0}, um die Dokumentation zu lesen.` and
the link wraps the right word; a French translator can move it elsewhere
in the sentence.

### Standalone placeholders

JSX constructs without children — self-closing icons, `<br/>`, zero-child
unmapped components, expression containers, conditional nodes — become a
single `PlaceholderRun` rather than a paired pair:

| Source                | Run                                                     |
| --------------------- | ------------------------------------------------------- |
| `<Icon/>`             | `ph { type: "jsx:element", equiv: "=m0" }`              |
| `<br/>`               | `ph { type: "jsx:element", subType: "br", equiv: "=m0" }` |
| `{userName}`          | `ph { type: "jsx:var", equiv: "userName" }`             |
| `{cond && <Banner/>}` | `ph { type: "jsx:node", equiv: "=m0", optional: true }` |

JSX-element placeholders share the `=m<N>` synthetic-id convention with
paired pairs; the difference is structural — a standalone is a single
`ph` run, a paired is a `pcOpen` + inner runs + `pcClose` triple. In flat
textual form a standalone token is `{=m0}` with no matching `{/=m0}`
close anywhere in the same scope. The runtime parser disambiguates by
look-ahead within the scope (see [Runtime rendering](#runtime-rendering)).

Variable expression containers (`{userName}`, `{count}`) keep the JS
identifier as their equiv so the flat form reads naturally to translators
and substitutes through the standard `{name}` parameter path at runtime.

### Auto-promoted containers

`<div>Hello</div>`, `<section>Intro copy</section>`, and similar
container elements are auto-promoted to translatable when they have at
least one direct non-whitespace JSXText child and only inline children
(`extract/translatable.ts`). Promotion is silent — the pattern is too
common in real React UIs to warn on every occurrence. Opt-out via
`translate="no"` or a `rules` entry.

Fragment roots (`<>…</>`) extract on the same terms, with the reserved
descriptor `fragment` (`FRAGMENT_DESCRIPTOR`) standing in for the tag
they don't have. A fragment is a real authoring idiom for
"inline content with no wrapper I'm allowed to add", and skipping it
would push authors toward a `<span>` they don't want in the DOM.

### Component vocabulary

Custom React components (`<TabsTrigger>`, `<DialogTitle>`,
`<MyButton>`) are extracted by default with a warning that suggests a
`componentMap` entry. With the entry, the component participates in
inline-vs-block classification and the resulting hash keys on the
mapped HTML element name rather than the React component identifier.

| Map entry                          | Behavior                                               |
| ---------------------------------- | ------------------------------------------------------ |
| `{ TabsTrigger: "button" }`        | Treats `<TabsTrigger>` as a translatable element.      |
| `{ Strong: "strong" }`             | Treats `<Strong>` as inline; eligible for paired pair. |
| `{ Icon: "x-icon" }` (no html tag) | Marks `<Icon>` as opaque inline (icon-tolerant).       |

The `componentMap` and the `rules` feed the hash, so the extract CLI
and the build plugin **must** be configured identically. A desync is
silent — both sides run without error and simply compute different
keys, so every affected string falls back to source. Projects should
keep one config file that both sides read.

### Attribute extraction is scoped by host

Translatable attributes come from two vocabularies with different
scopes (`plugin/defaults.ts`):

- `htmlTranslatableAttributes` — the HTML/ARIA names (`alt`, `title`,
  `placeholder`, `aria-label`, …). Standardised as user-visible text,
  so extracted on **any** element.
- `componentTranslatableAttributes` — React's prop-name conventions
  (`label`, `description`, `heading`, `helpText`, …). Extracted on
  **PascalCase components only**.

The scoping exists because the convention names are not reserved. On a
plain HTML element, `label`, `heading`, or `data` are far more often
DOM props, enum keys, or data-binding fields than copy, and extracting
them swept strings nobody wanted translated into the catalog. A
component author choosing `label=` for a visible prop is following a
convention; a `<div label="draft-pending">` is not. `isTranslatableAttribute(attr, tag)`
is the single predicate both the walker and the transform call.

### Key scheme

**The key is `FNV-1a 64` over `JSON.stringify(flatText) + "|" + desc`,
base62-encoded**, where `flatText` is the block's flat template (text
verbatim, expressions as `{name}`, inline elements as `{=mN}`) and
`desc` is a structural descriptor.

The descriptor is the element's **own resolved tag** — `"p"`,
`"button"`, `"fragment"`, or `"t\x1F<context>"` for `t()` calls.
Ancestors are deliberately excluded.

This is the load-bearing decision of the whole model, because the key
*is* the translator's contract. The property we need is: **a key
changes when, and only when, a translator should look at the string
again.** Rewording changes the key. Moving a paragraph into a new
wrapper does not — it is a layout refactor, and the German is still
the German.

Rejected alternatives:

- **Full ancestor path** (the original scheme). `"div > section > p"`
  as the descriptor made every layout refactor a silent mass-orphaning
  event: wrap a section in one new `<div>` and every string beneath it
  gets a new key, loses its translation, and quietly falls back to
  source. Structure is the *least* stable thing about a React
  codebase; keying on it inverts the stability we want.
- **Flat text alone.** Then `<p>Save</p>` and `<button>Save</button>`
  collapse to one key, and a translator can never give the noun and
  the verb different words. The immediate tag is the cheapest
  disambiguator that survives refactoring, and it maps to a real
  distinction (a button is not a paragraph).
- **Developer-invented keys.** The toil this project exists to remove.

Where the tag isn't enough — two buttons both reading "Open", one a
verb and one a state — the disambiguator is *explicit*: `data-i18n-note`
or `t(text, context)`, folded into `desc`. That is gettext's `msgctxt`
model, and it puts the decision where it belongs: with the author who
knows the two strings differ, not with an incidental fact about the
DOM tree.

64 bits (over the old 32) gives collision headroom for
million-string corpora; a 32-bit hash reaches ~50% birthday-collision
odds around 80k strings, which is inside the range a large app can hit.

### Hash and runtime dictionary

The hash plus a `fallback` string (a runtime-renderable representation
of the source) plus the elements map drives `__t` / `__tx`:

```ts
__tx(hash, fallback, elements, params);
```

At extract time, the transform replaces the original JSX with the
appropriate `__t` / `__tx` call site and bundles the per-locale
dictionary. The dictionary maps each hash to a translation expressed
in the **runtime textual projection** — a `flattenRuns`-style string
where:

- Variables use `{equiv}` with the JS identifier (`{userName}`, `{count}`).
- Standalone JSX placeholders use `{=m<N>}` with no matching close.
- Paired JSX elements use `{=m<N>}` … `{/=m<N>}` around their inner content.

This is the only textual form the runtime parses; every other consumer
uses one of the framework's other projections (see [AD-002 §
Boundaries](002-content-model.md)).

### Runtime rendering

`__tx` (`packages/kapi-react/src/runtime/index.ts`) resolves the hash to
the translation string, substitutes named-variable tokens (`{userName}`,
`{count}`), and walks the remaining `{=m<N>}` / `{/=m<N>}` tokens
interleaving React elements from the `elements` map.

The parser scans the resolved text once to identify pair scopes:

1. For every `{=m<N>}` open token, look ahead within the same scope for a
   matching `{/=m<N>}` close (LIFO well-formed nesting). Token pairs
   that match form a **paired** range.
2. Open tokens with no matching close are **standalone**.
3. For paired ranges, recursively render the slice between open and
   close as the children, then call `cloneElement` on `elements["=m<N>"]`
   with the rendered children — preserving event handlers and props
   from the original JSX.
4. For standalone tokens, substitute `elements["=m<N>"]` directly.

The output is a `React.Fragment` of interleaved strings and elements —
no wrapping `<span>`, so layout (e.g. shadcn-style buttons relying on
`items-center gap-N` between direct children) is not disrupted.

The runtime also resolves ICU inside the translated string: plural and
select forms via `Intl.PluralRules`, and `{n, number}` / `{d, date}` /
`{t, time}` skeletons via `Intl.NumberFormat` / `Intl.DateTimeFormat`
(`runtime/icu.ts`). The format lives in the *string*, so a translator
can add one to a target — German wants a grouped number where English
didn't — without a source change.

### One template builder, two consumers

Extract and transform must agree on the flat template **byte for
byte**: extract hashes it into the `.klf`, transform hashes it to look
the translation back up. They are two walks over the same AST in two
files, and for a while they were two *implementations* — which drifted,
and shipped two correctness bugs to prove it (paired inline elements
leaking a literal `{/=m0}` into the DOM, and `<Plural>` emitting
unparseable JSX).

The decision is therefore structural, not a fix: `extract/runs.ts`
owns the single `buildRuns()` builder, and the transform **consumes**
it (`plugin/transform.ts`), taking both the flat text and a list of
per-appearance `Occurrence` spans it maps back onto the source to
rewrite JSX. Parity is now a property of the code shape rather than of
two authors remembering to keep two walks in step. Any future front
end (MDX, another framework) plugs in at the same seam.

### Inline mode: reconstruction, and the ICU exception

Inline mode bakes the translation into the JSX at build time. For rich
text this means **rebuilding** the element: the translated template's
`{=mN}` / `{/=mN}` pairs are matched LIFO and each paired range is
re-wrapped in the original child's opening and closing tags (its props
and handlers preserved verbatim from source), while standalone tokens
splice the child's source in. The translator may reorder and renest;
the JSX follows. Anything the builder cannot account for is escaped as
text rather than emitted raw — a leaked interchange token in the DOM is
the one outcome that must be impossible.

ICU is the documented exception. A plural's pivot is a runtime value,
so no build step can choose the form; blocks carrying ICU therefore
keep a `__tx` call **even in inline mode**, with the translated
template baked in as the call's `fallbackOverride`. The dictionary
fetch disappears; the ~2 kB resolver stays. The alternative — emitting
the source-locale form, or attempting to inline a runtime choice — is
either silently wrong or invalid JSX, which is precisely the bug this
replaced.

### Lint validation

`@neokapi/kapi-react-lint` is a source-authoring ESLint/oxlint plugin.
Its rules catch i18n anti-patterns in the JSX/TSX *source* so content
extracts cleanly at build time — it does not validate translated output.
The plugin object works unchanged for both ESLint flat-config and oxlint
(oxlint's plugin API is a strict subset of ESLint v9's, so no adapter
layer is needed), and it ships shareable `recommended` /
`recommended-strict` configs (`packages/kapi-react-lint/src/configs/`).

The rules (`packages/kapi-react-lint/src/rules/`) flag patterns that
would fragment or break extraction:

| Rule                                | Flags                                                                 |
| ----------------------------------- | --------------------------------------------------------------------- |
| `t-literal-first-arg`               | A non-literal first argument to `t()`.                                |
| `t-no-concat`                       | String concatenation / template interpolation inside `t()`.           |
| `no-concat-in-translatable-attr`    | Concatenation in a translatable attribute (`alt`, `title`, `placeholder`, `aria-label`, …). |
| `no-string-literal-jsx-expr`        | A bare string literal in a JSX expression container.                  |
| `no-ternary-in-translatable-attr`   | A ternary in a translatable attribute.                                |
| `no-ternary-literals-in-jsx-child`  | A ternary with string-literal branches as a JSX child.                |
| `prefer-t-for-label-props`          | Label props that should be wrapped in `t()`.                          |
| `prefer-t-for-label-expr`           | Label expressions that should be wrapped in `t()`.                    |

Translation QA and validation are not the lint package's concern — the
kapi-react CLI (`packages/kapi-react/src/cli.ts`) routes those through
`kapi`, and `compile` only flattens target runs into per-locale
`{hash: text}` dictionaries.

## Consequences

- **One Block per translatable element.** Inline structure no longer
  produces a separate sub-Block per `<a>`, so TM keys on full sentences
  and translators see sentences with inline context. AI/MT quality
  improves measurably for sentences with inline links and emphasis.
- **Single emit path.** Inline-with-children → paired pair, regardless of
  whether the inner content is text, expressions, or icons. No special
  case for "only icons inside" or "only one variable inside."
- **Single textual grammar.** `{userName}`, `{count}` and similar carry
  named variables; `{=m<N>}` carries a JSX-element token; `{/=m<N>}` is
  the close half of a paired pair. The runtime decides standalone vs
  paired by looking for a matching close in the same scope — no separate
  marker prefix needed.
- **Lint keeps source extractable.** `@neokapi/kapi-react-lint` flags
  JSX-authoring anti-patterns — concatenation inside `t()`, string
  literals or ternaries in translatable attributes and children, labels
  that should be wrapped in `t()` — so strings extract cleanly into Blocks
  at build time rather than fragmenting or escaping extraction.
- **Framework convention extends to JSX.** kapi-react uses the same
  `Run[]` model as the HTML reader, the same paired-code semantics
  (`PcOpenRun` / `PcCloseRun`), and the same projections at boundaries.
  Tooling that already understands neokapi Blocks (the visual editor,
  XLIFF round-trip, TM matching, AI translate) works for kapi-react
  output without special cases.

## Related

- [AD-035: In-Context Review](035-in-context-review.md) — reviewing the
  extracted blocks on the running app; builds on the hashes and the
  transform decided here
- [AD-002: Content Model](002-content-model.md) — Run sequences, inline
  codes, projections at boundaries
- [AD-005: Format System](005-format-system.md) — readers/writers and how
  extractors plug into the pipeline
- [AD-006: Tool System](006-tool-system.md) — pipeline tools that consume
  extracted Blocks
