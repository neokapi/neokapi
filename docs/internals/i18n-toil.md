# i18n Toil Index: Axes, Grades & Audit

> This grades **other people's** frameworks. For how this repository keeps its
> own multilingual surfaces current, see [the dogfood loop in
> CI](l10n-ci.md).

This is the rubric behind kapi's i18n framework advice: a measure of the
**ongoing maintenance cost** an i18n stack imposes on a team: the recurring
cost, after setup. It is the contract that the `kapi` Agent Skill's i18n
references (`cli/skills/data/kapi/references/i18n/`), the
`refresh-i18n-framework` skill (`.skills/`), and the `i18n-triage` workflow
(`.claude/workflows/i18n-triage.js`) operate against. The research base lives
in the private strategy repository under `graveyard/i18n-skill/research-*.md`
(harvested and dated; re-verified by the triage workflow).

The thesis it operationalizes is the repo's own: **translation must be ambient,
not requested** (the strategy repository's `graveyard/skill-dogfood/translation-loop.md`). A source edit
creates pending target work that tooling absorbs; a human having to remember an
i18n chore is the failure mode. The Toil Index grades how close a given
framework, *in the configuration the skill recommends*, gets to that.

Two artifacts are published per framework, deliberately different things:

1. **The grade**: a single toil grade **T0–T4** (§1). This is what the skill
   tells users. T0–T1 is "adopt and forget"; T3–T4 comes with an explicit
   *you're-on-your-own* warning.
2. **The vector**: five axis scores 0–3 (§2), recorded in the registry
   (`frameworks.yaml`, §4) together with the evidence watermarks. The vector is
   diagnostic: it explains the grade and tells the skill which compensating
   tool to install during adoption.

Every framework gets **two vectors**: `stock` (the framework as its docs teach
it) and `recommended` (with the skill's prescribed configuration: lint rules,
merge tools, CI checks, and kapi itself). The grade users see is derived from
`recommended`; the delta between the two *is* the value the skill adds, so the
per-framework reference must name the tool that buys each improved axis.

## 1. Toil grades (what the skill promises)

Derived from the `recommended` vector: `total` = sum of the five axes.

| Grade | Meaning (agent-facing) | Criteria |
|---|---|---|
| **T0: add and forget** | A dev who has never heard of the i18n setup ships a correctly translated feature by writing plain source-language UI text; extraction, fill, checks, and sync happen in the build/CI. Residual human work: approving translation review. | total ≤ 2, no axis > 1, **and W = 0** (a wrap habit, however light, is T1 by definition) |
| **T1: one habit** | One habitual step per feature (wrap with source text, run extract); tooling catches everything else. | total 3–5 **and** no axis = 3 |
| **T2: standing chore** | Someone owns key hygiene and catalog sync; translation work shows up in sprint planning. | total 6–9 **and** no axis = 3 |
| **T3: recurring project** | Releases gate on manual translation passes; drift and untranslated leakage reach prod routinely. Warn the user. | total 10–12, **or** any axis = 3 |
| **T4: you're on your own** | Hand-edited per-locale files, no validation: expect dead keys, part-translated locales, and the documented 2–5× retrofit bill later. Warn explicitly and offer the nearest lower-toil path. | total ≥ 13, **or** (X = 3 and S ≥ 2) |

The grade is capped by the worst axis, never averaged past it: one manual
loop that recurs per feature dominates four automated ones (same reasoning as
format-maturity's minimum-over-gating-axes rule).

## 2. The five axes (the score)

Each axis is scored 0 (no toil) to 3 (heavy), against **testable criteria**.
Score what the tooling enforces, not what a disciplined team could do.

### 2.1 W: Marking toil *(who ensures new strings enter the system?)*

- **0**: Strings are translatable by default or compiler-detected; no wrapper
  needed (SwiftUI `Text` literals; neokapi-i18n plain JSX). Opting *out* is the
  explicit act (`Text(verbatim:)`).
- **1**: A wrapper is required but carries only the source text, no key
  invention (`_("Save changes")`, `t\`Save changes\``, `<Trans>`), **and** a
  lint rule failing CI on unmarked literals is available and prescribed.
- **2**: Wrapper plus a manually invented key (`t('checkout.save_button')`);
  naming conventions must be taught and reviewed.
- **3**: Wrapper + key + hand-editing one or more catalog files per string,
  with no lint enforcement.

### 2.2 X: Extraction toil *(how do strings get from code to catalog?)*

- **0**: Extraction runs automatically on every build/compile; the catalog
  cannot drift from the code (Xcode String Catalogs; neokapi-i18n bundler
  plugin; Paraglide compile).
- **1**: A one-command extractor exists and is wired into CI (xgettext /
  `lingui extract` / `ng extract-i18n`).
- **2**: An extractor exists but runs manually/ad hoc, or covers only part of
  the surface (goi18n's struct-literal-only extraction).
- **3**: No extractor; developers hand-author catalog entries (raw `en.json`
  maintenance, Java `.properties`, `.resx`).

### 2.3 S: Sync toil *(what happens on change/rename/delete, over time?)*

- **0**: Add/rename/delete propagate automatically; changed entries are
  flagged, not silently kept (msgmerge fuzzy, Xcode stale markers) or surface
  as compile errors (Paraglide); translations flow back through an automated
  pipeline; missing translation is a CI-visible state.
- **1**: CI reports coverage and missing/unused keys per locale; sync is
  scripted (i18n-tasks, `slang analyze`, ng-extract-i18n-merge,
  transloco-keys-manager).
- **2**: Sync is manual export/import; no unused-key detection; drift is
  found by humans.
- **3**: Each locale file is edited independently; nothing knows which keys
  are missing, stale, or dead.

### 2.4 R: Runtime-correctness toil *(what does the user see when things lag or break?)*

- **0**: Guaranteed graceful degradation: fallback chain to source language
  (never raw keys), CLDR-correct plurals, placeholder/message syntax validated
  before merge, pseudo-translation runnable in CI.
- **1**: Fallback configured and plurals CLDR-correct, but placeholder
  validation or pseudo-translation is missing.
- **2**: Raw keys can reach users on a missing translation, or pluralization
  invites string concatenation; a translator can break a view (unvalidated
  placeholders).
- **3**: Hand-rolled interpolation/plurals; a broken placeholder is a runtime
  exception; untranslated renders blank or as key soup.

### 2.5 E: Ecosystem risk *(will this still be low-toil in three years?)*

- **0**: Platform-vendor or standards-backed with a stable catalog format
  (gettext PO, Apple xcstrings, Android resources, ARB); majors have not
  rewritten the message corpus.
- **1**: Large, actively maintained OSS project; majors ship migration
  guides/codemods; the catalog format survived the last major (i18next,
  FormatJS, Lingui post-v4).
- **2**: Majors have rewritten catalogs or message syntax, stewardship
  recently changed hands, or the lib is coupled to one meta-framework's churn
  (Transloco ngneat→jsverse; ngx-translate's revival; Paraglide's v2 adapter
  deprecations).
- **3**: Unmaintained or bespoke: dormant lib (svelte-i18n, astro-i18next,
  easy_localization for production), in-house i18n layer, or a proprietary
  format with no export path.

## 3. Scoring rules (so any agent scores the same)

1. **Score the recommended configuration** for the published grade; record the
   stock vector alongside it. Every axis the recommendation improves must name
   the specific tool/config that buys it (e.g. Angular S: 3 → 1 *via
   ng-extract-i18n-merge*).
2. **Source-as-key wins W but must earn S.** A source-as-key stack scores
   W ≤ 1 by construction, but scores S ≤ 1 only if source edits are handled
   mechanically (fuzzy carry-over, stale flags, or compile errors). Without
   that, a source edit silently orphans translations and S ≥ 2.
3. **Explicit-ID stacks can never score W ≤ 1.** Recommend them only where
   copy is authored outside the code (TMS-driven copywriting, heavy A/B), and
   say so in the reference.
4. **"We use a TMS" without repo-side automation is S2, not S1.** Sync counts
   as scripted only when it runs in CI/build, not when a human remembers to
   export.
5. **No pseudo-translation path caps R at 1** (never 0). kapi's
   `pseudo-translate` supplies this for any catalog format kapi reads; note
   it in the recommended config where the stack lacks its own.
6. **Evidence or it didn't happen.** Version claims, maintenance status, and
   deprecations carry a source URL and a `verified` date in the registry.
   Scores changed without new evidence are reverted by the triage workflow
   (sticky-anchor rule, as in format-triage).
7. **Change control.** A change that improves a framework's published grade
   may not, in the same change, edit this rubric or the triage scorer. Fix the
   reference, not the gate.

### The kapi adjustment

kapi is itself a compensating tool the recommendation may prescribe, and it
adjusts axes the same way any other tool does, by criteria rather than by fiat:

- **S**: `kapi init --framework <id>` binds catalogs into a project whose
  convergence loop (`kapi run`, hooks, CI) turns source drift into pending
  target work that is included automatically; coverage and staleness become
  visible state (`kapi check`). Stacks at S2 typically land at S1; stacks
  whose catalog kapi fully mediates can reach S0.
- **R**: `kapi pseudo-translate --target-lang qps` provides the pseudo-loc
  path; the `translate-qa` gates validate placeholders/ICU before merge. R2 stacks
  typically land at R1; R1 stacks at R0.
- **W, X, E**: kapi does not change these (except neokapi-i18n, which is its
  own stack). A stack with no extractor still has no extractor; be honest
  about it.

## 4. The registry (`frameworks.yaml`)

Machine-readable source of truth for the skill's routing and the triage
workflow, at `cli/skills/data/kapi/references/i18n/frameworks.yaml` (it ships
inside the skill so the agent can consult it). Schema per entry:

```yaml
- id: react-i18next            # stable id; kapi preset id where one exists
  ecosystem: react             # react|vue|angular|svelte|astro|solid|flutter|ios|android|react-native|kmp|python|django|php|ruby|rails|go|jvm|dotnet|gettext|docs
  reference: react.md          # the per-ecosystem playbook covering it
  status: supported            # recommended | supported | caution | avoid
  detect:                      # detection signals, checked in order
    deps: [react-i18next, i18next]     # package manifests (package.json, pubspec.yaml, gemfile, go.mod, csproj…)
    files: [public/locales/*/]         # marker files/dirs/globs
  catalog:
    format: json               # kapi format id that reads it (json|po|arb|xcstrings|androidxml|applestrings|xliff|properties|resx|yaml|kbf)
    layout: public/locales/{lang}/*.json
  kapi:
    preset: react-i18next      # kapi init --framework id, or null
    flags: "--format i18next"  # extra flags the skill must pass, or null
  toil:
    stock:       { w: 2, x: 2, s: 2, r: 1, e: 1 }
    recommended: { w: 2, x: 1, s: 1, r: 0, e: 1 }
    grade: T2
    buys:                      # what buys each improvement; mirrored in prose in the reference
      - "x: i18next-parser in CI"
      - "s: kapi project convergence + i18next-parser"
      - "r: kapi pseudo-translate + translate-qa gate"
  watch:                       # drift watermarks for the triage workflow
    repo: https://github.com/i18next/react-i18next
    latest_verified: "15.x"
    verified: 2026-07-10
  notes: "Suffix plurals (key_one/key_other); pass --format i18next to kapi."
```

`status` semantics: `recommended` is the skill's default pick for its ecosystem;
`supported` is a sound choice with a playbook provided; `caution` means adoption
gets an explicit warning (grade T3+, or E ≥ 2); `avoid` means do not start new work on it
(dormant/dead), reference names the migration path.

## 5. Keeping it honest (maintenance contract)

- **Per-framework refresh**: the `refresh-i18n-framework` skill (`.skills/`)
  audits one framework: re-verify versions/stewardship on the web, re-score
  both vectors against §2, update the reference and registry, bump `verified`.
  Also the procedure for **adding** a framework (it contains the template).
- **Fleet sweep**: the `i18n-triage` workflow fans out one agent per registry
  entry, checks the watch watermarks for drift (new major, deprecation,
  stewardship change), proposes re-scores with citations, and applies the
  sticky-anchor rule: an uncited score move is suppressed. Run it periodically
  (quarterly, or when a big ecosystem event lands).
- **Skill evals**: `cli/skills/EVALS.md` carries the trigger scenarios for
  the adoption flows; re-run after changing the SKILL.md description.
- **Lockstep**: preset ids, format ids, and flags named in the references are
  claims about the CLI; a change to either side lands in one PR.

## 6. Open questions

- Should the toil vector be published on the website (like
  `/format-maturity`) or stay skill-internal? Skill-internal until the data
  model stabilizes.
- MessageFormat 2: ICU still ships it as tech preview and no major framework
  or TMS defaults to it (verified 2026-07). Reassess on any of: ICU
  de-previews MF2, TC39 `Intl.MessageFormat` reaches Stage 2.7, or a top-tier
  framework/TMS defaults to it.
- Whether `kapi init` should learn auto-detection (run the registry's `detect`
  signals itself) rather than the skill doing detection; candidate follow-up
  once the registry has proven its shape.
