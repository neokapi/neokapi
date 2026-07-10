# Go / JVM / .NET / Fluent i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| Go (go-i18n v2) | T2 — caution | The de-facto standard; verbose structs, multi-step merge — workable, not paved. |
| JVM (.properties) | **T3 — warn** | 25-year-old bedrock; no extraction exists, manual everything. |
| .NET (.resx) | **T3 — warn** | No extraction and no plural engine; great deploy story, poor authoring story. |
| Fluent (.ftl) | **T3 — warn** | Best message syntax ever designed; ecosystem energy moved to MessageFormat 2. |

These are the **no-extraction ecosystems**: no tool derives catalogs from
source, so every new string is a manual multi-file chore and locale files
drift with nothing watching. The theme of every recommended config below:
**make CI, not humans, the sync mechanism.** A
missing/unused/interpolation-mismatch check on every PR is the cheapest 80%
replica of what xgettext+msgmerge gives gettext users for free.

## Go — T2 (caution)

Tell the user up front: Go i18n is still comparatively painful in 2026 —
everything works, nothing is automatic.

- **Idiom:** `nicksnyder/go-i18n` v2 is the de-facto standard
  (`golang.org/x/text`'s `gotext` extractor is still experimental — don't
  build on it). Messages are `i18n.Message`/`LocalizeConfig` structs — the
  most verbose marking of any ecosystem here (W3 stock).
- **Recommended config (what earns T2):**
  - **Source-English-as-ID convention** (`ID: "Save changes"` — the English
    *is* the ID): gettext-ish fallback for free, and it takes W3 → W2.
  - **`goi18n extract && goi18n merge` enforced in CI.** merge hashes the
    source text, so a changed message drops its translations into the
    `translate.<lang>` file for re-work — a crude fuzzy analog. The loop is
    multi-step and easy to skip; CI is what makes it real.
  - **Minimize the translatable surface** — the strong opinion: Go services
    should mostly emit codes/enums and let the UI edge (where real extraction
    tooling exists) do the localizing. The best Go i18n is less Go i18n.
- **kapi:** no preset. Keep catalogs in JSON (goi18n supports
  `-format json`) — kapi has a JSON reader/writer but no TOML:
  ```bash
  kapi pseudo-translate active.en.json --target-lang qps
  kapi translate translate.fr.json --target-lang fr -o active.fr.json
  ```
- **Footguns:** `goi18n extract` only sees `i18n.Message` struct literals —
  strings not written that way are invisible, and there is no template
  extraction; per-request localizer plumbing through `context` is on you.

## JVM (ResourceBundle / Spring MessageSource) — T3, warn first

**Warn the user: no extraction exists on the JVM, and X=3 caps the grade at
T3 no matter what tooling is added.** Keys are authored by hand, sync between
locale `.properties` files is manual, and drift is found at runtime.
`ResourceBundle` is frozen bedrock — nothing has displaced it, and nothing
has improved its workflow.

- **Idiom:** keys in `messages.properties` +
  `messages_<locale>.properties` on the classpath; Spring `MessageSource`
  wraps it; automatic fallback chain.
- **Recommended config (the buys):**
  - **ICU4J MessageFormat from day one — never `ChoiceFormat`.** Most Java
    apps ship wrong plurals because ChoiceFormat was the default path; ICU4J
    gives real CLDR plurals/select and named args (spring-icu for Spring).
  - UTF-8 properties (fine since Java 9 — kill any ISO-8859-1 escaping).
  - A **typed `Messages` facade** (generated constants) so keys can't dangle
    into a runtime `MissingResourceException`.
  - **kapi convergence + a CI locale-parity check** — the platform ships no
    merge/fuzzy/unused tooling, so CI diffing the locale files *is* the sync
    layer (S3 → S1).
- **kapi:** properties is a full read/write format:
  ```bash
  kapi translate src/main/resources/messages.properties --target-lang de \
    -o src/main/resources/messages_de.properties
  ```
- **Footguns:** positional `{0}` args hurt translator comprehension (ICU
  named args fix it); properties escaping; an English copy edit silently
  leaves stale translations in every other locale — nothing detects it.

## .NET (.resx / IStringLocalizer) — T3, warn first

**Warn the user: .NET has no extraction (X=3 — the T3 stands), and
`string.Format` has NO plural engine** — "1 item(s)" is the default failure
mode of a stock .NET app.

- **Idiom:** `.resx` per culture compiled into satellite assemblies;
  hierarchical culture fallback (`fr-CA` → `fr` → neutral). Great deploy
  story, poor authoring story.
- **Recommended config (the buys):**
  - **String-as-key `IStringLocalizer`** (`_localizer["Welcome, {0}"]`): when
    no resource exists the key itself is returned, so English fallback is
    free — informal source-as-key, W2 → W1.
  - **ResXManager** (or a TMS) for cross-locale consistency views — the sync
    layer resx doesn't have.
  - **A real ICU MessageFormat library** (Messageformat.NET, SmartFormat) for
    any counted or gendered string.
  - kapi convergence over the `.resx` files (below).
  - **Typed resx classes only for desktop** (WPF/WinForms/MAUI
    satellite-assembly deployments); web apps take string-as-key.
- **kapi:** resx is supported (`.resx`/`.resw`):
  ```bash
  kapi translate Resources/Strings.resx --target-lang fr -o Resources/Strings.fr.resx
  ```
- **Footguns:** missing translations surface only at runtime; the Visual
  Studio resx editor degrades past a few languages; a copy fix means
  rebuilding satellite assemblies.

## Fluent (.ftl) — T3, warn before adopting

The best *message syntax* ever designed — asymmetric localization, terms,
attributes, per-locale variants — with best-in-class runtime correctness
(R=0). **But warn the user: ecosystem risk is the headline (E=3).**
Maintainer energy has moved to Unicode MessageFormat 2 (whose design Fluent
heavily informed); outside Mozilla, bindings are community-grade and PRs
stall. **Existing users should not panic-migrate** — Fluent is close enough
to MF2 that eventual conversion is plausible. **New projects should not start
on it.** Also: **kapi has no `.ftl` reader (known gap) — translations of
Fluent files are not kapi-mediated yet**, so pseudo-translate/translate-qa
don't apply to them.

## MessageFormat 2 status (verified 2026-07)

The spec is stable (approved into CLDR), but ICU4J/ICU4C ship it as a
**technical preview only**, `Intl.MessageFormat` is stuck at TC39 Stage 2,
and no major framework or TMS defaults to it. Don't bet the default path on
it yet. Keep message content simple enough that MF1 ↔ MF2 conversion stays
mechanical, and reassess when ICU de-previews it or a top-tier framework/TMS
adopts it.

## Verify (all paths)

Run the service/app in the target locale and confirm real output — not just
the catalog — is translated, including a plural with count 1 and count 2
(plurals are the top runtime failure in these stacks). Locale-parity CI check
green. In a `.kapi` project, finish with a green `kapi check --ship`.
