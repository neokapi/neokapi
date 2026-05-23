# i18next corpus — provenance

Genuine i18next / react-i18next resource bundles vendored verbatim from
upstream open-source repositories for byte-faithful round-trip and
consumer-acceptance testing. All sources are MIT-licensed (permissive).

Each file is an exact copy of an upstream file at the pinned commit; the exact
`curl` used to fetch it is recorded below for reproducibility.

| Local file | Upstream repo | License | Path |
|---|---|---|---|
| `i18next-typescript-ns1.json` | `i18next/i18next` | MIT | `examples/typescript/i18n/en/ns1.json` |
| `i18next-typescript-ns2.json` | `i18next/i18next` | MIT | `examples/typescript/i18n/en/ns2.json` |
| `i18next-compat-v1-en.json` | `i18next/i18next` | MIT | `test/compatibility/v1/locales/en/translation.json` |
| `react-i18next-icu.json` | `i18next/react-i18next` | MIT | `example/react-icu/public/locales/en/translations.json` |
| `react-i18next-react.json` | `i18next/react-i18next` | MIT | `example/react/public/locales/en/translation.json` |
| `react-i18next-storybook.json` | `i18next/react-i18next` | MIT | `example/storybook/public/locales/en/translation.json` |

## Pinned commits

- `i18next/i18next` — `22fb6ad013c9c069c33086eb3737b4371936d5ce` (master)
- `i18next/react-i18next` — `a46ad23ad07f1a3440d03cce80d0cab7ad23e2f0` (master)

## Exact fetch commands

```sh
I18N_SHA=22fb6ad013c9c069c33086eb3737b4371936d5ce
REACT_SHA=a46ad23ad07f1a3440d03cce80d0cab7ad23e2f0

curl -sSL "https://raw.githubusercontent.com/i18next/i18next/${I18N_SHA}/examples/typescript/i18n/en/ns1.json" \
  -o i18next-typescript-ns1.json
curl -sSL "https://raw.githubusercontent.com/i18next/i18next/${I18N_SHA}/examples/typescript/i18n/en/ns2.json" \
  -o i18next-typescript-ns2.json
curl -sSL "https://raw.githubusercontent.com/i18next/i18next/${I18N_SHA}/test/compatibility/v1/locales/en/translation.json" \
  -o i18next-compat-v1-en.json
curl -sSL "https://raw.githubusercontent.com/i18next/react-i18next/${REACT_SHA}/example/react-icu/public/locales/en/translations.json" \
  -o react-i18next-icu.json
curl -sSL "https://raw.githubusercontent.com/i18next/react-i18next/${REACT_SHA}/example/react/public/locales/en/translation.json" \
  -o react-i18next-react.json
curl -sSL "https://raw.githubusercontent.com/i18next/react-i18next/${REACT_SHA}/example/storybook/public/locales/en/translation.json" \
  -o react-i18next-storybook.json
```

## Constructs covered

- **Nested namespaces** — `i18next-typescript-ns1.json` (`description.part1/part2`),
  `i18next-compat-v1-en.json` (`test.simple_en`, `route.key1/key2`).
- **Plural sibling keys (v4 CLDR)** — `i18next-typescript-ns1.json`
  (`pl_one` / `pl_other`).
- **Context keys** — `i18next-typescript-ns1.json` (`some` / `some_me` /
  `some_1234`).
- **`{{interpolation}}`** — `i18next-typescript-ns1.json` (`inter`, `pl_other`).
- **ICU MessageFormat bodies & `<Trans>` placeholders** — `react-i18next-icu.json`
  (`{numPersons, plural, …}`, `<1>src/App.js</1>`, `{gender, select, …}`,
  flat dotted keys, and full-sentence keys). These use single-brace ICU syntax,
  which is NOT i18next's `{{...}}` interpolation and is correctly treated as
  prose (the `i18next-icu` plugin handles ICU at runtime, not the JSON layer).

## Note on the `react-i18next-icu` keys

`react-i18next-icu.json` contains keys whose names are full sentences with
single-brace `{name}` ICU markers and `<0>…</0>` Trans tags (e.g.
`"Welcome, {name}!"`). These are genuine react-i18next ICU example keys. The
i18next code-finder protects only the library's own `{{var}}` interpolation and
`$t(key)` nesting, so the single-brace ICU bodies pass through as ordinary
translatable text — the faithful behavior. Byte-faithful round-trip is verified
regardless.
