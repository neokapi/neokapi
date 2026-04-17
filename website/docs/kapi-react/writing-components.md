---
sidebar_position: 3
title: Writing translatable components
---

# Writing translatable components

Almost everything you already write is translatable. This page walks through the rules the plugin applies and the warnings it fires when it makes a judgement call you should know about.

## The short version

- **JSX text inside a translatable element** → extracted.
- **Direct text inside a container (`<div>`, `<section>`, …)** → extracted, with a warning.
- **Direct text inside an unmapped PascalCase component** → extracted, with a warning and a suggestion to add a `componentMap` entry.
- **Inline elements** (`<strong>`, `<a>`, `<em>`, `<span>`, …) **mixed with text** → captured as one translatable block, children replaced with position tokens the translator can reorder.
- **A set of attributes** — `title`, `subtitle`, `description`, `label`, `placeholder`, `alt`, `helpText`, `tooltip`, `aria-*` — on any element → extracted.
- **Non-translatable elements** (`<code>`, `<pre>`, `<kbd>`, `<var>`, `<script>`, `<style>`) → skipped.
- **Elements marked `translate="no"`** → skipped.

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

### Inline children — one block, not many

When an element mixes text with inline children, the whole thing becomes one translatable block. Inline children collapse to position tokens so translators can reorder them:

```tsx
<p>
  Click <a href="/docs">here</a> to read the docs.
</p>
```

The extractor stores this as `"Click {=m0} to read the docs."` with `{=m0}` bound to the `<a>` element. A German translator can write `"Klicken Sie {=m0}, um die Dokumentation zu lesen."` and the link moves with the token.

Inline elements include `<span>`, `<strong>`, `<em>`, `<b>`, `<i>`, `<a>`, `<code>`, `<kbd>`, `<small>`, `<sub>`, `<sup>`, `<time>`, `<u>`, `<var>`, `<wbr>`, `<del>`, `<ins>`.

### Auto-promoted containers

Strict W3C semantics would skip `<div>Hello</div>` — divs are classified as containers, not text. In real React codebases that's wrong: `<div>Label</div>`, `<section>Intro copy</section>` are everywhere.

kapi-react **auto-promotes** container elements when they have:

1. At least one direct non-whitespace JSXText child, AND
2. Only inline children (no nested block-level elements).

When promotion fires the plugin logs a warning so you know what happened:

```text
[neokapi] src/Settings.tsx:42: <div> contains translatable text — extracted.
  Add translate="no" on the element to opt out.
  ↳ <div className="mb-3 text-sm font-medium">Appearance</div>
```

To opt out: `<div translate="no">...</div>` or a rule:

```ts
neokapi({
  rules: [
    { selector: ".hero-video-caption", translate: false },
  ],
});
```

### Unknown components

Component libraries like shadcn, Radix, MUI, and your own internal components render to HTML but kapi-react can't know which one. By default, an unmapped PascalCase component with direct translatable text is extracted anyway, with a warning that suggests how to stabilise the hash:

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

| Bucket | Names |
|---|---|
| HTML | `alt`, `title`, `placeholder` |
| ARIA | `aria-label`, `aria-description`, `aria-placeholder`, `aria-roledescription`, `aria-valuetext` |
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

### Non-translatable elements

These render text-as-text, not natural language, so they're skipped:

`<code>`, `<pre>`, `<kbd>`, `<var>`, `<samp>`, `<script>`, `<style>`, `<textarea>`.

```tsx
<code>npm install @neokapi/kapi-react</code>    // ✗ not extracted
<pre>{licenseText}</pre>                        // ✗ not extracted
```

To flip one specific site: `<code translate="yes">...</code>`.

### Opting out with `translate="no"`

Standard HTML — works on any element:

```tsx
<h1 translate="no">API_KEY_PREFIX</h1>    // ✗ not extracted
<p>Your key starts with <code>ak-</code>…</p>
```

### Rules for recurring patterns

For patterns where you don't want to sprinkle `translate="no"` or rely on promotion warnings, use rules in your plugin config:

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

## What still needs `t()`

The one pattern where the extractor can't help is strings stored in JS data structures and fed to JSX as expressions:

```tsx
const THEMES = [
  { value: "system", label: "System" },   // ✗ not extractable
  { value: "light",  label: "Light" },
  { value: "dark",   label: "Dark" },
];

return THEMES.map(({ value, label }) => (
  <button>{label}</button>                // ✗ label is an expression
));
```

For this, use the [`t()` escape hatch](./t-escape-hatch).

## Translator notes

Attach a note to an element so translators see context when they open the block:

```tsx
<button data-i18n-note="verb: to close a dialog, not 'nearby'">Close</button>
```

Or via a rule:

```ts
rules: [
  { selector: ".legal-copy", locNote: "Legal team must review" },
]
```

## Summary: what goes where

| Source pattern | Extracted? | Notes |
|---|---|---|
| `<h1>Hello</h1>` | ✓ | standard translatable element |
| `<div>Hello</div>` | ✓ | auto-promoted, warning |
| `<TabsTrigger>Hello</TabsTrigger>` | ✓ | warning suggests `componentMap` |
| `<PageHeader title="Hi" />` | ✓ | `title` in the translatable-attributes set |
| `<MyComp description="Hi" />` | ✓ | `description` too |
| `<p>Click <a>here</a></p>` | ✓ | one block with `{=m0}` token |
| `<code>foo</code>` | ✗ | non-translatable element |
| `<h1 translate="no">X</h1>` | ✗ | explicit opt-out |
| `<button>{label}</button>` | ✗ | expression — use `t()` |
| `<div>{condition && 'Hi'}</div>` | ✗ | expression — use `t()` |

## Next

- [`t()` escape hatch](./t-escape-hatch) — for the JS-data-string pattern.
- [Plurals and select](./plurals-and-select) — count-aware and choice-based text.
- [Extract pipeline](./pipeline) — how the plugin ships this to translators.
