---
sidebar_position: 8
title: Runtime vs. Inline Mode
description: kapi-react has two production modes — runtime mode (one bundle, translations loaded at runtime via fetch) and inline mode (one bundle per locale, translations baked in at build time). Choose based on how you ship.
keywords: [runtime mode, inline mode, bundle, kapi-react, production, i18n modes, code splitting]
---

# Runtime vs. inline mode

The plugin has two production modes. Pick based on how you ship your app.

## Runtime mode

```ts
neokapi({ mode: "runtime" });
```

One bundle. Translations live in per-locale JSON files loaded at runtime via `fetch`.

Every translatable JSX site gets rewritten to a `__t(hash, fallback, params)` call:

```tsx
// Source
<h1>Welcome</h1>

// Output (runtime mode)
<h1>{__t("aB3", "Welcome")}</h1>
```

The runtime is ~2 kB gzipped; it holds the active dict and a subscriber set. When `loadTranslations(locale, url)` resolves, every subscribed component re-renders.

### When runtime mode fits

- You ship a single JS bundle to a CDN and flip locales based on user preference.
- You want to add new locales without re-deploying your JS.
- You have more than a handful of locales — the per-locale JSON is a small download compared to your JS bundle.
- You care about hot-swapping locale in-page (language picker, A/B).

### Load once, subscribe to changes

```tsx
import { loadTranslations, setTranslations, useNeokapi } from "@neokapi/kapi-react/runtime";

async function bootstrap() {
  const locale = navigator.language.split("-")[0];
  if (locale !== "en") {
    await loadTranslations(locale, `/translations/${locale}.json`).catch(() => {});
  }
  ReactDOM.createRoot(root).render(<App />);
}

// Somewhere in the tree that should re-render on locale change
function AppRoot() {
  useNeokapi(); // subscribe
  return <Routes />;
}
```

`useNeokapi()` subscribes to the translation store via `useSyncExternalStore`. When the dict updates, any component that called the hook re-renders, and the cascade takes care of the rest.

For a locale switcher UI: call `loadTranslations` or `setTranslations("en", {})` on change, and make sure `useNeokapi()` is called high enough in the tree for the whole visible surface to re-render.

Both also push the new locale onto `<html lang>` and `<html dir>` automatically — handy for screen readers, fonts, hyphenation, and RTL support. Opt out with `{ syncDocumentLocale: false }` if your app owns those attributes. Details: [Configuration → HTML `lang` and `dir`](./configuration#html-lang-and-dir-attributes).

### Lazy loading per route (code splitting)

For larger apps, the single-catalog-per-locale model downloads every string even for routes the user never visits. The plugin + runtime can split translations along the same lines the bundler splits code:

1. In runtime mode, the Vite/Rollup plugin emits `translations-manifest.json` next to your JS chunks — a `{chunkName: hashes[]}` map of which strings each chunk needs.
2. `kapi-react split` slices each master `{locale}.json` into per-chunk subsets (`{locale}/{chunkName}.json`), duplicating strings shared across chunks so each file is independently loadable.
3. The runtime's `loadTranslationChunk(locale, url)` fetches one subset and merges it into the active dict. Concurrent requests for the same `(locale, url)` pair share a single fetch.

Wire it into a React Router lazy route:

```tsx
import { loadTranslationChunk } from "@neokapi/kapi-react/runtime";

const routes = [
  {
    path: "/settings",
    lazy: async () => {
      const [mod] = await Promise.all([
        import("./SettingsPage"),
        loadTranslationChunk(locale, `/translations/${locale}/SettingsPage.json`),
      ]);
      return { Component: mod.default };
    },
  },
];
```

Build pipeline:

```bash
vite build                                       # emits dist/translations-manifest.json
kapi-react compile i18n/ --out public/translations
kapi-react split \
  --manifest dist/translations-manifest.json \
  --locales public/translations \
  --out dist/translations
```

Missing hashes fall back to the source text baked into each `__t` / `__tx` call at build time — a late-arriving chunk is never fatal. Users see English for ~100ms while the chunk streams in, not a broken render.

If `merge: true` is passed to `setTranslations` or `loadTranslations`, the incoming entries OR into the existing dict instead of replacing it. `loadTranslationChunk` uses this internally. Switching locale (without `merge`) drops any in-flight chunk loads for the previous locale so their payloads can't poison the new dict.

### Runtime pseudo-translation

Runtime mode can apply pseudo-translation **on the fly**, no build step, no catalog — useful for dev ergonomics, layout QA, and debugging which strings flow through the translation system:

```tsx
import { setPseudoMode } from "@neokapi/kapi-react/runtime/pseudo";

// Turn on with defaults (▒-wrapped, accented)
setPseudoMode({});

// Tune
setPseudoMode({
  prefix: "« ",
  suffix: " »",
  expansion: 30, // +30% padding to test layout
  alphabet: "accented", // ASCII → accented variants (the default)
});

// Off
setPseudoMode(null);
```

The transform stacks on top of whatever's in the runtime dict — so you can load a real French catalog and THEN flip pseudo on to see what French looks like at +30% length, with markers showing which strings got translated vs. which fell through to source. `{param}` / `{=m0}` tokens are preserved verbatim so param substitution still works.

**Works without a catalog.** The source string lands in the `__t` / `__tx` call as the `fallback` argument at build time. When the dict is empty the runtime uses the fallback, and pseudo transforms it. Edit `<h1>Welcome</h1>` → save → HMR replaces the module → React re-renders → `"▒ Ŵéļçöḿé ▒"`. No extract step, no compile step, no rebuild — just your source text flowing through the transform. A plain `neokapi({ mode: "runtime" })` in `vite.config.ts` is the only prerequisite; without runtime mode the plugin no-ops and there's no `__t` wrapper for pseudo to hook into.

If you want pseudo to be the default in dev, wire it at the top of `main.tsx` guarded on `import.meta.env.DEV`, then keep the dev console handle available for tuning:

```tsx
import { setPseudoMode } from "@neokapi/kapi-react/runtime/pseudo";

if (import.meta.env.DEV) {
  setPseudoMode({ expansion: 30 });
  // @ts-expect-error — dev-only global so you can re-tune from the console
  window.setPseudoMode = setPseudoMode;
}
```

The pseudo module lives at a separate subpath (`@neokapi/kapi-react/runtime/pseudo`) so importing it is opt-in — the main runtime stays ~2 kB. Internally it uses `setStringTransform`, a general post-lookup hook also exported from the main runtime for custom transforms (debug markers, letter-spacing audits, etc.).

## Inline mode

```ts
neokapi({
  mode: "inline",
  locale: "fr",
  translationsDir: "./translations",
});
```

One bundle **per locale**. Every JSX text node is replaced at build time with the translated literal. No runtime dict lookup, no subscription, no `loadTranslations()`:

```tsx
// Source
<h1>Welcome</h1>

// Output (inline mode, locale=fr)
<h1>Bienvenue</h1>
```

### When inline mode fits

- You ship per-locale builds (`www-fr.example.com`, `www-de.example.com`).
- You care about first-paint bundle size — no runtime, no dict fetch.
- You have SSR / SSG and want pre-rendered HTML in the target locale.
- Your locale set is small and rarely changes.

### Typical inline setup

```bash
# Build one app per locale in CI
for locale in en fr de ja; do
  LOCALE=$locale vite build --outDir dist/$locale
done
```

```ts title="vite.config.ts"
export default defineConfig({
  plugins: [
    neokapi({
      mode: "inline",
      locale: process.env.LOCALE ?? "en",
      translationsDir: "./translations",
      strict: "error", // fail the build on missing translations
    }),
    react(),
  ],
});
```

`strict: "error"` turns missing translations into a build error — nothing untranslated ships. For markets-by-market rollouts you'd keep `strict: "warn"` (default) during development, flip to `"error"` before the final release build.

### Fallback chain

When a translation is missing in the primary locale, inline mode can consult fallback locales before giving up:

```ts
neokapi({
  mode: "inline",
  locale: "de-AT",
  fallbackLocales: ["de", "en"],
});
```

For the Austrian-German build, a missing `de-AT` entry falls back to `de`, then to `en`, then to the source text.

## Hybrid: inline core, runtime optional locales

A common pattern is to inline the primary locale at build time (so the default market loads instantly) and make secondary locales available via the runtime dict:

```ts
// Primary build
neokapi({ mode: "inline", locale: "en" });

// Secondary locales still get an OTA dict loaded via loadTranslations
// when the user switches, compiled from the same KLF directory.
```

Mixing modes within a single build is not supported — you pick one per deploy.

## Mode comparison

|                         | runtime                     | inline                                       |
| ----------------------- | --------------------------- | -------------------------------------------- |
| Number of builds        | 1                           | 1 per locale                                 |
| Runtime bundle cost     | ~2 kB                       | 0                                            |
| Dict fetch at runtime   | yes (per locale)            | no                                           |
| Missing translation     | fallback to source text     | warn or error at build                       |
| Hot-swap locale in-page | yes                         | full page reload / swap bundle               |
| Best for                | app shells, SaaS dashboards | marketing sites, SSR, SSG, locale-per-domain |

## What doesn't change between modes

- The extractor (`kapi-react extract`) produces the same `.klf` regardless of mode.
- Hashes are mode-independent.
- `<Plural>` / `<Select>` / `t()` all work the same in authoring.
- Unmapped-component warnings fire identically.

The mode decision is purely about how the translated output lands in the user's browser.

## Next

- [Translating with kapi](./translating-with-kapi) — AI translation, pseudo-translation, round-trip QA.
- [Configuration](./configuration) — componentMap, rules, Storybook, custom warning routing.
