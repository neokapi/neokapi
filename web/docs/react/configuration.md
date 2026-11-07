---
sidebar_position: 11
title: neokapi-i18n Configuration Reference
description: "Full reference for the neokapi-i18n Vite plugin options: mode, locale, fallbackLocales, translationsDir, componentMap, Storybook integration, and CLI flags for extract and compile."
keywords: [neokapi-i18n, configuration, plugin options, Vite, locale, componentMap, CLI flags, Storybook]
---

# Configuration

These settings control what neokapi-i18n extracts, how it keys each string and how translations reach the page. The plugin options come first, then the `neokapi-i18n` CLI flags, Storybook, the document locale and debugging.

## Plugin options

```ts
import neokapi from "@neokapi/i18n-react/vite";

neokapi({
  mode: "runtime",
  locale: "fr",
  fallbackLocales: ["fr", "en"],
  translationsDir: "./translations",
  componentMap: { TabsTrigger: "button" },
  rules: [{ selector: ".hero-caption", translate: false }],
  strict: "warn",
  warnUnmapped: false,
  communityManifestDir: "./i18n-manifests",
  review: false,
  reviewKbfDir: "i18n",
  onWarning: (msg) => logger.warn(msg),
});
```

### `mode: "runtime" | "inline"`

See [Runtime vs. inline mode](./modes).

- `"runtime"`: one bundle, dict loaded at runtime.
- `"inline"`: one bundle per locale, translations inlined.
- Omitted: the plugin does nothing, and source text renders as written. Use this in development when you don't need extraction.

### `locale` (inline mode only)

The target locale. It selects the `translations/<locale>.json` file the plugin reads at build time.

### `fallbackLocales` (inline mode only)

Ordered list of locales tried when the primary is missing a translation.

```ts
neokapi({
  mode: "inline",
  locale: "de-AT",
  fallbackLocales: ["de", "en"],
});
```

### `translationsDir` (inline mode only)

Directory holding `<locale>.json` files. Default: `./translations`.

### `componentMap`

Maps React components to their underlying HTML element so hashes stay stable across refactors:

```ts
neokapi({
  componentMap: {
    // Internal components
    PageHeader: "header",
    Heading: "h2",
  },
});
```

Before it reads this option, the plugin **resolves mappings itself** for every non-relative import it sees. It looks in three places, in order:

1. **Library-shipped manifest**: `<package>/i18n-manifest.json`. This source comes first, and it is the one to use if you author a library; see [Authoring i18n manifests](#authoring-i18n-manifests-for-libraries).
2. **Community manifest directory**: `<communityManifestDir>/<package-name>.json`, if you've configured one.
3. **`.d.ts` heuristic**: regex-match for `React.ForwardRefExoticComponent<... & RefAttributes<HTMLXxxElement>>` in the package's declared types. This finds most pre-React-19 shadcn / Radix / MUI components.

Your `componentMap` entries merge over the resolved map, so an explicit entry wins. Components from a library with proper types or a shipped manifest, such as shadcn-style components, need no entry.

An unmapped component still extracts through the promotion rule, with a warning. Adding an entry silences the warning and re-keys the block's hash from `Component` to the underlying HTML tag.

### Authoring i18n manifests for libraries

Ship an `i18n-manifest.json` at the root of your component library so consumers don't need to maintain `componentMap` entries:

```json title="packages/ui/i18n-manifest.json"
{
  "components": {
    "Button": "button",
    "Badge": "span",
    "CardTitle": "h3",
    "CardDescription": "p",
    "Label": "label",
    "TabsTrigger": "button",
    "SelectItem": "option",

    "Input": null,
    "Textarea": null,
    "Skeleton": null
  },
  "aliases": {
    "Trigger": "TabsTrigger"
  }
}
```

- Keys are the exported component names.
- Values are the underlying HTML element name, or `null` to explicitly opt out of translation.
- `aliases` map alternative export names onto canonical ones (useful for Radix-style namespace re-exports like `Tabs.Trigger`).

The plugin loads this file when any file imports from the library. [`@neokapi/ui-primitives/i18n-manifest.json`](https://github.com/neokapi/neokapi/blob/main/packages/ui/i18n-manifest.json) is a manifest in production use.

### `rules`

Declarative overrides keyed on selectors:

```ts
neokapi({
  rules: [
    // Turn translation off for specific matches
    { selector: ".code-block", translate: false },
    { selector: "[data-testid]", translate: false },

    // Attach a translator note
    { selector: ".legal-copy", locNote: "Must match legal-approved wording" },

    // Turn translation on for a container that wouldn't normally auto-promote
    { selector: ".hero-tagline", translate: true },
  ],
});
```

Selector forms:

- Bare tag: `code` (matches `<code>`).
- Class: `.className` (matches an element whose `className` contains the name).
- Attribute presence: `[data-testid]`.
- Attribute value: `[role="alert"]`.

### `strict`

How the plugin handles missing translations in inline mode:

- `"warn"` (default): log a console warning, fall back to source text.
- `"error"`: throw a build error.
- `false`: silent, fall back to source text.

### `onWarning`

Override where unmapped-component warnings go. Defaults to `console.warn`.

```ts
neokapi({
  onWarning: (msg) => {
    logger.warn(msg);
    stats.increment("neokapi.warning");
  },
});
```

Use it to silence warnings in tests, or to send them to your project's logger.

### `warningsAsErrors`

Promote extraction-time warnings (currently: `unknown-component`) to a thrown build error. `strict` above governs missing translations in inline mode; `warningsAsErrors` governs the authoring issues the extractor records.

```ts
neokapi({
  warningsAsErrors: process.env.CI === "true",
});
```

Together with [`@neokapi/i18n-react-lint`](./linting), this keeps authoring mistakes off your main branch.

### `review` / `reviewKbfDir`

Turn on [in-context review](./in-context-review): the transform stamps `data-kapi-id` / `data-kapi-loc` / `data-kapi-attr` onto extracted elements, the dev server mounts the review middleware at `/__kapi/review` over `reviewKbfDir` (default `i18n`), and the overlay is injected into `index.html`.

```ts
neokapi({
  review: true, // or KAPI_REVIEW=1
  reviewKbfDir: "i18n",
});
```

Dev and staging only; never ship a production build with it on.

## CLI flags

`neokapi-i18n extract`:

```bash
neokapi-i18n extract \
  --src "src/**/*.{tsx,jsx}" \
  --ignore "src/stories/**" \
  --ignore "**/*.test.tsx" \
  --out i18n \
  --config i18n.config.json \
  --project my-app \
  --source-locale en \
  --target-locale fr \
  --target-locale de

# or stream mode for pipes into any kapi-aware consumer:
neokapi-i18n extract --stream | any-kapi-tool

# CI-friendly: fail on any recorded warning.
neokapi-i18n extract --strict

# Record every warning as JSON (kind, file, line, tag), for a check that
# compares the warnings with a baseline instead of failing on all of them.
neokapi-i18n extract --warnings-json build/extract-warnings.json
```

`--ignore` is repeatable and accepts any glob, which the CLI passes to
Node's `fs/promises.glob` `exclude` option. Use it to keep
fixture-only code (`src/stories/**`, test helpers) out of the catalog,
and exclude the same files in your lint config (see [Linting → Excluding fixture
code](./linting#excluding-fixture-code)).

`neokapi-i18n compile` (accepts a `.kbf.json` file, a directory of them, or `-` for NDJSON stdin):

```bash
neokapi-i18n compile \
  i18n/ \
  --out public/translations \
  --locale fr            # optional: filter to a single locale
```

`neokapi-i18n explain` (audit what extracts, and why):

```bash
neokapi-i18n explain src/Settings.tsx
neokapi-i18n explain "src/**/*.tsx" --extracted   # only the elements that extracted
```

```text
L3    <div>          [container] skipped: has block-level children (they extract separately)
L4    <h1>           [translatable] extracted  hash=cYEMc2v3JVx
L6    <code>         [non-translatable] skipped: classified non-translatable
L7    <input>        [container] skipped: no translator-editable text
        ↳ placeholder [attribute] extracted  hash=i42kuGUFbb4
```

Each line gives the W3C ITS classification, the rule that decided, and the block's hash. Run it when a string is missing from the catalog, or when one appears that you didn't expect.

`neokapi-i18n split` slices master dicts into per-chunk subsets for lazy loading; see [Lazy loading per route](./modes#lazy-loading-per-route-code-splitting).

### Share one config between the CLI and the plugin

The `componentMap` and `rules` feed the **hash**. If the extract CLI and the build plugin are configured differently, they compute different keys for the same element. Every affected string then falls back to source text, and no error reports it.

Keep one JSON file and have both read it:

```json title="neokapi-i18n.config.json"
{
  "componentMap": {
    "TabsTrigger": "button",
    "PageHeader": "header"
  },
  "rules": [{ "selector": "[data-testid]", "translate": false }]
}
```

```ts title="vite.config.ts"
import neokapiI18nConfig from "./neokapi-i18n.config.json";

neokapi({ mode: "runtime", ...neokapiI18nConfig });
```

```bash
neokapi-i18n extract --config neokapi-i18n.config.json
```

You choose the file name. The CLI reads it through `--config`, and the plugin takes the parsed object.

## Storybook integration

`@neokapi/i18n-react/storybook` exports a toolbar entry, a loader and a decorator for switching locales inside Storybook:

```ts title=".storybook/preview.ts"
import type { Preview } from "@storybook/react-vite";
import {
  neokapiDecorator,
  neokapiGlobalType,
  neokapiLoader,
} from "@neokapi/i18n-react/storybook";

const i18n = {
  locales: [
    { value: "en", title: "English" },
    { value: "fr", title: "French", url: "/translations/fr.json" },
    { value: "qps", title: "Pseudo", url: "/translations/qps.json" },
  ],
};

const preview: Preview = {
  globalTypes: {
    locale: neokapiGlobalType(i18n),
  },
  loaders: [neokapiLoader(i18n)],
  decorators: [neokapiDecorator(i18n)],
};

export default preview;
```

All three take the same options object. The loader fetches the catalog before the decorator renders the story. Storybook then runs the story's `play` function. Loading the catalog at this stage prevents a later catalog update from remounting the story and discarding the state produced by `play`.

And in `.storybook/main.ts`, enable the plugin so stories get the runtime transform:

```ts title=".storybook/main.ts"
import neokapi from "@neokapi/i18n-react/vite";

export default {
  stories: ["../src/**/*.stories.tsx"],
  async viteFinal(config) {
    config.plugins?.push(neokapi({ mode: "runtime" }));
    return config;
  },
};
```

A globe icon appears in the Storybook toolbar, and switching locale re-renders every story. Use it for design review, translation checks and RTL layout testing.

## HTML `lang` and `dir` attributes

`setTranslations()` and `loadTranslations()` set the locale on the document root:

```html
<!-- before -->
<html lang="en" dir="ltr">
  <!-- after loadTranslations("ar", …) -->
  <html lang="ar" dir="rtl"></html>
</html>
```

The runtime sets `dir="rtl"` for the common RTL primary subtags (`ar`, `dv`, `fa`, `he`, `ku`, `ps`, `sd`, `ur`, `yi`, and a few more), and `dir="ltr"` for everything else. Browsers use these attributes for hyphenation, spell checking and font fallback, and a screen reader uses `lang` to choose the language it reads in.

### Initial page load

Your `index.html` renders with the `lang` you hard-code, typically `en`. `loadTranslations()` resolves after the first paint, and the runtime then updates the attribute. A user on the default locale sees no change. A user whose language loads at boot sees a brief `en` → `<their-locale>` flip on first render. To avoid it, set `lang` on the server from the user's cookie or header before serving the HTML.

### Opting out

If your app manages `<html lang>` itself (SSR with preset lang, framework-owned locale routing, multi-locale surfaces on one page), pass `syncDocumentLocale: false`:

```ts
import { setTranslations } from "@neokapi/i18n-react/runtime";

setTranslations("ja", dict, { syncDocumentLocale: false });
// or:
await loadTranslations("ja", "/translations/ja.json", {
  syncDocumentLocale: false,
});
```

The option defaults to `true` when `document` is defined and to `false` otherwise, so `setTranslations` is safe to call from Node during SSR.

### Manual sync

When you need to push locale state without swapping the dict (e.g. your app has the dict inlined and you only want to set `<html lang>`), use `syncDocumentLocale` directly:

```ts
import { syncDocumentLocale } from "@neokapi/i18n-react/runtime";

syncDocumentLocale("fr");
```

### Custom RTL detection

For a different mapping, such as a rare script in one project or a custom pseudo-locale that should render RTL, manage `<html dir>` yourself with `syncDocumentLocale: false`:

```ts
setTranslations(locale, dict, { syncDocumentLocale: false });
document.documentElement.setAttribute("lang", locale);
document.documentElement.setAttribute("dir", myRTLPolicy(locale) ? "rtl" : "ltr");
```

## Opt-out and override patterns

### Per element

```tsx
<h1 translate="no">SDK_VERSION_4_2</h1>
```

### Per selector

```ts
rules: [
  { selector: ".monospace", translate: false },
  { selector: "[aria-hidden]", translate: false },
];
```

### Per attribute on a component

The convention props (`label`, `description`, `heading`, …) extract on PascalCase components only, so `<div label="draft-pending">` produces no block. If one of *your* components uses one of those names for an internal value (`description="internal-id"`), rename the prop, mark the element `translate="no"`, or exclude it with a `[selector]` rule.

### Per file (glob-based)

Use the CLI `--src` flag to scope extraction. The plugin still runs for the Vite build, but omitted files produce no `.kbf.json` entries.

## Debugging

### "I changed a string but translations still load the old text"

Changing the text changed the hash. Run `neokapi-i18n extract` and update the translation dict; a stale `.kbf.json` holds the old hashes.

### "My custom component's text isn't getting translated"

Run `neokapi-i18n explain <file>` to see the decision for every element in the file. The usual causes:

1. Does the component have direct JSXText children? The `<MyWidget>some text</MyWidget>` pattern auto-extracts with a warning.
2. Is the prop translatable *here*? HTML/ARIA names extract anywhere; convention names like `helpText` extract on PascalCase components only.
3. Is the text a JS variable? Use `t()`.

### "Warnings are flooding my console"

You're probably building Storybook or running tests with the plugin active. Route warnings to a logger with `onWarning` or turn the plugin off in those configs.

### "Hash mismatch between extract and transform"

The usual cause is a `componentMap` mismatch: the plugin and the CLI computed different keys because they were configured differently. [Share one config file](#share-one-config-between-the-cli-and-the-plugin) between them. `neokapi-i18n explain` prints the hash each element gets, so you can compare it against the `.kbf.json` directly.

### "A string renders in English in a pseudo build, but the component looks translatable"

Check these causes in order:

1. **A stale build.** The plugin re-reads a translation file whenever
   its mtime changes, so re-running `neokapi-i18n compile` while the
   dev server is up is enough. If a *code* change seems missing, Vite's
   dep cache holds the old module: stop the dev server and
   `rm -rf node_modules/.vite`.
2. **Linked workspace package**: your app's extract only walks
   its own `src/**` by default. A JSX string in a linked workspace
   package gets the runtime `__t()` rewrite (via Vite's plugin)
   but no extracted catalog entry, so the lookup falls back to
   source. Pass another `--src` glob for each package, or run
   each package's extract into a shared `i18n/` directory.
3. **Double-wrap detection**: see "Translated content shows `▒ ▒ … ▒ ▒`
   in pseudo" below.

### "Translated content shows `▒ ▒ … ▒ ▒` in pseudo"

Two translation layers are stacking. An inner `t()` call produces a
pseudo-translated string, and an outer element extracts its whole body,
that string included, as a block of its own and adds a second pair of
markers. Dynamic labels often do this:

```tsx
// meta.label is already a t()-resolved string from categoryMeta()
<Button>
  {meta.label} ({catTools.length})
</Button>
// pseudo: ▒ ▒ Utility ▒ (32) ▒   ← double wrap
```

Mark the outer element `translate="no"` so only the inner `t()` wraps:

```tsx
<Button translate="no">
  {meta.label} ({catTools.length})
</Button>
// pseudo: ▒ Utility ▒ (32)       ← single wrap
```

### "A `{placeholder}` name is rendering as `{ᴘʟᴀᴄᴇʜᴏʟᴅᴇʀ}` in pseudo"

kapi's pseudo-translate tool preserves `{…}` contents verbatim through
the accent transform, so a mangled name means the catalog is stale.
Regenerate it (typically `npm run extract && kapi pseudo-translate … && npm run
compile`, or whatever script your project wires up).

## Related

- [C-01 Kapi Project Model](/contribute/architecture/context/c-01-project-model): project layout and block store.
- [kapi CLI overview](/kapi/cli): translation commands that consume your `.kbf.json`.
