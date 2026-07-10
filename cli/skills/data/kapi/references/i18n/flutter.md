# Flutter i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **gen_l10n (ARB)** | **T2** | The docs-taught default: ICU-complete ARB, typed `AppLocalizations`, vendor-backed. |
| slang | T2 | Terse `t.checkout.title` + the best drift tooling in Flutter; third-party owns your string layer. |
| easy_localization | **avoid** | Runtime string keys — typos ship to production. Prototypes only. |

**Opinionated defaults:** greenfield → **gen_l10n**: it's what the Flutter
docs teach, what hires know, what TMSs support (ARB), and it's ICU-complete.
Choose **slang** when DX and drift analysis matter more than
vendor-officialness (large key sets, feature-module namespaces, teams that
hate `of(context)` boilerplate). **Warn before any easy_localization
adoption** (below). Migrations slang↔ARB exist in both directions, so
lock-in is moderate. Nothing in Flutter extracts literals from source — all
three are catalog-first: add the key + text, run codegen, reference it.

## gen_l10n — T2 (recommended)

- **Idiom:** camelCase keys in a template ARB (`lib/l10n/app_en.arb`) with
  `@key` metadata (description, typed placeholders); full ICU MessageFormat
  values (plural, select/gender, date/number) — the richest message grammar
  of any mobile stack. Call sites are
  `AppLocalizations.of(context)!.checkoutTitle`; add the standard extension
  so they read `context.l10n.checkoutTitle`:
  ```dart
  extension L10nX on BuildContext {
    AppLocalizations get l10n => AppLocalizations.of(this)!;
  }
  ```
- **Recommended config (what earns T2):**
  1. `l10n.yaml` + `generate: true` in `pubspec.yaml` so codegen runs on
     every `flutter run`/`build`. The synthetic `package:flutter_gen` is
     **removed** — generate into the source tree and import the generated
     file directly; warn the user if they have `synthetic-package: true` or
     `package:flutter_gen` imports (broke around Flutter 3.29–3.32).
     ```yaml
     # l10n.yaml
     arb-dir: lib/l10n
     template-arb-file: app_en.arb
     output-localization-file: app_localizations.dart
     untranslated-messages-file: build/untranslated.json
     ```
  2. **`untranslated-messages-file` report wired into CI** (the sync buy):
     it's a machine-readable gap report per locale — fail or post it on PRs,
     and pair it with kapi convergence to fill the gaps. Known caveats:
     false positives for intentionally-inherited regional locales, and no
     configurable fallback language.
  3. The build **fails on template placeholder errors** (a missing
     placeholder referenced in `app_en.arb`) — that's the r-axis working;
     don't suppress it.
- **kapi:** preset `flutter` — scaffold once for repeatable runs:
  ```bash
  kapi init --framework flutter --target-locale fr --target-locale de
  ```
  writes the recipe mapping `lib/l10n/app_en.arb` → `lib/l10n/app_{lang}.arb`.
  ARB is auto-detected natively; ICU placeholders and `@`/`@@` metadata are
  preserved. Always pseudo-translate first:
  ```bash
  kapi pseudo-translate lib/l10n/app_en.arb --target-lang qps
  kapi translate lib/l10n/app_en.arb --target-lang fr -o lib/l10n/app_fr.arb
  kapi run translate-qa -i lib/l10n/app_en.arb --target-lang de --json
  ```
  Then rerun codegen (`flutter gen-l10n` or any build) to pick up new locales.
- **Footguns:** verbose call sites (the extension fixes it); the
  synthetic-package migration churn if following pre-2025 tutorials; ARB is
  niche next to strings.xml/xcstrings — fewer editors, though every major
  TMS and kapi handle it; adding a key always means touching the ARB *and*
  regenerating — stale codegen shows as "getter not defined" errors.

## slang — T2 (supported)

- **Idiom:** hierarchical JSON/YAML/CSV in `i18n/` (per-locale files,
  optional per-feature namespaces); the *structure* is the key, accessed as
  `t.checkout.title` — no `context` needed. Codegen via `dart run slang`
  (watch mode available; no build_runner).
- **Recommended config:** **`dart run slang analyze` in CI** (the buy) — it
  finds both **missing and unused** translations, the best drift tooling in
  the Flutter ecosystem, and the reason slang holds T2 despite being
  third-party.
- **kapi:** no preset; the JSON catalogs are read natively:
  ```bash
  kapi pseudo-translate i18n/en.json --target-lang qps
  kapi translate i18n/en.json --target-lang fr -o i18n/fr.json
  ```
- **Footguns — warn before adopting:** a third-party project owns your
  entire string layer (single-maintainer-ish, mitigated by high activity and
  v4 maturity) — that ecosystem-risk premium is the price of the DX. Its
  plural/select model is its own (CLDR-aware, "custom contexts"), not ICU;
  and generic-JSON TMS integration loses ICU niceties unless you use its ARB
  mode.

## easy_localization — avoid for new production apps

Tell the user plainly before they adopt or extend it: keys are resolved **at
runtime** (`'checkout.title'.tr()`), so **typos ship to production** as raw
key strings; it won't tell you where a missing key is or which locales are
incomplete (S=3 — the gap-finding is entirely on you); maintenance is
donation-dependent; runtime asset loading adds startup cost; and it has a
history of `intl` version-pinning conflicts with `flutter_localizations`.
Fine for prototypes. For production, name gen_l10n as the same-effort,
compile-safe default — and if they're already on easy_localization, both
gen_l10n (via ARB) and slang are reachable migrations.

## Verify

Run the app with the target locale (device language, or
`MaterialApp(locale: Locale('fr'))` for a quick check) and confirm the
visible UI is translated. Still in English? Usual causes: `supportedLocales`
/ `localizationsDelegates` not wired on `MaterialApp`, codegen not rerun
after the ARB changed, or the string never moved into the catalog (grep for
literal `Text('` in changed files). Check an ICU plural with 0/1/2/many and
a `select` case if used. The CI gate is an empty (or reviewed)
`untranslated-messages-file`; in a `.kapi` project, finish with a green
`kapi check --ship`.
