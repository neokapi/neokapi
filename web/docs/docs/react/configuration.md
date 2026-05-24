---
sidebar_position: 11
title: Configuration
---

# Configuration

The `neokapi(...)` plugin options, the `kapi-react` CLI flags, and the ecosystem bits (Storybook, custom warning routing).

## Plugin options

```ts
import neokapi from "@neokapi/react/vite";

neokapi({
  mode: "runtime",
  locale: "fr",
  fallbackLocales: ["fr", "en"],
  translationsDir: "./translations",
  componentMap: { TabsTrigger: "button" },
  rules: [{ selector: ".hero-caption", translate: false }],
  strict: "warn",
  onWarning: (msg) => logger.warn(msg),
});
```

### `mode: "runtime" | "inline"`

See [Runtime vs. inline mode](./modes).

- `"runtime"` — one bundle, dict loaded at runtime.
- `"inline"` — one bundle per locale, translations inlined.
- Omitted — plugin is a no-op. Useful for dev mode (no extraction, source text renders as-is).

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

1. **Library-shipped manifest** — `<package>/i18n-manifest.json`. This is the first-priority source and the pattern we recommend for library authors; see [Authoring i18n manifests](#authoring-i18n-manifests-for-libraries).
2. **Community manifest directory** — `<communityManifestDir>/<package-name>.json`, if you've configured one.
3. **`.d.ts` heuristic** — regex-match for `React.ForwardRefExoticComponent<... & RefAttributes<HTMLXxxElement>>` in the package's declared types. Picks up most pre-React-19 shadcn / Radix / MUI components for free.

Your `componentMap` entries merge on top of the auto-resolved map, so explicit overrides always win. The common case — using shadcn-style components from a library with proper types or a shipped manifest — needs no `componentMap` entry at all.

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
- Values are the underlying HTML element name — or `null` to explicitly opt out of translation.
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

- `"warn"` (default) — log a console warning, fall back to source text.
- `"error"` — throw a build error.
- `false` — silent, fall back to source text.

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

Promote extraction-time warnings (currently: `unknown-component`) to a thrown build error. Orthogonal to `strict` above — `strict` is about missing translations at inline time, this is about authoring-time issues the walker records.

```ts
neokapi({
  warningsAsErrors: process.env.CI === "true",
});
```

Pair with [`@neokapi/kapi-react-lint`](./linting) to get a fully-enforced "no authoring mistakes land on main" story.

## CLI flags

`kapi-react extract`:

```bash
kapi-react extract \
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
kapi-react extract --stream | any-kapi-tool

# CI-friendly: fail on any recorded warning.
kapi-react extract --strict
```

`--ignore` is repeatable and accepts any glob; it's piped through to
Node's `fs/promises.glob` `exclude` option. Use it to keep
fixture-only code (`src/stories/**`, test helpers) out of the catalog
— your lint config should agree (see [Linting → Excluding fixture
code](./linting#excluding-fixture-code)).

`kapi-react compile` (accepts `.klf`, `.klf` directory, or `-` for NDJSON stdin):

```bash
kapi-react compile \
  i18n/ \
  --out public/translations \
  --locale fr            # optional — filter to a single locale
```

The extract CLI reads the same `componentMap` / `rules` from a JSON config file:

```json title="i18n.config.json"
{
  "componentMap": {
    "TabsTrigger": "button",
    "PageHeader": "header"
  },
  "rules": [{ "selector": "[data-testid]", "translate": false }]
}
```

Keep the Vite config and `i18n.config.json` in sync — both sides need the same map for hashes to align.

## Storybook integration

`@neokapi/react/storybook` exports a decorator and toolbar entry for switching locales inside Storybook:

```ts title=".storybook/preview.ts"
import type { Preview } from "@storybook/react-vite";
import { neokapiDecorator, neokapiGlobalType } from "@neokapi/react/storybook";

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
import neokapi from "@neokapi/react/vite";

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
  <!-- after loadTranslations("ar-SA", …) -->
  <html lang="ar-SA" dir="rtl"></html>
</html>
```

The runtime also swaps `dir="rtl"` for the common RTL primary subtags (`ar`, `dv`, `fa`, `he`, `ku`, `ps`, `sd`, `ur`, `yi`, and a few more). Everything else defaults to `dir="ltr"`. The attribute drives browser-level hyphenation, spelling, font fallbacks, and — most importantly — screen-reader language announcements.

### Initial page load

Your `index.html` renders with whatever `lang` you hard-code, typically `en`. When `loadTranslations()` resolves (async, happens after initial paint), the runtime syncs the attribute. A user on the default locale sees no flash; a user whose language is loaded at boot sees a very brief `en` → `<their-locale>` flip on first render. If that matters, set `lang` on the server to match the user's cookie / header before serving the HTML.

### Opting out

If your app manages `<html lang>` itself (SSR with preset lang, framework-owned locale routing, multi-locale surfaces on one page), pass `syncDocumentLocale: false`:

```ts
import { setTranslations } from "@neokapi/react/runtime";

setTranslations("ja-JP", dict, { syncDocumentLocale: false });
// or:
await loadTranslations("ja-JP", "/translations/ja-JP.json", {
  syncDocumentLocale: false,
});
```

SSR is handled automatically — the option defaults to `true` when `document` is defined and `false` otherwise, so `setTranslations` is safe to call from Node.

### Manual sync

When you need to push locale state without swapping the dict (e.g. your app has the dict inlined and you only want to set `<html lang>`), use `syncDocumentLocale` directly:

```ts
import { syncDocumentLocale } from "@neokapi/react/runtime";

syncDocumentLocale("fr-FR");
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

There's no built-in "don't translate this prop" — the assumption is that props in `translatableAttributes` always carry user-visible text. If you have a component that reuses one of those names for something internal (e.g. `description="internal-id"`), rename the prop or use a `[selector]` rule with a class.

### Per file (glob-based)

Use the CLI `--src` flag to scope extraction. The plugin still runs for the Vite build, but omitted files produce no `.klf` entries.

## Debugging

### "I changed a string but translations still load the old text"

Hash changed; run `kapi-react extract` and update the translation dict. A stale `.klf` means stale hashes.

### "My custom component's text isn't getting translated"

Check:

1. Does the component have direct JSXText children? The `<MyWidget>some text</MyWidget>` pattern auto-extracts with a warning.
2. Is the prop in `translatableAttributes`? `<MyWidget helpText="…" />` yes, `<MyWidget tooltipText="…" />` no (add it via rules or — if it's a convention — open an issue).
3. Is the text a JS variable? Use `t()`.

### "Warnings are flooding my console"

You're probably building Storybook or running tests with the plugin active. Route warnings to a logger with `onWarning` or turn the plugin off in those configs.

### "Hash mismatch between extract and transform"

Almost always a `componentMap` desync — the Vite plugin and the CLI must use the same map. Either point both at a shared JSON config (`--config i18n.config.json`) or share a TS module both import from.

### "A string renders in English in a pseudo build, but the component looks translatable"

Usually one of these three:

1. **Stale Vite dep cache** — the plugin got cached from before a
   change. Kill any running dev server and `rm -rf node_modules/.vite`
   before restarting.
2. **Linked workspace package** — your app's extract only walks
   its own `src/**` by default. A JSX string in a linked workspace
   package gets the runtime `__t()` rewrite (via Vite's plugin)
   but no extracted catalog entry, so the lookup falls back to
   source. Pass another `--src` glob for each package, or run
   each package's extract into a shared `i18n/` directory.
3. **Double-wrap detection** — see "Translated content shows `▒ ▒ … ▒ ▒`
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

Fixed in kapi's pseudo-translate tool; the accent transform preserves
`{…}` contents verbatim. Regenerate the catalog to pick up the fix
(typically `npm run extract && kapi pseudo-translate … && npm run
compile`, or whatever script your project wires up).

## Next

- [AD-008 Kapi Project Model](/contribute/architecture/008-project-model) — project layout and block store.
- [kapi CLI overview](/cli/overview) — translation commands that consume your `.klf`.
