# Tests and evals

The `/evals` page publishes evaluation evidence from `scripts/evalindex`.
This guide explains how to register evaluations and publish their results.

## Three bands

Evaluations are grouped by the subject under test.

| Band | Subject | Evidence | Gates CI |
| --- | --- | --- | --- |
| Engine and formats | kapi's own code | deterministic | yes |
| AI and context | model output under governance | sampled | no |
| Agent skills | an agent driving kapi | scenario-scored | no, never runs in CI |

Within each band, evaluations follow the six architecture decision series
(F, E, C, M, A, S). Each evaluation appears beside its relevant decision.
Series without evidence remain visible as coverage gaps.

## Adding an eval

1. Write the harness. It owns its own cadence and its own dataset.
2. Add a card to `scripts/evalindex/evals.go` and list its id under a layer in
   `registry.go`.
3. `make eval-index`, then `go test ./scripts/evalindex/`.

Run `make eval-index` after **re-running** an eval too, not only after adding
one. The index records each dataset's date, so a refresh makes the committed
index stale and its drift test fails. The format-maturity publish
(`scripts/format-ops/bootstrap-publish.mjs`) rebuilds the index as its last
step, and `make pre-push` runs `go test ./scripts/evalindex/` whenever a
dataset, the index, `scripts/evalindex/` or the Makefile changes.

Registration tests verify that each card references an existing Makefile target,
page and dataset. Judged evaluations must declare their validation status.
Use the `Misses` field to state limitations and unmeasured behavior.

## Datasets record their own date

`scripts/evalindex/freshness.go` reads timestamps from datasets. Give new
harness output a `generated` field so readers can assess its age. Do not
enter dates manually on cards or store computed ages in committed artifacts;
the page calculates age from the recorded date.

## Each card shows its number

`scripts/evalindex/headline.go` extracts a headline result from each dataset.
Register an extractor there instead of entering a result manually on the card.
`TestARegisteredHeadlineResolves` fails if a registered extractor returns no
result, including when the dataset schema changes.

Ensure comparisons use equivalent inputs. For example, comparing total engine
runtimes is misleading when the engines succeed on different files. A speed
comparison must use the files processed successfully by both engines.

## Dataset publication guards

Two CI guards apply to published datasets:

- **No absolute home paths** (`scripts/check-abs-paths.sh`). Scrub recorded
  paths before writing transcripts and benchmark errors. The guard permits
  placeholder user names `me`, `dev`, `demo`, `user`, `test` and `you` for
  path-scrubbing tests.
- **No downstream product references** (`scripts/check-docs-bowrain-clean.sh`).
  Content published under `web/src` must follow this rule, including scenario
  text and agent transcripts.

When a transcript contains a downstream product reference, `skilleval` omits
it from publication and records the omission. The result retains its verdict,
gate results, message counts and file changes. The report counts omissions so
readers can see what evidence is unavailable. This publication guard does not
apply to runs written elsewhere with `-out`.

## Evals that spend

Evaluations that call a model commit their datasets so builds can use the
results without making paid calls. Record sampling settings on the card and
set them explicitly in the harness:

```go
Temperature: new(0.0)   // request greedy decoding
```

`Config.Temperature` is a `*float64` so an explicit zero remains distinct from
an omitted setting. `TestEveryProviderSendsTemperature` verifies that providers
forward the configured value. A fixed temperature reduces sampling variation;
it does not guarantee identical results across runs.

## Validating a judge

A judged score cannot be trusted above the judge's measured agreement with a
person, and until that measurement exists the dashboard withholds the judged
dimension. Three steps, and the middle one needs a human:

```bash
make judge-candidates   # sweep and save every scored translation (costs calls)
make judge-label        # answer y/n per criterion, resumable, roughly 20 minutes
make judge-validate     # measure Cohen's kappa and record it in the history
```

Labeling hides the judge's verdict, producing model and experimental condition
to reduce bias. Items are shuffled with a fixed seed, preserving the order
when a session resumes. Record uncertain items as skipped; the report counts
them separately.

The minimum is 100 labeled items, with a target of 150 per session. Smaller
samples can produce confidence intervals too wide to assess agreement.

## Evals that drive an agent

`scripts/skilleval` runs the agent-skill and MCP scenarios. It never runs in CI:
it drives `claude -p` with local credentials and consumes account allowance.
Builds use the committed dataset and display its measurement date.

```bash
make skill-eval             # does the skill fire (cheap, 4-turn cap, 3 repeats)
make mcp-eval               # does an agent pick the right MCP tool
make skill-eval-completion  # does it finish the job (slow, 40-turn floor)
```

The same program runs the agent evaluation, which scores whether agents apply
and record a project's conventions against a generated fixture's answer key.
Its phases and budget are in the [agent evaluation runbook](agent-evaluation.md).

When adding a scenario:

- **Provide realistic fixtures.** Every file named in the prompt must exist.
  A cross-format task needs content in multiple formats; a Markdown-only task
  may be better served by native search tools.
- **Separate trigger and completion budgets.** Trigger mode uses a short turn
  cap to check skill selection. Completion mode needs enough turns to finish
  the task and run its gate.
- **Define a completion gate.** Scenarios without one receive `no gate` and
  are counted separately from passes.
- **Pass `-p .` to project commands in gates.** The isolation contract sets
  `KAPI_NO_PROJECT=1`, disabling automatic recipe discovery.

Run each gate against both incomplete and completed workspaces.
`TestAGateIsRedBeforeTheAgentRuns` verifies that every gate fails against its
initial fixture. Use `kapi voice validate` to assess whether a profile is
usable: a check alone can report no findings for an empty profile.
`TestEveryFixtureRecipeLoads` verifies fixture recipes through kapi's loader.

### The session is published, not summarised

Each session records assistant messages, tool calls and tool results. These
records let readers investigate outcomes beyond the dataset's counts and tool
list.

Per-scenario transcripts are staged under `web/static/skill-eval/transcripts/`
and published to the documentation CDN with the run artifacts. The page fetches
a transcript when its row is opened, keeping it out of the main dataset bundle.
Transcripts retain complete messages and tool results. `Run.record` scrubs
paths before storing events.

Pruning is scoped to the evaluated surface: `-only mcp` replaces MCP
transcripts while retaining skill transcripts. A run written elsewhere with
`-out` keeps events inline. Use `-transcripts <dir>` to write separate files.

## What the control arm found

The first fully gated sweep with the unaided control, over 17 scenarios:

```
kapi enabled 3, eased 1, hindered 3, neither 10
```

`hindered` means the agent with kapi failed where the unaided one passed. This outcome is counted separately from scenarios where both arms succeed or
both fail.

Two of the three are the same failure: **the kapi route extracts a catalog and
stops.** p09 produced `i18n/src/App.klf` and never touched `src/App.jsx`; p14
produced the catalog, edited `App.jsx`, and left `<h1>Welcome back, Alex</h1>`
in it. Neither app is translatable, and both look finished from the catalog
alone. The completion gate therefore checks both catalog creation and removal of
the hardcoded string from the component.

The third is #2227.

The counts are also not the whole comparison. The unaided arm was shorter on
most scenarios and often several times shorter, so the page reports the message
totals beside the outcome counts. In this sweep, some tasks required kapi, while others took fewer messages
without it.

## Where the gaps are

See the generated `/evals` page for current coverage gaps.

Building the last four evals turned up five bugs, four of them since fixed, and they are
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
  timings ([#2221](https://github.com/neokapi/neokapi/issues/2221) tracks the
  republished dataset).

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
answer on the term where a person reads it in a diff. `kapi terms expand`
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
