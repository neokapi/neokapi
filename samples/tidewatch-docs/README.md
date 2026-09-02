# Tidewatch docs: the documentation-site sample

The Tidewatch operator handbook is a documentation site that converges in CI. It
exists to show the other half of the multilingual journey: not the delivery edge
(that is [`../compass/`](../compass/)), but the continuous condition a translated
site lives in, with the source moving every week, the languages following at their
own pace, and a build that must never fail because one of them is behind.

It is the same fictional company and the same point as the monolingual sample:
`northsea/docs`, one voice profile, one vocabulary, plus a locale axis.

Everything here runs offline from the committed files: the AI leg is the built-in
`demo` provider, which needs no key and no network.

This README follows the project's own register
([brand-communication.md](../../docs/internals/brand-communication.md)); the
content **inside** `docs/` is Northsea's own handbook prose.

## The shape

```
samples/tidewatch-docs/
├── kapi.yaml                 # the recipe: one point, two target languages
├── docusaurus.config.ts      # the i18n block, and the source-strict/target-warn line
├── .gitignore                # i18n/ is build output
├── .github/workflows/kapi.yml # the CI leg: plan on a PR, converge on main, build regardless
├── .kapi/
│   ├── voice.yaml            # the Northsea voice, cut to the docs channel
│   ├── terms.json            # the committed vocabulary record
│   ├── memory/               # approved Norwegian wording — the recycle corpus
│   ├── state/                # the committed review record
│   └── .gitignore
└── docs/
    ├── index.md              # what Tidewatch reads and produces
    ├── berths.md             # declaring a berth; constraints changing mid-window
    ├── alerts.md             # the alert lifecycle
    └── integrating.md        # the read API, and the field name the wire keeps
```

## What is committed, and what is not

This sample and `compass/` treat their targets differently, and the difference is
the point of having both:

| | `compass/` | `tidewatch-docs/` |
| --- | --- | --- |
| The target | `site/locales/<lang>.json`, **committed** | `i18n/<lang>/…`, **gitignored** |
| Why | the app's catalogs are the record a reviewer diffs | a Docusaurus i18n tree is build output, regenerated from the source plus the committed context |
| The committed record is | the catalogs plus `.kapi/state/` | `.kapi/memory/` plus `.kapi/state/` |

Both are real conventions. This repository's own docs collections are arranged
the second way. What matters is that the record is committed somewhere and that
generated translations never arrive in a pull-request diff.

## Front matter is content

A reader sees the title in the sidebar and the description in a search result, so
`title`, `description` and `sidebar_label` converge with the prose. Every other
front-matter key is configuration and is left alone.

```yaml
format:
  name: markdown
  config:
    translateFrontMatter: true
    frontMatterKeys: [title, description, sidebar_label]
```

The target path is Docusaurus's own default translation path, so there is no copy
step and no plugin between the loop and the build:

```
i18n/{lang}/docusaurus-plugin-content-docs/current/{path}.md
```

## The CI leg

`.github/workflows/kapi.yml` uses the two published actions as published.
`neokapi/setup-kapi@v1` installs and caches the CLI, `neokapi/kapi-action@v1`
runs one kapi command and reports what it found. Delivery is the workflow's own
step, which is why the action does not commit for you.

Three jobs, and the relationship between them is the whole arrangement:

| Job | On | What it does |
| --- | --- | --- |
| `plan` | pull request | a dry run: what is pending, what recycles, what the remainder would cost, posted as one sticky comment, plus a coverage summary |
| `converge` | push to `main` | runs the loop and commits what moved under `.kapi/` |
| `build` | both | builds the site, and **depends on neither of the other two** |

`fail-on-parked` stays `false`, deliberately. *Parked* means work remains that the
loop could not carry to the ship gate, a language awaiting review most often,
and that is the ordinary state of a translated site rather than a broken build.

## Target-language drift never blocks

Three independent mechanisms, all visible in this sample:

1. **The gate is on delivery, not on the build.** Dutch sits at `blocked: review`
   and the site still builds and still serves. Whatever reads
   `kapi status --ship` withholds Dutch; nothing withholds the site. The Dutch
   drafts are in the project store, so `kapi status` grades them and
   `kapi status --review` lists all 60 of them while `i18n/nl/` stays empty. A
   reviewer works on the locale where it stands, and the next `kapi up` reads
   the same drafts back rather than paying a provider for them again
   (`content memory 0 · drafts 60 · AI 0`).
2. **The source gate is about the source.** `kapi check --strict` passes here with
   one advisory finding, `integrating.md` keeping `mooring_id`, the field name
   the published wire contract keeps after the vocabulary retired the word. A
   retired term is a **minor** finding, because a migration that fails builds is a
   migration nobody finishes.
3. **The build policy is asymmetric.** `docusaurus.config.ts` throws on a broken
   link in the source locale and warns in a target locale, for the same reason.

## Running the journey

```bash
kapi up                                   # converge; writes the i18n tree
kapi status                               # coverage on both axes
head -6 i18n/nb/docusaurus-plugin-content-docs/current/index.md
```

Report coverage the way CI does:

```bash
kapi status --json --jq '.locales[]
  | "\(.locale)  translated \(.pct.translated)%  reviewed \(.pct.reviewed)%"'
```

Review Norwegian, and it clears the ship gate while Dutch does not:

```bash
kapi status --review --json --jq '.pending[]
  | select(.locale == "nb")
  | {kind: "review", op: "add", file, id: .key, locale, status: "reviewed"}' > nb.json
kapi apply nb.json
kapi commit
kapi status
kapi check --strict                       # PASS — the source is what a PR is held to
```

Dutch takes the same route, from the drafts in the project store. Nothing under
`i18n/nl/` exists yet; the review queue lists the units all the same, and the
`kapi up` after the decisions delivers the locale:

```bash
kapi status --review --json --jq '.pending[]
  | select(.locale == "nl")
  | {kind: "review", op: "add", file, id: .key, locale, status: "reviewed"}' > nl.json
kapi apply nl.json
kapi commit
kapi up                                   # 0 passes, 8 files materialized
ls i18n/nl
```

## Definition of done

| # | Point | Standing |
| --- | --- | --- |
| 1 | Onboarded through the discovery path, so the graph arrives as reviewable files | **MET**: recipe, voice profile and vocabulary carried forward from the monolingual sample rather than re-authored |
| 2 | Governance bound at the point day one; review workflow on | **MET**: `profiles.northsea` binds voice and channel; the review round-trip runs offline through `apply` + `commit` |
| 3 | First converge shows recycle numbers and an estimate before it spends | **MET**: `plan: 96 unit(s) missing · 15 exact-content memory · 81 AI · ≈2k tokens`, then per-locale `(content memory 23 · AI 37)`. No credential spent |
| 4 | Governed review exercised, with a decision that changes an outcome | **MET**: 60 decisions committed to `.kapi/state/`, moving `nb` from `blocked: review` to `ready` while `nl` stays pending until its own drafts are reviewed |
| 5 | Delivery proven, the CI leg | **PARTIAL**: the workflow is authored against the published actions and every kapi command in it is verified locally; it is not executed, because a sample workflow inside `samples/` is not a repository workflow and running it would mean a public sample repository, which this stream does not create |
| 6 | Recorded as a harness walkthrough | **PARTIAL**: `harness/demos/s2-tidewatch-docs/` is authored and capture-verified; nothing has been rendered or published for English, and the Norwegian render is held by [#2032](https://github.com/neokapi/neokapi/issues/2032) |
| 7 | Carries no internal information; lives where a reader can clone it | **MET**: one fictional company, in-repo under `samples/` |

## Known gaps this sample exercises

Running the journey is how these were found.

| Gap | Issue |
| --- | --- |
| The render and CDN publish of the walkthrough, in Norwegian | [#2032](https://github.com/neokapi/neokapi/issues/2032) |

Fixed while this sample was being built: **a review decision recorded without
its file**, because a docs collection repeats its block ids by construction, so every
page carries an `h` and a `p`, and a record keyed on the id alone kept one
approval per id and discarded the rest without a word; the pages that lost
theirs then read as stale against a source nobody had edited, and re-reviewing
them lost the same decisions again
([#2030](https://github.com/neokapi/neokapi/issues/2030)); a collection's reader
config reaching the loop but not the coverage path, so the pipeline line read
`60/48 units` and coverage was computed over a different denominator than the
run produced against
([#1933](https://github.com/neokapi/neokapi/issues/1933)); **a source edit never reaching its
translation**, where rewriting an English sentence left the Norwegian on the old
wording, still counted `translated`, still carrying its approval, still
`✓ shippable`, because a review decision bound only to the target hash and the
plan asked whether a target existed rather than whether it translated the current
source ([#1932](https://github.com/neokapi/neokapi/issues/1932)); the source axis
reporting `checked 0%` right after a run that reported 48 blocks checked
([#1928](https://github.com/neokapi/neokapi/issues/1928)); and a translated heading re-addressing every
block beneath it, so a fully translated page paired with its source only above the
first heading and measured 33% translated, could not be reviewed unit by unit, and
could never clear a ship gate ([#1931](https://github.com/neokapi/neokapi/issues/1931),
mitigated at the pairing site; the structural answer is a language-independent
unit identity on the file-scan path).

## Where it is used

- `harness/demos/s2-tidewatch-docs/` records the journey above as a narrated
  walkthrough, seeded straight from this directory (`fixturesFrom`).
- [`../compass/`](../compass/) is the same company, the same loop, at the delivery
  edge instead of in CI.
