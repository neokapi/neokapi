---
sidebar_position: 12
title: kapi-react vs. Alternatives
description: A comparison of kapi-react with react-i18next, FormatJS (react-intl), LinguiJS, fbtee, and Paraglide — covering source identifiers, JSX wrapping, extraction, format, and runtime tradeoffs.
keywords: [react-i18next, FormatJS, react-intl, LinguiJS, fbtee, Paraglide, alternatives, i18n comparison, kapi-react]
---

# Alternatives

A quick reference for teams already using — or evaluating — another React i18n library. All of these are solid projects; the differences below are about fit, not quality.

## react-i18next

The incumbent. Uses developer-authored keys and a `t(key)` / `<Trans>` runtime.

|                   | react-i18next                         | kapi-react                                                  |
| ----------------- | ------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Developer-invented key (natural-language keys also supported) | Source text + structural context                            |
| JSX wrapping      | `t("key")` or `<Trans i18nKey="...">` | Plain JSX                                                   |
| Extraction        | `i18next-cli` / `i18next-parser`, or manual | Plugin during normal build                            |
| Format            | JSON (nested or flat); XLIFF via external conversion | KLF with structural context, placeholders, plural forms |

Migrating from react-i18next typically means dropping the `t()` / `<Trans>` wrappers and re-running the extract against the bare JSX. Existing translations can be loaded as-is if you key them by the same source text; otherwise it's a one-time re-translation pass through your TM.

## FormatJS (react-intl)

Developer-authored message descriptors with ICU formatting baked in.

|                   | FormatJS                                               | kapi-react                                   |
| ----------------- | ------------------------------------------------------ | -------------------------------------------- |
| Source identifier | Developer-invented id (or auto-hash of the descriptor) | Source text + structural context             |
| JSX wrapping      | `<FormattedMessage>` or `useIntl().formatMessage()`    | Plain JSX                                    |
| Plurals / select  | Raw ICU message strings                                | `<Plural>` / `<Select>` authoring components |
| Extraction        | `@formatjs/cli`                                        | Plugin during normal build                   |

FormatJS's ICU-in-source approach is powerful for complex message composition, but forces translators (and developers) to work in ICU directly. kapi-react keeps the source looking like React, then emits the canonical ICU template for translators' CAT tools downstream.

## Lingui

The closest in philosophy — Lingui uses macros (`<Trans>`, `t` tagged templates) to rewrite source text into hashed-key runtime lookups at build time.

|                   | Lingui                                   | kapi-react                                                  |
| ----------------- | ---------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Source text (Babel macro; experimental SWC plugin) | Source text + structural context (via SWC plugin)           |
| JSX wrapping      | `<Trans>Hello</Trans>`, `t\`...\`` macro | Plain JSX                                                   |
| Extraction        | `lingui extract`                         | Plugin during normal build                                  |
| Format            | PO (default), JSON, CSV                   | KLF with structural context, placeholders, plural forms |

Lingui and kapi-react agree on "source text as key". The core difference: Lingui asks you to opt every string into the macro (`<Trans>`, `` t`...` ``); kapi-react opts in by default. `t()` in kapi-react is a small escape hatch for non-JSX strings, not the normal authoring pattern.

## fbtee

The modern continuation of Meta's `fbt` (Meta archived `fbt` in late 2024). fbtee rebuilds it for TypeScript, React 19, ESM, and Vite / Next.js with both Babel and SWC transforms, while keeping fbt's authoring model: every translatable string is wrapped in an explicit `<fbt>` marker, and the source text is the key.

|                   | fbtee                                                  | kapi-react                                               |
| ----------------- | ------------------------------------------------------ | -------------------------------------------------------- |
| Source identifier | Source text + required `desc`                          | Source text + structural context                        |
| JSX wrapping      | `<fbt desc="...">`, `fbt()` / `fbs()`                  | Plain JSX                                                |
| Plurals / gender  | `<fbt:plural>`, `<fbt:pronoun>`, `<fbt:enum>`          | `<Plural>` / `<Select>` authoring components             |
| Extraction        | `fbtee collect` → `prepare-translations` → `translate` | Plugin during normal build                               |
| Format            | JSON (`source_strings.json` + per-locale files)        | KLF with structural context, placeholders, plural forms |

fbtee shares kapi-react's "source text as key" philosophy, but takes the opposite stance on wrapping: it deliberately requires an `<fbt>` marker (with a `desc`) around every translatable string so the Babel / SWC compiler and ESLint plugin can statically analyse, type-check, and extract it. That buys compile-time guarantees and declarative inline plural / gender handling, at the cost of wrapping ceremony on every string — the same wrapping tax kapi-react removes by extracting plain JSX automatically.

## Paraglide (Inlang)

Typed, per-message functions generated at build time. A message `welcome` becomes `m.welcome()`.

|                   | Paraglide                                            | kapi-react                       |
| ----------------- | ---------------------------------------------------- | -------------------------------- |
| Source identifier | Developer-invented message id                        | Source text + structural context |
| JSX wrapping      | Generated function call (`m.welcome()`)              | Plain JSX                        |
| Tree-shakeability | Every message is a function — excellent tree-shaking | Dict lookup — dict is one object |

Paraglide's typed-function model gives strong refactoring support but requires the ids-as-function-names model. kapi-react is source-text-as-key; the two can coexist in a codebase if needed, but usually you pick one.

## Which to pick

- **You want zero-wrapper ergonomics and your strings mostly live in JSX** → kapi-react.
- **You want typed message functions with best-in-class tree-shaking** → Paraglide.
- **You're deeply invested in ICU-as-source** → FormatJS.
- **You want explicit, compile-time-checked inline markers with declarative plural/gender** → fbtee.
- **You have a large existing react-i18next codebase** → stay with react-i18next unless you're doing a rewrite anyway.

## When kapi-react isn't the right fit

See the same section in the [Introduction](./introduction#when-kapi-react-isnt-the-right-fit).
