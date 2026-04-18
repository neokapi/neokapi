---
sidebar_position: 9
title: Linting
---

# Linting

kapi-react's build-time transform catches a lot, but some authoring mistakes only show up *after* extraction (a `t(variable)` that can't be extracted, a label string hidden in a data array that the extractor never walks). `@neokapi/kapi-react-lint` gives you editor squigglies for those cases.

The same rule objects work under **ESLint** and **oxlint** — oxlint's plugin API is ESLint v9 compatible, so you install one plugin and wire it into whichever linter you already use.

## The three layers

| Layer | When it runs | What catches | How loud |
|---|---|---|---|
| **Lint rules** | editor / `oxlint` / `eslint` | single-file authoring mistakes (unextractable `t()` calls, concat in translatable attrs) | per-rule severity in your config |
| **Plugin warnings** | build-time transform | cross-cutting issues that need config context (unmapped components → componentMap) | `console.warn` by default |
| **Enforcement** | CI | both of the above, promoted to errors | `--strict` on the extract CLI or `warningsAsErrors: true` in plugin config |

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
    "kapi-react/no-string-literal-jsx-expr": "warn"
  }
}
```

## ESLint (flat config)

```js title="eslint.config.js"
import { recommended } from '@neokapi/kapi-react-lint/eslint';

export default [
  {
    files: ['**/*.{ts,tsx,js,jsx}'],
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: 'module',
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
  },
  recommended,
];
```

The shareable configs are `recommended` (safe defaults) and `recommendedStrict` (everything as `error`, including the higher-FP `prefer-t-for-label-props`).

## Rules

### `t-literal-first-arg`

Flags `t(variable)` / `t(getLabel())` / `t(cond ? 'A' : 'B')`. The extractor reads the first argument of `t()` statically at build time; anything that isn't a literal produces nothing to translate.

```tsx
// ✓ fine
t('Sign in');
t('Sign in', 'Button label');  // with context

// ✗ not extractable
t(label);
t(labels[key]);
t(ok ? 'Save' : 'Cancel');
```

### `t-no-concat`

Flags `t('Hello ' + name)` and `` t(`Hello ${name}`) `` — neither extracts because the full string isn't visible at build time. Use a placeholder pattern instead.

```tsx
// ✗ broken
t('Welcome ' + user.name);
t(`You have ${count} messages`);

// ✓ extractable, rendered via runtime substitution
t('Welcome {name}', { name: user.name });
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

### `no-string-literal-jsx-expr`

`<p>{'Hello'}</p>` — a bare string literal wrapped in an expression container. Looks extractable but isn't: the transform walks JSX text nodes, not expression containers that happen to hold a string. Auto-fixes to `<p>Hello</p>`.

### `prefer-t-for-label-props` (off by default)

The classic "label hidden in a data array" pattern:

```tsx
// ✗ 'System' never gets extracted
const THEMES = [
  { value: 'system', label: 'System' },
  { value: 'light', label: 'Light' },
];
return THEMES.map(({ value, label }) => <button>{label}</button>);

// ✓ the literals are now visible to extraction
const THEMES = [
  { value: 'system', label: t('System') },
  { value: 'light', label: t('Light') },
];
```

Off by default because it can fire on internal-only data arrays. Turn it on with the `recommendedStrict` preset or add it explicitly:

```json
{
  "rules": { "kapi-react/prefer-t-for-label-props": "error" }
}
```

It only fires for string literals stored under canonical UI-label keys (`label`, `title`, `description`, `subtitle`, `tooltip`, `placeholder`, `text`, `name`, `heading`, `caption`). Override with the `keys` option if you use different names.

## CI enforcement

Two ways to fail the build on warnings:

**CLI:**

```bash
kapi-react extract --strict
```

Exits non-zero if any warning was recorded. Suitable for a CI step run alongside your lint step.

**Plugin (build-time):**

```ts title="vite.config.ts"
import { neokapi } from '@neokapi/kapi-react/vite';

export default {
  plugins: [neokapi({ warningsAsErrors: process.env.CI === 'true' })],
};
```

This promotes any warning the transform records (currently: `unknown-component`) to a thrown error. Use `process.env.CI` to keep local dev ergonomic.

## Follow-ups

These rules are planned but not yet shipped — they need TypeScript type information or cross-file analysis that a simple ESLint rule can't do on its own:

- `translatable-attr-expects-string` — catch `<PageHeader title={x} />` where `x` is `ReactNode`, not `string`
- `unmapped-component-in-editor` — mirror the plugin's `unknown-component` warning in the editor
- `unused-componentmap-entry` — flag componentMap keys that no source file references

Track progress on [issue #381](https://github.com/neokapi/neokapi/issues/381).
