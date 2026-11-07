---
sidebar_position: 8
title: Runtime vs. Inline Mode
description: "neokapi-i18n has two production modes: runtime mode (one bundle, translations loaded at runtime via fetch) and inline mode (one bundle per locale, translations baked in at build time). Choose based on how you ship."
keywords: [runtime mode, inline mode, bundle, neokapi-i18n, production, i18n modes, code splitting]
---

import BrowserOnly from "@docusaurus/BrowserOnly";
import { PseudoModeExplorer } from "@site/src/components/react/PseudoModeExplorer";
import { PipelineDiagram } from "@neokapi/docs-shared";

# Runtime vs. inline mode

The mode decides how translated text reaches the browser. Runtime mode ships one bundle and fetches a dict for the active locale, and inline mode builds one bundle per locale with the translated text already in it. Pick the one that matches how you deploy.

## Runtime mode

```ts
neokapi({ mode: "runtime" });
```

You ship one bundle, and the app fetches a per-locale JSON file at runtime.

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "Your JSX", role: "io" },
    { label: "One bundle", sub: "__t(hash) lookups", role: "tool" },
    { label: "fetch", sub: "{locale}.json", role: "translate" },
    { label: "Rendered page", sub: "any locale", role: "io" },
  ]}
  caption="Runtime mode: one build, and the dict for the active locale loads in the browser."
/>

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
- You have more than a handful of locales; the per-locale JSON is a small download compared to your JS bundle.
- You want to switch locale in the page (language picker, A/B).

### Load once, subscribe to changes

```tsx
import { loadTranslations, setTranslations, useNeokapi } from "@neokapi/i18n-react/runtime";

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

`useNeokapi()` subscribes to the translation store via `useSyncExternalStore`. When the dict updates, every component that called the hook re-renders, and React re-renders its children as usual.

For a locale switcher UI: call `loadTranslations` or `setTranslations("en", {})` on change, and make sure `useNeokapi()` is called high enough in the tree for the whole visible surface to re-render.

To skip that decision, wrap the tree in `<NeokapiProvider>`. It subscribes for you and re-keys its children on every locale change, so the whole subtree remounts and re-reads the dict:

```tsx
import { NeokapiProvider } from "@neokapi/i18n-react/runtime";

ReactDOM.createRoot(root).render(
  <NeokapiProvider>
    <App />
  </NeokapiProvider>,
);
```

A remount discards the component state below it. That is usually fine on a locale switch and usually wrong in the middle of a form. `useNeokapi()` re-renders only the components that subscribe, and `NeokapiProvider` reaches every component in the tree.

### Fallback chain

`loadTranslations` takes a list of URLs as well as a single one. The list is a fallback chain: fallbacks first, primary last:

```tsx
await loadTranslations("pt-BR", ["/translations/pt.json", "/translations/pt-BR.json"]);
```

Everything merges into one dict, with later entries overriding earlier ones, so `pt-BR` wins where it has a string and `pt` fills the rest. If a fallback fails to load, you get the strings that did load. If the **primary** fails, the call rejects, so you see the error.

`loadTranslations` and `setTranslations` also set `<html lang>` and `<html dir>` to the new locale, which screen readers, fonts, hyphenation and RTL layout depend on. Opt out with `{ syncDocumentLocale: false }` if your app owns those attributes. Details: [Configuration → HTML `lang` and `dir`](./configuration#html-lang-and-dir-attributes).

### Lazy loading per route (code splitting)

In a larger app, one catalog per locale means the user downloads every string, including those for routes they never visit. The plugin and the runtime can split translations along the same lines the bundler splits code:

1. In runtime mode, the plugin emits `translations-manifest.json` next to your JS chunks, a `{chunkName: hashes[]}` map of which strings each chunk needs. Vite, Rollup, webpack, Rspack, and esbuild all emit it (esbuild needs `metafile: true`).
2. `neokapi-i18n split` slices each master `{locale}.json` into per-chunk subsets (`{locale}/{chunkName}.json`), duplicating strings shared across chunks so each file is independently loadable.
3. The runtime's `loadTranslationChunk(locale, url)` fetches one subset and merges it into the active dict. Concurrent requests for the same `(locale, url)` pair share a single fetch.

Wire it into a React Router lazy route:

```tsx
import { loadTranslationChunk } from "@neokapi/i18n-react/runtime";

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
neokapi-i18n compile i18n/ --out public/translations
neokapi-i18n split \
  --manifest dist/translations-manifest.json \
  --locales public/translations \
  --out dist/translations
```

A missing hash renders the source text baked into its `__t` / `__tx` call at build time, so a late chunk never breaks the render. The user sees English for about 100ms while the chunk loads.

With `merge: true`, `setTranslations` and `loadTranslations` merge the incoming entries into the existing dict instead of replacing it. `loadTranslationChunk` uses this internally. Switching locale without `merge` drops any in-flight chunk loads for the previous locale, so their payloads can't overwrite entries in the new dict.

### Runtime pseudo-translation

Runtime mode can pseudo-translate **on the fly**, with no build step and no catalog. Use it during development to check layout and to see which strings pass through translation:

```tsx
import { setPseudoMode } from "@neokapi/i18n-react/runtime/pseudo";

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

The transform applies on top of the runtime dict. You can load a real French catalog, then turn pseudo on to see French at +30% length, with markers that show which strings were translated and which fell back to source. `{param}` / `{=m0}` tokens are preserved verbatim, so param substitution still works.

Text inside a `<code>`, a `<kbd>`, a `<samp>` or a `<var>` comes through as the author wrote it, the same bytes a `kapi pseudo-translate` catalog carries. `Run <code>kapi check --ship</code> first.` reads as accented prose around a command that still runs. A `translate="yes"` on the span claims its text back for the transform. Padding skips those spans too, because a command keeps its length in a real translation.

**Works without a catalog.** The source string lands in the `__t` / `__tx` call as the `fallback` argument at build time. When the dict is empty the runtime uses the fallback, and pseudo transforms it. Edit `<h1>Welcome</h1>` → save → HMR replaces the module → React re-renders → `"▒ Ŵéļçöḿé ▒"`. You need no extract, compile or rebuild step. The only prerequisite is `neokapi({ mode: "runtime" })` in `vite.config.ts`; without runtime mode the plugin does nothing, and there is no `__t` call for pseudo to transform.

The panel below runs the real neokapi-i18n runtime in your browser, with no catalog loaded. Toggle pseudo mode and the same strings flip to accented, expanded text; `{name}` stays literal because `{param}` tokens are preserved through the transform:

<BrowserOnly>{() => <PseudoModeExplorer />}</BrowserOnly>

To make pseudo the default in development, call it at the top of `main.tsx`, guarded on `import.meta.env.DEV`, and keep a console handle for tuning:

```tsx
import { setPseudoMode } from "@neokapi/i18n-react/runtime/pseudo";

if (import.meta.env.DEV) {
  setPseudoMode({ expansion: 30 });
  // @ts-expect-error: dev-only global so you can re-tune from the console
  window.setPseudoMode = setPseudoMode;
}
```

The pseudo module has its own subpath (`@neokapi/i18n-react/runtime/pseudo`), so you import it only when you want it, and the main runtime stays ~2 kB. It is built on `setStringTransform`, a post-lookup hook that the main runtime also exports for custom transforms such as debug markers or letter-spacing audits.

A custom transform takes the message text and a context. The context carries `markers`, one entry per inline element that answers for the text inside it: `"no"` for a span holding a command, a key or an identifier, `"yes"` where the source claimed it back. `protectionMask(text, markers)`, exported from the runtime, turns those answers into a per-character array so a transform can rewrite the prose and leave the rest:

```ts
import { setStringTransform, protectionMask } from "@neokapi/i18n-react/runtime";

setStringTransform((text, context) => {
  const guarded = protectionMask(text, context?.markers);
  return [...text].map((ch, i) => (guarded?.[i] ? ch : ch.toUpperCase())).join("");
});
```

## Inline mode

```ts
neokapi({
  mode: "inline",
  locale: "fr",
  translationsDir: "./translations",
});
```

You build one bundle **per locale**, and the build replaces every JSX text node with the translated literal. There is no runtime dict lookup, subscription or `loadTranslations()` call:

<PipelineDiagram
  channelLabel=""
  stages={[
    { label: "Your JSX", role: "io" },
    { label: "Build per locale", sub: "translations/fr.json", role: "translate" },
    { label: "Bundle for fr", sub: "text inlined", role: "tool" },
    { label: "Rendered page", sub: "fr only", role: "io" },
  ]}
  caption="Inline mode: one build per locale, with the translated text written into each bundle."
/>

```tsx
// Source
<h1>Welcome</h1>

// Output (inline mode, locale=fr)
<h1>Bienvenue</h1>
```

Rich text is rebuilt as JSX, so a translator is free to move the link, and the markup follows:

```tsx
// Source
<p>Read the <a href="/docs">docs</a> first.</p>

// Output (inline mode, locale=de): the link moved, the href stayed
<p>Lies zuerst die <a href="/docs">Dokumentation</a>.</p>
```

### The ICU exception

Inline mode is zero-runtime with one documented exception: **`<Plural>`, `<Select>`, and any string a translator gave an ICU format to keep a runtime call.**

A plural's pivot (the count) is known only when the component renders, so no build step can pick the form. Inline mode does the half it can: the translated ICU template is baked into the call as its fallback, so there's no dictionary to fetch and no network on the critical path. What stays is the ~2 kB ICU resolver.

If you have no plurals and no ICU formats, inline mode ships nothing. If you have one, you ship the resolver. Either way the output is valid JSX, and the string renders in the target language.

### When inline mode fits

- You ship per-locale builds (`www-fr.example.com`, `www-de.example.com`).
- You care about first-paint bundle size: no runtime, no dict fetch.
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

`strict: "error"` turns missing translations into a build error: nothing untranslated ships. For a market-by-market rollout, keep `strict: "warn"` (the default) during development and switch to `"error"` for the release build.

### Build-time fallback chain

Inline mode has its own fallback chain, resolved at build rather than at load (the runtime-mode equivalent is [`loadTranslations` with a URL list](#fallback-chain)). When a translation is missing in the primary locale, it consults fallback locales before giving up:

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
// when the user switches, compiled from the same KBF directory.
```

A single build uses one mode, and you choose the mode per deploy.

## Mode comparison

|                         | runtime                     | inline                                       |
| ----------------------- | --------------------------- | -------------------------------------------- |
| Number of builds        | 1                           | 1 per locale                                 |
| Runtime bundle cost     | ~2 kB                       | 0, or ~2 kB if you use ICU / plurals         |
| Dict fetch at runtime   | yes (per locale)            | no                                           |
| Missing translation     | fallback to source text     | warn or error at build                       |
| Hot-swap locale in-page | yes                         | full page reload / swap bundle               |
| Best for                | app shells, SaaS dashboards | marketing sites, SSR, SSG, locale-per-domain |

A translation that is present but unsound counts as missing too: `neokapi-i18n compile` leaves out any target that does not carry every placeholder its source has, so in runtime mode that entry falls back to source text until the next translation pass repairs it.

## What doesn't change between modes

- The extractor (`neokapi-i18n extract`) produces the same `.kbf.json` regardless of mode.
- Hashes are mode-independent.
- `<Plural>` / `<Select>` / `t()` all work the same in authoring.
- Unmapped-component warnings fire identically.

## Related

- [Translating with kapi](./translating-with-kapi): AI translation, pseudo-translation, round-trip checks.
- [Configuration](./configuration): componentMap, rules, Storybook, custom warning routing.
