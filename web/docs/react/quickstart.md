---
sidebar_position: 2
title: Quick start
description: "Add neokapi-i18n to a Vite + React project in about five minutes: install the plugin, write plain JSX, run kapi pseudo-translate, and flip between locales from a toolbar."
keywords: [neokapi-i18n, quick start, Vite, React, install, pseudo-translate, i18n setup]
---

# Quick start

In about five minutes you'll have a Vite + React app that switches between English and pseudo-English from a language picker. You write the JSX as usual, and the build finds the text to translate.

## 1. Install

```bash
npm install -D @neokapi/i18n-react
```

The package ships a build plugin (Vite, Rollup, webpack, Rspack, esbuild), the `extract` / `compile` / `split` / `explain` CLI subcommands, and the runtime (~2 kB). No peer dependencies beyond React 18+.

The [`kapi` CLI](/kapi/cli) fills the KBF directory that neokapi-i18n extracts with translations. This guide uses it for pseudo-translation. Install it too:

```bash
# macOS / Linux
brew install neokapi/tap/kapi-cli
```

On Windows, take a build from the
[direct downloads](/kapi/get-started/installation#binary-downloads). The WinGet
package is behind the current release.

Use `@neokapi/i18n-react` 3 with kapi 1.3.0 or later. Both read and write the
KBF schema 2 catalogs that carry each string's source and translations together.
An older kapi cannot read the catalogs `@neokapi/i18n-react` 3 extracts, and an
older `@neokapi/i18n-react` compiles none of the translations a current kapi
writes. `kapi inspect` on a catalog exits with an error when the installed kapi
cannot read it.

## 2. Add the plugin to `vite.config.ts`

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import neokapi from "@neokapi/i18n-react/vite";

export default defineConfig({
  plugins: [
    neokapi({ mode: "runtime" }), // ← add this
    react(),
  ],
});
```

Two modes are available; pick `runtime` for now:

- `runtime`: ship one bundle; load a translation dict at runtime via `fetch`. Good for apps that ship many locales from a CDN.
- `inline`: produce one bundle per locale with translations pre-inlined. Zero runtime lookup, fastest first paint.

## 3. Write JSX as you normally would

```tsx title="src/App.tsx"
export default function App() {
  return (
    <main>
      <h1>Welcome to Acme</h1>
      <p>Ship your product in every language your users speak.</p>
      <button>Get started</button>
    </main>
  );
}
```

There are no `t(...)` calls and no keys. The plugin walks the JSX at build time and rewrites each translatable site to a hash-based lookup.

## 4. Extract to a KBF directory

Wire the extractor and the compiler into your package scripts:

```json title="package.json"
{
  "scripts": {
    "extract": "vp neokapi-i18n extract",
    "compile": "vp neokapi-i18n compile i18n/ --out public/translations"
  }
}
```

The `vp` prefix is the Vite+ runner, used throughout these pages. A project without it runs `neokapi-i18n extract` directly: the dev dependency puts the binary on the script's `PATH`.

Run extract:

```bash
npm run extract
```

Output:

```
Scanning 1 files...
Extracted 3 blocks from 1 files → i18n/
```

`i18n/` is a directory carrying one `.kbf.json` document per source file, mirroring
your source tree (e.g. `i18n/src/App.kbf.json`). The three blocks are
"Welcome to Acme", the paragraph, and "Get started". Each file is plain JSON, as
the suffix says, so your editor, `jq` and GitHub read it with no setup and its
diffs stay readable.

## 5. Pseudo-translate with `kapi`

Pseudo-translation produces accented strings such as `▒ Wëlcömé tö Âcmé ▒`. Every extracted string changes, so a string that still reads as plain English is one the extractor missed.

```bash
kapi pseudo-translate i18n/
```

## 6. Compile to a runtime dict

`neokapi-i18n compile` turns the translated KBF into a `{locale}.json` file per locale:

```bash
npm run compile
```

Output:

```
Compiled 3 entries → public/translations/qps.json
```

The JSON is `{ "<hash>": "<flattened target text>" }`.

## 7. Load the translation at runtime

Two lines in your app bootstrap:

```tsx title="src/main.tsx"
import { loadTranslations } from "@neokapi/i18n-react/runtime";
import ReactDOM from "react-dom/client";
import App from "./App";

async function bootstrap() {
  await loadTranslations("qps", "/translations/qps.json").catch(() => {});
  ReactDOM.createRoot(document.getElementById("root")!).render(<App />);
}

void bootstrap();
```

`loadTranslations(locale, url)` fetches the dict and activates it. After it resolves, every rendered `<h1>Welcome to Acme</h1>` renders as `▒ Wëlcömé tö Âcmé ▒`. To build a [fallback chain](./modes#fallback-chain), such as `pt-BR` over `pt`, pass an array of URLs instead of one.

## 8. Add a language switcher (optional)

A 10-line language picker wired to `setTranslations` / `loadTranslations`:

```tsx
import { loadTranslations, setTranslations, useNeokapi } from "@neokapi/i18n-react/runtime";

export function LocaleSwitcher() {
  useNeokapi(); // subscribe so the component re-renders on locale change

  return (
    <select
      onChange={async (e) => {
        const value = e.target.value;
        if (value === "en") setTranslations("en", {});
        else await loadTranslations(value, `/translations/${value}.json`);
      }}
    >
      <option value="en">English</option>
      <option value="qps">Pseudo-English</option>
    </select>
  );
}
```

`useNeokapi()` subscribes the component to neokapi-i18n's translation store, so a locale change re-renders it and its subtree without a navigation.

## What each step did

1. **You wrote plain JSX**, with no wrappers.
2. **The plugin** found every translatable element at build time, computed stable hashes, and rewrote the JSX to look them up at render time.
3. **kapi** added `qps` targets to the KBF catalogs.
4. **neokapi-i18n compile** turned the catalogs into a JSON dict your app loads.
5. **The runtime** resolved each hash on render. An unknown hash renders the JSX source text, so the app never shows a raw identifier.

## Related

- [Writing translatable components](./writing-components): what neokapi-i18n picks up automatically, what it warns about, and how to opt out.
- [Plurals and select](./plurals-and-select): CLDR-aware plural authoring without ICU strings in your source.
- [Extract → translate → compile pipeline](./pipeline): AI translation, incremental extracts, CI integration.
- [`t()` escape hatch](./t-escape-hatch): for the strings that genuinely belong outside JSX.
