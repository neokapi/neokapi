# iOS / macOS i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **String Catalogs (.xcstrings)** | **T0** | Compiler extracts on every build; the add-and-forget reference point for all of mobile. |
| Legacy `.strings`/`.stringsdict` | — | Migration target; one-click Xcode migrator. kapi handles them until you migrate. |

**Opinionated defaults:** every Xcode project → **String Catalogs**. They are
the only mobile system where marking is ~free, extraction is a build
side-effect, and sync is automatic with a sane stale-state model — the T0
reference point the whole Toil Index anchors on. Existing `.strings`/
`.stringsdict` apps: migrate via Xcode's built-in migrator; until then kapi
translates them directly (`--format applestrings`). **Do not adopt SwiftGen
for strings** — warn the user if they ask: its xcstrings support was never
completed and its release cadence has stalled; Xcode 26 typed symbols are the
first-party replacement.

## String Catalogs (.xcstrings) — T0 (recommended)

- **Idiom:** source-as-key, single JSON file (`Localizable.xcstrings`)
  containing *all* languages, states, and plural/device variations. SwiftUI
  literals are localizable by default — `Text("Welcome")` is a
  `LocalizedStringKey`, no wrapper needed. The compiler extracts on every
  build; removed strings are marked **stale** (not deleted), untranslated
  stale entries are pruned. Since Xcode 26, typed symbols
  (`Text(.welcomeMessage)`) via the "Generate String Catalog Symbols" build
  setting — on by default in new projects. Xcode 27 adds agentic localization
  (in-IDE "Generate Translations" per language).
- **Marking discipline (the only code habit):** Foundation/UIKit strings need
  `String(localized: "…")` (or `NSLocalizedString`) — a bare literal is
  invisible. Strings built by **concatenation or interpolation outside those
  initializers silently escape extraction**; compose full sentences inside
  `String(localized:)`/SwiftUI literals instead.
- **Recommended config (what earns T0):** the residual toil is git hygiene,
  not code work, so install exactly two guards:
  1. `.gitattributes` **union merge driver** — the single-JSON-all-languages
     design makes concurrent edits conflict structurally (a conflict marker
     mid-JSON makes the file unparseable):
     ```
     *.xcstrings merge=union
     ```
     Split very large catalogs per feature/table if conflicts persist.
  2. **CI stale-count check** — stale-but-translated entries accumulate and
     waste translator budget; fail or report when the count grows (the
     catalog is JSON: count entries with `extractionState == "stale"`).
  kapi's `pseudo-translate`/`translate-qa` directly on the `.xcstrings` file
  is the r-axis buy — no export step needed.
- **kapi:** no preset needed — `.xcstrings` is auto-detected. It is a single
  multi-locale file, so **output is the same file**:
  ```bash
  kapi pseudo-translate Localizable.xcstrings --target-lang qps   # readiness first
  kapi translate Localizable.xcstrings --target-lang fr -o Localizable.xcstrings
  kapi run translate-qa -i Localizable.xcstrings --target-lang fr --json
  ```
  This fills the `fr` entries in place; keys, states, plural variations, and
  substitution metadata are preserved.
- **Footguns:**
  - **Source-as-key fragility:** editing the English copy creates a new key
    and orphans every translation. For copy that churns (marketing strings),
    use Xcode 26 symbols or explicit keys instead of the literal.
  - **`xcodebuild -exportLocalizations` is flaky** — it builds all targets,
    fails on multi-platform quirks (e.g. watchOS xcframeworks), and
    occasionally emits empty XLIFF. Prefer kapi translating the `.xcstrings`
    file directly over XLIFF round-trips.
  - **Catalog ownership races:** a TMS writing translations back races the
    build's auto-sync (and serializes JSON differently, producing whole-file
    diffs). Decide explicitly who owns the catalog — the build or the
    external tool — and route the other through PRs. With kapi, run it
    between builds and commit; the union merge driver absorbs the rest.
  - **No ICU select/gender axis** — the variation model is plural/device
    only. Workarounds: separate keys per case, or Apple's `inflect:`
    automatic agreement (a handful of languages only).

## Legacy .strings / .stringsdict

Warn the user: this is a migration target, not a place for new work — Xcode's
one-click "Migrate to String Catalog" moves both files losslessly, and all
new tooling investment (symbols, agentic localization) lands on `.xcstrings`
only. Until the migration happens, kapi reads and writes both via
`--format applestrings` (the flag is required — `.strings` alone doesn't
identify the dialect):

```bash
kapi translate en.lproj/Localizable.strings --format applestrings \
  --target-lang fr -o fr.lproj/Localizable.strings
```

`.stringsdict` plural files go through the same format. Do the migration
first if you can — translating legacy files entrenches them.

## Verify

Run the app in the target locale (scheme → Options → App Language, or
`-AppleLanguages (fr)`), and confirm the visible UI is translated. Still in
English? Untranslated entries fall back to source silently — the usual
causes: the string escaped extraction (concatenation/interpolation outside
`String(localized:)` — grep for `+` on user-facing strings), or the entry is
stale/new in the catalog (open it in Xcode and check per-language state).
Check plural cases with values 1, 2, and a `many`-category number for the
target language. In a `.kapi` project, finish with a green `kapi check --ship`.
