---
sidebar_position: 10
title: kapi-react Linting
description: kapi-react-lint provides ESLint and oxlint rules that catch unextractable strings, unsafe patterns, and authoring mistakes before they reach the translator — with CI enforcement via --strict.
keywords: [linting, kapi-react-lint, ESLint, oxlint, i18n lint rules, extraction, CI enforcement]
---

# Linting

kapi-react's build-time transform catches a lot, but some authoring mistakes only show up _after_ extraction (a `t(variable)` that can't be extracted, a label string hidden in a data array that the extractor never walks, a ternary that smuggles two literals past the JSX walker). `@neokapi/kapi-react-lint` gives you editor squigglies for those cases.

The same rule objects work under **ESLint** and **oxlint** — oxlint's plugin API is ESLint v9 compatible, so you install one plugin and wire it into whichever linter you already use. Oxlint is recommended for speed (typically 100–200ms on a few hundred files).

## The three layers

| Layer               | When it runs                 | What catches                                                                                                                           | How loud                                                                   |
| ------------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| **Lint rules**      | editor / `oxlint` / `eslint` | single-file authoring mistakes (unextractable `t()` calls, concat in translatable attrs, ternaries smuggling literals past extraction) | per-rule severity in your config                                           |
| **Plugin warnings** | build-time transform         | cross-cutting issues that need config context (unmapped components → componentMap, ternary attrs the extractor can't resolve)          | `console.warn` by default                                                  |
| **Enforcement**     | CI                           | both of the above, promoted to errors                                                                                                  | `--strict` on the extract CLI or `warningsAsErrors: true` in plugin config |

Keep the loudest layer (enforcement) off in day-to-day authoring, and turn it on in CI once the codebase is clean.

## Install

```bash
vp install -D @neokapi/kapi-react-lint
```

## Oxlint

Add to `.oxlintrc.json`:

```json
{
  "jsPlugins": ["@neokapi/kapi-react-lint/oxlint"],
  "rules": {
    "kapi-react/t-literal-first-arg": "error",
    "kapi-react/t-no-concat": "error",
    "kapi-react/no-concat-in-translatable-attr": "error",
    "kapi-react/no-ternary-in-translatable-attr": "error",
    "kapi-react/no-ternary-literals-in-jsx-child": "error",
    "kapi-react/no-string-literal-jsx-expr": "warn",
    "kapi-react/prefer-t-for-label-expr": "warn"
  },
  "overrides": [
    {
      "files": ["src/stories/**"],
      "rules": {
        "kapi-react/no-ternary-literals-in-jsx-child": "off",
        "kapi-react/prefer-t-for-label-expr": "off"
      }
    }
  ]
}
```

The `overrides` block disables the two higher-FP rules for Storybook fixture files, where demo strings don't warrant the same rigor.

## ESLint (flat config)

```js title="eslint.config.js"
import { recommended } from "@neokapi/kapi-react-lint/eslint";

export default [
  {
    files: ["**/*.{ts,tsx,js,jsx}"],
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: "module",
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
  },
  recommended,
];
```

The shareable configs are `recommended` (safe defaults — the five core rules at `error`, the two label rules at `warn`, `prefer-t-for-label-props` off) and `recommendedStrict` (everything at `error`, including `prefer-t-for-label-props` and `prefer-t-for-label-expr`).

## The W3C `translate="no"` escape hatch

All rules in this package respect `translate="no"` on the element itself **or any JSX ancestor**. The kapi-react extractor already honours it; the lint rules match those semantics.

```tsx
// Rule fires — {meta.label} looks like user-visible copy.
<h1>{meta.label}</h1>

// Rule silent — author explicitly marked the subtree as non-translatable.
<h1 translate="no">{meta.label}</h1>

// Rule silent — an ancestor opted out, so the whole subtree is quiet.
<section translate="no">
  <div>
    <h1>{meta.label}</h1>
  </div>
</section>
```

Use it for data that's legitimately dynamic — backend identifiers, file paths, version strings, user-provided names — without polluting the lint config with per-line disables.

## Rules

### `t-literal-first-arg`

Flags `t(variable)` / `t(getLabel())` / `t(cond ? 'A' : 'B')`. The extractor reads the first argument of `t()` statically at build time; anything that isn't a literal produces nothing to translate.

```tsx
// ✓ fine
t("Sign in");
t("Sign in", "Button label"); // with context

// ✗ not extractable
t(label);
t(labels[key]);
t(ok ? "Save" : "Cancel");
```

### `t-no-concat`

Flags `t('Hello ' + name)` and ``t(`Hello ${name}`)`` — neither extracts because the full string isn't visible at build time. Use a placeholder pattern instead.

```tsx
// ✗ broken
t("Welcome " + user.name);
t(`You have ${count} messages`);

// ✓ extractable, rendered via runtime substitution
t("Welcome {name}", { name: user.name });
// or use <Plural>/<Select> for pluralisation
```

### `no-concat-in-translatable-attr`

Any attribute in kapi-react's translatable-attribute set (`alt`, `title`, `placeholder`, `aria-label`, `label`, `description`, `helpText`, …) must be a string literal or a literal with placeholders — not a runtime concat.

```tsx
// ✗ alt won't extract
<img alt={'Logo ' + brand} />

// ✓ if you need dynamic parts, compute via t() and pass the result
<img alt={t('Logo for {brand}', { brand })} />
```

### `no-ternary-in-translatable-attr`

Sibling of `no-concat-in-translatable-attr`. Flags translatable attributes whose value is a ternary with at least one _non-string-literal_ branch. The all-string-literal case (`title={cond ? "A" : "B"}`) is extracted by the kapi-react walker as two blocks — no warning. The mixed case is unextractable.

```tsx
// ✓ both branches are string literals — extractor handles them.
<PageHeader title={isProjectMode ? "Project Flows" : "Flows"} />

// ✓ both branches are t() calls — the t-call walker handles them.
<Input placeholder={disabled ? t("Off") : t("On")} />

// ✗ one literal, one computed — the computed branch silently bypasses translation.
<Input placeholder={disabled ? getLabel() : "Type here…"} />
```

Fix by wrapping the computed branch with `t()` too, or by lifting the logic so both branches resolve to string literals.

### `no-ternary-literals-in-jsx-child`

Catches the JSX-children counterpart of the attribute rule:

```tsx
// ✗ neither literal gets extracted — the extractor treats the
// whole conditional as a single opaque placeholder.
<Button>{loading ? "Saving..." : "Save"}</Button>
```

Why this slips through everything else: kapi-react's walker sees one `JSXExpressionContainer` and emits one `jsx:var` placeholder for it. It never looks inside at the branches — `"Saving..."` and `"Save"` are both invisible to extraction.

Fix with `t()`:

```tsx
// ✓ each branch extracts as its own block; the branch's value flows through
// the button's `__tx` call at render time.
<Button>{loading ? t("Saving...") : t("Save")}</Button>
```

Variants the rule handles cleanly:

- Both branches string literals → flagged (either/both lost).
- One string literal, one `t()` call → flagged (the literal branch is lost).
- Both `t()` calls → **not** flagged (goes through the t-call path).
- Template literals with alphabetic text (`` `Loading ${n}...` ``) → flagged.
- Format-only templates with no alphabetic quasi (`` `${pct}%` ``, `` `v${version}` ``) → **not** flagged (code-level formatting, not UI copy).

### `no-string-literal-jsx-expr`

`<p>{'Hello'}</p>` — a bare string literal wrapped in an expression container. Looks extractable but isn't: the transform walks JSX text nodes, not expression containers that happen to hold a string. Auto-fixes to `<p>Hello</p>`.

### `prefer-t-for-label-expr`

The render-side companion to `prefer-t-for-label-props` below. Flags `{obj.label}` / `{item.title}` / `{entry.caption}` rendered as JSX text:

```tsx
// ✗ `meta.label` looks user-visible; the extractor can't see the string
// it will resolve to at runtime.
<h1>{meta.label}</h1>;

// ✓ wrap the source data so the literal is visible to extraction
const categoryMeta = {
  utility: { label: t("Utility") },
  // …
};
```

Only fires on a narrow set of property names that _almost always_ name user-visible copy: `label`, `title`, `heading`, `caption`, `subtitle`, `tooltip`, `placeholder`, `summary`. Deliberately excludes `.name`, `.description`, `.text`, `.message` — those overwhelmingly name backend / runtime data in real React apps and would create too much noise.

Customise via the `keys` option:

```json
{ "rules": { "kapi-react/prefer-t-for-label-expr": ["warn", { "keys": ["label", "cta"] }] } }
```

Suppress false positives on a specific element with `translate="no"`:

```tsx
// file.name is an OS path, not UI copy
<option value={f.path} translate="no">
  {f.name}
</option>
```

### `prefer-t-for-label-props`

The classic "label hidden in a data array" pattern — the _declaration side_ of the same idea:

```tsx
// ✗ 'System' never gets extracted
const THEMES = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
];
return THEMES.map(({ value, label }) => <button>{label}</button>);

// ✓ the literals are now visible to extraction
const THEMES = [
  { value: "system", label: t("System") },
  { value: "light", label: t("Light") },
];
```

Only in the `recommendedStrict` preset by default because it can fire on internal-only data arrays. Same narrow key list as `prefer-t-for-label-expr`. Turn on individually:

```json
{ "rules": { "kapi-react/prefer-t-for-label-props": "error" } }
```

### Module-level `t()` gotcha

All `t()`-wrapping fixes above assume the calls happen **per render**. A module-level const freezes each `t()` call at whatever the dict said when the module first loaded — typically the fallback language, because translations load _after_ the initial import.

```tsx
// ✗ Frozen at load time. "Utility" will still say "Utility" in pseudo.
const categoryMeta = {
  utility: { label: t("Utility") },
};

// ✓ Per-render: each invocation picks up the current dict.
function categoryMeta(cat: string) {
  switch (cat) {
    case "utility":
      return { label: t("Utility") };
    // …
  }
}
```

Wrap non-trivial lookup tables in a function that returns fresh values per render. See [The `t()` escape hatch → Module-level gotcha](./t-escape-hatch#module-level-t-gotcha) for more.

## CI enforcement

Two ways to fail the build on warnings:

**Lint step:**

```bash
vp lint                  # or: oxlint / eslint
```

Non-zero exit when any rule at severity `error` fires. Wire it alongside your typecheck step in CI (or run it from a Git pre-commit hook).

**Extract CLI:**

```bash
vpx kapi-react extract --strict
```

Exits non-zero if the extractor recorded any warning (`unknown-component`, `ternary-attr-complex`, `dyn-label-splice`). Good for catching authoring issues the lint rules can't see from a single file.

**Plugin (build-time):**

```ts title="vite.config.ts"
import neokapi from "@neokapi/kapi-react/vite";

export default {
  plugins: [neokapi({ warningsAsErrors: process.env.CI === "true" })],
};
```

Promotes transform-side warnings (unknown-component, etc.) to thrown build errors. Use `process.env.CI` to keep local dev ergonomic.

## Excluding fixture code

Stories, mocks, and fixtures don't usually warrant the same i18n rigor as shipped components. Two complementary ways to exclude them:

**From lint** — `.oxlintrc.json` `overrides` block (see above).

**From extraction** — `--ignore` flag:

```json title="package.json"
{
  "scripts": {
    "extract": "vpx kapi-react extract --out i18n/ --ignore 'src/stories/**' --ignore '**/*.test.tsx'"
  }
}
```

The flag is repeatable and passed through to Node's `fs/promises.glob` `exclude` option.

## Follow-ups

These rules are planned but not yet shipped — they need TypeScript type information or cross-file analysis that a simple ESLint rule can't do on its own:

- `translatable-attr-expects-string` — catch `<PageHeader title={x} />` where `x` is `ReactNode`, not `string`
- `unmapped-component-in-editor` — mirror the plugin's `unknown-component` warning in the editor
- `unused-componentmap-entry` — flag componentMap keys that no source file references

Track progress on [issue #381](https://github.com/neokapi/neokapi/issues/381).
