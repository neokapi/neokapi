# Svelte / Astro / Solid i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| Paraglide JS | T1 | Compiled typed message functions; the default across SvelteKit, Astro, and Solid. |
| svelte-i18n | T3 — avoid | Dormant since early 2024; migrate to Paraglide. |
| astro-i18next | T4 — avoid | Dead since 2023; never start on it. |
| @solid-primitives/i18n | T3 — caution | Pleasant minimalist primitives; extraction/sync/routing are all DIY. |

**Opinionated defaults:** SvelteKit → **Paraglide** via the official
`npx sv add paraglide` — it is what the Svelte CLI itself scaffolds, and
svelte-i18n has been dormant since early 2024. Astro → **built-in i18n routing
+ Paraglide** for UI strings (the docs-recipe `ui.ts` dictionary is fine for
tiny sites with <10 strings). Solid: small SPA → `@solid-primitives/i18n`;
SolidStart SSR or more than 2 locales → **Paraglide**; runtime/CMS-driven
catalogs → solid-i18next. Never start new work on svelte-i18n or
astro-i18next.

## Paraglide JS — T1 (recommended; the ecosystem default)

- **Idiom:** messages authored as plain JSON in `messages/{lang}.json` (inlang
  message format, configured by `project.inlang/settings.json`); a compiler
  turns them into **per-message typed functions** —
  `import { m } from '$lib/paraglide/messages'; m.hello({ name })`. There is
  no runtime dictionary: unused messages are tree-shaken away and a **missing
  message is a compile error**, which is why extraction and sync toil are zero
  and the grade is T1.
- **Recommended config (what earns T1):**
  - **SvelteKit: scaffold with `npx sv add paraglide`** — the official Svelte
    CLI add-on wires the inlang settings, Vite plugin, hooks-based locale
    resolution, and `lang`/`dir` attributes. Astro/Solid use the same
    framework-agnostic Vite plugin (v2); the old per-framework v1 adapters
    (`@inlang/paraglide-sveltekit`, `paraglide-astro`) are deprecated —
    migrate off them if found.
  - URL locale strategy for SEO-relevant sites; cookie strategy for apps.
  - **v2 variants** for plurals/gender (built on `Intl.PluralRules`); ICU
    MessageFormat 1 is available via an inlang plugin if the team needs it.
  - **kapi pseudo-translate + translate-qa on `messages/*.json`** (the
    registry's `buys`) — the compiler proves presence, not correctness; kapi
    covers the runtime-correctness axis.
- **kapi:** no preset — translate the message files directly, then rerun the
  build so the compiler regenerates the functions:
  ```bash
  kapi pseudo-translate messages/en.json --target-lang qps   # readiness first
  kapi translate messages/en.json --target-lang fr -o messages/fr.json
  kapi run translate-qa -i messages/en.json --target-lang fr --json
  ```
  Add the new locale to `project.inlang/settings.json` (`locales`) or the
  compile fails to pick it up.
- **Footguns:** **static keys only** — `m[runtimeKey]` defeats the model;
  apps with CMS/runtime-driven strings need a runtime library instead. All
  locales of a *used* message are bundled together — efficient under ~20
  locales, a real cost beyond that. The steward is Opral, a startup that
  already deprecated every v1 framework adapter in the 2.0 move — expect API
  churn appetite. Variants are younger and less battle-tested than
  formatjs/ICU.

## svelte-i18n — T3, avoid: dormant

**Warn the user explicitly before touching this:** svelte-i18n is dormant —
last release v4.0.1 in early 2024, single inactive maintainer, 60+ open issues
(Svelte 5/runes-era reports sit unanswered). It still works, but adopting it
in 2026 is betting on abandonware; Paraglide is the migration target.

- Its one enduring technical advantage is full ICU MessageFormat at runtime
  (built on formatjs) — if that is genuinely required with runtime-loaded
  catalogs, i18next + a Svelte wrapper is the safer runtime pick.
- Known traps while it remains installed: locale is a global store, so SSR
  misuse leaks locale across requests; the ICU parser ships in the client
  bundle; async dictionaries need `$isLoading` gating.
- **Migration path to Paraglide:** `npx sv add paraglide`; move each
  `{lang}.json` dictionary into `messages/{lang}.json` (flatten nested keys —
  `page.title` → `page_title`); convert ICU plural strings to Paraglide v2
  variants (or the ICU MF1 inlang plugin); replace `$_('page.title')` /
  `$t(...)` with `m.page_title()`; delete `register()`/`init()`/`$isLoading`
  wiring. The compiler then reports every miss as a type error — that is the
  migration checklist.
- **kapi:** no preset. While migrating, existing `src/lib/i18n/{lang}.json`
  dictionaries can still be translated per file
  (`kapi translate src/lib/i18n/en.json --target-lang fr -o src/lib/i18n/fr.json`).

## Astro

- **Built-in i18n routing** (Astro 4+, carried into 5): `i18n: { locales,
  defaultLocale, routing, fallback }` plus `astro:i18n` helpers
  (`getRelativeLocaleUrl` etc.). It handles **URLs, fallbacks, and detection
  only — there is no message-catalog system**, so pair it with one:
  - **Anything real:** built-in routing + **Paraglide** via the unified Vite
    plugin — messages are tree-shaken per island, so only client-component
    messages ship JS. Same kapi workflow as above (`messages/{lang}.json`).
  - **Tiny sites** (<10 UI strings, few locales): the official docs-recipe
    `ui.ts` dictionary + `useTranslations()` helper — zero dependencies.
  - Markdown-heavy content is per-locale content collections: the translation
    unit is per-locale content files, not catalogs.
- **astro-i18next — T4, avoid. Warn the user explicitly:** it is dead — last
  release 1.0.0-beta.21 (March 2023), never reached 1.0, unresponsive to
  Astro 4/5 issues. Do not start on it; migrate to Paraglide or the recipe.
  During migration kapi can still translate its leftover catalogs:
  `kapi translate public/locales/en/translation.json --format i18next
  --target-lang fr -o public/locales/fr/translation.json`.

## Solid / SolidStart

SolidStart has no first-party i18n; the community standard is
`@solid-primitives/i18n`.

- **@solid-primitives/i18n — T3, caution. Tell the user before adopting:**
  it is minimalist *by design* — composable primitives (`flatten`,
  `translator`, `resolveTemplate`) over plain JSON dictionaries, with **no
  extractor, no ICU, no routing**; the whole workflow (finding strings, sync,
  SEO) is DIY, which is what keeps it at T3. Name Paraglide as the lower-toil
  alternative, and prefer it outright for SolidStart SSR or more than 2
  locales.
- **Recommended config (what earns T3 rather than worse):** maintained by
  solidjs-community; type safety is inferred from the base dictionary (keys
  and params — quite good); lazy per-locale loading via `createResource`;
  locale switching through `startTransition`. Plurals: roll your own or embed
  functions in the dictionary. The registry's `buys` is **kapi convergence
  over the JSON dictionaries** — with no extractor, kapi's catalog diffing is
  the only drift detection you get.
- **solid-i18next** when runtime/CMS-driven catalogs are needed: the full
  i18next ecosystem (ICU plugin, backends, detection) at i18next runtime
  cost; translate its catalogs with `--format i18next`.
- **kapi:** no preset — translate the dictionaries per file:
  ```bash
  kapi pseudo-translate src/i18n/en.json --target-lang qps
  kapi translate src/i18n/en.json --target-lang fr -o src/i18n/fr.json
  kapi run translate-qa -i src/i18n/en.json --target-lang fr --json
  ```

## Verify (all paths)

Pseudo-translate first — render the `qps` locale and walk the UI: any English
that survives is a hardcoded string no compiler will catch. Then run the app
in the target locale (URL strategy: the prefixed route; cookie strategy: set
it) and confirm the visible UI is translated. Paraglide showing source
language usually means the compile step didn't rerun after kapi wrote the
catalog, or the locale is missing from `project.inlang/settings.json`; runtime
libs showing source language usually means the dictionary never loaded or the
key fell back silently. In a `.kapi` project, finish with a green
`kapi check --ship`.
