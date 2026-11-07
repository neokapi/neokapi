---
sidebar_position: 12
title: neokapi-i18n vs. Alternatives
description: "A comparison of neokapi-i18n with react-i18next, FormatJS (react-intl), LinguiJS, fbtee, and Paraglide, covering source identifiers, JSX wrapping, extraction, format, and runtime tradeoffs."
keywords: [react-i18next, FormatJS, react-intl, LinguiJS, fbtee, Paraglide, alternatives, i18n comparison, neokapi-i18n]
---

# Alternatives

If you use or are evaluating another React i18n library, these tables show where neokapi-i18n differs: how a string is identified, how much you wrap, and what ships at runtime. Every library here is well maintained, and the comparison is about fit for your codebase.

## react-i18next

The most widely used option. It uses developer-authored keys and a `t(key)` / `<Trans>` runtime.

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

FormatJS's ICU-in-source approach handles complex message composition well, and developers and translators both work in ICU. neokapi-i18n keeps the source in React components and gives translators' CAT tools the canonical ICU template.

## Lingui

The closest in approach: Lingui uses macros (`<Trans>`, `t` tagged templates) to rewrite source text into hashed-key runtime lookups at build time.

|                   | Lingui                                   | neokapi-i18n                                                  |
| ----------------- | ---------------------------------------- | ----------------------------------------------------------- |
| Source identifier | Source text (Babel macro or SWC plugin)  | Source text + own element tag (via SWC plugin)              |
| JSX wrapping      | `<Trans>Hello</Trans>`, `t\`...\`` macro | Plain JSX                                                   |
| Extraction        | `lingui extract`                         | Plugin during normal build                                  |
| Format            | PO (default), JSON, CSV                   | KBF with structural context, placeholders, plural forms |
| Runtime cost      | Small `@lingui/core` runtime + compiled catalogs; dict lookup | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

Lingui and neokapi-i18n both use the source text as the key. They differ on opt-in. With Lingui you put every string into a macro (`<Trans>`, `` t`...` ``). neokapi-i18n extracts JSX text by default and keeps `t()` for strings outside JSX.

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

fbtee also uses the source text as the key, and it requires an `<fbt>` marker with a `desc` around every translatable string. The marker lets the Babel / SWC compiler and the ESLint plugin analyse, type-check and extract each string statically. You get compile-time guarantees and declarative inline plural and gender handling, and you write a wrapper on every string. neokapi-i18n extracts plain JSX with no wrapper.

## Paraglide (Inlang)

Typed, per-message functions generated at build time. A message `welcome` becomes `m.welcome()`.

|                   | Paraglide                                            | neokapi-i18n                       |
| ----------------- | ---------------------------------------------------- | -------------------------------- |
| Source identifier | Developer-invented message id                        | Source text + own element tag |
| JSX wrapping      | Generated function call (`m.welcome()`)              | Plain JSX                        |
| Tree-shakeability | Every message is a function; excellent tree-shaking  | Dict lookup; the dict is one object |
| Runtime cost      | Minimal runtime; tree-shaken per-message functions, so unused messages cost ~0 | Inline mode: zero runtime (~2 kB if you use ICU/plurals); runtime mode: one dict lookup |

Paraglide's typed functions give strong refactoring support, with message ids as function names. neokapi-i18n keys by source text. The two can coexist in one codebase, though most projects pick one.

## Two properties the tables leave out

- **Keys survive refactoring.** The key comes from the source text *and the element's own tag*, and never from its ancestors. You can wrap a section in a new `<div>`, move a paragraph into a `<Card>` or restructure the page around it, and every key and translation stays. The element still keeps a button's "Open" apart from a menu item's. Where it doesn't, you add a note to tell them apart.
- **[Review happens on the running app](./in-context-review).** ALT+click a string to see its source and edit its target. Terminology and check findings appear on the live text, and the edit is written back to the `.kbf.json` as a git diff. Review needs no account and no network, because the strings are files in your repository. Anyone with a checkout can review.

## Which to pick

- **You want zero-wrapper ergonomics and your strings mostly live in JSX** → neokapi-i18n.
- **You want typed message functions that tree-shake to the messages you use** → Paraglide.
- **You're deeply invested in ICU-as-source** → FormatJS.
- **You want explicit, compile-time-checked inline markers with declarative plural/gender** → fbtee.
- **You have a large existing react-i18next codebase** → stay with react-i18next unless you're doing a rewrite anyway.

## Related

- [When neokapi-i18n isn't the right fit](./introduction#when-neokapi-i18n-isnt-the-right-fit): the cases where another approach serves you better.
- [Quick start](./quickstart): try neokapi-i18n in a Vite + React project.
