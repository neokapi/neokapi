---
sidebar_position: 2
title: Quick start
description: Add kapi-react to a Vite + React project in about five minutes — install the plugin, write plain JSX, run kapi pseudo-translate, and flip between locales from a toolbar.
keywords: [kapi-react, quick start, Vite, React, install, pseudo-translate, i18n setup]
---

# Quick start

Add kapi-react to a Vite + React project. ~5 minutes; you'll finish with a running app that flips between English and pseudo-English from a toolbar.

## 1. Install

```bash
npm install -D @neokapi/kapi-react
```

The package ships a Vite plugin, extract + compile CLI subcommands, and the tiny runtime (~2 kB). No peer dependencies beyond React 18+.

The [`kapi` CLI](/kapi/cli) is the translation pipeline that produces pseudo-translations from the KLF directory kapi-react extracts. Install it too:

```bash
# macOS
brew install neokapi/tap/kapi-cli

# Linux / Windows — download a release from github.com/neokapi/neokapi/releases
```

## 2. Add the plugin to `vite.config.ts`

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import neokapi from "@neokapi/kapi-react/vite";

export default defineConfig({
  plugins: [
    neokapi({ mode: "runtime" }), // ← add this
    react(),
  ],
});
```

Two modes are available — pick `runtime` for now:

- `runtime` — ship one bundle; load a translation dict at runtime via `fetch`. Good for apps that ship many locales from a CDN.
- `inline` — produce one bundle per locale with translations pre-inlined. Zero runtime lookup, fastest first paint.

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

No `t(...)` calls, no keys. The plugin walks the JSX at build time and rewrites each translatable site to a hash-based lookup.

## 4. Extract to a KLF directory

Wire the extractor + pack into your package scripts:

```json title="package.json"
{
  "scripts": {
    "extract": "vp kapi-react extract",
    "compile": "vp kapi-react compile i18n/ --out public/translations"
  }
}
```

Run extract:

```bash
npm run extract
```

Output:

```
Scanning 1 files...
Extracted 3 blocks from 1 files → i18n/
```

`i18n/` is a directory carrying one `.klf` document per source file, mirroring
your source tree (e.g. `i18n/src/App.klf`). The three blocks are
"Welcome to Acme", the paragraph, and "Get started". Each `.klf` is plain JSON —
human-readable and git-diffable.

## 5. Pseudo-translate with `kapi`

Pseudo-translation generates `[Wëlcömé tö Âcmé]`-style accented strings that make it obvious what's been picked up for translation — and which strings are still English. Perfect first pass.

```bash
kapi pseudo-translate i18n/ --target-lang qps
```

## 6. Compile to a runtime dict

`kapi-react compile` turns the translated KLF into a `{locale}.json` file per locale:

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
import { loadTranslations } from "@neokapi/kapi-react/runtime";
import ReactDOM from "react-dom/client";
import App from "./App";

async function bootstrap() {
  await loadTranslations("qps", "/translations/qps.json").catch(() => {});
  ReactDOM.createRoot(document.getElementById("root")!).render(<App />);
}

void bootstrap();
```

`loadTranslations(locale, url)` fetches the dict and activates it. After it resolves, every rendered `<h1>Welcome to Acme</h1>` renders as `[Wëlcömé tö Âcmé]`.

## 8. Add a language switcher (optional)

A 10-line language picker wired to `setTranslations` / `loadTranslations`:

```tsx
import { loadTranslations, setTranslations, useNeokapi } from "@neokapi/kapi-react/runtime";

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

`useNeokapi()` wires the root of your tree into kapi-react's translation store so a locale change re-renders the whole subscribed subtree — no navigation required.

## What just happened

- **Zero wrappers** — you wrote normal JSX.
- **Plugin extracted** every translatable element at build time, computed stable hashes, and rewrote the JSX to look them up at render time.
- **kapi pseudo-translated** the KLF → another KLF with `qps` targets populated.
- **kapi-react compiled** that KLF to a JSON dict your app loads.
- **The runtime** resolved each hash on render; unknown hashes fall back to the JSX source text, so the app never shows raw identifiers.

## Next steps

- [Writing translatable components](./writing-components) — what kapi-react picks up automatically, what it warns about, and how to opt out.
- [Plurals and select](./plurals-and-select) — CLDR-aware plural authoring without ICU strings in your source.
- [Extract → translate → compile pipeline](./pipeline) — AI translation, incremental extracts, CI integration.
- [`t()` escape hatch](./t-escape-hatch) — for the strings that genuinely belong outside JSX.
