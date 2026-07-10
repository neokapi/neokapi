# Rails i18n — adoption playbook

Grades are the recommended-config Toil Index (see [../i18n.md](../i18n.md);
scores in [frameworks.yaml](frameworks.yaml)).

| Stack | Grade | One-liner |
|---|---|---|
| **rails-i18n** (key-based YAML) | **T2** | The paved road; S3 stock — i18n-tasks in CI is what makes it livable. |
| gettext_i18n_rails | T2 | Source-as-key alternative; hybrid toil — see [gettext.md](gettext.md). |

**Opinionated defaults:** any Rails app stays on rails-i18n — it is the paved
road, and everything in the ecosystem (gems, Devise, validation messages)
speaks it. But quote the honest stock grade: **without the tooling below this
is S3 sync — effectively T3.** The recommended config is not optional polish;
it is what earns the T2. Never adopt without it.

## The stale-translation problem (why stock is S3)

rails-i18n has **no fuzzy concept**. Edit the English copy in
`config/locales/en.yml` and every other locale keeps serving the old
translation — silently, live, in production — and nothing anywhere detects
it. This is the classic key-based failure, and Rails ships zero mitigation.
The fix has two parts:

1. **i18n-tasks as a CI gate from day one** (below) — catches missing,
   unused, and interpolation drift mechanically.
2. **A key-rename-on-copy-change convention:** when the *meaning* of the
   English changes, rename the key in the same commit — the rename is what
   makes the string go missing in every other locale and re-flags
   translators. A pure typo fix may keep the key. No tool enforces this;
   put it in the team's PR checklist.

## Idiom

- **Keys, not source strings:** nested dot-keys in `config/locales/*.yml`
  (`t('users.show.title')`), conventionally split per model/view domain.
  Keep the idiom — do not convert Rails to source-as-key in the same change
  as adoption.
- **Lazy lookup** `t('.title')` resolves the prefix from the controller/view
  path. Concise, but it **couples keys to file paths: moving or renaming a
  view or controller silently breaks every lazy key inside it** — say so.
  Prefer full keys in shared partials and anything likely to move.
- **Plurals:** CLDR `count:` subkeys (`one:`/`few:`/`many:`/`other:`) — not
  ICU MessageFormat; there is no select/gender nesting.
- **There is no extraction:** English strings are authored by hand in YAML,
  in a different file from the code. That is the standing key tax (W2/X2)
  you accept for staying mainstream.

## Recommended config (what earns T2)

```ruby
# Gemfile
gem "i18n-tasks", group: :development
```

```bash
i18n-tasks health                            # the CI gate: missing + unused + normalized
i18n-tasks missing                           # keys in en absent from other locales
i18n-tasks unused                            # dead keys
i18n-tasks normalize                         # canonical order/format (kills YAML merge conflicts)
i18n-tasks check-consistent-interpolations   # %{name} parity across locales
```

- **Gate CI on `i18n-tasks health` from day one.** This single step is the
  registry buy that takes sync S3 → S1 — the justification for the T2 grade.
- `config.i18n.raise_on_missing_translations = true` in the **test**
  environment, so missing keys fail tests instead of rendering
  "translation missing" in production.
- The key-rename-on-copy-change convention, in the PR checklist.
- kapi convergence to fill what i18n-tasks flags (next section).

## kapi

No Rails preset — Rails YAML rides on kapi's generic `yaml` format: the key
hierarchy is preserved, only values translate, and `%{interpolations}`
survive.

```bash
kapi pseudo-translate config/locales/en.yml --target-lang qps
kapi translate config/locales/en.yml --target-lang fr -o config/locales/fr.yml
kapi run translate-qa -i config/locales/en.yml --target-lang fr --json
```

One catch: Rails nests everything under a locale root key (`en:`). kapi
preserves keys verbatim, so after translating confirm the target file's root
key says `fr:` (rename it if needed) or Rails will never look inside it. For
recurring locales, `kapi init --target-locale fr` with a content mapping over
`config/locales/` turns every source edit into visible pending target work.

## gettext_i18n_rails (the source-as-key alternative)

`fast_gettext` + `gettext_i18n_rails` bring gettext's `_("…")` economics to
Rails: no key naming, xgettext-style extraction, msgmerge fuzzy on source
change. **Warn before recommending it: hybrid toil** — the Rails ecosystem
still speaks I18n keys, so you run both systems side by side forever. When
that trade pays off, and the full playbook: [gettext.md](gettext.md).

## Verify

Run the app in the target locale (`I18n.locale = :fr` via the params/session
mechanism the app uses) and confirm the visible UI is translated.
`i18n-tasks health` green in CI. A deliberately missing key fails a test
(proves `raise_on_missing_translations` is wired). In a `.kapi` project,
finish with a green `kapi check --ship`.
