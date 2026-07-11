# React Native / Expo i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **react-i18next + expo-localization / react-native-localize** | **T2** | The de-facto default; holds T2 only with i18next-cli + iOS Intl polyfills. |
| Lingui | T2 | Source-as-key macros, real extraction, ICU, ~3KB runtime; smaller ecosystem. |

No vendor recommendation exists (Meta ships nothing), so **the RN i18n stack
is assembled from four separate decisions** — locale detection, i18n
framework, ICU polyfills, and extraction tooling — and every team's setup
differs. Make all four explicitly; a stack missing any one of them is where
the toil hides.

**Opinionated defaults:** Expo → **react-i18next + expo-localization**; bare
RN → **react-i18next + react-native-localize**. Choose **Lingui** when
extraction hygiene, ICU fidelity, and bundle size outweigh ecosystem breadth.
**Shared web+RN codebases: whichever the web side already uses wins** — never
run two i18n frameworks over one catalog (see [react.md](react.md) for the
web-side setups).

## Default stack — react-i18next + expo-localization / react-native-localize — T2 (supported)

- **Idiom:** developer-invented keys in namespaced JSON
  (`locales/{lang}/*.json`), `t('checkout:title')`; device locale from
  expo-localization (Expo) or react-native-localize (bare, zoontek). Stable
  feature-scoped IDs + `defaultValue` is the maintainers' idiom — keep it.
- **Recommended config (what earns T2 — skip these and it degrades to T3;
  say so):**
  1. **Official `i18next-cli`: `extract --ci` in CI + `sync`** (the x/s buy)
     — catalogs stay aligned with `t()` calls mechanically. Warn if the
     project uses i18next-parser: it is deprecated; migrate to i18next-cli.
  2. **FormatJS Intl polyfills on iOS** (the r buy): Hermes ships full
     ECMA-402 on Android but **NOT on iOS** — `Intl.PluralRules` (and
     `Intl.Locale`) must be polyfilled or plural selection silently
     misbehaves for iOS users only. Warn the user this is not optional:
     ```ts
     import '@formatjs/intl-locale/polyfill-force';
     import '@formatjs/intl-pluralrules/polyfill-force';
     import '@formatjs/intl-pluralrules/locale-data/fr';
     ```
     Use the `polyfill-force` imports and load only the locale data you ship.
  3. **Typed keys via `CustomTypeOptions`** module augmentation — compile-time
     key checking when catalogs are statically imported.
  4. **Per-app language bookkeeping:** declare supported locales via the
     config plugin (expo-localization) / native project entries so the iOS
     and Android 13+ per-app language settings actually list your locales —
     this is config-plugin work nothing else will do for you.
- **kapi:** no preset; pass **`--format i18next`** on the JSON catalogs so
  plural-suffix keys (`key_one`/`key_other`) and `{{interpolation}}`/`$t()`
  nesting are grouped and protected. Pseudo-translate first:
  ```bash
  kapi pseudo-translate locales/en/translation.json --format i18next --target-lang qps
  kapi translate locales/en/translation.json --format i18next \
    --target-lang fr -o locales/fr/translation.json
  kapi run translate-qa -i locales/en/translation.json --target-lang fr --json
  ```
- **Footguns:**
  - Dynamic keys (`t(variable)`) defeat both extraction and TS typing —
    they're invisible to `i18next-cli` and ship untranslated; refactor to
    static keys or an explicit key map.
  - Polyfill weight/perf on iOS — hence `polyfill-force` + per-locale data,
    not the blanket polyfills.
  - OTA-updated JS bundles carry catalogs that can drift from store-listing
    language metadata and the per-app language declarations — update both
    when adding a locale.
  - TS slows down on huge typed catalogs; split namespaces.

## Lingui — the extraction-hygiene alternative (T2)

- **Idiom:** source-as-key macros — `<Trans>Welcome {name}</Trans>` /
  `` t`Welcome` `` — no key invention; `lingui extract` walks source and
  marks obsolete messages (the only RN stack with gettext-style
  extract/merge hygiene), `lingui compile` emits optimized catalogs; native
  ICU MessageFormat; ~3KB runtime. First-party React Native tutorial exists
  (https://lingui.dev/tutorials/react-native).
- **Needs the same iOS Intl polyfills as the default stack** — warn the user
  Lingui does not exempt them from the Hermes gap.
- **kapi:** no preset; kapi reads the PO catalogs natively:
  ```bash
  kapi pseudo-translate src/locales/en/messages.po --target-lang qps
  kapi translate src/locales/en/messages.po --target-lang fr -o src/locales/fr/messages.po
  ```
- **Footguns — warn before adopting:** smaller community than i18next, and RN
  setup has historically meant babel-macro/metro fiddling — documented, but
  real friction for Expo-managed newcomers.

## Verify

Change the device (or per-app) language to the target locale, fully relaunch
the app (locale detection usually runs at startup), and confirm the visible
UI is translated. Still in source language? Check: catalog actually bundled
(or loaded) for that locale, detection wiring
(expo-localization/react-native-localize → i18next `lng`), and that the key
isn't dynamic. **Test plurals on an iOS device/simulator specifically** — a
plural that works on Android but picks the wrong form on iOS means the Intl
polyfills are missing. In a `.kapi` project, finish with a green
`kapi check --ship`.
