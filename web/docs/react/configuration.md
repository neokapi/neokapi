---
sidebar_position: 11
title: neokapi-i18n Configuration Reference
description: "Full reference for the neokapi-i18n Vite plugin options: mode, locale, fallbackLocales, translationsDir, componentMap, Storybook integration, and CLI flags for extract and compile."
keywords: [neokapi-i18n, configuration, plugin options, Vite, locale, componentMap, CLI flags, Storybook]
---

# Configuration

The `neokapi(...)` plugin options, the `neokapi-i18n` CLI flags, and the ecosystem bits (Storybook, custom warning routing).

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
- Omitted: plugin is a no-op. Useful for dev mode (no extraction, source text renders as-is).

### `locale` (inline mode only)

The target locale. Drives which `translations/<locale>.json` file the plugin reads at build time.

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

Before consulting this option, the plugin **auto-resolves** mappings for every non-relative import it sees, in three stages:

1. **Library-shipped manifest**: `<package>/i18n-manifest.json`. This is the first-priority source and the pattern we recommend for library authors; see [Authoring i18n manifests](#authoring-i18n-manifests-for-libraries).
2. **Community manifest directory**: `<communityManifestDir>/<package-name>.json`, if you've configured one.
3. **`.d.ts` heuristic**: regex-match for `React.ForwardRefExoticComponent<... & RefAttributes<HTMLXxxElement>>` in the package's declared types. Picks up most pre-React-19 shadcn / Radix / MUI components for free.

Your `componentMap` entries merge on top of the auto-resolved map, so explicit overrides always win. The common case (using shadcn-style components from a library with proper types or a shipped manifest) needs no `componentMap` entry at all.

Unmapped components still auto-extract via the promotion rule, but each one fires a warning. Adding an entry silences the warning and re-keys the block's hash from `Component` to the underlying HTML tag.

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

The plugin loads this file automatically when any file imports from the library. See [`@neokapi/ui-primitives/i18n-manifest.json`](https://github.com/neokapi/neokapi/blob/main/packages/ui/i18n-manifest.json) for a production reference.

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

Useful for tests (suppress noise) or to integrate with a project logger.

### `warningsAsErrors`

Promote extraction-time warnings (currently: `unknown-component`) to a thrown build error. Orthogonal to `strict` above: `strict` is about missing translations at inline time, this is about authoring-time issues the walker records.

```ts
neokapi({
  warningsAsErrors: process.env.CI === "true",
});
```

Pair with [`@neokapi/i18n-react-lint`](./linting) to get a fully-enforced "no authoring mistakes land on main" story.

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
```

`--ignore` is repeatable and accepts any glob; it's piped through to
Node's `fs/promises.glob` `exclude` option. Use it to keep
fixture-only code (`src/stories/**`, test helpers) out of the catalog;
your lint config should agree (see [Linting → Excluding fixture
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
L3    <div>          [container] skipped — has block-level children (they extract separately)
L4    <h1>           [translatable] extracted  hash=cYEMc2v3JVx
L6    <code>         [non-translatable] skipped — classified non-translatable
L7    <input>        [container] skipped — no translator-editable text
        ↳ placeholder [attribute] extracted  hash=i42kuGUFbb4
```

Every line is the W3C ITS classification, the gate that fired, and the hash the block got. Reach for it when a string you expected didn't make the catalog, or one you didn't expect did.

`neokapi-i18n split` slices master dicts into per-chunk subsets for lazy loading; see [Lazy loading per route](./modes#lazy-loading-per-route-code-splitting).

### Share one config between the CLI and the plugin

The `componentMap` and `rules` feed the **hash**. If the extract CLI and the build plugin disagree about them, the two sides compute different keys for the same element, and every affected string silently falls back to source text, a failure with no error message anywhere.

So don't maintain two copies. Keep one JSON file and have both sides read it:

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

The file name is yours to choose: the CLI takes it via `--config` and the plugin just takes the object. What matters is that there is exactly one of them.

## Storybook integration

`@neokapi/i18n-react/storybook` exports a decorator and toolbar entry for switching locales inside Storybook:

```ts title=".storybook/preview.ts"
import type { Preview } from "@storybook/react-vite";
import { neokapiDecorator, neokapiGlobalType } from "@neokapi/i18n-react/storybook";

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
  decorators: [neokapiDecorator(i18n)],
};

export default preview;
```

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

A globe icon appears in the Storybook toolbar; switching locale re-renders every story. Useful for design review, translator QA, and RTL layout testing.

## HTML `lang` and `dir` attributes

`setTranslations()` and `loadTranslations()` push the locale onto the document root automatically:

```html
<!-- before -->
<html lang="en" dir="ltr">
  <!-- after loadTranslations("ar", …) -->
  <html lang="ar" dir="rtl"></html>
</html>
```

The runtime also swaps `dir="rtl"` for the common RTL primary subtags (`ar`, `dv`, `fa`, `he`, `ku`, `ps`, `sd`, `ur`, `yi`, and a few more). Everything else defaults to `dir="ltr"`. The attribute drives browser-level hyphenation, spelling, font fallbacks, and, above all, screen-reader language announcements.

### Initial page load

Your `index.html` renders with whatever `lang` you hard-code, typically `en`. When `loadTranslations()` resolves (async, happens after initial paint), the runtime syncs the attribute. A user on the default locale sees no flash; a user whose language is loaded at boot sees a very brief `en` → `<their-locale>` flip on first render. If that matters, set `lang` on the server to match the user's cookie / header before serving the HTML.

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

SSR is handled automatically: the option defaults to `true` when `document` is defined and `false` otherwise, so `setTranslations` is safe to call from Node.

### Manual sync

When you need to push locale state without swapping the dict (e.g. your app has the dict inlined and you only want to set `<html lang>`), use `syncDocumentLocale` directly:

```ts
import { syncDocumentLocale } from "@neokapi/i18n-react/runtime";

syncDocumentLocale("fr");
```

### Custom RTL detection

The built-in RTL set covers the common cases. If you need a different mapping (sparse script for a specific project, custom pseudo-locale that should render RTL, etc.), manage `<html dir>` yourself with `syncDocumentLocale: false`:

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

The convention props (`label`, `description`, `heading`, …) only extract on PascalCase components, so `<div label="draft-pending">` is already left alone. If one of *your* components reuses one of those names for something internal (`description="internal-id"`), rename the prop, mark the element `translate="no"`, or scope it out with a `[selector]` rule.

### Per file (glob-based)

Use the CLI `--src` flag to scope extraction. The plugin still runs for the Vite build, but omitted files produce no `.kbf.json` entries.

## Debugging

### "I changed a string but translations still load the old text"

Hash changed; run `neokapi-i18n extract` and update the translation dict. A stale `.kbf.json` means stale hashes.

### "My custom component's text isn't getting translated"

Run `neokapi-i18n explain <file>`: it prints the decision for every element on the page, so you rarely have to guess. The usual causes:

1. Does the component have direct JSXText children? The `<MyWidget>some text</MyWidget>` pattern auto-extracts with a warning.
2. Is the prop translatable *here*? HTML/ARIA names extract anywhere; convention names like `helpText` extract on PascalCase components only.
3. Is the text a JS variable? Use `t()`.

### "Warnings are flooding my console"

You're probably building Storybook or running tests with the plugin active. Route warnings to a logger with `onWarning` or turn the plugin off in those configs.

### "Hash mismatch between extract and transform"

Almost always a `componentMap` desync: the plugin and the CLI computed different keys because they were configured differently. [Share one config file](#share-one-config-between-the-cli-and-the-plugin) between them. `neokapi-i18n explain` prints the hash each element gets, so you can compare it against the `.kbf.json` directly.

### "A string renders in English in a pseudo build, but the component looks translatable"

Usually one of these three:

1. **A stale build, not a stale dict.** The plugin re-reads a
   translation file whenever its mtime changes, so re-running
   `neokapi-i18n compile` while the dev server is up is enough; there
   is no need to restart it. If a *code* change seems not to have
   landed, that's Vite's dep cache: kill the dev server and
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

Two translation layers stacking: an inner `t()` call produces a
pseudo-translated string, then an outer element wraps its whole body
(including that already-translated string) as its own block, adding a
second pair of markers. Common with dynamic label patterns:

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

## Next

- [C-01 Kapi Project Model](/contribute/architecture/context/c-01-project-model): project layout and block store.
- [kapi CLI overview](/kapi/cli): translation commands that consume your `.kbf.json`.
