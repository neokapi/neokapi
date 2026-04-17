---
sidebar_position: 9
title: Configuration
---

# Configuration

The `neokapi(...)` plugin options, the `kapi-react` CLI flags, and the ecosystem bits (Storybook, custom warning routing).

## Plugin options

```ts
import neokapi from "@neokapi/kapi-react/vite";

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
    // shadcn / Radix wrappers
    TabsTrigger: "button",
    TabsList: "div",
    DialogTitle: "h2",
    DialogDescription: "p",

    // Internal components
    PageHeader: "header",
    Heading: "h2",
  },
});
```

Unmapped components still auto-extract via the promotion rule, but each one fires a warning. Adding the entry silences the warning and re-keys the block's hash from `Component` to the underlying HTML tag.

### `rules`

Declarative overrides keyed on selectors:

```ts
neokapi({
  rules: [
    // Turn translation off for specific matches
    { selector: ".code-block",    translate: false },
    { selector: "[data-testid]",  translate: false },

    // Attach a translator note
    { selector: ".legal-copy",    locNote: "Must match legal-approved wording" },

    // Turn translation on for a container that wouldn't normally auto-promote
    { selector: ".hero-tagline",  translate: true },
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

Override where auto-promotion and unmapped-component warnings go. Defaults to `console.warn`.

```ts
neokapi({
  onWarning: (msg) => {
    logger.warn(msg);
    stats.increment("neokapi.warning");
  },
});
```

Useful for tests (suppress noise) or to integrate with a project logger.

## CLI flags

`kapi-react extract`:

```bash
kapi-react extract \
  --src "src/**/*.{tsx,jsx}" \
  --out i18n/extracted.klz \
  --config i18n.config.json \
  --project my-app \
  --source-locale en \
  --target-locale fr \
  --target-locale de
```

`kapi-react compile`:

```bash
kapi-react compile \
  i18n/translated.klz \
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
  "rules": [
    { "selector": "[data-testid]", "translate": false }
  ]
}
```

Keep the Vite config and `i18n.config.json` in sync — both sides need the same map for hashes to align.

## Storybook integration

`@neokapi/kapi-react/storybook` exports a decorator and toolbar entry for switching locales inside Storybook:

```ts title=".storybook/preview.ts"
import type { Preview } from "@storybook/react-vite";
import { neokapiDecorator, neokapiGlobalType } from "@neokapi/kapi-react/storybook";

const i18n = {
  locales: [
    { value: "en",  title: "English" },
    { value: "fr",  title: "French",  url: "/translations/fr.json" },
    { value: "qps", title: "Pseudo",  url: "/translations/qps.json" },
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
import neokapi from "@neokapi/kapi-react/vite";

export default {
  stories: ["../src/**/*.stories.tsx"],
  async viteFinal(config) {
    config.plugins?.push(neokapi({ mode: "runtime" }));
    return config;
  },
};
```

A globe icon appears in the Storybook toolbar; switching locale re-renders every story. Useful for design review, translator QA, and RTL layout testing.

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
]
```

### Per attribute on a component

There's no built-in "don't translate this prop" — the assumption is that props in `translatableAttributes` always carry user-visible text. If you have a component that reuses one of those names for something internal (e.g. `description="internal-id"`), rename the prop or use a `[selector]` rule with a class.

### Per file (glob-based)

Use the CLI `--src` flag to scope extraction. The plugin still runs for the Vite build, but omitted files produce no `.klz` entries.

## Debugging

### "I changed a string but translations still load the old text"

Hash changed; run `kapi-react extract` and update the translation dict. A stale `.klz` means stale hashes.

### "My custom component's text isn't getting translated"

Check:

1. Does the component have direct JSXText children? The `<MyWidget>some text</MyWidget>` pattern auto-extracts with a warning.
2. Is the prop in `translatableAttributes`? `<MyWidget helpText="…" />` yes, `<MyWidget tooltipText="…" />` no (add it via rules or — if it's a convention — open an issue).
3. Is the text a JS variable? Use `t()`.

### "Warnings are flooding my console"

You're probably building Storybook or running tests with the plugin active. Route warnings to a logger with `onWarning` or turn the plugin off in those configs.

### "Hash mismatch between extract and transform"

Almost always a `componentMap` desync — the Vite plugin and the CLI must use the same map. Either point both at a shared JSON config (`--config i18n.config.json`) or share a TS module both import from.

## Next

- [AD-045 KLF/KLZ format](/docs/ad/045-klf-klz-spec) — the on-disk schema, for TMS integrators.
- [kapi CLI overview](/docs/kapi-cli/overview) — translation commands that consume your `.klz`.
