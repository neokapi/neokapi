---
sidebar_position: 1
title: Introduction
slug: /react/introduction
description: "neokapi-i18n is a zero-toil i18n library for React: no key strings, no wrapping calls. The Vite plugin extracts translatable JSX at build time; kapi translates the KBF archive with AI or content memory."
keywords: [neokapi-i18n, React i18n, internationalization, Vite plugin, JSX extraction, KBF, zero-toil]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# neokapi-i18n

neokapi-i18n translates a React app from the JSX you already write. A build plugin finds the user-visible text, gives each string a stable key, and writes a catalog that kapi or your translators fill, with no wrapper calls and no keys in your source.

## The problem with traditional i18n

Translating a React app usually means wrapping every user-visible string in a translation call:

```tsx
// The traditional way: call-based (a translation function wraps every string)
<h1>{t("welcome.heading")}</h1>
<p>{t("welcome.description")}</p>
<button>{t("welcome.getStarted")}</button>
```

Newer libraries drop the key but keep the wrapper: you mark every translatable
fragment with an explicit JSX element instead.

```tsx
// The traditional way: element-based (a marker component wraps every fragment)
<h1><T desc="welcome heading">Welcome</T></h1>
<p><T>Ship your product in every language your users speak.</T></p>
<button><T>Get started</T></button>
```

Either way, you write the UI and then annotate it. The annotation costs you work in three places:

- **Writing the wrapper.** Every string gets a translation call or a marker element. Every prop that might be translated becomes an expression or a marker component instead of a plain literal. The element-based style trades the key for extra JSX, and the wrapping stays.
- **Inventing and maintaining keys.** You pick `welcome.heading`, `welcome.description` and `welcome.getStarted`, rename them when the copy changes, hunt for collisions and review them in diffs. Element-based libraries replace the keys with `desc`/`context` props that you still write and keep accurate.
- **Keeping them in sync.** The translation file, the call sites and the docs drift apart. When they do, the app shows `welcome.heading` to the user, or renders the wrong text.

You pay that cost on day one, again when a new engineer joins, and every time a designer changes a word of copy.

## What neokapi-i18n does differently

neokapi-i18n extracts translatable content from plain JSX:

```tsx
// The neokapi-i18n way
<h1>Welcome</h1>
<p>Ship your product in every language your users speak.</p>
<button>Get started</button>
```

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "Your JSX", sub: "plain text", role: "io" },
    { label: "Find text", sub: "W3C rules", role: "annotate" },
    { label: "Hash", sub: "text + own tag", role: "tool" },
    { label: "i18n/", sub: "KBF archive", role: "io" },
  ]}
  caption="At build time the plugin walks your JSX, keys each string by its text and tag, and writes the catalog."
/>

At build time a Vite, webpack, Rollup or esbuild plugin:

1. Walks your JSX and finds everything that should be translated: headings, buttons, attribute values, and so on.
2. Computes a stable hash from the source text and the element's own tag.
3. Writes a KBF directory archive, the exchange format your translators or an AI read.
4. Rewrites the JSX to look up the hash at render time, or, in inline mode, writes the translated text into the build so nothing is looked up at runtime.

The source text is the identifier. When the copy changes, you change the JSX, and there is no key to rename or table to update. Existing translations still resolve. A new string gets a fresh hash, and the next extract picks it up.

## What you stop doing

- **Wrapping normal JSX in `t()`.** `<h1>Welcome</h1>` is translatable as written, and so are element children and translatable props on your own components.
- **Inventing keys.** The hash of the source text and the element's own tag is the key. The runtime dict reads `{ "aB3": "Bienvenue", ... }`, where a key-based library has `{ "welcome.heading": "Bienvenue", ... }`.
- **Losing translations in a refactor.** Ancestors are not part of the key. You can wrap a `<p>` in a new `<div>`, move it into a `<Card>` or restructure the page around it, and its key and translations stay. A key changes when the words change, which is when a translator should look again.
- **Editing translation files.** Developers write JSX and translators write translations. The `.kbf.json` archive is the contract between them.
- **Marking strings, except in JS data.** `t()` is the one explicit marker, for strings that belong in JS data: button-label arrays, error messages returned from reducers, refs.

## What's included

- **Automatic JSX extraction** with W3C HTML5 translatability rules: headings, paragraphs, buttons, labels, options, `<span>`, `<strong>`, `<em>`, links, ARIA-backed attributes.
- **Defaults for idiomatic React**: `<div>Label</div>`, `<section>...</section>`, and unmapped components like `<TabsTrigger>General</TabsTrigger>` extract with a warning.
- **Translatable props**: the HTML and ARIA set (`alt`, `title`, `placeholder`, `aria-label`, …) on any element, plus React's prop-name conventions (`subtitle`, `description`, `label`, `helpText`, …) on your own components. `<PageHeader title="Content Memory" />` extracts as written.
- **`<Plural>` / `<Select>` authoring components** with CLDR plural rules through `Intl.PluralRules`, and ICU number, date and time formatting through `Intl`. A translator can write `{n, number}` or `{d, date, long}` into a target with no code change.
- **The `t()` escape hatch** for the few strings that belong in data.
- **Two build modes**: inline (no runtime, one build per locale) and runtime (one bundle, dict loaded OTA).
- **[In-context review](./in-context-review) on the running app**: ALT+click any string to see its source, edit its translation and write it back to the `.kbf.json`. Terms and check findings appear on the live text.
- **An exchange format**, KBF (see [C-01](/contribute/architecture/context/c-01-project-model)), that carries structural context, placeholders, plural forms and annotation overlays. A flat key-value JSON holds none of these.
- **Integration with `kapi`** for pseudo-translation, AI translation, checks, content-memory leverage and terminology. kapi handles the archive with the same tools it uses for XLIFF, JSON, Markdown, HTML and its other formats.

## When neokapi-i18n isn't the right fit

- **Server-rendered HTML pipelines without a React build step.** If a non-React framework outputs your HTML, use kapi's HTML or XLIFF formats directly.
- **Large catalogs built mostly from programmatic composition.** If most of your strings come from templates such as `t("error.code." + code)`, the source-text-as-key model works against you. neokapi-i18n fits best when the strings are visible in the source.
- **Round-tripping through several TMS vendors with existing translation keys.** If your workflow depends on keys inherited from another system, moving to neokapi-i18n's hashes is a migration.

For product UI, marketing sites, internal dashboards, extension pages and embedded apps, neokapi-i18n needs no per-string annotation.

## Related

- [Quick start](./quickstart): add neokapi-i18n to a Vite + React project in 5 minutes.
- [Writing translatable components](./writing-components): what extracts automatically, and when a warning fires.
- [`t()` escape hatch](./t-escape-hatch): marking strings that live outside JSX.
- [Extract → translate → compile](./pipeline): the full end-to-end flow with `kapi`.
- [Alternatives](./alternatives): how neokapi-i18n compares with other React i18n libraries.
