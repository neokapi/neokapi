---
sidebar_position: 7
title: Runtime vs. inline mode
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
  useNeokapi();   // subscribe
  return <Routes />;
}
```

`useNeokapi()` subscribes to the translation store via `useSyncExternalStore`. When the dict updates, any component that called the hook re-renders, and the cascade takes care of the rest.

For a locale switcher UI: call `loadTranslations` or `setTranslations("en", {})` on change, and make sure `useNeokapi()` is called high enough in the tree for the whole visible surface to re-render.

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
      strict: "error",   // fail the build on missing translations
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
// when the user switches, compiled from the same .klz.
```

Mixing modes within a single build is not supported — you pick one per deploy.

## Mode comparison

| | runtime | inline |
|---|---|---|
| Number of builds | 1 | 1 per locale |
| Runtime bundle cost | ~2 kB | 0 |
| Dict fetch at runtime | yes (per locale) | no |
| Missing translation | fallback to source text | warn or error at build |
| Hot-swap locale in-page | yes | full page reload / swap bundle |
| Best for | app shells, SaaS dashboards | marketing sites, SSR, SSG, locale-per-domain |

## What doesn't change between modes

- The extractor (`kapi-react extract`) produces the same `.klz` regardless of mode.
- Hashes are mode-independent.
- `<Plural>` / `<Select>` / `t()` all work the same in authoring.
- Warnings (auto-promotion, unmapped components) fire identically.

The mode decision is purely about how the translated output lands in the user's browser.

## Next

- [Translating with kapi](./translating-with-kapi) — AI translation, pseudo-translation, round-trip QA.
- [Configuration](./configuration) — componentMap, rules, Storybook, custom warning routing.
