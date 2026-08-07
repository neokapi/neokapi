# @neokapi/i18n-react-lint

Authoring-time lint rules for [neokapi-i18n](../i18n-react/README.md). The same
rule objects run under **oxlint** (the primary target — rules are authored
against `@oxlint/plugins` types) and **ESLint** (8.57+, 9, and 10). oxlint's
plugin API is a strict subset of ESLint v9's, so you install one package and
wire it into whichever linter you already use.

neokapi-i18n's build-time transform catches a lot, but some authoring mistakes
only show up _after_ extraction:

- `t(variable)` — unextractable, the extractor only sees literals
- `<img alt={'Logo ' + brand} />` — the attribute is translatable, the concat isn't
- `const ITEMS = [{ label: 'Light' }]` — the string never reaches the JSX walker

These rules give you editor squigglies for those cases, plus a `--strict`
path for CI enforcement.

## Install

```bash
vp install -D @neokapi/i18n-react-lint
```

## Oxlint

```jsonc title=".oxlintrc.json"
{
  "jsPlugins": ["@neokapi/i18n-react-lint/oxlint"],
  "rules": {
    "neokapi-i18n/t-literal-first-arg": "error",
    "neokapi-i18n/t-no-concat": "error",
    "neokapi-i18n/no-concat-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-in-translatable-attr": "error",
    "neokapi-i18n/no-ternary-literals-in-jsx-child": "error",
    "neokapi-i18n/no-string-literal-jsx-expr": "warn",
    "neokapi-i18n/prefer-t-for-label-expr": "warn",
  },
}
```

## ESLint (flat config)

Supported ESLint versions: **8.57+, 9, and 10** (flat config).

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

Also available: `recommendedStrict` (turns everything on as `error`, including
`prefer-t-for-label-props`).

## Rules

| Rule                               | In `recommended` | Description                                        |
| ---------------------------------- | ---------------- | -------------------------------------------------- |
| `t-literal-first-arg`              | `error`          | `t()` first argument must be a string literal      |
| `t-no-concat`                      | `error`          | No string concat / template interpolation in `t()` |
| `no-concat-in-translatable-attr`   | `error`          | No concat in `alt` / `title` / `aria-label` / …    |
| `no-ternary-in-translatable-attr`  | `error`          | No ternary in `alt` / `title` / `aria-label` / …   |
| `no-ternary-literals-in-jsx-child` | `error`          | No string-literal ternary branches as JSX children |
| `no-string-literal-jsx-expr`       | `warn`           | `<p>{'Hello'}</p>` should be `<p>Hello</p>`        |
| `prefer-t-for-label-expr`          | `warn`           | Suggest `t()` for label expressions                |
| `prefer-t-for-label-props`         | off              | Suggest `t()` for label strings in data arrays     |

See the [full documentation](../../web/docs/react/linting.md) for
examples, FP notes, and the planned follow-up rules (type-info-aware,
cross-file).

## License

Apache-2.0.
