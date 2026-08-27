# Tests and evals

Everything kapi publishes as evidence lives at `/evals`, generated from
`scripts/evalindex`. This is the working note for adding to it.

## Three bands

The top-level split is by **what an eval has under test**, because the subject
decides what its numbers can mean.

| Band | Subject | Evidence | Gates CI |
| --- | --- | --- | --- |
| Engine and formats | kapi's own code | deterministic | yes |
| AI and context | what a model writes under governance | sampled | no |
| Agent skills | an agent driving kapi | scenario-scored | no, never runs in CI |

Inside a band the structure is the six AD series (F, E, C, M, A, S), so an eval
sits beside the architecture decision describing what it measures, and a series
with nothing behind it shows as a hole rather than as an absence from a list.

## Adding an eval

1. Write the harness. It owns its own cadence and its own dataset.
2. Add a card to `scripts/evalindex/evals.go` and list its id under a layer in
   `registry.go`.
3. `make eval-index`, then `go test ./scripts/evalindex/`.

Run `make eval-index` after **re-running** an eval too, not only after adding
one. The index records each dataset's date, so a refresh makes the committed
index stale and its drift test fails.

The tests will refuse the card unless every claim in it resolves: a `make`
target must exist in the Makefile, a page must have a file, a dataset must
exist, a judged eval must declare its validation or admit it has none. That is
not ceremony. The first draft of the registry named four commands that do not
exist and linked a page retired two releases earlier, and every one of them read
perfectly plausibly.

`Misses` is the field that makes a card evidence rather than advertising. A card
that says only what it covers invites a reader to assume the rest, and the
assumption is always more generous than the truth.

## Datasets record their own date

`scripts/evalindex/freshness.go` opens each dataset and takes the timestamp out
of it. Do not type a date on a card: a hand-written date is a date nobody
updates.

Give a new harness a `generated` field. Without one the page cannot show an age,
and a stale dataset reads exactly like a fresh one. That is how `/pseudobench`
came to render results measured on 2026-05-20 from a file committed in July with
nothing saying so.

Record the **date only**, never a computed age. An age baked into a committed
artifact is wrong the next morning, and the drift test that keeps the index
honest would fail daily and be muted inside a week. The page does the
subtraction.

## Each card shows its number

A status is not a result: "partial" is true of an eval at 96% and of one at 40%.
`scripts/evalindex/headline.go` extracts one number per eval **from its dataset**
and the row renders it before anyone clicks.

Register an extractor there rather than typing the number on the card. A typed
number goes wrong exactly the way a typed date does, and in the flattering
direction. `TestARegisteredHeadlineResolves` fails if a registered extractor
returns nothing, which is what happens when a dataset's shape moves under it.
Three of the first five read keys that do not exist, and the only symptom was a
blank column.

Pick the comparison the number claims to be, and check it is the one being
computed. Engine speed divided the two engines' total wall times, and the
engines do not succeed on the same files (725 against 802), so it compared one
engine's corpus against another's. Summing only the files both read gives 23.8×
where the totals gave 22.7×. Close enough that nobody would have caught it, and
no reason the next run would be as kind.

## Two rules the committed datasets keep tripping over

Both are CI guards, and both fail on a build that looks unrelated to whatever
you changed.

- **No absolute paths** (`scripts/check-abs-paths.sh`). An agent transcript or a
  benchmark error carries the temp workspace and the developer's home. Run
  recorded strings through a scrubber before they reach the file. Note the guard
  allows the placeholder names `me`, `dev`, `demo`, `user`, `test`, `you`, since a
  test for a path scrubber has to contain paths.
- **No downstream product references** (`scripts/check-docs-bowrain-clean.sh`).
  The datasets land under `web/src`, which the docs site serves and which is
  held to zero mentions. Scenario text is under the suite's control and a test
  keeps it clean; a transcript is not, because the shipped skill's own
  description names the platform and an agent can repeat it on any scenario.

  `skilleval` withholds the transcript rather than refusing the run. Refusing
  was the first version, and it threw away a seventeen-scenario sweep for one
  word in one closing message. The result keeps its verdict, gate, message
  counts and file changes and loses the prose, and the omission is recorded on
  the result and counted on the report so the page can say what is missing. An
  omission a reader can see is not a quiet rewrite. The guard applies only when
  publishing under `web/src`; a run sent elsewhere with `-out` is for reading.

## Evals that spend

Anything calling a model commits its dataset; a build cannot be asked to pay for
it. State the sampling settings on the card, and pin them in the harness:

```go
Temperature: new(0.0)   // greedy, so a re-run lands on the same numbers
```

Set it with `new(expr)`. `Config.Temperature` is a `*float64` because 0 is a
real request rather than an absent one, and it used to be a bare `float64` with
`omitempty`, which made greedy decoding the one value you could not ask for.
Worse, four of six providers accepted the field and never sent it, so every eval
in this repo sampled at whatever the API defaulted to and none of them wrote it
down. `TestEveryProviderSendsTemperature` keeps that from coming back.

## Validating a judge

A judged score cannot be trusted above the judge's measured agreement with a
person, and until that measurement exists the dashboard withholds the judged
dimension. Three steps, and the middle one needs a human:

```bash
make judge-candidates   # sweep and save every scored translation (costs calls)
make judge-label        # answer y/n per criterion, resumable, roughly 20 minutes
make judge-validate     # measure Cohen's kappa and record it in the history
```

The loop is blind on purpose. It never shows the judge's verdict, the model, or
whether the translation came from the steered or the bare pass, because a
labeller who sees any of them agrees with it and the result becomes a
measurement of suggestibility. Items are shuffled deterministically, so fatigue
does not correlate with condition, and the same seed replays the same order on
a resume.

"I am not sure" is an answer. A forced guess is noise that kappa cannot tell
from disagreement, and it drags the estimate toward chance in a way that looks
like a bad judge. Skipped items are recorded and counted.

The floor is 100 items and a session aims for 150. Thirty was the old number
and it was optimistic: a kappa over thirty items carries an interval wide
enough to span "substantial" and "poor", which is the situation this exercise
exists to get out of.

## Evals that drive an agent

`scripts/skilleval` runs the agent-skill and MCP scenarios. It never runs in CI:
it drives `claude -p` with local credentials and real money, so the committed
dataset is all a build sees, and the date on it is the real currency.

```bash
make skill-eval             # does the skill fire (cheap, 4-turn cap, 3 repeats)
make mcp-eval               # does an agent pick the right tool of nineteen
make skill-eval-completion  # does it finish the job (slow, 40-turn floor)
```

Four things about scenarios are worth knowing before writing one:

- **The fixture is the scenario.** A prompt about `pitch.pptx` in an empty
  directory tests nothing, and a "sweep `docs/`" scenario whose `docs/` holds
  only Markdown correctly does *not* trigger, because native grep is the better
  tool there. Both look like skill defects and neither is. The cross-format
  sweep searched for a word its `.docx` did not contain and so only ever spanned
  the Markdown file; it now renames a product appearing nine times inside two
  binaries.
- **Triggering and finishing are different budgets.** Four turns is plenty to
  see a skill fire and stops a positive running away into metered translation.
  Completing takes tens of turns, and gates that failed at eight passed at
  forty, which measured the cap rather than the skill.
- **A scenario with no gate is not a pass.** It has no definition of done, so
  nothing about it was verified; the verdict is `no gate` and it is counted
  separately. Scoring those on triggering once read as "17 pass" on a sweep
  where three scenarios were checked and two of the three failed.
- **A gate that touches a project needs `-p .`.** The isolation contract sets
  `KAPI_NO_PROJECT=1`, so discovery is off and a bare `kapi status` cannot find
  the recipe the agent just wrote.

Run each new gate by hand first, in both directions, against a workspace shaped
like the one an agent leaves behind. A gate that cannot fail proves nothing, and
one that cannot pass wastes a metered sweep discovering its own bug.

`TestAGateIsRedBeforeTheAgentRuns` now does that automatically: it builds every
scenario's fixture and runs the gate against it, requiring a red exit. Reading
the gate as a string was the first version and it missed three, all green before
the agent started. One asked `kapi voice check`, which accepts any YAML at all:
every profile field is optional, so an empty file scores 100/100 with no
findings, and a directory merely containing a `.yaml` satisfied it. Use
`kapi voice validate` when the question is whether a profile is usable.

The fixtures need the same treatment. Both project fixtures were invented, and
every key was wrong. `version: "1"` where the loader wants `v1`,
`source:`/`targets:` for `source_language`/`target_languages`, `include:` for
`content: - path:`. Only the last was ever reported, because `Defaults` and
`Collection` end in an inline `Extras` map and an unrecognised key is preserved
rather than rejected ([#2223](https://github.com/neokapi/neokapi/issues/2223)).
Agents were handed a project kapi refuses to load, and the sweep said nothing.
`TestEveryFixtureRecipeLoads` loads every fixture recipe through kapi.

## What the control arm found

The first fully gated sweep with the unaided control, over 17 scenarios:

```
kapi enabled 3, eased 1, hindered 3, neither 10
```

`hindered` means the agent with kapi failed where the unaided one passed. It had
no name in the first version, which counted it as `neither` and put it beside
scenarios where both arms sailed through. Three of seventeen is the number that
would have been lost.

Two of the three are the same failure: **the kapi route extracts a catalog and
stops.** p09 produced `i18n/src/App.klf` and never touched `src/App.jsx`; p14
produced the catalog, edited `App.jsx`, and left `<h1>Welcome back, Alex</h1>`
in it. Neither app is translatable, and both look finished from the catalog
alone. That is why the gate asks for the string to have left the component as
well: a catalog beside an untouched component is the likelier half-finished
outcome, and it is what an agent following the extraction path produces.

The third is #2227.

The counts are also not the whole comparison. The unaided arm was shorter on
most scenarios and often several times shorter, so the page reports the message
totals beside the outcome counts. kapi reaches answers the unaided agent cannot,
and it is not the cheaper route to the ones it can.

## Where the gaps are

`/evals` is the answer, and it is generated, so it does not go stale here.

Building the last four evals turned up five bugs, all since fixed, and they are
worth keeping here because in each case the eval's first result was about kapi
rather than about the thing the eval set out to measure:

- **`kapi exec voice-check` and `voice-infer` write nothing and exit 0**, under
  every provider, profile and input tried, while sibling tools on the same file
  print results ([#2225](https://github.com/neokapi/neokapi/issues/2225)). Both
  declare `model.AnnoVoice` output, which appears nowhere under `cli/` or
  `host/`. So two of the three authoring evals had no output to score, and
  `voice-infer-quality` is `blocked` rather than absent: its comparison is
  written and runs the moment there is a draft.
- **`kapi apply` rejects every edit to a block with paired inline codes**, even
  one whose codes are byte-identical to the source's
  ([#2227](https://github.com/neokapi/neokapi/issues/2227)). The reader numbers
  a close as its open's id plus one and the guard requires them to match, so
  bold, italic and hyperlink spans in a .docx cannot be edited through the
  `inspect | edit | apply` path the help advertises. 14 of 26 coded blocks
  across the repo's own fixtures; `simple.docx` is entirely uneditable. It was
  found because a scenario failed with kapi and passed without it, and the
  transcript showed the agent producing a correct ten-block change-set and
  then spending a 40-turn budget on the one rejection.
- **A forbidden term matches no inflection**, so a profile forbidding `utilize`
  passes "the platform utilizes your data"
  ([#2226](https://github.com/neokapi/neokapi/issues/2226)). That was the single
  term-mechanism miss in the authoring corpus, and the fix took two attempts;
  see [below](#fixing-the-inflection-miss-took-two-attempts).
- **An empty voice profile scores 100/100** and reports the text as on brand
  ([#2224](https://github.com/neokapi/neokapi/issues/2224)).
- **The engine benchmark wrote 844 files where nobody looked for them.** A
  repo-relative `-output` resolved against each engine's scratch `cmd.Dir`, so
  every file scored "no output written" and the run reported 0/844 with real
  timings ([#2221](https://github.com/neokapi/neokapi/issues/2221), fixed in
  #2220).

Four of the five are the same shape: a surface reporting a refusal, or an empty
result, so quietly that the caller reads it as success. An eval is the only
thing that notices, because each failure is invisible from the outside.

The fifth had a different cause. Nothing about
`apply` looked wrong from the inside: it has a faithfulness guard, the guard
fires, and it reports what it refused. What surfaced it was the control arm:
the scenario failed with kapi and passed without it, and that comparison is the
only signal that pointed at a tool doing its job correctly and uselessly.

## Fixing the inflection miss took two attempts

The obvious fix for #2226 is to derive the forms from the term: add `s`, `d`,
`ing`, handle a trailing `e`. That is what shipped first, and the corpus went
from 12 of 13 terms to 13 of 13.

Then the same check ran against nb, which this repo actually publishes in. It
caught the bare stem and nothing else, missing utnytter, utnyttet, løsningen,
løsninger and løsningene, while generating løsninges and utnytted, which are not
words. It had also needed a floor on term length, because `Go` matched inside
"going" and a test in `core/profile` says it must not. That floor took the forms
away from `use`.

The English suffix rules were not an approximation of morphology. They were
morphology for one language, applied silently to every locale, in a gate.

The tools that do this at scale split along one line, and it is not
mechanical against AI:

| | how a term is matched | what it costs |
| --- | --- | --- |
| Vale | declared strings, exact | nothing, and no morphology at all |
| Lucene, Snowball | stemmed at index and query time | a stemmer per language, and conflations a search can absorb |
| LanguageTool, Acrolinx | full morphology | a linguistic pack per language, which is the product |
| Grammarly, MS Editor | a model reads the sentence | a model call per document, and a gate that varies run to run |

Stemming is the tempting middle and it belongs to search: Snowball folds
"universe" and "university" together, which costs a search engine a place in a
ranking and costs a check an accusation against text that broke no rule.

So the axis is *when* the language knowledge is applied. A model has the
morphology for every language, and asking it once, at authoring time, puts the
answer in the profile where a person reads it in a diff. `kapi voice expand`
does that, and the check that consumes the result stays exact, free,
deterministic and language-neutral. Norwegian goes from 0 of 5 to 5 of 5 and the
English corpus holds at 13 of 13.

The reason this is in the eval notes rather than in an architecture note: the
first fix passed the corpus that found the bug. It had to be run against a
second language before it was visibly wrong, and this repo had one to hand only
because it publishes in it.

## The authoring evals

```bash
make authoring-eval          # all three: checks, infer, the voice guide
make authoring-eval-checks   # the checks alone, free and offline
```

The corpus is synthesized and says so in the data rather than only in the prose
around it. Two of the three questions need ground truth no repository carries,
a profile a person wrote from a known corpus and prose whose every violation is
marked, and labelling real material to that standard is the eval rather than
preparation for it.

Recall is measured over the documents written against the profile, where every
violation is marked, and false positives over the documents written to it, where
the right answer is silence. Neither half substitutes for the other. An
off-profile document contains violations beyond the marked ones, so counting
unmarked findings there measures how complete the marking is rather than how
good the check is; the first version pooled them into one precision figure and
reported 61%, none of which was about kapi.

Split recall by which of the three mechanisms a profile states a rule through,
because the answer differs completely between them: terms and patterns are
matched offline, and `active_voice`, `person_pov` and the rest of the enum
fields are not evaluated by anything offline at all. They reach the guide and
the LLM check. A profile saying `active_voice: true` scores dense passive prose
100/100 offline, and a reader who does not know which mechanism a rule uses
cannot tell a clean document from an unchecked one.

## Comparing against other tools

```bash
make conversion-eval   # every converter installed, over the parity corpus
```

`scripts/conversioneval` compares document converters, and the hard part is
ground truth. Scoring against pandoc's output would measure agreement with
pandoc. OOXML avoids that: the spec designates which elements carry text, so
each document states its own contents and no converter stands in for the answer.

Three things that comparison has to get right, and each was wrong first:

- **Ask each tool only for what it claims.** `--convert-to txt` has no Impress
  target, so LibreOffice was scored eight failures for a capability it does not
  offer, and it looked broken across two thirds of the corpus.
- **Weight by content, not by file.** The corpus holds two-word fixtures;
  averaging per-file recall gives one of those the same vote as a full report,
  and every converter scored 0% on the same two-word document.
- **Then check what the weight lands on.** Weighting by content makes one large
  document the score: `large.xlsx` carries 99% of the spreadsheet ground truth,
  so the .xlsx row is one workbook wearing a corpus's clothes. The dataset now
  records the top file's share and the page says so above 50%.
- **Read the cells, not the string table.** `xl/sharedStrings.xml` holds each
  distinct string once and the sheets refer to it by index, so a string in five
  hundred cells appeared once in the truth and five hundred times in the output.
  Recall is min(output, truth)/truth, so undercounting the truth made every
  score easier, and both converters returned exactly 100.0% on .xlsx. A metric
  that cannot fail looks precisely like a good result.
- **Compare within a format.** The tools accept different ones, so a single
  column ranks them by what they declined.

The corpus is the okapi-testdata tree the parity harness already downloads. It
matters that it was collected by another project for another purpose.
