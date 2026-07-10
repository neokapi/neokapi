# Docs sites i18n — adoption playbook (Docusaurus, Sphinx, Hugo)

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **Docusaurus** | **T1** | UI strings extracted to JSON; content is whole-file copies — kapi convergence supplies the missing drift detection. |
| Sphinx | T1 | Full gettext pipeline over doc paragraphs; automated but churn-heavy for living docs. |
| Hugo | **T3 — warn** | Manual key-based UI strings + per-language content copies; fully manual sync. |

Docs content is prose — whole files, not string catalogs — so the failure
mode is **drift** (source edited, translation stale), not missing keys. Two
policies apply to every generator here:

- **Target-locale artifacts are generated, never hand-edited.** Treat
  `i18n/<locale>/` (and equivalents) as build output of the translation
  pipeline: regenerate, don't patch.
- **Source locale strict, target locales warn.** Broken links in the source
  fail the build (source integrity matters); stale links or missing pages in
  a target locale are *pending work* and must never block the build — drift
  is the normal continuous state that kapi convergence exists to absorb.

## Docusaurus — T1 (recommended)

- **Idiom:** two surfaces. Theme/UI strings: `npm run write-translations`
  extracts them to `i18n/<locale>/code.json` (+ per-plugin JSON). Content:
  whole `.md`/`.mdx` files copied under
  `i18n/<locale>/docusaurus-plugin-content-docs/current/…` — and there is
  **zero native drift detection** on those copies: when a source page
  changes, nothing flags the translated copy.
- **Recommended config (what earns T1):** the registry buy is **kapi project
  convergence over the docs tree** — a `.kapi` project with content mappings
  turns every source edit into visible pending target work, which is exactly
  the drift detection Docusaurus lacks:
  ```bash
  kapi init --target-locale nb    # then map docs/** → the i18n/nb tree in the recipe
  kapi run                        # converge: (re)translate what changed
  ```
  Build target locales with broken-link *warnings*, keeping strict link
  checking only for the source-locale build (the policy above).
- **kapi (one-off):**
  ```bash
  kapi pseudo-translate docs/intro.md --target-lang qps
  kapi translate docs/intro.md --target-lang nb
  kapi translate i18n/nb/code.json --target-lang nb   # UI strings are plain JSON
  ```
- **Footguns:** hand-edits inside `i18n/<locale>/` are silently lost the next
  time the pipeline regenerates the tree — enforce generated-only; each
  locale is a separate build/deploy (`--locale`), so CI time scales with
  locale count.

## Sphinx — T1 (supported)

- **Idiom:** the full gettext pipeline applied to docs. The builder emits
  `.pot` per document (`make gettext`), `sphinx-intl update -p _build/gettext
  -l fr` msgmerges the per-language `.po` under
  `locales/<lang>/LC_MESSAGES/`, and the site builds with `-D language=fr`.
- Drift handling is real gettext: **paragraph-level msgids** mean an edited
  source paragraph goes fuzzy in every locale — detected mechanically, never
  silently stale. The cost: **any edit fuzzies the whole paragraph**, so
  living docs generate heavy review churn. Budget for it, or batch doc edits.
- **kapi:** the catalogs are ordinary PO — msgctxt and plural forms are
  preserved:
  ```bash
  kapi pseudo-translate _build/gettext/index.pot --target-lang qps
  kapi translate locales/fr/LC_MESSAGES/index.po --target-lang fr \
    -o locales/fr/LC_MESSAGES/index.po   # fills the untranslated entries in place
  ```
- **Footguns:** reST/MyST markup inside msgids is easy to mangle in
  translation — run `kapi run translate-qa` on the PO; keep `sphinx-intl
  update` in CI so the catalogs can't lag the source.

## Hugo — T3, warn before scaling it

**Warn the user: Hugo i18n is fully manual beyond a small site.** UI strings
live in hand-authored key-based `i18n/<lang>.toml` files with **no
extraction** — every `{{ i18n "key" }}` needs a hand-written entry per
locale — and content is per-language page copies (`content/fr/…` or
`about.fr.md`) with no drift detection. Nothing syncs any of it.

- **Mitigation (the registry buy): kapi convergence** over the i18n files and
  the content copies — a `.kapi` project with content mappings turns source
  edits into pending target work, replacing the manual sweep.
- Prefer YAML for the UI-string files (`i18n/en.yaml` — Hugo accepts
  YAML/TOML/JSON) so kapi can mediate them; kapi has no TOML reader.
  ```bash
  kapi translate i18n/en.yaml --target-lang fr -o i18n/fr.yaml
  kapi translate content/about.md --target-lang fr
  ```
- **Footguns:** missing UI keys render as empty/fallback silently (set
  `enableMissingTranslationPlaceholders = true` while developing); a page
  missing from a locale 404s or falls back per config — click through the
  locale nav after each converge.

## Verify (all paths)

Build and serve the site in the target locale and click through: nav,
sidebar, theme UI strings, and a recently edited page. The source-locale
build is strict and green; target-locale builds may warn. Confirm no hand
edits live in generated target trees (they would be lost on regeneration).
In a `.kapi` project, finish with a green `kapi check --ship`.
