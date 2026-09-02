---
sidebar_position: 12
title: neokapi-i18n vs. Alternatives
description: "A comparison of neokapi-i18n with react-i18next, FormatJS (react-intl), LinguiJS, fbtee, and Paraglide, covering source identifiers, JSX wrapping, extraction, format, and runtime tradeoffs."
keywords: [react-i18next, FormatJS, react-intl, LinguiJS, fbtee, Paraglide, alternatives, i18n comparison, neokapi-i18n]
---

# Alternatives

A quick reference for teams already using, or evaluating, another React i18n library. All of these are solid projects; the differences below are about fit, not quality.

## react-i18next

The incumbent. Uses developer-authored keys and a `t(key)` / `<Trans>` runtime.

|                   | react-i18next                         | neokapi-i18n                                                  |
| ----------------- | ------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Developer-invented key (natural-language keys also supported) | Source text + own element tag                               |
| JSX wrapping      | `t("key")` or `<Trans i18nKey="...">` | Plain JSX                                                   |
| Extraction        | `i18next-cli` / `i18next-parser`, or manual | Plugin during normal build                            |
| Format            | JSON (nested or flat); XLIFF via external conversion | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Ships the i18next runtime (interpolation, plural resolution, resource store); dict loaded at runtime | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

Migrating from react-i18next typically means dropping the `t()` / `<Trans>` wrappers and re-running the extract against the bare JSX. Existing translations can be loaded as-is if you key them by the same source text; otherwise it's a one-time re-translation pass through your content memory.

## FormatJS (react-intl)

Developer-authored message descriptors with ICU formatting baked in.

|                   | FormatJS                                               | neokapi-i18n                                   |
| ----------------- | ------------------------------------------------------ | -------------------------------------------- |
| Source identifier | Developer-invented id (or auto-hash of the descriptor) | Source text + own element tag                |
| JSX wrapping      | `<FormattedMessage>` or `useIntl().formatMessage()`    | Plain JSX                                    |
| Plurals / select  | Raw ICU message strings                                | `<Plural>` / `<Select>` authoring components |
| Extraction        | `@formatjs/cli`                                        | Plugin during normal build                   |
| Runtime cost      | Ships `intl-messageformat` (ICU parser/formatter); can precompile to AST to drop the parser | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

FormatJS's ICU-in-source approach handles complex message composition well, but forces translators (and developers) to work in ICU directly. neokapi-i18n keeps the source looking like React, then emits the canonical ICU template for translators' CAT tools downstream.

## Lingui

The closest in philosophy: Lingui uses macros (`<Trans>`, `t` tagged templates) to rewrite source text into hashed-key runtime lookups at build time.

|                   | Lingui                                   | neokapi-i18n                                                  |
| ----------------- | ---------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Source text (Babel macro or SWC plugin)  | Source text + own element tag (via SWC plugin)              |
| JSX wrapping      | `<Trans>Hello</Trans>`, `t\`...\`` macro | Plain JSX                                                   |
| Extraction        | `lingui extract`                         | Plugin during normal build                                  |
| Format            | PO (default), JSON, CSV                   | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Small `@lingui/core` runtime + compiled catalogs; dict lookup | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

Lingui and neokapi-i18n agree on "source text as key". The core difference: Lingui asks you to opt every string into the macro (`<Trans>`, `` t`...` ``); neokapi-i18n opts in by default. `t()` in neokapi-i18n is a small escape hatch for non-JSX strings, not the normal authoring pattern.

## fbtee

The modern continuation of Meta's `fbt`. fbtee rebuilds it for TypeScript, React 19, ESM, and Vite / Next.js with both Babel and SWC transforms, while keeping fbt's authoring model: every translatable string is wrapped in an explicit `<fbt>` marker, and the source text is the key.

|                   | fbtee                                                  | neokapi-i18n                                               |
| ----------------- | ------------------------------------------------------ | -------------------------------------------------------- |
| Source identifier | Source text + required `desc`                          | Source text + own element tag                           |
| JSX wrapping      | `<fbt desc="...">`, `fbt()` / `fbs()`                  | Plain JSX                                                |
| Plurals / gender  | `<fbt:plural>`, `<fbt:pronoun>`, `<fbt:enum>`          | `<Plural>` / `<Select>` authoring components             |
| Extraction        | `fbtee collect` → `prepare-translations` → `translate` | Plugin during normal build                               |
| Format            | JSON (`source_strings.json` + per-locale files)        | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Ships the fbt runtime to resolve params/plural/pronoun at render; translations loaded | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

fbtee shares neokapi-i18n's "source text as key" philosophy, but takes the opposite stance on wrapping: it deliberately requires an `<fbt>` marker (with a `desc`) around every translatable string so the Babel / SWC compiler and ESLint plugin can statically analyse, type-check, and extract it. That buys compile-time guarantees and declarative inline plural / gender handling, at the cost of wrapping ceremony on every string, the same wrapping tax neokapi-i18n removes by extracting plain JSX automatically.

## Paraglide (Inlang)

Typed, per-message functions generated at build time. A message `welcome` becomes `m.welcome()`.

|                   | Paraglide                                            | neokapi-i18n                       |
| ----------------- | ---------------------------------------------------- | -------------------------------- |
| Source identifier | Developer-invented message id                        | Source text + own element tag |
| JSX wrapping      | Generated function call (`m.welcome()`)              | Plain JSX                        |
| Tree-shakeability | Every message is a function; excellent tree-shaking  | Dict lookup; the dict is one object |
| Runtime cost      | Minimal runtime; tree-shaken per-message functions, so unused messages cost ~0 | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

Paraglide's typed-function model gives strong refactoring support but requires the ids-as-function-names model. neokapi-i18n is source-text-as-key; the two can coexist in a codebase if needed, but usually you pick one.

## Where neokapi-i18n is unusual

Two properties follow from choices the tables above don't capture:

- **Keys survive refactoring.** The key is derived from the source text *and the element's own tag*, and deliberately not from its ancestors. Wrapping a section in a new `<div>`, moving a paragraph into a `<Card>`, restructuring the page around it: none of these change a key, so none of them orphan a translation. The element is still enough to keep a button's "Open" distinct from a menu item's; where it isn't, you disambiguate explicitly with a note.
- **[Review happens on the running app](./in-context-review).** ALT+click a string to see its source and edit its target, with terminology and check findings painted onto the live text, and the edit written straight back to the `.kbf.json` as a git diff. Review needs no account and no network: the strings are files in your repository, so a reviewer with a checkout is a reviewer.

## Which to pick

- **You want zero-wrapper ergonomics and your strings mostly live in JSX** → neokapi-i18n.
- **You want typed message functions that tree-shake to the messages you use** → Paraglide.
- **You're deeply invested in ICU-as-source** → FormatJS.
- **You want explicit, compile-time-checked inline markers with declarative plural/gender** → fbtee.
- **You have a large existing react-i18next codebase** → stay with react-i18next unless you're doing a rewrite anyway.

## When neokapi-i18n isn't the right fit

See the same section in the [Introduction](./introduction#when-neokapi-i18n-isnt-the-right-fit).
