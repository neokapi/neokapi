---
sidebar_position: 3
title: Writing Translatable React Components
description: "How to write React components for neokapi-i18n extraction: what the plugin picks up automatically, which patterns require t(), how inline elements become paired markers, and what to avoid."
keywords: [React components, JSX extraction, translatable, neokapi-i18n, inline markers, componentMap, i18n patterns]
---

# Writing translatable components

Almost everything you already write is translatable. This page walks through the rules the plugin applies, the warnings it fires when it makes a judgement call you should know about, and the half-dozen patterns that break extraction silently.

## The short version

- **JSX text inside a translatable element** → extracted.
- **Direct text inside a container (`<div>`, `<section>`, …)** → extracted (auto-promotion, silent).
- **Direct text inside an unmapped React component** → extracted, with a warning and a suggestion to add a `componentMap` entry.
- **Inline elements with children** (`<strong>foo</strong>`, `<a href="…">here</a>`, `<em>{name}</em>`) → captured as one translatable block; the inline element becomes a **paired marker** wrapping its inner content, so the translator sees the inner words and can move the wrapping around.
- **Zero-children inline elements** (`<br/>`, `<Icon/>`, `<Spinner/>`, `<Badge/>`) → become **standalone markers** (`{=mN}` with no matching close) in the surrounding text.
- **HTML and ARIA text attributes** (`alt`, `title`, `placeholder`, `aria-label`, …) on **any** element → extracted, including on an inline child whose text the sentence around it already carries.
- **React prop-name conventions** (`label`, `description`, `heading`, `helpText`, `tooltip`, …) on **PascalCase components only** → extracted. On a plain `<div>` these names are usually DOM props or enum keys, not copy.
- **Translatable attributes with string-literal ternaries** (`title={cond ? "A" : "B"}`) → each branch extracted as its own block.
- **Code spans inside a sentence** (`<code>`, `<kbd>`, `<samp>`, `<var>`) → the sentence extracts as one block, the span becomes a paired marker, and its text is carried through verbatim.
- **A control inside a sentence** (`<button>`, `<label>`, `<select>`, `<img>`, …) → the sentence extracts as one block and the control becomes a paired marker, with its label translatable inside. A control that is its parent's only content keeps its own block.
- **JSX inside a conditional** (`{cond && <span>…</span>}`, a ternary, a `.map()`) → a standalone marker in the sentence, and the elements inside it are extracted and translated on their own.
- **Non-translatable elements on their own** (`<code>`, `<pre>`, `<kbd>`, `<var>`, `<script>`, `<style>`, `<textarea>`) → skipped.
- **Elements marked `translate="no"`** (or any ancestor) → skipped.

## The detail

### Translatable JSX text

Headings, paragraphs, buttons, labels, options, `<span>`, `<strong>`, `<em>`, `<a>`, `<b>`, `<i>`: the whole set of elements the W3C HTML5 spec classifies as phrasing or translatable block content.

```tsx
<h1>Welcome</h1>                        // ✓ extracted
<p>Ship in every language.</p>          // ✓ extracted
<button>Get started</button>            // ✓ extracted
<label>Email address</label>            // ✓ extracted
<a href="/docs">Read the docs</a>       // ✓ extracted
<option value="fr">French</option>      // ✓ extracted
```

### Inline children: one block, paired markers

When an element mixes text with inline children, the whole thing becomes one translatable block. Each inline element with children becomes a **paired marker** in the parent's text: the translator sees the inner words and can move the wrapping around:

```tsx
<p>
  Click <a href="/docs">here</a> to read the docs.
</p>
```

The extractor stores this as `"Click {=m0}here{/=m0} to read the docs."`. A German translation reads `"Klicken Sie {=m0}hier{/=m0}, um die Dokumentation zu lesen."`: the link wraps the right word, and a French translator can move it elsewhere in the sentence entirely.

Inline elements that produce paired markers: `<span>`, `<strong>`, `<em>`, `<b>`, `<i>`, `<a>`, `<small>`, `<sub>`, `<sup>`, `<time>`, `<u>`, `<wbr>`, `<del>`, `<ins>`, plus `<code>`, `<kbd>`, `<samp>` and `<var>`, whose text is carried through verbatim (see [Code spans inside prose](#code-spans-inside-prose)).

The form controls join that list where the sentence around them has words of its
own (see [Controls inside prose](#controls-inside-prose)).

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

Lots of real React UI looks like `<Button><Icon />Open File...</Button>`, an icon component followed by text. Empty inline elements (zero children) become a single standalone marker, leaving the surrounding text to extract normally:

```tsx
<Button>
  <FolderOpen size={12} />
  Open File...
</Button>
```

Extracts as `"{=m0} Open File..."` with `{=m0}` bound to the `<FolderOpen />` element (standalone, with no matching `{/=m0}` close). Works the same for Radix icons, lucide-react, Heroicons, custom `<Spinner />` components: anything with no children.

Unmapped React components with children are still treated as block-level by default (the warning suggests a `componentMap` entry; see "Unknown components" below). The narrow rule for _zero-children_ unmapped components prevents false positives on custom block-level components like `<Panel><Heading>…</Heading></Panel>`.

### Auto-promoted containers

Strict W3C semantics would skip `<div>Hello</div>`: divs are classified as containers rather than text. In real React codebases that's wrong: `<div>Label</div>`, `<section>Intro copy</section>` are everywhere.

neokapi-i18n **auto-promotes** container elements when they have:

1. At least one direct non-whitespace JSXText child, AND
2. Only inline children (no nested block-level elements).

Promotion is silent: `<div>Label</div>` is the dominant idiom and warning on every occurrence would just be noise. (Unmapped React components still warn; see below.)

To opt out: `<div translate="no">...</div>` or a rule:

```ts
neokapi({
  rules: [{ selector: ".hero-video-caption", translate: false }],
});
```

### Fragments

A fragment root with inline content extracts as one block, same as a promoted container:

```tsx
<>
  Signed in as <strong>{user.name}</strong>.
</>
```

The fragment has no tag of its own, so its descriptor is the literal `fragment`, meaning a fragment and a `<p>` carrying the same words hash apart, and moving content between them is a re-key. Prefer a real element when the content is a paragraph; fragments are for the cases where the surrounding markup genuinely can't take a wrapper.

### Unknown components

Component libraries like shadcn, Radix, MUI, and your own internal components render to HTML but neokapi-i18n can't know which one. By default, an unmapped React component with direct text content is extracted anyway, with a warning that suggests how to stabilise the hash:

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

**Why bother?** Because the hash is part of the translator's contract. If you later refactor by changing `TabsTrigger` → a different library's `Tab`, and the underlying HTML is still `button`, the hashes stay stable if you had the `componentMap` entry; translators don't need to re-review.

### Translatable attributes

Two buckets, with different scopes:

| Bucket            | Extracted on              | Names                                                                                                                                                              |
| ----------------- | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| HTML              | any element               | `alt`, `title`, `placeholder`                                                                                                                                      |
| ARIA              | any element               | `aria-label`, `aria-description`, `aria-placeholder`, `aria-roledescription`, `aria-valuetext`                                                                      |
| React conventions | **PascalCase components** | `subtitle`, `description`, `label`, `heading`, `caption`, `helpText`, `helperText`, `errorMessage`, `hint`, `tooltip`, `emptyMessage`, `emptyStateText`, `filterPlaceholder` |

The HTML and ARIA names are standardised: wherever they appear, they carry user-visible text. The convention names are not: `label` on a `<Field>` is copy, but `label` on a `<div>` is far more often a DOM prop, an enum key, or a data-binding field. Scoping that bucket to PascalCase components is what keeps `<div label="draft-pending">` out of your translator's queue.

So these all work out of the box:

```tsx
<input placeholder="Search..." aria-label="Search products" />
<img alt="Company logo" />
<button title="Save draft">💾</button>

<PageHeader title="Content Memory" subtitle="Approved terms and past translations" />
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

Mixed-shape ternaries (one literal, one computed, or both templates) _aren't_ statically extractable; the lint rule [`no-ternary-in-translatable-attr`](./linting#no-ternary-in-translatable-attr) flags them. Fix by wrapping both branches with `t()` so the t-call walker picks them up:

```tsx
// ✗ extractor can't see the template-literal branch
<Input placeholder={disabled ? `Disabled (${reason})` : "Enabled"} />

// ✓ both branches flow through the t() extraction path
<Input placeholder={disabled ? t("Disabled ({reason})", { reason }) : t("Enabled")} />
```

### Non-translatable elements

These render text-as-text, not natural language, so their contents never enter
the catalog:

`<code>`, `<pre>`, `<kbd>`, `<var>`, `<samp>`, `<script>`, `<style>`, `<textarea>`.

```tsx
<code>npm install @neokapi/i18n-react</code>    // ✗ not extracted
<pre>{licenseText}</pre>                        // ✗ not extracted
```

To flip one specific site: `<code translate="yes">...</code>`.

### Code spans inside prose

`<code>`, `<kbd>`, `<samp>` and `<var>` are also phrasing elements: a sentence
mentioning a flag or a format id is one sentence, and cutting it at the span
would leave the reader half a paragraph in their own language. So the parent
extracts as one block, the span becomes a paired marker like any other inline
element, and the text between the tags is marked protected:

```tsx
<p>
  Say <code>json</code> for the faithful readers.
</p>
```

The block reads `"Say {=m0}json{/=m0} for the faithful readers."`, and its runs
carry `json` with a do-not-translate flag. Translation, an AI pass and the `qps`
pseudo-locale all leave it alone, so the reader gets the bytes you wrote while
the prose around it is translated.

An element holding a code span and nothing else has no prose in it, so it stays
out of the catalog:

```tsx
<p>
  <code>kapi up</code>
</p>
// ✗ no block: the element holds an identifier only
```

`translate="yes"` on the span opts its text back in, inside a sentence or on its
own.

### Controls inside prose

HTML5 lets a control sit in the middle of a sentence, and a call to action is
usually written that way:

```tsx
<p>
  <button onClick={run}>Try it live</button> lists the registered formats.
</p>
```

The paragraph extracts as one block reading
`"{=m0}Try it live{/=m0} lists the registered formats."`. The button is a paired
marker, and the words inside it stay the translator's: a code span protects its
text, a control's label is prose. At render time the runtime clones the button
with the translated label as its children, so the handler, the type and the
classes are the ones you wrote.

The controls this covers are `<button>`, `<label>`, `<select>`, `<input>`,
`<output>`, `<img>`, `<audio>`, `<video>`, `<meter>` and `<progress>`.

Where the control is all its parent holds, the parent stays out and the control
keeps its own block:

```tsx
<div className="toolbar">
  <button>Save</button>                        // ✓ one block: "Save"
</div>

<div>
  Autosave is on. <button>Save now</button>    // ✓ one block for the sentence
</div>
```

That is what keeps a toolbar of buttons a message each, rather than one message
holding every label in the row.

### Attributes on an inline child

The words between an inline child's tags belong to the sentence around it. Its
attributes do not: the call site carries the element with its own props, so
copy in an `alt`, a `title`, an `aria-label`, a `placeholder` or a component's
`label` prop is a block of its own.

```tsx
<p>
  Use <abbr title="Content memory">CM</abbr> for that.
</p>
```

Two blocks: `"Use {=m0}CM{/=m0} for that."` and `"Content memory"`. The same
holds for a control that carries all its copy in an attribute, so an icon
button or an image keeps the prose around it:

```tsx
<p>
  Press <button aria-label="Run the demo"><Play /></button> to start.
</p>
// two blocks: "Press {=m0} to start." and "Run the demo"
```

The attribute's key reads `abbr[title]`, `button[aria-label]`, `Badge[label]`,
and so on. It names the element and the attribute and nothing else, so a
string keeps its key whether the element stands alone or sits in a sentence,
and wrapping prose around an existing element does not orphan its translation.
Nesting goes as deep as the markup does: `<a title="…"><img alt="…" /></a>`
inside a paragraph is three blocks. An element marked `translate="no"` keeps
its attributes out of the catalog along with its text.

### Conditional JSX inside a sentence

A conditional renders an element or nothing, so the sentence around it cannot
carry its words:

```tsx
<p>Saved {unsaved && <span className="badge">with changes pending</span>} just now</p>
```

The paragraph extracts as `"Saved {=m0} just now"` with `{=m0}` standing for the
whole conditional, and the badge extracts as a second block reading
`"with changes pending"`. The translator gets both, and the badge is rendered
through its own lookup, so a translation reaches the reader whether or not the
condition holds.

This covers every expression that can carry JSX: `&&`, `||`, `??`, a ternary
(each branch extracts separately), a `.map()` over a list, and a call taking an
element as an argument. It applies to attributes and `t()` calls inside the
conditional as well:

```tsx
<p>Saved {unsaved && <button aria-label="Discard changes"><X /></button>} just now</p>
```

The same holds for JSX passed in a prop, whether or not the element around it
extracts:

```tsx
<div actions={<Button>Publish</Button>}>Ready to go.</div>
// two blocks: "Ready to go." and "Publish"
```

What a conditional cannot rescue is a bare string literal in a branch
(`{cond ? "A" : "B"}`), which stays opaque; see
[Ternary with string literals as JSX children](#ternary-with-string-literals-as-jsx-children).

### Opting out with `translate="no"`

Standard HTML; it works on any element and its descendants:

```tsx
<h1 translate="no">API_KEY_PREFIX</h1>         // ✗ not extracted

<section translate="no">
  <h2>Debug payload</h2>                       // ✗ not extracted
  <pre>{json}</pre>                            // ✗ not extracted
</section>
```

Both the extractor and every lint rule in `@neokapi/i18n-react-lint` walk up the ancestor chain looking for `translate="no"`. A single marker at the top of a subtree silences everything inside; there is no need to sprinkle it on every element.

`translate="no"` is also the right answer when you're intentionally rendering an already-translated value (see "Module-level `t()` gotcha" below), or when your content is code-like and shouldn't be flagged as missing translation.

Inside a sentence, a marked child stays an island. Its text is kept out of the message and the element travels as one placeholder, so a translator sees the sentence around it and the reader sees the element exactly as you wrote it:

```tsx
<div>
  Saved to <span translate="no">{path}</span> just now
</div>
// message: Saved to {=m0} just now
```

A parent whose only text sits inside such a child carries no translatable text at all, so it stays out of the catalog:

```tsx
<SimpleTooltip content={full}>
  <span translate="no">
    {file}:{key}
  </span>
</SimpleTooltip>
// ✗ no message
```

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

The extractor can only see what it can statically reason about. These patterns slip through; each has a canonical fix.

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

neokapi-i18n treats the whole ternary as a single opaque placeholder; it never looks inside at the branches. Wrap each branch with `t()`:

```tsx
<Button>{saving ? t("Saving...") : t("Save")}</Button>
```

Caught by [`no-ternary-literals-in-jsx-child`](./linting#no-ternary-literals-in-jsx-child). Same fix applies to template literals with actual copy: `` `Loading ${n}...` `` → `t("Loading {n}...", { n })`.

### Module-level `t()` gotcha

`t()` reads the active dictionary **at call time**. A module-level const evaluates once, at import time, typically _before_ `loadTranslations()` has finished. The const freezes at the fallback language forever.

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

A subtle pattern that only shows up in pseudo. If you render a `t()`-resolved string as a child of an element the extractor also wraps as a block, pseudo-translation gets applied _twice_: the inner `t()` adds its markers, and the outer element's translation wraps around them:

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

Alternative: lift the whole string into a single `t()` call with placeholders, but that's awkward when one half is a translated label and the other is a numeric count.

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
| `<h1>Hello</h1>`                          | yes        | standard translatable element                                            |
| `<div>Hello</div>`                        | yes        | auto-promoted silently                                                   |
| `<>Hello <b>you</b></>`                   | yes        | fragment root; descriptor is `fragment`                                  |
| `<Button><Icon/>Save</Button>`            | yes        | "Save" extracts with `{=m0}` standalone for the icon                     |
| `<TabsTrigger>Hello</TabsTrigger>`        | yes        | warning suggests `componentMap`                                          |
| `<PageHeader title="Hi" />`               | yes        | `title` is an HTML attribute; any element                                |
| `<PageHeader title={cond ? "A" : "B"} />` | yes        | both branches; one block each                                            |
| `<MyComp description="Hi" />`             | yes        | `description` is a convention prop; components only                      |
| `<div label="draft-pending" />`           | no         | convention prop on a plain element; not copy                             |
| `<p>Click <a>here</a></p>`                | yes        | one block, `<a>` becomes paired `{=m0}…{/=m0}`                           |
| `<code>foo</code>`                        | no         | non-translatable element                                                 |
| `<p>Say <code>json</code> now</p>`        | yes        | one block; `json` carried through as protected text                      |
| `<p>Use <abbr title="X">CM</abbr> ok</p>` | yes        | two blocks: the sentence, and the `title`                                |
| `<h1 translate="no">X</h1>`               | no         | explicit opt-out (suppresses lint too)                                   |
| `<button>{label}</button>`                | no         | bare expression; use `t()` on the source                                 |
| `<button>{obj.label}</button>`            | no         | flagged by `prefer-t-for-label-expr`; wrap the source                    |
| `<button>{cond ? "A" : "B"}</button>`     | no         | flagged by `no-ternary-literals-in-jsx-child`; wrap branches with `t()`  |
| `<div>{cond && 'Hi'}</div>`               | no         | expression; use `t()`                                                    |
| `<p>Saved {cond && <b>a note</b>}</p>`    | yes        | two blocks: the sentence, and the conditional's own element              |
| `<div actions={<Button>Go</Button>}>Hi</div>` | yes    | two blocks: JSX in a prop extracts as well                               |

## Next

- [`t()` escape hatch](./t-escape-hatch): for the JS-data-string and ternary-in-JSX patterns.
- [Plurals and select](./plurals-and-select): count-aware and choice-based text.
- [Linting](./linting): editor squigglies for every anti-pattern on this page.
- [Extract pipeline](./pipeline): how the plugin ships this to translators.
