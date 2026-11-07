---
sidebar_position: 10
title: neokapi-i18n Linting
description: "i18n-react-lint provides ESLint and oxlint rules that catch unextractable strings, unsafe patterns, and authoring mistakes before they reach the translator, with CI enforcement via --strict."
keywords: [linting, i18n-react-lint, ESLint, oxlint, i18n lint rules, extraction, CI enforcement]
---

# Linting

`@neokapi/i18n-react-lint` marks, in your editor, the code whose text the extractor can't see: a `t(variable)` call, a label string in a data array the extractor never walks, a template literal in a ternary branch. Without it, these surface only after extraction, as English strings in a translated build.

The same rule objects work under **ESLint** and **oxlint**. oxlint's plugin API is ESLint v9 compatible, so you install one plugin and add it to the linter you already use. Oxlint is the faster choice, typically 100–200ms on a few hundred files.

## The three layers

| Layer               | When it runs                 | What it catches                                                                                                                        | How loud                                                                   |
| ------------------- | ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| **Lint rules**      | editor / `oxlint` / `eslint` | single-file authoring mistakes (unextractable `t()` calls, concat in translatable attrs, template literals in ternary branches) | per-rule severity in your config                                           |
| **Plugin warnings** | build-time transform         | cross-cutting issues that need config context (unmapped components → componentMap, ternary attrs the extractor can't resolve)          | `console.warn` by default                                                  |
| **Enforcement**     | CI                           | both of the above, promoted to errors                                                                                                  | `--strict` on the extract CLI or `warningsAsErrors: true` in plugin config |

Keep the loudest layer (enforcement) off in day-to-day authoring, and turn it on in CI once the codebase is clean.

## Install

```bash
npm install -D @neokapi/i18n-react-lint
```

## Oxlint

Add to `.oxlintrc.json`:

```json
{
  "jsPlugins": ["@neokapi/i18n-react-lint/oxlint"],
  "rules": {
    "neokapi-i18n/t-literal-first-arg": "error",
    "neokapi-i18n/t-no-concat": "error",
    "neokapi-i18n/no-concat-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-literals-in-jsx-child": "error",
    "neokapi-i18n/no-string-literal-jsx-expr": "warn",
    "neokapi-i18n/prefer-t-for-label-expr": "warn"
  },
  "overrides": [
    {
      "files": ["src/stories/**"],
      "rules": {
        "neokapi-i18n/no-ternary-literals-in-jsx-child": "off",
        "neokapi-i18n/prefer-t-for-label-expr": "off"
      }
    }
  ]
}
```

The `overrides` block turns off the two rules most prone to false positives for Storybook fixture files, whose demo strings don't need translating.

## ESLint (flat config)

```js title="eslint.config.js"
import { recommended } from "@neokapi/i18n-react-lint/eslint";

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

The shareable configs are `recommended` (safe defaults: the five core rules at `error`, the two label rules at `warn`, `prefer-t-for-label-props` off) and `recommendedStrict` (everything at `error`, including `prefer-t-for-label-props` and `prefer-t-for-label-expr`).

## The W3C `translate="no"` escape hatch

Every rule in this package respects `translate="no"` on the element itself **or any JSX ancestor**, the same way the extractor does.

```tsx
// Rule fires: {meta.label} looks like user-visible copy.
<h1>{meta.label}</h1>

// Rule silent: author explicitly marked the subtree as non-translatable.
<h1 translate="no">{meta.label}</h1>

// Rule silent: an ancestor opted out, so the whole subtree is quiet.
<section translate="no">
  <div>
    <h1>{meta.label}</h1>
  </div>
</section>
```

Use it for data that is dynamic by nature (backend identifiers, file paths, version strings, user-provided names), so the lint config needs no per-line disables.

## Rules

### `t-literal-first-arg`

Flags `t(variable)` / `t(getLabel())` / `t(cond ? 'A' : 'B')`. The extractor reads the first argument of `t()` statically at build time, and any expression other than a literal produces nothing to translate.

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

Flags `t('Hello ' + name)` and ``t(`Hello ${name}`)``. Neither extracts, because the full string is unknown at build time. Use a placeholder instead.

```tsx
// ✗ broken
t("Welcome " + user.name);
t(`You have ${count} messages`);

// ✓ extractable, rendered via runtime substitution
t("Welcome {name}", { name: user.name });
// or use <Plural>/<Select> for pluralisation
```

### `no-concat-in-translatable-attr`

Any attribute in neokapi-i18n's translatable-attribute set (`alt`, `title`, `placeholder`, `aria-label`, `label`, `description`, `helpText`, …) must be a string literal or a literal with placeholders; a runtime concat is unextractable.

```tsx
// ✗ alt won't extract
<img alt={'Logo ' + brand} />

// ✓ if you need dynamic parts, compute via t() and pass the result
<img alt={t('Logo for {brand}', { brand })} />
```

### `no-ternary-in-translatable-attr`

Companion to `no-concat-in-translatable-attr`. Flags translatable attributes whose value is a ternary with at least one _non-string-literal_ branch. The all-string-literal case (`title={cond ? "A" : "B"}`) is extracted by the neokapi-i18n walker as two blocks (no warning). The mixed case is unextractable.

```tsx
// ✓ both branches are string literals; the extractor handles them.
<PageHeader title={isProjectMode ? "Project Flows" : "Flows"} />

// ✓ both branches are t() calls; the t-call walker handles them.
<Input placeholder={disabled ? t("Off") : t("On")} />

// ✗ one literal, one computed: the computed branch silently bypasses translation.
<Input placeholder={disabled ? getLabel() : "Type here…"} />
```

Fix by wrapping the computed branch with `t()` too, or by lifting the logic so both branches resolve to string literals.

### `no-ternary-literals-in-jsx-child`

Catches a template literal in the branch of a JSX-children ternary:

```tsx
// ✗ the words inside the template never extract
<span>{count > 0 ? `Loading ${count}...` : "Idle"}</span>
```

A template literal is one expression, so extraction reads it whole and the words in it stay English.

Fix with `t()`, passing the interpolation as a parameter:

```tsx
// ✓ the t() call extracts as its own block, and the runtime substitutes count
<span>{count > 0 ? t("Loading {count}...", { count }) : "Idle"}</span>
```

Variants the rule handles cleanly:

- A template literal with alphabetic text (`` `Loading ${n}...` ``) → flagged, in either branch.
- Format-only templates with no alphabetic quasi (`` `${pct}%` ``, `` `v${version}` ``) → **not** flagged (code-level formatting, not UI copy).
- A plain string literal (`"Save"`) → **not** flagged. It extracts as its own block, keyed by the branch's slot; see [Literals in a conditional's branches](./writing-components#literals-in-a-conditionals-branches).
- A `t()` call, an element or a computed value → **not** flagged.

### `no-string-literal-jsx-expr`

`<p>{'Hello'}</p>`: a bare string literal wrapped in an expression container. It looks extractable, but the extractor reads JSX text, and treats an expression container as a placeholder even when it holds a string. The rule auto-fixes it to `<p>Hello</p>`.

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

It fires only on property names that _almost always_ hold user-visible copy: `label`, `title`, `heading`, `caption`, `subtitle`, `tooltip`, `placeholder`, `summary`. It skips `.name`, `.description`, `.text` and `.message`, which in React apps mostly hold backend or runtime data.

Customise via the `keys` option:

```json
{ "rules": { "neokapi-i18n/prefer-t-for-label-expr": ["warn", { "keys": ["label", "cta"] }] } }
```

Suppress false positives on a specific element with `translate="no"`:

```tsx
// file.name is an OS path, not UI copy
<option value={f.path} translate="no">
  {f.name}
</option>
```

### `prefer-t-for-label-props`

The _declaration side_ of the same pattern: a label string in a data array.

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

It is on only in the `recommendedStrict` preset, because it can fire on data arrays that never reach the UI. It uses the same key list as `prefer-t-for-label-expr`. To turn it on by itself:

```json
{ "rules": { "neokapi-i18n/prefer-t-for-label-props": "error" } }
```

### Module-level `t()` gotcha

All `t()`-wrapping fixes above assume the calls happen **per render**. A module-level const freezes each `t()` call at whatever the dict said when the module first loaded, typically the fallback language, because translations load _after_ the initial import.

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

Three places can fail the build on warnings:

**Lint step:**

```bash
vp lint                  # or: oxlint / eslint
```

Non-zero exit when any rule at severity `error` fires. Wire it alongside your typecheck step in CI (or run it from a Git pre-commit hook).

**Extract CLI:**

```bash
vp neokapi-i18n extract --strict
```

Exits non-zero if the extractor recorded any warning (`unknown-component`, `ternary-attr-complex`, `dyn-label-splice`). This catches authoring issues that a single-file lint rule can't see.

**Plugin (build-time):**

```ts title="vite.config.ts"
import neokapi from "@neokapi/i18n-react/vite";

export default {
  plugins: [neokapi({ warningsAsErrors: process.env.CI === "true" })],
};
```

Promotes transform-side warnings (unknown-component, etc.) to thrown build errors. Gating on `process.env.CI` keeps local builds at warnings.

## Excluding fixture code

Stories, mocks and fixtures rarely need translating. Exclude them in two places:

**From lint**: the `.oxlintrc.json` `overrides` block (see above).

**From extraction**: the `--ignore` flag:

```json title="package.json"
{
  "scripts": {
    "extract": "vp neokapi-i18n extract --out i18n/ --ignore 'src/stories/**' --ignore '**/*.test.tsx'"
  }
}
```

The flag is repeatable, and the CLI passes it to Node's `fs/promises.glob` `exclude` option.

## Related

- [Writing translatable components](./writing-components): the extraction rules the lint rules follow.
- [The `t()` escape hatch](./t-escape-hatch): the fix most rules point to.
- [Configuration](./configuration): `warningsAsErrors`, `strict` and `onWarning`.
