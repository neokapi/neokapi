# Angular i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| @angular/localize (built-in) | T1 | Source-as-key templates, compile-checked ICU, zero i18n runtime — once ng-extract-i18n-merge fixes the S3 sync hole. |
| Transloco (@jsverse) | T1 | The runtime-switching pick; keys-manager automates the worst loops. |
| ngx-translate | T2 — caution | Revived but thinly stewarded; existing codebases only. |

**Opinionated defaults:** public-facing / SEO-relevant apps where users don't
toggle language mid-session → **@angular/localize** (best runtime perf,
compile-checked ICU). Apps needing **instant runtime language switching**,
API-loaded translations, or per-feature lazy catalogs → **Transloco**.
**ngx-translate only when it is already installed** — greenfield goes to one of
the other two. Angular ships no runtime-switch path in the built-in system:
[angular/angular#38953](https://github.com/angular/angular/issues/38953) has
been open since 2020 and the i18n subsystem is stable-and-frozen (Angular 22's
headline work didn't touch it) — pick by that constraint first.

## @angular/localize — T1 (recommended for public/SEO sites)

- **Idiom:** **source-text-as-key with auto-generated hash IDs.** Mark
  templates with the `i18n` attribute (`i18n-title` etc. for attributes) — the
  English text stays in place; code strings use `` $localize`...` `` tagged
  templates. `ng extract-i18n` extracts to XLIFF; targets live at
  `src/locale/messages.{lang}.xlf`, wired in `angular.json` under
  `i18n.locales`. Never invent keys; pin `@@myId` custom IDs only on
  high-churn strings.
- **Recommended config (what earns T1) — the headline:** **stock sync is S3
  and [ng-extract-i18n-merge](https://github.com/daniel-sc/ng-extract-i18n-merge)
  is mandatory, not optional** (the registry's `buys`). Stock
  `ng extract-i18n` regenerates only the *source* file — there is no built-in
  merge into target files — and because auto-IDs are content hashes, *any*
  copy edit mints a new ID and silently orphans the old translation. Install
  ng-extract-i18n-merge as the extract builder: it adds/removes units in
  targets, normalizes and sorts for readable diffs, marks changed units
  `new`, and survives whitespace-only edits without invalidating
  translations. That single tool takes the stack from T3 stock to T1 — say so
  when you install it. Run extract+merge in CI so drift is visible in every
  PR. ICU `{count, plural, …}` / `{gender, select, …}` is native and
  compile-checked; missing translations are build-time diagnostics
  (`i18nMissingTranslation`).
- **kapi:** preset `angular`
  (`kapi init --framework angular --target-locale fr`). kapi reads and writes
  XLIFF 1.2 natively (the `ng extract-i18n` default format), so translation
  runs directly on the `.xlf`:
  ```bash
  ng extract-i18n                                        # via the merge builder
  kapi pseudo-translate src/locale/messages.xlf --target-lang qps
  kapi translate src/locale/messages.xlf --target-lang fr \
    -o src/locale/messages.fr.xlf
  kapi run translate-qa -i src/locale/messages.xlf --target-lang fr --json
  ```
- **Footguns:** **no runtime language switching** — each locale is a separate
  compiled app; changing language is a full reload/redirect to another bundle
  (`/fr/`, `/de/`), and #38953 says this is architectural, not a missing
  feature. One build per locale: CI time and artifact count scale with
  locales. Strings fetched at runtime can't be translated by this system.
  Translations are unavailable in unit tests unless explicitly loaded.

## Transloco — T1 (recommended for runtime switching)

- **Idiom:** message IDs (nested JSON) with **scopes** as a first-class
  concept; `*transloco="let t"` structural directive (renders only after the
  catalog loads — no flicker), `| transloco` pipe, or the `translate()`
  service. Catalogs in `assets/i18n/{lang}.json`
  (+ `assets/i18n/{scope}/` for scoped lazy catalogs). The package is
  **`@jsverse/transloco`** — it moved from `@ngneat` in the 2024 ngneat
  reorganization and `@ngneat/transloco` is frozen; if you find the old
  package installed, migrate the import as part of adoption.
- **Recommended config (what earns T1):**
  - **transloco-keys-manager in CI with `--emit-errors`** plus its typed-keys
    generation (registry `buys`) — it scans templates/code, builds/updates
    catalogs, and fails CI on missing/extra keys: the best extraction story
    among Angular runtime libs and the reason this grades T1.
  - **Scopes matching lazy routes**, so each feature lazy-loads only its own
    catalog.
  - **`@jsverse/transloco-messageformat` from day one** (registry `buys`) for
    ICU plurals/select, plus kapi pseudo-translate/translate-qa for the
    runtime-correctness gate.
- **kapi:** no preset — translate the JSON catalogs per file:
  ```bash
  kapi pseudo-translate assets/i18n/en.json --target-lang qps
  kapi translate assets/i18n/en.json --target-lang fr -o assets/i18n/fr.json
  kapi run translate-qa -i assets/i18n/en.json --target-lang fr --json
  ```
  Repeat per scope directory.
- **Footguns:** the ngneat→jsverse rename left stale docs/tutorials pointing
  at the frozen package; small maintainer team (bus factor); like all runtime
  libs, missing keys are a runtime problem (that's what `--emit-errors` in CI
  is for) and per-locale JSON adds client payload.

## ngx-translate — T2, caution: existing codebases only

**Warn the user before any greenfield adoption:** ngx-translate was abandoned
by its original author (~2018–2023) and revived by Andreas Löw / CodeAndWeb in
2024 — the revival is real but recent, stewardship is a single company, and
**only the most recent major is supported**, forcing upgrades on their
schedule. Staying is defensible; starting new is not — name Transloco (runtime
switching) or @angular/localize (public sites) as the lower-toil picks.

- **Idiom:** message IDs (nested JSON), runtime lookup:
  `{{ 'HOME.TITLE' | translate }}`, `[translate]`, or
  `translate.instant()/get()/stream()`. Catalogs at `public/i18n/{lang}.json`
  (v16+ loader default; older apps use `assets/i18n/`), loaded over HTTP.
- **Recommended config (what earns T2):** upgrade to **v18** (Angular 18–22;
  earlier majors are EOL). Install the registry `buys`:
  **`@vendure/ngx-translate-extract`** in CI (the maintained fork — the
  original extractor was itself abandoned) and
  **`ngx-translate-messageformat-compiler`** for ICU plurals — there is no
  plural support built in, only interpolation. Add a typed-key wrapper; keys
  are otherwise unchecked strings that slip to production.
- **kapi:** no preset — translate the JSON per file:
  ```bash
  kapi pseudo-translate public/i18n/en.json --target-lang qps
  kapi translate public/i18n/en.json --target-lang fr -o public/i18n/fr.json
  kapi run translate-qa -i public/i18n/en.json --target-lang fr --json
  ```
- **Footguns:** missing keys render the raw key (or fallback-language text)
  at runtime and nothing flags stale translations; SSR needs a server-side
  loader + TransferState to avoid double-fetch/flicker; v16 changed the
  standalone providers/loader API — budget that migration when upgrading old
  apps.

## Verify (all stacks)

Pseudo-translate first — rendering the `qps` build/locale is the readiness
gate that surfaces unmarked strings. Then confirm the *rendered* app is
translated in the target locale. Built-in: serve the locale-specific build
(e.g. `ng serve --configuration=fr`, or the deployed `/fr/` path) — there is
no in-app switch to test. Transloco/ngx-translate: switch at runtime and watch
the network panel — if the UI stays in source language, the catalog didn't
load (wrong loader path, scope not declared, locale not registered) or the
key is missing and silently falling back. In a `.kapi` project, finish with a
green `kapi check --ship`.
