---
sidebar_position: 3
title: Writing Translatable React Components
description: How to write React components for kapi-react extraction — what the plugin picks up automatically, which patterns require t(), how inline elements become paired markers, and what to avoid.
keywords: [React components, JSX extraction, translatable, kapi-react, inline markers, componentMap, i18n patterns]
---

# Writing translatable components

Almost everything you already write is translatable. This page walks through the rules the plugin applies, the warnings it fires when it makes a judgement call you should know about, and the half-dozen patterns that break extraction silently.

## The short version

- **JSX text inside a translatable element** → extracted.
- **Direct text inside a container (`<div>`, `<section>`, …)** → extracted (auto-promotion, silent).
- **Direct text inside an unmapped React component** → extracted, with a warning and a suggestion to add a `componentMap` entry.
- **Inline elements with children** (`<strong>foo</strong>`, `<a href="…">here</a>`, `<em>{name}</em>`) → captured as one translatable block; the inline element becomes a **paired marker** wrapping its inner content, so the translator sees the inner words and can move the wrapping around.
- **Zero-children inline elements** (`<br/>`, `<Icon/>`, `<Spinner/>`, `<Badge/>`) → become **standalone markers** (`{=mN}` with no matching close) in the surrounding text.
- **A set of attributes** — `title`, `subtitle`, `description`, `label`, `placeholder`, `alt`, `helpText`, `tooltip`, `aria-*` — on any element → extracted.
- **Translatable attributes with string-literal ternaries** (`title={cond ? "A" : "B"}`) → each branch extracted as its own block.
- **Non-translatable elements** (`<code>`, `<pre>`, `<kbd>`, `<var>`, `<script>`, `<style>`, `<textarea>`) → skipped.
- **Elements marked `translate="no"`** (or any ancestor) → skipped.

## The detail

### Translatable JSX text

Headings, paragraphs, buttons, labels, options, `<span>`, `<strong>`, `<em>`, `<a>`, `<b>`, `<i>` — the whole set of elements the W3C HTML5 spec classifies as phrasing or translatable block content.

```tsx
<h1>Welcome</h1>                        // ✓ extracted
<p>Ship in every language.</p>          // ✓ extracted
<button>Get started</button>            // ✓ extracted
<label>Email address</label>            // ✓ extracted
<a href="/docs">Read the docs</a>       // ✓ extracted
<option value="fr">French</option>      // ✓ extracted
```

### Inline children — one block, paired markers

When an element mixes text with inline children, the whole thing becomes one translatable block. Each inline element with children becomes a **paired marker** in the parent's text — the translator sees the inner words and can move the wrapping around:

```tsx
<p>
  Click <a href="/docs">here</a> to read the docs.
</p>
```

The extractor stores this as `"Click {=m0}here{/=m0} to read the docs."`. A German translation reads `"Klicken Sie {=m0}hier{/=m0}, um die Dokumentation zu lesen."` — the link wraps the right word, and a French translator can move it elsewhere in the sentence entirely.

Inline elements that produce paired markers: `<span>`, `<strong>`, `<em>`, `<b>`, `<i>`, `<a>`, `<small>`, `<sub>`, `<sup>`, `<time>`, `<u>`, `<wbr>`, `<del>`, `<ins>`. (`<code>`, `<kbd>`, `<var>`, `<samp>` render code-as-code and are non-translatable — see below.)

The rule is uniform: **any inline element with at least one child → paired pair**, regardless of whether the inner content is text, an expression, an icon, or further nested elements. Empty inline elements become **standalone markers** instead. A few examples:

| Source                          | Extracted form                          |
| ------------------------------- | --------------------------------------- |
| `<a>here</a>`                   | `"{=m0}here{/=m0}"`                     |
| `<a><Icon/></a>`                | `"{=m0}{=m1}{/=m0}"`                    |
| `<a>{userName}</a>`             | `"{=m0}{userName}{/=m0}"`               |
| `<strong>{count}</strong>`      | `"{=m0}{count}{/=m0}"`                  |
| `<a>read <em>the</em> docs</a>` | `"{=m0}read {=m1}the{/=m1} docs{/=m0}"` |
| `<Icon/>` (no children)         | `"{=m0}"` (no matching `{/=m0}` close)  |
| `<br/>` (no children)           | `"{=m0}"` (no matching `{/=m0}` close)  |

JSX-element tokens always read `{=m<N>}`; the runtime tells standalone from paired by looking for a matching `{/=m<N>}` close in the same scope. Variable tokens (`{userName}`, `{count}`) carry the JS identifier directly.

### Empty inline elements as standalone markers

Lots of real React UI looks like `<Button><Icon />Open File...</Button>` — an icon component followed by text. Empty inline elements (zero children) become a single standalone marker, leaving the surrounding text to extract normally:

```tsx
<Button>
  <FolderOpen size={12} />
  Open File...
</Button>
```

Extracts as `"{=m0} Open File..."` with `{=m0}` bound to the `<FolderOpen />` element (standalone — no matching `{/=m0}` close). Works the same for Radix icons, lucide-react, Heroicons, custom `<Spinner />` components — anything with no children.

Unmapped React components with children are still treated as block-level by default (warning suggests a `componentMap` entry — see "Unknown components" below). The narrow rule for _zero-children_ unmapped components prevents false positives on custom block-level components like `<Panel><Heading>…</Heading></Panel>`.

### Auto-promoted containers

Strict W3C semantics would skip `<div>Hello</div>` — divs are classified as containers, not text. In real React codebases that's wrong: `<div>Label</div>`, `<section>Intro copy</section>` are everywhere.

kapi-react **auto-promotes** container elements when they have:

1. At least one direct non-whitespace JSXText child, AND
2. Only inline children (no nested block-level elements).

Promotion is silent — `<div>Label</div>` is the dominant idiom and warning on every occurrence would just be noise. (Unmapped React components still warn; see below.)

To opt out: `<div translate="no">...</div>` or a rule:

```ts
neokapi({
  rules: [{ selector: ".hero-video-caption", translate: false }],
});
```

### Unknown components

Component libraries like shadcn, Radix, MUI, and your own internal components render to HTML but kapi-react can't know which one. By default, an unmapped React component with direct translatable text is extracted anyway, with a warning that suggests how to stabilise the hash:

```tsx
<TabsTrigger value="general">General</TabsTrigger>
```

```text
[neokapi] src/Settings.tsx:19: <TabsTrigger> is an unmapped component with
  translatable text — extracted. Add a componentMap entry to stabilise
  hashes: { TabsTrigger: '<underlying-html-tag>' }.
  ↳ <TabsTrigger value="general">General</TabsTrigger>
```

Adding the hint removes the warning and changes the hash from one keyed on `TabsTrigger` to one keyed on `button`:

```ts
neokapi({
  componentMap: {
    TabsTrigger: "button",
    TabsList: "div",
    DialogTitle: "h2",
  },
});
```

**Why bother?** Because the hash is part of the translator's contract. If you later refactor by changing `TabsTrigger` → a different library's `Tab`, and the underlying HTML is still `button`, the hashes stay stable if you had the `componentMap` entry — translators don't need to re-review.

### Translatable attributes

These attribute names are extracted on any element (mapped or not):

| Bucket            | Names                                                                                                                 |
| ----------------- | --------------------------------------------------------------------------------------------------------------------- |
| HTML              | `alt`, `title`, `placeholder`                                                                                         |
| ARIA              | `aria-label`, `aria-description`, `aria-placeholder`, `aria-roledescription`, `aria-valuetext`                        |
| React conventions | `subtitle`, `description`, `label`, `heading`, `caption`, `helpText`, `helperText`, `errorMessage`, `hint`, `tooltip` |

So these all work out of the box:

```tsx
<input placeholder="Search..." aria-label="Search products" />
<img alt="Company logo" />
<button title="Save draft">💾</button>

<PageHeader title="Translation Memories" subtitle="Glossaries" />
<EmptyState title="No projects yet" description="Create one to get started." />
<LoadingSpinner helpText="Contacting the server…" />
<Tooltip tooltip="Retry the last operation" />
```

Each attribute becomes its own translatable block.

### Ternary attribute values

When a translatable attribute's value is a ternary with _both branches as plain string literals_, each branch extracts as its own block:

```tsx
<PageHeader title={isProjectMode ? "Project Flows" : "Flows"} />
```

Both `"Project Flows"` and `"Flows"` get extracted (with `::0` / `::1` suffixes on the context to keep the hashes distinct). At runtime the transform rewrites each literal branch with its own `__t()` lookup; the condition still fires at render time.

Mixed-shape ternaries (one literal, one computed, or both templates) _aren't_ statically extractable — the lint rule [`no-ternary-in-translatable-attr`](./linting#no-ternary-in-translatable-attr) flags them. Fix by wrapping both branches with `t()` so the t-call walker picks them up:

```tsx
// ✗ extractor can't see the template-literal branch
<Input placeholder={disabled ? `Disabled (${reason})` : "Enabled"} />

// ✓ both branches flow through the t() extraction path
<Input placeholder={disabled ? t("Disabled ({reason})", { reason }) : t("Enabled")} />
```

### Non-translatable elements

These render text-as-text, not natural language, so they're skipped:

`<code>`, `<pre>`, `<kbd>`, `<var>`, `<samp>`, `<script>`, `<style>`, `<textarea>`.

```tsx
<code>npm install @neokapi/kapi-react</code>    // ✗ not extracted
<pre>{licenseText}</pre>                        // ✗ not extracted
```

To flip one specific site: `<code translate="yes">...</code>`.

### Opting out with `translate="no"`

Standard HTML — works on any element and its descendants:

```tsx
<h1 translate="no">API_KEY_PREFIX</h1>         // ✗ not extracted

<section translate="no">
  <h2>Debug payload</h2>                       // ✗ not extracted
  <pre>{json}</pre>                            // ✗ not extracted
</section>
```

Both the extractor and every lint rule in `@neokapi/kapi-react-lint` walk up the ancestor chain looking for `translate="no"`. A single marker at the top of a subtree silences everything inside — no need to sprinkle it on every element.

`translate="no"` is also the right answer when you're intentionally rendering an already-translated value (see "Module-level `t()` gotcha" below), or when your content is code-like and shouldn't be flagged as missing translation.

### Rules for recurring patterns

For patterns where you don't want to sprinkle `translate="no"` everywhere, use rules in your plugin config:

```ts
neokapi({
  rules: [
    { selector: ".monospaced-input", translate: false },
    { selector: "[data-testid]", translate: false },
    { selector: ".legal-copy", locNote: "Must match legal-approved wording verbatim" },
  ],
});
```

Selectors: plain tag (`code`), class (`.code-block`), attribute presence (`[data-testid]`), or attribute value (`[role="alert"]`).

## What still needs explicit handling

The extractor can only see what it can statically reason about. These patterns slip through — each has a canonical fix.

### Strings in JS data structures

```tsx
const THEMES = [
  { value: "system", label: "System" }, // ✗ not extractable
  { value: "light", label: "Light" },
];

return THEMES.map(({ value, label }) => (
  <button>{label}</button> // ✗ label is an expression
));
```

Fix with the [`t()` escape hatch](./t-escape-hatch):

```tsx
const THEMES = [
  { value: "system", label: t("System") },
  { value: "light", label: t("Light") },
];
```

Caught by the `prefer-t-for-label-props` lint rule (off by default; opt in via `recommendedStrict`).

### Dynamic label expressions

The render-side mirror of the above: `{obj.label}` / `{item.title}` rendered as JSX text. The extractor sees an expression container and emits a placeholder; the string it resolves to at runtime never becomes a translation unit.

```tsx
// ✗ meta.label is invisible to extraction
<h1>{meta.label}</h1>
```

Fix by wrapping the _source_ data with `t()` (same as "Strings in JS data structures" above). The lint rule [`prefer-t-for-label-expr`](./linting#prefer-t-for-label-expr) flags the render site to prompt the refactor.

### Ternary with string literals as JSX children

```tsx
// ✗ neither "Saving..." nor "Save" gets extracted
<Button>{saving ? "Saving..." : "Save"}</Button>
```

kapi-react treats the whole ternary as a single opaque placeholder — it never looks inside at the branches. Wrap each branch with `t()`:

```tsx
<Button>{saving ? t("Saving...") : t("Save")}</Button>
```

Caught by [`no-ternary-literals-in-jsx-child`](./linting#no-ternary-literals-in-jsx-child). Same fix applies to template literals with actual copy: `` `Loading ${n}...` `` → `t("Loading {n}...", { n })`.

### Module-level `t()` gotcha

`t()` reads the active dictionary **at call time**. A module-level const evaluates once, at import time — typically _before_ `loadTranslations()` has finished. The const freezes at the fallback language forever.

```tsx
// ✗ "Utility" will still say "Utility" in pseudo.
const categoryMeta = {
  utility: { label: t("Utility") },
  pipeline: { label: t("Pipeline") },
};
```

Fix: wrap the lookup in a function that runs per render.

```tsx
// ✓ each render resolves the label against the current dict.
function categoryMeta(cat: string) {
  switch (cat) {
    case "utility":
      return { label: t("Utility") };
    case "pipeline":
      return { label: t("Pipeline") };
    // …
  }
}

function Chip({ cat }: { cat: string }) {
  const meta = categoryMeta(cat);
  return <span>{meta.label}</span>;
}
```

### Double-translation: already-translated values inside translatable blocks

A subtle pattern that only shows up in pseudo. If you render a `t()`-resolved string as a child of an element the extractor also wraps as a block, pseudo-translation gets applied _twice_ — the inner `t()` adds its markers, and the outer element's translation wraps around them:

```tsx
<Button>
  {meta.label} ({catTools.length})
</Button>
// Pseudo renders: ▒ ▒ Utility ▒ (32) ▒   ← two layers of wrapping
```

Fix: mark the outer element `translate="no"` so the inner `t()` call owns the translation.

```tsx
<Button translate="no">
  {meta.label} ({catTools.length})
</Button>
// Pseudo renders: ▒ Utility ▒ (32)       ← the inner t() wrap is the only one
```

Alternative: lift the whole string into a single `t()` call with placeholders — but that's awkward when one half is a translated label and the other is a numeric count.

## Translator notes

Attach a note to an element so translators see context when they open the block:

```tsx
<button data-i18n-note="verb: to close a dialog, not 'nearby'">Close</button>
```

Or via a rule:

```ts
rules: [{ selector: ".legal-copy", locNote: "Legal team must review" }];
```

## Summary: what goes where

| Source pattern                            | Extracted? | Notes                                                                    |
| ----------------------------------------- | ---------- | ------------------------------------------------------------------------ |
| `<h1>Hello</h1>`                          | ✓          | standard translatable element                                            |
| `<div>Hello</div>`                        | ✓          | auto-promoted silently                                                   |
| `<Button><Icon/>Save</Button>`            | ✓          | "Save" extracts with `{=m0}` standalone for the icon                     |
| `<TabsTrigger>Hello</TabsTrigger>`        | ✓          | warning suggests `componentMap`                                          |
| `<PageHeader title="Hi" />`               | ✓          | `title` in the translatable-attributes set                               |
| `<PageHeader title={cond ? "A" : "B"} />` | ✓          | both branches — one block each                                           |
| `<MyComp description="Hi" />`             | ✓          | `description` too                                                        |
| `<p>Click <a>here</a></p>`                | ✓          | one block, `<a>` becomes paired `{=m0}…{/=m0}`                           |
| `<code>foo</code>`                        | ✗          | non-translatable element                                                 |
| `<h1 translate="no">X</h1>`               | ✗          | explicit opt-out (suppresses lint too)                                   |
| `<button>{label}</button>`                | ✗          | bare expression — use `t()` on the source                                |
| `<button>{obj.label}</button>`            | ✗          | flagged by `prefer-t-for-label-expr` — wrap the source                   |
| `<button>{cond ? "A" : "B"}</button>`     | ✗          | flagged by `no-ternary-literals-in-jsx-child` — wrap branches with `t()` |
| `<div>{cond && 'Hi'}</div>`               | ✗          | expression — use `t()`                                                   |

## Next

- [`t()` escape hatch](./t-escape-hatch) — for the JS-data-string and ternary-in-JSX patterns.
- [Plurals and select](./plurals-and-select) — count-aware and choice-based text.
- [Linting](./linting) — editor squigglies for every anti-pattern on this page.
- [Extract pipeline](./pipeline) — how the plugin ships this to translators.
