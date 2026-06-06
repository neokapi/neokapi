---
sidebar_position: 1
title: Introduction
slug: /react/introduction
description: kapi-react is a zero-toil i18n library for React — no key strings, no wrapping calls. The Vite plugin extracts translatable JSX at build time; kapi translates the KLF archive with AI, MT, or TM.
keywords: [kapi-react, React i18n, internationalization, Vite plugin, JSX extraction, KLF, zero-toil]
---

# kapi-react

Zero-toil internationalisation for React.

## The problem with traditional i18n

Localising a React app usually means wrapping every user-visible string in a translation call:

```tsx
// The traditional way — call-based (react-i18next, react-intl)
<h1>{t("welcome.heading")}</h1>
<p>{t("welcome.description")}</p>
<button>{t("welcome.getStarted")}</button>
```

Newer libraries drop the key but keep the wrapper: you mark every translatable
fragment with an explicit JSX element instead.

```tsx
// The traditional way — element-based (fbtee, Lingui, react-i18next <Trans>)
<h1><fbt desc="welcome heading">Welcome</fbt></h1>
<p><Trans>Ship your product in every language your users speak.</Trans></p>
<button><Trans>Get started</Trans></button>
```

Either way, you don't just write the UI — you also annotate it. That creates
three kinds of toil:

- **Writing the wrapper** — every string in your app gets `t(...)`, `<Trans>`, or `<fbt>` around it, and every prop that might ever be translated becomes an expression or a marker component instead of a plain literal. The element-based libraries trade the key for boilerplate JSX, but the wrapping tax stays.
- **Inventing and maintaining keys** — `welcome.heading`, `welcome.description`, `welcome.getStarted`. You pick them, you rename them when the copy changes, you hunt for collisions, you diff them in review. (Element-based libraries swap explicit keys for `desc`/`context` props you still have to author and keep accurate.)
- **Keeping them in sync** — the translation file, the call sites, the docs. When any of them drifts, the app shows `welcome.heading` to the user or (worse) renders the wrong text.

You pay that cost on day one, the day a new engineer joins, and every time a designer changes a word of copy.

## What kapi-react does differently

kapi-react extracts translatable content from the JSX you already write — no wrappers, no keys.

```tsx
// The kapi-react way
<h1>Welcome</h1>
<p>Ship your product in every language your users speak.</p>
<button>Get started</button>
```

At build time a [SWC](https://swc.rs/)-based Vite / webpack / Rollup / esbuild plugin:

1. Walks your JSX and finds everything that ought to be translated (heading, button, attribute values, …).
2. Computes a stable hash from the source text + its structural context.
3. Emits a KLF directory archive — the exchange format your translators (or an AI) consume.
4. Rewrites the JSX to look up the hash at render time when a translation is loaded, or inlines the translated text at build time for zero-runtime-lookup mode.

The source text is the identifier. When the copy changes, you change the JSX — no key to rename, no translation table to keep in sync. Translations that already exist still resolve; new strings get a fresh hash, and your extract pipeline picks them up automatically.

## What "no-toil" means in practice

- **No `t()` wrapping for normal JSX.** `<h1>Welcome</h1>` is translatable as written — so are element children and translatable props on your own components.
- **No key invention.** The hash of the source text + structural context is the key. The runtime dict is `{ "aB3": "Bienvenue", ... }` — not `{ "welcome.heading": "Bienvenue", ... }`.
- **No translation-file edits from developers.** Developers write JSX. Translators write translations. The `.klf` archive is the contract between them.
- **One explicit marker — `t()` — for strings that legitimately live in JS data** (button-label arrays, error messages returned from reducers, refs). That's it.

## What you get in the box

- **Automatic JSX extraction** with W3C HTML5 translatability rules — headings, paragraphs, buttons, labels, options, `<span>`, `<strong>`, `<em>`, links, ARIA-backed attributes.
- **Smart defaults for idiomatic React** — `<div>Label</div>`, `<section>...</section>`, and unmapped components like `<TabsTrigger>General</TabsTrigger>` auto-extract with a warning, not a silent drop.
- **Translatable props on any component** — `title`, `subtitle`, `description`, `label`, `helpText`, `errorMessage`, `tooltip`, and the usual HTML+ARIA set. `<PageHeader title="Translation Memories" />` just works.
- **`<Plural>` / `<Select>` authoring components** with CLDR-aware runtime resolution via `Intl.PluralRules`.
- **`t()` escape hatch** for the small set of strings that genuinely belong in data.
- **Two build modes** — inline (zero runtime, builds per locale) and runtime (single bundle, dict loaded OTA).
- **A proper exchange format** — KLF (see [AD-008](/contribute/architecture/008-project-model)) — that carries structural context, placeholders, plural forms, and annotation overlays. Not a flat key-value JSON.
- **Full integration with `kapi`** for pseudo-translation, AI translation, QA, TM leverage, and terminology. The same toolchain that handles XLIFF, JSON, Markdown, HTML, and every other format kapi supports.

## When kapi-react isn't the right fit

- **Server-rendered HTML pipelines without a React build step.** If you're outputting raw HTML from a non-React framework, use kapi's HTML / XLIFF filters directly instead.
- **Large string catalogs with heavy programmatic composition.** If 80% of your strings are assembled from programmatic templates — `t("error.code." + code)` — the source-text-as-key model fights you. kapi-react is happiest when strings are visible in the source.
- **Need for multi-vendor TMS round-tripping with pre-existing translation keys.** If your workflow already depends on specific translation keys inherited from another system, kapi-react's hash model would require a migration.

For everything else — product UI, marketing sites, internal dashboards, extension pages, embedded apps — kapi-react removes the i18n tax entirely.

## Next steps

- [Quick start](./quickstart) — add kapi-react to a Vite + React project in 5 minutes.
- [Writing translatable components](./writing-components) — what gets picked up automatically, and when a warning fires.
- [`t()` escape hatch](./t-escape-hatch) — marking strings that live outside JSX.
- [Extract → translate → compile](./pipeline) — the full end-to-end flow with `kapi`.

Already using another React i18n library? See [Alternatives](./alternatives) for how kapi-react compares.
