---
sidebar_position: 4
title: The t() Escape Hatch
sidebar_label: The t() escape hatch
description: "Use the t() function in neokapi-i18n to mark strings for extraction when they live outside JSX: in arrays, reducers, refs, or any expression the plugin cannot walk statically."
keywords: [t(), escape hatch, extraction, non-JSX strings, neokapi-i18n, i18n, React]
---

# The `t()` escape hatch

`t()` marks a string for extraction when it lives outside JSX: a button-label array fed into a `.map`, an error message returned from a reducer, a tooltip stored in a ref. The extractor can't see a string behind an expression, and `t()` gives it a block with placeholders and context, like any JSX text.

## The pattern

```tsx
import { t } from "@neokapi/i18n-react/runtime";

const UI_LANGUAGES = [
  { value: "en", label: t("English") },
  { value: "qps", label: t("Pseudo English (qps)") },
];

const THEMES = [
  { value: "system", icon: Monitor, label: t("System") },
  { value: "light", icon: Sun, label: t("Light") },
  { value: "dark", icon: Moon, label: t("Dark") },
];

function greet(user: User) {
  return t("Hello, {name}!", { name: user.displayName });
}
```

At build time the plugin rewrites every `t("...")` call bound to `@neokapi/i18n-react/runtime` into a hash-based lookup:

```ts
// Input
t("English");

// Output (runtime mode)
__t("aB3xZ", "English");
```

In dev mode (plugin not active) `t` returns the source text verbatim, with `{name}` substitutions applied. You can call it anywhere: tests, SSR, Storybook, the dev server.

## Why a separate marker?

JSX needs no wrappers, because the element tells the extractor what it holds. In a JS data structure, nothing in the syntax marks `label: "English"` as copy; it could as well be an ID, an enum value or a CSS class.

`t()` marks it as copy, with one function call per string, and JSX stays free of wrappers.

## Parameters

```tsx
t("Hello, {name}!", { name: "Alice" });
// → "Hello, Alice!" in dev mode
// → translation with {name} substituted at runtime in production
```

Parameter syntax mirrors what the JSX extractor uses (`{name}`), so a translator sees the same placeholder shape in a string from JSX and in one from `t()`.

## Context: disambiguating identical source strings

Some strings are spelled the same in English but mean different things. Shown "State" alone in a CAT tool, a translator can't tell whether it means a US state, a workflow status or a physics state.

Pass a positional context as the second argument:

```tsx
t("State", "US state"); // → address form field
t("State", "workflow status"); // → task lifecycle
t("State", "physics lecture"); // → h / cold / gas / plasma
```

Each of those is a **separate block** with a different hash, so translators can give each one its own target string.

With params, context comes first:

```tsx
t("Hello, {name}!", "greeting", { name: user.name });
```

Context only affects the hash at extract / transform time. It's stripped from the emitted `__t()` call and never ships to the runtime; the hash already encodes the disambiguation.

Context works like gettext's `msgctxt`.

## Import-name tracking

The plugin only rewrites `t` identifiers bound to `@neokapi/i18n-react/runtime`. A local helper named `t` or a `t` imported from a different library is left alone:

```tsx
import { t } from "@neokapi/i18n-react/runtime";
import { t as styled } from "styled-components"; // ← unrelated

const Wrapper = styled.div`...`; // ← not rewritten

const label = t("Hello"); // ← rewritten to __t("hash", "Hello")
```

Aliases work too:

```tsx
import { t as tr } from "@neokapi/i18n-react/runtime";

const label = tr("Hello"); // ← rewritten
```

## Where the hash comes from

`t()` calls hash on a separate **channel** from JSX extraction:

```
hash = hashKey(text, "t\x1F" + context)   // context is "" when you pass none
```

A JSX block's descriptor is its element (`"button"`, `"p"`); a `t()` call's is the literal `t`. So `t("Save")` and `<button>Save</button>` produce **different** hashes. They are different surfaces, and a translator may want German "Speichern" for the button and "Gespeichert!" for a toast's `t("Saved")`. Two `t()` calls with the same text collapse to one key unless you distinguish them with the second argument (`t("Open", "verb")` vs `t("Open", "adjective")`), the same `msgctxt` model gettext uses.

## Module-level `t()` gotcha

`t()` reads the active dictionary **at call time**. A module-level
const evaluates once, at import, typically _before_ the app has
finished calling `loadTranslations()`, so it keeps the
fallback-language text for the life of the page:

```tsx
// ✗ Frozen at load time. "Utility" still says "Utility" in pseudo.
const categoryMeta = {
  utility: { label: t("Utility") },
  pipeline: { label: t("Pipeline") },
};
```

Fix: wrap the lookup in a function that runs per render. Each call
reads the current dict:

```tsx
// ✓ Per-render resolution.
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
  return <span translate="no">{meta.label}</span>;
  //           ^ prevents double-wrap; see below.
}
```

**Why the `translate="no"`?** If the parent would extract on its own
(it has static text, inline children and so on), it would wrap the
already-translated `meta.label` in a _second_ translation layer,
showing `▒ ▒ Utility ▒ ▒` in pseudo. `translate="no"` tells the
extractor that the inner `t()` owns the translation for this subtree. See [Writing components → Double-translation](./writing-components#double-translation-already-translated-values-inside-translatable-blocks).

## Ternary children with string literals

A plain string literal in a branch of a JSX-children conditional
extracts on its own, keyed by the branch's slot, so it needs no `t()`:

```tsx
// ✓ "Saving..." and "Save" each extract as a block.
<Button>{saving ? "Saving..." : "Save"}</Button>
```

[Literals in a conditional's branches](./writing-components#literals-in-a-conditionals-branches)
describes the slot keys.

A template literal with static copy inside is one expression, and
extraction reads it whole. Wrap that branch with `t()`:

```tsx
// ✗ template never extracts
<span>{count > 0 ? `Loading ${count}...` : "Idle"}</span>

// ✓ placeholder-aware t()
<span>
  {count > 0 ? t("Loading {count}...", { count }) : t("Idle")}
</span>
```

Wrapping `"Idle"` in `t()` is optional, because the plain literal
extracts on its own. A format-only template with no alphabetic text,
such as `` `${pct}%` `` or `` `v${version}` ``, is code-level
formatting and needs no `t()`. The lint rule
[`no-ternary-literals-in-jsx-child`](./linting#no-ternary-literals-in-jsx-child)
does not flag it.

## When to use `t()` vs. refactor to JSX

Sometimes it is clearer to move the string into JSX:

```tsx
// Data-driven, needs t()
const THEMES = [
  { value: "system", label: t("System") },
  { value: "light",  label: t("Light") },
];

// Unrolled, no t() needed
<button onClick={() => setTheme("system")}>System</button>
<button onClick={() => setTheme("light")}>Light</button>
```

Heuristics:

- **3 items or fewer, and the render is a simple `.map`** → unrolling is usually clearer and removes the `t()` calls.
- **Data lives in a module other than the one rendering it, or is assembled dynamically** → use `t()`.
- **The data already carries non-string metadata (icons, callbacks, IDs)** → keep it as data, use `t()` for labels.

## Runtime fallback behaviour

In production (plugin active), `__t(hash, fallback, params)` does:

1. Look up `hash` in the loaded dict.
2. Resolve ICU plural / select forms if present.
3. Substitute `{name}` tokens.
4. Return the translated string (or the fallback if no entry).

In dev (plugin not active), `t(text, params)` does:

1. Substitute `{name}` tokens in the source text.
2. Return it.

Both return a `string`. For a ReactNode result, write the text as JSX.

## ESLint / oxlint: validate `t()` calls {#eslint--oxlint-keep-t-honest}

The extractor cannot hash the runtime value in `t(someVariable)`. [`@neokapi/i18n-react-lint`](./linting) has rules for ESLint and oxlint that catch this and related patterns: `t('Hello ' + name)`, `<img alt={'Logo ' + brand} />`, and string literals hidden in JSX expression containers.

## Related

- [Plurals and select](./plurals-and-select): count-aware and choice-based text, written with authoring components.
- [Pipeline](./pipeline): how `t()` blocks flow through extract/translate/compile alongside JSX blocks.
