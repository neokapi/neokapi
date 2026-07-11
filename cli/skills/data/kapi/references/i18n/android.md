# Android i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

**`res/values/strings.xml` is the only option — T2 (recommended).** There is
no alternative string system on Android; the decision space is *around* it:
lint enforcement, per-app language, packaging filters, and the translation
pipeline. Stock it sits at S2/R2 (manual key lifecycle, runtime-only failure
discovery); the recommended config below buys it down to T2's best case.

## strings.xml — T2 (recommended; the only option)

- **Idiom:** **IDs are mandatory.** Every string is a developer-invented key
  (`android:name`), referenced as `stringResource(R.string.checkout_title)`
  (Compose) or `getString(R.string.checkout_title)` (Views). There is no
  source-as-key mode — keep the ID idiom and a naming convention
  (`feature_element_purpose`); don't fight it.
- **Marking / extraction:** there is **no automatic extraction** — in
  Compose, `Text("Hello")` compiles happily and **ships hardcoded,
  silently**. Warn the user: nothing in the toolchain catches this by
  default. The workflow is Android Studio's "Extract string resource"
  quick-fix (Alt+Enter) per literal, policed by lint (below).
- **Recommended config (what earns the grade):**
  1. **`HardcodedText` + `MissingTranslation` lint as CI ERRORS** — both are
     warning-level and commonly suppressed by default, which is exactly how
     hardcoded strings and 40-key locale gaps reach production. In
     `build.gradle.kts`:
     ```kotlin
     android { lint { error += listOf("HardcodedText", "MissingTranslation") } }
     ```
     Caveat: `HardcodedText` coverage of Compose string literals is weaker
     than XML layouts — pseudo-translation (below) is the backstop.
  2. **Per-app language (API 33+):** AGP auto-generates the locale config —
     `android { androidResources { generateLocaleConfig = true } }` — and
     `AppCompatDelegate.setApplicationLocales(...)` gives a backward-
     compatible in-app switcher; the system settings page lists your locales.
  3. **Packaging:** `androidResources.localeFilters` controls which locales
     ship. Warn if the project still uses `resourceConfigurations`/
     `resConfigs` — deprecated as of AGP 8.8; migrate to `localeFilters`.
  4. **kapi convergence + qa** (the s/r buy): route all catalog fills through
     kapi and gate with `translate-qa` — the platform ships no sync tooling.
- **Plural traps (all real, all shipped by someone):**
  - **English `quantity="zero"` is silently ignored** — English grammar
    treats 0 like 2, so your "No items" string never renders. Branch in code
    on `count == 0` or use a separate plain string.
  - **A bad placeholder crashes only in that locale:** `%s` mistyped in one
    translation means `getString` format crashes for those users only —
    invisible in English QA. kapi preserves printf placeholders and
    `translate-qa` validates them; that plus pseudo-translation is the guard.
  - Wrap non-translatable fragments in `<xliff:g id="…" example="…">` so
    translators (human or machine) can't touch them.
  - `MissingQuantity`/`UnusedQuantity` lint police per-language CLDR category
    sets (cs/fr/lt/sk need `many`/decimal handling many tools miss).
- **kapi:** **you MUST pass `--format androidxml`** — `.xml` is a shared
  extension, so the format is never inferred from the filename:
  ```bash
  kapi pseudo-translate app/src/main/res/values/strings.xml \
    --format androidxml --target-lang qps          # readiness check first
  kapi translate app/src/main/res/values/strings.xml --format androidxml \
    --target-lang fr -o app/src/main/res/values-fr/strings.xml
  kapi run translate-qa -i app/src/main/res/values/strings.xml --target-lang fr --json
  ```
  `<plurals>`, `<string-array>`, `<xliff:g>` guards, printf placeholders, and
  per-locale CLDR quantity variations are preserved; only values translate.
  Target files may be partial — resource resolution falls back to
  `values/` — so incremental locale fills are safe to commit.
- **Footguns:**
  - Missing translations are masked at runtime by fallback to the default
    locale — gaps surface in QA at best. The lint-as-error config is the fix;
    without it, tell the user they're at the stock S2/R2 grade.
  - Escaping rules (`\'`, `%%`, `formatted="false"`) trip translators —
    another reason to route through kapi rather than hand-edit targets.
  - Compose: caching resolved strings in remembered state or ViewModels goes
    stale on locale change — resolve `stringResource` in composition.
  - Key naming and file organization at scale is pure convention; a single
    giant `strings.xml` is common but splitting per feature file works fine
    (all `values/` XML is merged).

## Compose Multiplatform / KMP (brief)

Two options, both Android-style (mandatory IDs, `strings.xml` syntax, manual
marking, no extraction — same T2 profile; the registry has no separate
entry): JetBrains' built-in **Compose Multiplatform resources**
(`composeResources/values*/strings.xml`, generated `Res.string.*`,
`stringResource()` in common code) — the default for Compose-everywhere
apps — and **moko-resources** when shared Kotlin logic must serve *native*
SwiftUI/UIKit screens. Everything above applies, including
`--format androidxml` for kapi. Warn about the rough edges: historical plural
placeholder bugs, resources not exported in xcframeworks before ~1.8.1, and
locale switching on iOS being less seamless than Android.

## Verify

Switch the device (or per-app) language to the target locale and confirm the
visible UI is translated — remember fallback masks gaps, so also run the
pseudo-translated `qps` build and hunt for un-pseudo'd strings (those are
hardcoded literals lint missed). Exercise plurals with 0, 1, 2, and a large
count in the target locale; a format crash there means a corrupted
placeholder in that locale's file. In a `.kapi` project, finish with a green
`kapi check --ship`.
