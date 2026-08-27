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
  held to zero mentions. `skilleval` refuses to publish a run that would break
  this rather than editing the transcript, because quietly rewriting recorded
  evidence to get past a lint is not something an evidence page should do.

## Evals that spend

Anything calling a model commits its dataset; a build cannot be asked to pay for
it. State the sampling settings on the card. Every spending eval currently
answers "not recorded", which is true: `providers/ai` carries a
`Config.Temperature` that Ollama honours, Bedrock honours unless it is zero, and
Anthropic, OpenAI, Azure and Gemini drop on the floor. A cloud eval therefore
runs at whatever the API defaults to, and a command alone does not reproduce a
number that was sampled.

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

## Where the gaps are

`/evals` is the answer, and it is generated, so it does not go stale here. What
it shows at the time of writing: four evals not built (three on the authoring
side, one comparing kapi to other converters), the parity suite publishing
nothing, and the engine benchmark reporting a run in which no file succeeded
([#2221](https://github.com/neokapi/neokapi/issues/2221)).
