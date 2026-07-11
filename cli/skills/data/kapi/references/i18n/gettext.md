# gettext family i18n — adoption playbook (C/C++, Python, Django, PHP/WordPress, Ruby)

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **gettext** (C/C++, Python, PHP, CLI/desktop) | **T1** | Source-as-key `_()`, total extraction, fuzzy merge — the reference workflow. |
| **Django** | **T1** | makemessages/compilemessages wrap the same pipeline; the best-integrated web-framework i18n anywhere. |
| WordPress | T1 | `wp i18n make-pot` + text domains; WP.org language packs make hosted plugins near-T0. |
| Ruby gettext | T2 | fast_gettext + gettext_i18n_rails — source-as-key, but hybrid toil against the Rails mainstream. |

**Opinionated defaults:** if the ecosystem has a decent gettext binding, use
it — this is the workflow every other stack in this skill is measured against,
and its residual toil is exactly one thing: reviewing fuzzies. **Never invent
JSON catalogs in a gettext-capable ecosystem** — you would discard the best
extraction/merge tooling there is and rebuild its guarantees by hand.

## Why gettext is the T1 reference

- **Source-as-key.** The `msgid` *is* the English string (`_("Save changes")`);
  no invented identifiers, and the untranslated fallback is automatically
  correct. Optional `msgctxt` disambiguates homonyms.
- **Total extraction.** `xgettext` parses ~30 source languages (C, Python, JS,
  Rust, shell, …) into a `.pot` template. Catalogs are derived artifacts,
  never hand-authored by developers.
- **Mechanical merge.** `msgmerge` carries translations across re-extraction;
  a changed source string degrades to a `#, fuzzy` entry queued for review —
  it can never silently serve a stale translation, which is the structural
  failure of every key-based stack.
- **PO is the lingua franca** — plain text, diffable, understood by every TMS
  and editor since 1995 (and by kapi natively).
- Actively modernizing: **gettext 1.0 shipped January 2026**, adding
  `po-fetch` and `msgpre` local-LLM pre-translation of PO files.

**Recommended config (what earns R0 → T1):** `msgfmt -c` on every locale in CI
(validates printf-style placeholders across translations) + `kapi
pseudo-translate` on the `.po` as the readiness gate. That is the whole buy.

## Python (stdlib gettext + Babel; Flask/FastAPI)

```bash
pybabel extract -F babel.cfg -o locales/messages.pot .
pybabel update -i locales/messages.pot -d locales   # msgmerge semantics incl. fuzzy
pybabel compile -d locales                          # .po → .mo
```

Layout: `locales/<lang>/LC_MESSAGES/messages.po`. Flask: `flask-babel` is
mature. FastAPI has no official i18n story — locale middleware is DIY (use
contextvars for async safety) and `fastapi-babel` is a thin community wrapper;
keep Babel + PO underneath either way. Watch lazy vs eager evaluation for
module-level strings.

## Django — T1 (recommended)

```bash
django-admin makemessages -l fr               # xgettext/msguniq/msgmerge over Python + templates
django-admin makemessages -d djangojs -l fr   # JS domain
django-admin compilemessages                  # msgfmt → .mo
```

Layout: `locale/<lang>/LC_MESSAGES/django.po`. Mark with
`gettext`/`gettext_lazy` in Python, `{% translate %}`/`{% blocktranslate %}`
in templates, `pgettext` for context.

**Footguns — tell the user all three:**

- **GNU gettext binaries (≥ 0.15) are a real CI/deploy dependency.** Install
  them in the container/CI image or makemessages/compilemessages fail —
  especially painful on Windows and slim containers.
- **`gettext_lazy` vs `gettext`:** module-level strings (model fields, form
  labels) need the lazy variant, or they freeze in the boot-time locale.
- **`compilemessages` SILENTLY SKIPS fuzzy entries.** A source edit fuzzies
  the string, which then ships in English until a human reviews it — no error
  anywhere. Review (or deliberately clear) fuzzies before every release.

## PHP / WordPress — T1

- Marking: `__( 'Text', 'my-domain' )`, `_e()`, `_n()` for plurals, `_x()` for
  context. The text domain is mandatory boilerplate — lint for mismatches.
- Extraction: `wp i18n make-pot . languages/my-plugin.pot` (plus
  `make-php`/`make-json`); plain PHP uses xgettext.
- Since WP 6.5, translations load from precompiled `.l10n.php` files in
  preference to `.mo` — gettext format at rest, PHP arrays at runtime.
- **WP.org-hosted plugins/themes are near-T0:** translate.wordpress.org
  language packs mean the community translates for you.
- Footguns: a text-domain mismatch silently disables translation (PHPCS i18n
  sniffs catch it); JS strings need the second pipeline
  (`wp_set_script_translations` + JSON).

## Ruby gettext — T2, hybrid-toil warning

`fast_gettext` (the maintained runtime) + `gettext_i18n_rails` bring
source-as-key `_("…")` to Rails, with rake-task extraction and msgmerge/fuzzy
via `rake gettext:find`. **Warn before adopting: the Rails ecosystem speaks
I18n keys** — gems, Devise, ActiveRecord validation messages all use
`config/locales` YAML, so you run *both* systems forever. That hybrid is T2
territory; it pays off only in app-heavy codebases where prose-key naming
dominates. Mainstream Rails apps go to [rails.md](rails.md).

## kapi on PO catalogs

kapi reads and writes PO natively — `msgctxt` and plural forms are preserved;
only translation values change. No preset needed; run on the files directly:

```bash
kapi pseudo-translate po/en.po --target-lang qps       # readiness gate first
kapi translate po/en.po --target-lang fr -o po/fr.po   # or start from the .pot
kapi run translate-qa -i po/en.po --target-lang fr --json   # translate + QA in one flow
```

For recurring locales scaffold a project (`kapi init --target-locale fr …`) so
convergence owns the loop; one-off files need no project.

## Verify (all paths)

Compile (`msgfmt -c`, `pybabel compile`, `compilemessages`) and run the app in
the target locale; confirm the visible strings are translated. Still English?
Check in order: string not marked with `_()`, catalog not
re-extracted/merged, entry still fuzzy (excluded from compiled output), or the
`.mo`/`.l10n.php` not regenerated after the PO changed. In a `.kapi` project,
finish with a green `kapi check --ship`.
