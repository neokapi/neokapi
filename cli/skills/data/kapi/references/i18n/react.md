# React / Next.js i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **kapi-react** | **T0** | Plain JSX is the catalog; bundler extracts at build time. The greenfield default. |
| next-intl | T2 | The Next.js App Router standard; IDs + hand-authored catalogs, best RSC ergonomics. |
| Lingui | T2 | Source-as-key macros + PO catalogs; lowest marking cost of the runtime libs. |
| react-i18next | T2 | The incumbent; biggest ecosystem, key hygiene forever. |
| next-i18next | T2 | Only for existing i18next investments on Next.js (v16 revived it). |
| react-intl | **T3 — warn** | Maximum boilerplate + hash-ID churn; right only with an ICU/TMS mandate. |

**Opinionated defaults:** greenfield React (Vite/webpack) → **kapi-react**.
Next.js App Router with no i18n yet → **next-intl** (kapi-react works via the
webpack hook, but next-intl is the ecosystem-native choice — offer both).
Team wants source-as-key with a classic library → **Lingui**. Existing
library already installed → keep it, install its recommended config below.
Never convert a stack to a different key model in the same change as adoption.

## Path A — adopt @neokapi/kapi-react (zero-wrapper, T0)

Write plain JSX; a bundler plugin extracts it at build time. See the
[kapi-react quickstart](https://neokapi.github.io/web/neokapi/react/quickstart).

**Change as little as possible:**

- **Leave translatable JSX text exactly as written.** The plugin extracts plain
  text nodes (`<h1>Your notes</h1>`, `<p>Welcome back, {name}.</p>`) at build
  time. Do **not** wrap JSX text in `t()` — that's the whole point.
- **Use `t("…")` only for strings the plugin can't see as JSX** — text held in
  variables/data arrays, `alert()`/toast messages, and translatable attributes
  (`placeholder`, `aria-label`, `title`, `alt`). The English source is the key.
  **Don't eyeball which strings these are — the linter finds them (step 4).**
  They render reactively only inside `<NeokapiProvider>`, so wrap the app.
- **The source English text is the key — never invent message IDs.**
- The adoption diff should be: config (plugin, provider, scripts),
  `"use client"` on components that render translated text, and `t()` on the
  handful of non-JSX strings — nothing more. If you're rewriting component
  bodies, you're doing too much.

The translation pipeline (step 5) is **CLI-driven** — `kapi-react` and `kapi`
run on the source files; you don't need to build or run the app to translate,
and there is **no `.kapi` project**, so project-scoped `kapi check --ship` does not
apply — the readiness gate is `kapi pseudo-translate`. The npm install, bundler
plugin, and runtime loader only wire compiled translations into the running
app; add them as config and don't block translation on them.

1. Install: `npm install -D @neokapi/kapi-react` (+ `kapi` CLI on PATH).
2. Bundler plugin (Vite shown; webpack/rollup/esbuild also exported):
   ```ts
   import neokapi from "@neokapi/kapi-react/vite";
   export default defineConfig({ plugins: [neokapi({ mode: "runtime" }), react()] });
   ```
3. Scripts in `package.json`:
   ```json
   { "scripts": {
       "extract": "kapi-react extract",
       "compile": "kapi-react compile out/ --out public/translations"
   } }
   ```
4. **Run the linter to find the strings the extractor can't see.**
   `@neokapi/kapi-react-lint` (oxlint or ESLint 8.57+/9/10) flags exactly the
   escape-hatch cases: user-facing strings in data arrays (`{ title: 'Q3 plan' }`
   rendered as `{title}`), translatable attributes, bare `{'literal'}` JSX
   expressions. Use the **strict** preset for the adoption pass, autofix what it
   can, then wrap the rest in `t()`:
   ```jsonc
   // .oxlintrc.json  → { "jsPlugins": ["@neokapi/kapi-react-lint/oxlint"], "rules": { … } }
   // or eslint.config.js → import { recommendedStrict } from "@neokapi/kapi-react-lint/eslint";
   ```
   ```bash
   npx oxlint --fix          # autofixes bare {'literal'} JSX; reports the rest
   ```
   After this pass every user-facing string is either plain JSX (auto-extracted)
   or `t()`-wrapped. Keep the linter on in CI — it is what holds the W=0 grade.
5. Translate the extracted KLF with the `kapi` CLI:
   ```bash
   npm run extract                                   # JSX + t() calls → source KLF in i18n/
   kapi pseudo-translate i18n/ --target-lang qps     # readiness check first
   kapi translate i18n/ --target-lang fr             # translated KLF → out/ (brand-aware if a profile is bound)
   npm run compile                                   # compile out/ → public/translations
   ```
   `kapi translate <dir>` writes translated `.klf` to `out/`, so `compile`
   reads `out/`, not the source `i18n/`.
6. Load at runtime: in Vite,
   `await loadTranslations("fr", "/translations/fr.json")` in `main.tsx`
   before render. Next.js App Router: see below.

**Next.js App Router:** the runtime is client-side, so any component rendering
translated JSX must be a Client Component (`"use client"`). Add the plugin via
the `webpack` hook:

```js
import neokapi from "@neokapi/kapi-react/webpack";
export default { webpack: (config) => { config.plugins.push(neokapi({ mode: "runtime", translationsDir: "./public/translations" })); return config; } };
```

Wrap the app in `<NeokapiProvider>` and gate the first paint on the dictionary
load (a flash of source language means the gate is missing):

```tsx
"use client";
import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { NeokapiProvider, loadTranslations } from "@neokapi/kapi-react/runtime";

function Gate({ children }: { children: React.ReactNode }) {
  const lang = useSearchParams().get("lang") ?? "en";
  const [ready, setReady] = useState(lang === "en"); // English is the source — no load needed
  useEffect(() => {
    if (lang === "en") return;
    loadTranslations(lang, `/translations/${lang}.json`).finally(() => setReady(true));
  }, [lang]);
  if (!ready) return null; // or a spinner — never the un-translated frame
  return <NeokapiProvider>{children}</NeokapiProvider>;
}

export default function I18nProvider({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={null}><Gate>{children}</Gate></Suspense>;
}
```

Wrap the app in `<I18nProvider>` in `app/layout.tsx`. (The locale can come from
a route segment or cookie the same way.)

## Path B — plug kapi into an existing React library

For repeatable runs scaffold a project (`kapi init --framework <preset>
--target-locale <l>`); for one-offs translate the catalog file directly. Always
pseudo-translate first, and verify the target locale *renders*.

### next-intl — T2 (recommended for Next.js App Router)

- **Idiom:** namespaced dot-key IDs, hand-authored `messages/en.json`. Keep the
  IDs — its docs recommend them; its extraction (`useExtracted`) is still
  experimental.
- **Recommended config (what earns T2):** shallow feature-scoped namespaces;
  pass only needed namespaces to `NextIntlClientProvider`;
  [lingualdev/i18n-check](https://github.com/lingualdev/i18n-check) in CI —
  missing keys otherwise surface only at runtime as rendered key paths.
- **kapi:** preset `nextjs`
  (`kapi init --framework nextjs --target-locale fr`), or per file:
  ```bash
  kapi translate messages/en.json --target-lang fr -o messages/fr.json --json
  kapi run translate-qa -i messages/en.json --target-lang de --json
  ```
- **Footguns:** Next.js majors break its middleware/routing wiring (Next 16's
  middleware→proxy rename did); solo maintainer; message payloads ship to the
  client unless namespaces are scoped.

### Lingui — T2 (recommended when the team wants source-as-key)

- **Idiom:** `<Trans>Hello {name}</Trans>` / `` t`Hello` `` macros; generated
  hash IDs from source text. Don't fight it with explicit IDs.
- **Recommended config:** `lingui extract` in CI with a dirty-catalog check;
  `--clean` only on release branches; PO catalog format (the default — best
  TMS/editor support); Babel macro plugin if your Next.js upgrade cadence is
  aggressive (the SWC plugin pins to exact Next versions).
- **kapi:** no preset; kapi reads PO natively:
  ```bash
  kapi pseudo-translate src/locales/en/messages.po --target-lang qps
  kapi translate src/locales/en/messages.po --target-lang fr -o src/locales/fr/messages.po
  ```
- **Footguns:** `@lingui/swc-plugin` ↔ Next.js version pinning; v6 is ESM-only
  (Node ≥22); obsolete messages purge with `--clean` — review those diffs.

### react-i18next — T2 (the incumbent)

- **Idiom:** stable feature-scoped IDs + `defaultValue`, namespaced JSON in
  `public/locales/{lng}/{ns}.json`. Natural-language keys are possible but the
  maintainers recommend against them.
- **Recommended config:** the official **i18next-cli** (`extract --ci`,
  `sync`, `status`, `types`) — **i18next-parser is deprecated**; few coarse
  namespaces; `saveMissing` in development only; typed keys via
  `CustomTypeOptions`.
- **kapi:** preset `react-i18next`; pass `--format i18next` so plural-suffix
  keys (`key_one`/`key_other`) and `{{interpolation}}`/`$t()` nesting are
  grouped and protected:
  ```bash
  kapi translate public/locales/en/translation.json --format i18next \
    --target-lang fr -o public/locales/fr/translation.json --json
  ```
- **Footguns:** Suspense flicker with `useTranslation`; TS slowdown on huge
  catalogs (use the selector API); no RSC support in react-i18next itself —
  App Router users need next-i18next v16.

### next-i18next — T2 (existing i18next investments only)

v16 (2026-03) finally added App Router support: `getT()` in Server
Components, `useT()` in Client Components, `createProxy()` for Next 16. Choose
it only when i18next catalogs/TMS wiring already exist — the App Router
surface is young (score r=2); greenfield Next.js goes to next-intl. Same
kapi integration and CLI tooling as react-i18next.

### react-intl (FormatJS) — T3, warn before adopting

Every string is `<FormattedMessage defaultMessage=…/>` or
`intl.formatMessage(…)` — the most verbose mainstream option (W=3, which caps
it at T3 no matter what tooling is added) — and extractor hash IDs churn on
every copy edit, orphaning translations unless a TM-capable TMS absorbs it.
Choose it only with an existing FormatJS/ICU mandate and a TMS. If adopting:
never hand-write IDs, wire `formatjs extract`/`compile` into CI immediately.
kapi: preset `react-intl` (`src/lang/{lang}.json`). Tell the user the toil
story and name Lingui as the same-ICU, lower-toil alternative.

## Verify (all paths)

Run the app in the target locale (e.g. `/?lang=ja` or the route/cookie the app
uses) and confirm the visible UI is translated. Still in source language? The
wiring is wrong, not the translation — usual causes: string not extracted
(check the catalog and compiled output), rendering component is a Server
Component (needs `"use client"` for client-side runtimes), or the dictionary
loads after first paint (gate the render). In a `.kapi` project, finish with a
green `kapi check --ship`.
