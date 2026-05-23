# @neokapi/kapi-react-lint

Authoring-time lint rules for [kapi-react](../kapi-react/README.md). The same
rule objects run under **oxlint** (the primary target — rules are authored
against `@oxlint/plugins` types) and **ESLint** (8.57+, 9, and 10). oxlint's
plugin API is a strict subset of ESLint v9's, so you install one package and
wire it into whichever linter you already use.

kapi-react's build-time transform catches a lot, but some authoring mistakes
only show up _after_ extraction:

- `t(variable)` — unextractable, the extractor only sees literals
- `<img alt={'Logo ' + brand} />` — the attribute is translatable, the concat isn't
- `const ITEMS = [{ label: 'Light' }]` — the string never reaches the JSX walker

These rules give you editor squigglies for those cases, plus a `--strict`
path for CI enforcement.

## Install

```bash
vp install -D @neokapi/kapi-react-lint
```

## Oxlint

```jsonc title=".oxlintrc.json"
{
  "jsPlugins": ["@neokapi/kapi-react-lint/oxlint"],
  "rules": {
    "kapi-react/t-literal-first-arg": "error",
    "kapi-react/t-no-concat": "error",
    "kapi-react/no-concat-in-translatable-attr": "error",
    "kapi-react/no-string-literal-jsx-expr": "warn",
  },
}
```

## ESLint (flat config)

Supported ESLint versions: **8.57+, 9, and 10** (flat config).

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

Also available: `recommendedStrict` (turns everything on as `error`, including
`prefer-t-for-label-props`).

## Rules

| Rule                             | In `recommended` | Description                                        |
| -------------------------------- | ---------------- | -------------------------------------------------- |
| `t-literal-first-arg`            | `error`          | `t()` first argument must be a string literal      |
| `t-no-concat`                    | `error`          | No string concat / template interpolation in `t()` |
| `no-concat-in-translatable-attr` | `error`          | No concat in `alt` / `title` / `aria-label` / …    |
| `no-string-literal-jsx-expr`     | `warn`           | `<p>{'Hello'}</p>` should be `<p>Hello</p>`        |
| `prefer-t-for-label-props`       | off              | Suggest `t()` for label strings in data arrays     |

See the [full documentation](https://kapi-react.dev/docs/kapi-react/linting) for
examples, FP notes, and the planned follow-up rules (type-info-aware,
cross-file).

## License

Apache-2.0.
