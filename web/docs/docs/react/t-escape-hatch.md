---
sidebar_position: 4
title: The t() escape hatch
sidebar_label: The t() escape hatch
---

# The `t()` escape hatch

Some strings don't live in JSX. A button-label array fed into a `.map`, an error message returned from a reducer, a tooltip stored in a ref — the extractor can't see strings hidden behind expressions.

Use `t()` to mark them for extraction without leaving the translator's flow.

## The pattern

```tsx
import { t } from "@neokapi/react/runtime";

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

At build time the plugin rewrites every `t("...")` call bound to `@neokapi/react/runtime` into a hash-based lookup:

```ts
// Input
t("English");

// Output (runtime mode)
__t("aB3xZ", "English");
```

In dev mode (plugin not active) `t` is a no-op that returns the source text verbatim, with `{name}` substitutions applied. So you can use it unconditionally — tests, SSR, storybook, dev server: all fine.

## Why a separate marker?

kapi-react's promise is zero wrappers for JSX. JS data structures are different — the extractor has no AST-level signal that `label: "English"` is a translatable string rather than an ID, an enum value, a CSS class, or anything else.

`t()` is the explicit "treat this as translatable" marker for that context. It's the minimum necessary handoff — one function call per string — and it keeps the JSX story wrapper-free.

## Parameters

```tsx
t("Hello, {name}!", { name: "Alice" });
// → "Hello, Alice!" in dev mode
// → translation with {name} substituted at runtime in production
```

Parameter syntax mirrors what the JSX extractor uses (`{name}`), so a translator editing an entry sees the same placeholder shape whether it came from JSX or `t()`.

## Context — disambiguating identical source strings

Some strings are spelled the same in English but mean different things. A CAT tool showing "State" out of nowhere gives a translator no way to know whether it means a US state, a workflow status, or a physics state.

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

Context only affects the hash at extract / transform time. It's stripped from the emitted `__t()` call and never ships to the runtime — the hash already encodes the disambiguation.

Context mirrors gettext's `msgctxt` for teams familiar with the pattern.

## Import-name tracking

The plugin only rewrites `t` identifiers bound to `@neokapi/react/runtime`. A local helper named `t` or a `t` imported from a different library is left alone:

```tsx
import { t } from "@neokapi/react/runtime";
import { t as styled } from "styled-components"; // ← unrelated

const Wrapper = styled.div`...`; // ← not rewritten

const label = t("Hello"); // ← rewritten to __t("hash", "Hello")
```

Aliases work too:

```tsx
import { t as tr } from "@neokapi/react/runtime";

const label = tr("Hello"); // ← rewritten
```

## Where the hash comes from

`t()` calls hash on a separate **channel** from JSX extraction:

```
hash = hashKey(text, "t\x1F")
```

So `t("Save")` and `<button>Save</button>` produce **different** hashes. That's intentional: the JSX call site has structural context (inside a button, inside a form, etc.) that a standalone string doesn't. A translator might want German "Speichern" for the button and "Gespeichert!" for a toast's `t("Saved")` — separating channels lets them diverge.

## Module-level `t()` gotcha

`t()` reads the active dictionary **at call time**. A module-level
const evaluates once, at import — typically _before_ the app has
finished calling `loadTranslations()`. The const freezes at the
fallback language forever:

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

**Why the `translate="no"`?** If the parent would be extractable on
its own (has static text, inline children, etc.), it'd wrap the
already-translated `meta.label` in a _second_ translation layer,
showing `▒ ▒ Utility ▒ ▒` in pseudo. `translate="no"` tells the
extractor the inner `t()` is the single source of truth for this
subtree. See [Writing components → Double-translation](./writing-components#double-translation-already-translated-values-inside-translatable-blocks).

## Ternary children with string literals

kapi-react treats the _whole_ `JSXExpressionContainer` as one
placeholder — it never looks inside a ternary at its branches:

```tsx
// ✗ Neither "Saving..." nor "Save" gets extracted.
<Button>{saving ? "Saving..." : "Save"}</Button>
```

Wrap each branch with `t()`:

```tsx
<Button>{saving ? t("Saving...") : t("Save")}</Button>
```

Template literals with static copy inside: same treatment.

```tsx
// ✗ template never extracts
<span>{count > 0 ? `Loading ${count}...` : "Idle"}</span>

// ✓ placeholder-aware t()
<span>
  {count > 0 ? t("Loading {count}...", { count }) : t("Idle")}
</span>
```

Purely-format templates (no alphabetic text: `` `${pct}%` ``,
`` `v${version}` ``) don't need `t()` — they're code-level
formatting, not UI copy — and the lint rule
[`no-ternary-literals-in-jsx-child`](./linting#no-ternary-literals-in-jsx-child)
knows not to flag them.

## When to use `t()` vs. refactor to JSX

Sometimes the cleanest fix is to hoist the string into JSX instead:

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

In prod (plugin active), `__t(hash, fallback, params)` does:

1. Look up `hash` in the loaded dict.
2. Resolve ICU plural / select forms if present.
3. Substitute `{name}` tokens.
4. Return the translated string (or the fallback if no entry).

In dev (plugin not active), `t(text, params)` does:

1. Substitute `{name}` tokens in the source text.
2. Return it.

Both return a `string`. No ReactNode result — for that you need the JSX path.

## ESLint / oxlint: keep `t()` honest

`t(someVariable)` defeats the point — the extractor has no text to hash. Install [`@neokapi/kapi-react-lint`](./linting) which ships rules for both ESLint and oxlint that catch this and the related pitfalls (`t('Hello ' + name)`, `<img alt={'Logo ' + brand} />`, string literals hidden in JSX expression containers).

## Next

- [Plurals and select](./plurals-and-select) — the other pattern where you need explicit markers (for the plural/case authoring components).
- [Pipeline](./pipeline) — how `t()` blocks flow through extract/translate/compile alongside JSX blocks.
