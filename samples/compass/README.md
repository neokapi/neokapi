# Compass — the app-interface sample

Compass is the Northsea product whose interface strings the monolingual sample
already governs, given three target languages and nothing else. It exists to show
that the multilingual loop is an **extension** of the first journey rather than a
second product: the same point, the same voice profile, the same vocabulary, one
more axis.

It also carries the delivery edge. `site/` is a deployable page whose language
picker is built from `kapi status --ship`, so the effect of a review is visible
in the product rather than only in a report. One language is deliberately left
mid-loop, and the picker withholds it until the review clears.

Everything here runs offline from the committed files: the AI leg is the built-in
`demo` provider, which needs no key and no network.

This README follows the project's own register
([brand-communication.md](../../docs/internals/brand-communication.md)); the
content **inside** the sample is Northsea's own product copy.

## The fiction

Northsea Maritime Systems sells software to port and fleet operators. The names,
the products and the domain vocabulary are shared with the monolingual sample
([`../northsea/`](../northsea/)) and with the context evaluation corpus
(`scripts/contexteval/`), so one fictional company serves every fixture, eval and
recording.

**Compass** is the berth plan and fleet view a duty officer keeps open all day.
It ships in English at four North Sea terminals, and its operators read Norwegian,
German and Dutch.

## The shape

```
samples/compass/
├── kapi.yaml                 # the recipe: one point, three target languages, two gates
├── .kapi/
│   ├── voice.yaml            # the Northsea voice, cut to the one channel this ships on
│   ├── terms.json            # the committed vocabulary record
│   ├── memory/               # approved wording, per language — the recycle corpus
│   │   ├── compass-nb.memory.json
│   │   └── compass-de.memory.json
│   ├── state/                # the committed review record: who approved what, and of which text
│   └── .gitignore            # work/ is derived; everything else is committed
└── site/
    ├── index.html            # the deployable page
    ├── language-picker.js    # the picker, built from ship.json
    ├── ship.json             # written by `kapi status --ship --emit`
    └── locales/
        ├── en-GB.json        # the source catalog
        ├── nb.json           # the target catalogs the loop owns
        ├── de.json
        └── nl.json
```

There is no copy step between the loop and the page. `base: site/locales` binds
the collection where the site loads its catalogs from, so `kapi up` materializes
straight into the deployed tree and the page reads exactly what converged.

## The point, and the axis that was added

| Axis | Value |
| --- | --- |
| Profile | `northsea` |
| Channel | `app` |
| Point | `northsea/app` — the same point the monolingual sample governs |
| Source language | `en-GB` |
| Target languages | `nb`, `de`, `nl` |

`kapi context site/locales/en-GB.json` answers with the profile, the channel, the
voice profile in force and the collection — the same answer the monolingual
sample gives for `app/strings.en.json`. Adding `target_languages` did not move
the content or change what governs it.

The voice profile governs the translations too, not only the source. A target is
drafted under the register and the vocabulary bound at the point its source sits
at, which is what stops *vessel* becoming *ship* on the way into another language.

## The two gates

A language picker asks two independent questions, and the recipe answers them
with two gates over the same lifecycle ladder:

| Gate | Recipe | Question it answers |
| --- | --- | --- |
| `ship_gate` | `translated: 100`, `reviewed: 50` | Is this language safe to offer at all? |
| `verified_gate` | `reviewed: 100` | Has a person signed off every string in it? |

One bar for every language, deliberately. A per-locale bar would make the
picker's three states an artefact of the recipe rather than of the work.

The picker reads `kapi status --ship`, whose whole output is locale →
`{shippable, verified}`:

| Ship state | `shippable` | `verified` | In the picker |
| --- | --- | --- | --- |
| governed | true | true | offered, unmarked |
| ai-shippable | true | false | offered, marked **AI** |
| pending | false | — | not offered |

The third row is the point of the sample. `nl.json` exists, it holds Dutch, and
until the gate clears it the picker does not offer it. A catalog being present is
not a decision; the gate is.

## The state this sample starts in

The committed tree is what a duty officer's project looks like on a Tuesday. The
source gained a **Tide window** panel last week — four strings — and no language
carries it yet:

| Language | Catalog | Review record | Standing |
| --- | --- | --- | --- |
| `nb` | 34 of 38 strings | all 34 reviewed (2026-03-04) | the mature language |
| `de` | 34 of 38 strings | 20 of 34 reviewed (2026-06-18) | mid-review |
| `nl` | 20 of 38 strings | nothing reviewed | the newest language |

So `ship.json` starts empty of offers, and the deployed page shows English alone.
Four untranslated strings is not a build failure anywhere — target-language drift
is the ordinary, continuous work the loop exists to absorb — but it is enough to
hold every language behind the ship gate, which is the honest answer.

## Running the journey

From a copy of this directory (the commands assume kapi on `PATH`):

```bash
kapi status                                       # both axes, on one screen
kapi context site/locales/en-GB.json              # the point, and what governs here
kapi status --ship --emit site/ship.json          # what the site may offer today
```

Converge. Approved wording is recycled first; the remainder is drafted by the
`demo` provider, whose output is deliberately and visibly synthetic:

```bash
kapi up
```

```
seeded: 5 concept(s) and 5 content-memory entry(ies) from 3 committed source(s)
absorbed: 86 pair(s) from 3 committed target document(s) — 30 learned, 0 reconciled
plan: 26 unit(s) missing · drafting 2 unit(s) the content memory does not answer · 5 exact-content memory · 23 AI · ≈241 tokens
  nb         38/38 units  (content memory 37 · AI 1)
  de         38/38 units  (content memory 35 · AI 3)
  nl         38/38 units  (content memory 19 · AI 19)
```

Norwegian and German now clear the ship gate on drafted work; Dutch does not,
because nothing in it has been reviewed:

```bash
kapi status                                       # nl: blocked: review
kapi status --ship --emit site/ship.json          # nb, de offered (AI); nl withheld
```

Review is a change-set, and the review queue is already in the shape of one. This
approves the Dutch a person wrote and leaves what the stub drafted:

```bash
kapi status --review --json --jq '.pending[]
  | select(.locale == "nl" and (.target | startswith("⟦") | not))
  | {kind: "review", op: "add", file, id: .key, locale, status: "reviewed"}' > dutch.json
kapi apply dutch.json
kapi commit                                       # into .kapi/state — attributable, committed
kapi status --ship --emit site/ship.json          # Dutch is offered now, marked AI
```

Finish Norwegian and its marker comes off:

```bash
kapi status --review --json --jq '.pending[]
  | select(.locale == "nb")
  | {kind: "review", op: "add", file, id: .key, locale, status: "reviewed"}' > norwegian.json
kapi apply norwegian.json
kapi commit
kapi status --ship --emit site/ship.json          # nb: shippable and verified
```

Serve `site/` and the picker follows every one of those steps with no other
input:

```bash
cd site && python3 -m http.server 8000
```

## Definition of done

The seven points the shaped samples are held to, as this sample meets them.

| # | Point | Standing |
| --- | --- | --- |
| 1 | Onboarded through the discovery path — the graph arrives as reviewable files | **MET** — `kapi.yaml`, `.kapi/voice.yaml` and `.kapi/terms.json` are the drafted-then-corrected artifacts the monolingual sample established, carried forward rather than re-authored |
| 2 | Governance bound at the point from day one; review workflow on | **MET** — `profiles.northsea` binds voice and channel; `.kapi/state/` carries 54 committed decisions before the loop is ever run |
| 3 | First converge shows recycle numbers and an estimate before it spends | **MET** — `plan: 26 unit(s) missing · drafting 2 unit(s) the content memory does not answer · 5 exact-content memory · 23 AI · ≈241 tokens`, then per-locale `(content memory N · AI M)` summing to the same 23. No credential is spent: the AI leg is the `demo` provider |
| 4 | Governed review exercised, with a decision that changes an outcome | **MET** — the Dutch review moves `nl` from withheld to offered, and the Norwegian review removes its AI marker. Both are `kapi apply` + `kapi commit` round-trips landing in `.kapi/state/` |
| 5 | Delivery proven | **MET** — `kapi up` materializes into `site/locales/`, `kapi status --ship --emit` writes `site/ship.json`, and the deployed page reads both. No copy step, no second pipeline |
| 6 | Recorded as a harness walkthrough | **PARTIAL** — `harness/demos/s1-compass-multilingual/` is authored and capture-verified; nothing has been rendered or published for English, and the Norwegian render is held by [#2032](https://github.com/neokapi/neokapi/issues/2032) |
| 7 | Carries no internal information; lives where a reader can clone it | **MET** — one fictional company, in-repo under `samples/` per the sample conventions |

## Known gaps this sample exercises

Running the journey is how these were found.

| Gap | Issue |
| --- | --- |
| The render and CDN publish of the walkthrough — nothing published for English, and Norwegian held besides | [#2032](https://github.com/neokapi/neokapi/issues/2032) |

Fixed while this sample was being built, each found by running the journey on it:
`kapi status` reporting source readiness as `checked 0%` immediately after a
`kapi up` that reported `Settled source: 38 block(s) checked`, because the source
ladder was re-derived from files a JSON catalog cannot write a per-block status
into ([#1928](https://github.com/neokapi/neokapi/issues/1928)); the built-in
`translate` flow's AI step overwriting every unit `recycle` had just
filled from approved wording; the record absorber dropping a translation
identical to its source, so `settings.terminal` was re-drafted on every pass and
the reviewer's approval discarded with it
([#1927](https://github.com/neokapi/neokapi/issues/1927)); a source rewrite
mispairing every locale that held no decision, so `nb` and `nl` went on serving
the translation of a deleted sentence
([#1964](https://github.com/neokapi/neokapi/issues/1964)); the checks gate reporting
that approved identical translation as untranslated, and disagreeing with the
checks the loop runs about which units it holds back
([#1973](https://github.com/neokapi/neokapi/issues/1973)); the plan pricing a
produced unit by whether a target file exists while the pass drafts whatever the
content memory does not answer, so a rewrite quoted one provider call and spent
three ([#1974](https://github.com/neokapi/neokapi/issues/1974)); `kapi apply` refusing
the indented change-set that `kapi status --review --json --jq` prints, so the
review round-trip did not compose; and `kapi commit` writing absolute machine
paths into `.kapi/state/` when the recipe was named by a relative `-p`.

## Where it is used

- `harness/demos/s1-compass-multilingual/` records the journey above as a
  narrated walkthrough, seeded straight from this directory (`fixturesFrom`), so
  the recording and the sample cannot drift apart.
- [`../northsea/`](../northsea/) is the same company, the same point, one
  language — the journey this one extends.
