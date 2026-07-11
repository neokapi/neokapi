# Vue / Nuxt i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| vue-i18n | T2 | The Vue 3 standard; IDs + hand-authored catalogs, Composition API only. |
| @nuxtjs/i18n | T2 | The only sane default on Nuxt; owns routing/SEO/detection, inherits vue-i18n's message toil. |

**Opinionated defaults:** plain Vue 3 → **vue-i18n** (v11+, Composition API).
Nuxt → **@nuxtjs/i18n** (v10) — never hand-roll vue-i18n inside Nuxt; the
module owns routing, detection, and SEO. Both are T2 because keys are invented
by hand and there is no first-party extractor — the lint stack below is what
holds the grade, so install it in the same change as adoption. Never adopt the
Legacy (Options) API in new code: it is deprecated in v11 and removed in v12.

## vue-i18n — T2 (recommended for plain Vue 3)

- **Idiom:** message IDs (nested JSON keys like `home.title`), hand-authored
  `src/locales/{lang}.json`; `t('key')` from `useI18n()` in setup code,
  `{{ $t('key') }}` in templates. Keep the ID model — source-text-as-key is
  not this stack's idiom. Do not use the `v-t` directive (deprecated in v11,
  removed in v12).
- **Recommended config (what earns T2):**
  - **Composition API only.** New marking goes through `useI18n()`; Legacy
    API code is a rewrite you budget separately, not something to extend.
  - **`@intlify/unplugin-vue-i18n` is mandatory, not optional** (registry
    `buys`): it precompiles catalogs at build time so the message compiler
    stays out of the client bundle (perf) and the app is CSP-safe — the full
    build's runtime compiler historically required `unsafe-eval`. Alias to the
    runtime-only build; the plugin's `include` glob defines the catalog layout.
  - **`eslint-plugin-vue-i18n` in CI with `no-raw-text` + `no-missing-keys`**
    (registry `buys`): with no extractor, lint is the only thing that catches
    unmarked strings and dangling keys before runtime.
  - **Schema-typed `useI18n`:** type the resource schema so keys get
    completion and checking — it is off by default; turn it on.
- **kapi:** preset `vue-i18n`
  (`kapi init --framework vue-i18n --target-locale fr`), or per file:
  ```bash
  kapi pseudo-translate src/locales/en.json --target-lang qps   # readiness first
  kapi translate src/locales/en.json --target-lang fr -o src/locales/fr.json
  kapi run translate-qa -i src/locales/en.json --target-lang fr --json
  ```
- **Footguns:** messages use the **Intlify format, not ICU** — plurals are
  pipe syntax (`no apples | one apple | {count} apples`) with named `{param}`
  interpolation and linked messages; kapi preserves these placeholders, but
  don't paste ICU `{count, plural, …}` syntax into catalogs. v9/v10 hit EOL
  July 2025 — be on v11 (npm may show a spurious "deprecated" notice on v11;
  that was a v9/v10 deprecation mistake, not real). Renamed keys silently
  orphan translations — rename via lint-verified refactors. Messages fetched
  from an API at runtime still need the message compiler (JIT since v9.3
  avoids `unsafe-eval`). Vanilla Vue SSR needs a per-request i18n instance or
  locales leak across requests — on Nuxt, use the module instead.

## @nuxtjs/i18n — T2 (recommended on Nuxt)

- **Idiom:** same message-ID model as vue-i18n (it wraps vue-i18n v11), with
  per-locale files under `i18n/locales/` declared as
  `locales: [{ code, file }]`, plus localized URLs — use `<NuxtLinkLocale>` /
  `localePath()` for links, never hardcoded paths.
- **Recommended config (what earns T2):**
  - **v10** (built for Nuxt 4): locale detection/redirects run in Nitro server
    middleware, which fixes the historical hydration/detection edge cases.
  - **`strategy: 'prefix_except_default'`** for URL structure, and **lazy
    per-locale files** so only the active locale ships.
  - **`useLocaleHead()`** for `hreflang`/canonical SEO tags (or the
    experimental `strictSeo` mode which manages them for you).
  - **`experimental.typedOptionsAndMessages`** for typed keys — note it
    currently gives autocomplete without strict errors (a nonexistent key
    still type-checks) and requires Nuxt's `experimental.typedPages`.
  - **Same lint stack as vue-i18n** (registry `buys`):
    `eslint-plugin-vue-i18n` `no-raw-text` + `no-missing-keys` in CI.
- **kapi:** **no preset** — translate the per-locale files directly:
  ```bash
  kapi pseudo-translate i18n/locales/en.json --target-lang qps
  kapi translate i18n/locales/en.json --target-lang fr -o i18n/locales/fr.json
  kapi run translate-qa -i i18n/locales/en.json --target-lang fr --json
  ```
  Then add the new locale to `locales: [{ code: 'fr', file: 'fr.json' }]` in
  the module config — kapi writes the catalog; the module won't see it until
  it is declared.
- **Footguns:** every major migration has broken something — v8→v9 moved the
  directory layout under `i18n/`, v9→v10 changed redirect behavior
  (`prefix` + `redirectOn: 'root'` no longer redirects non-root paths) and
  deprecated Legacy API mode. Your vue-i18n major is chosen for you by the
  module version. Plurals are Intlify format, not ICU (inherited).

## Verify (both stacks)

Pseudo-translate before real translation — `qps` output rendering in the app
is the readiness gate that catches hardcoded strings the lint rules missed.
Then run the app in the target locale (plain Vue: however the app selects
locale; Nuxt: the prefixed route, e.g. `/fr/`) and confirm the visible UI is
translated. Still in source language? The wiring is wrong, not the
translation — usual causes: key missing from the target catalog (vue-i18n
falls back to the source locale silently — this is why `no-missing-keys` runs
in CI), the locale file not declared in `locales:` (Nuxt), or the unplugin
`include` glob not matching the new file. In a `.kapi` project, finish with a
green `kapi check --ship`.
