---
title: "AD-036: LLM prompts, batching, and the model boundary"
sidebar_label: "036: LLM prompts & batching"
description: How kapi builds the prompts it sends to a language model, how much it sends at once, and what contains the damage when the content is hostile.
---

# AD-036: LLM prompts, batching, and the model boundary

## Context

kapi sends the user's content to a language model on their behalf. Three questions
follow, and for a long time we answered all three by accident:

1. **What is in a prompt, and who owns each part of it?**
2. **How much content goes into one call?**
3. **What happens when that content is hostile — or merely looks like it?**

The prompts were written inline at each call site, so nothing could see them: not
the user, not the docs, not a test. The batch size was a fixed 100, exposed as a
form field. And the payload was a numbered text blob, mapped back by position.

Each of those was a defect, and they turned out to be the same defect wearing
three hats.

## Decision

### Prompts are built, not written

Every prompt kapi sends is rendered by a typed builder in `core/ai/prompt` from
**attributed sections** — each section carries a `Kind` (task, constraint,
instruction, voice, glossary, content) and an `Origin` saying what produced it.

This buys three things at once:

- **`--explain-prompts`** can show the exact prompt of a real run, with each
  section attributed to the thing that produced it.
- **The [Prompt Reference](/reference/prompts) is generated** from the same
  builders the binary uses, and a CI drift gate fails the build if the committed
  reference stops matching. The docs cannot describe a prompt kapi does not send.
  (They previously did: the old page claimed prompts carried surrounding blocks
  and TM matches. Neither was ever true.)
- **The prompt is fingerprinted** into the translate config, so rewording a prompt
  invalidates cached targets rather than silently serving a translation produced
  by a prompt that no longer exists.

The corollary is that **prompts never fork per provider**. Providers differ in
*mechanism* (Anthropic constrains output by grammar, OpenAI by `response_format`,
Gemini by `response_format`, Ollama by `format`) and in *capability* (schema
subsets, output ceilings), and those differences are absorbed by the
`ChatStructured(messages, JSONSchema)` seam and a declared `Limits()`. They are
never absorbed by rewording a prompt. A provider-specific prompt would escape the
drift gate, the reference and the fingerprint — and we know exactly what that
costs, because Azure once quietly sent a different translate prompt than every
other provider and nobody noticed for months.

### Batch size is derived, not configured

**The binding constraint is max output tokens, not the context window.** For
translation, output tracks input: translating N tokens produces roughly N tokens.
Current frontier models read 1M tokens and emit at most 128k — an order of
magnitude apart — so the ceiling on "how much can we translate in one call" is
what the model can *say*, not what it can *read*. Filling a 1M context with source
text you cannot emit translations for is not a strategy; it is a truncated reply.

**A fixed block count is the wrong unit.** The same 100 blocks are 300 tokens of UI
strings or 200k tokens of prose. kapi packs each call against a token budget
derived from the model's declared output limit, with a backstop on the number of
segments (`MaxBlocksPerCall`).

The block cap started at 16, borrowed from batch-prompting results on adjacent
tasks. Having now measured translation itself (below), it is 64: the sweep found
no structural degradation at any batch size up to 600, and the token budget binds
first on anything longer than a UI string anyway. The cap is a backstop, not the
decision.

**And it is not a decidable question for a user.** The right count depends on the
model's ceiling, on the length of *their* segments, and on a quality-versus-N
curve that has never been published for segment translation. A form field
defaulting to 100 did not give anyone control; it gave them a way to break their
run. The user-facing choice is now `batching: auto | single` — a question they can
actually answer. A numeric pin survives as a hidden override, for recipes that
must reproduce a historical run and for the eval sweep that will measure the curve
we currently take on faith.

**Batching was assumed to be a cost optimisation with a quality cost. Measurement
inverted both halves.** The quality cost did not materialise: structural integrity
was perfect from N=32 to N=600 on every model swept, and the *only* breaks measured
anywhere were at N=8 and N=16. And the cost saving is real but provider-dependent —
it is the per-call fixed overhead (system prompt + JSON schema) being amortised, so
it is large where that overhead is large (Bedrock, ~985 tokens per call) and
negligible where it is small (Gemini, ~106).

What batching genuinely costs is bounded by the **output ceiling**: a batch whose
reply would exceed `NonStreamingMaxOutputTokens` (16k) comes back truncated,
`splitAndRetry` halves it, and the work is redone. That is billed, not broken —
which is why it hid behind a flat 100% integrity line until the tokens were looked
at. A 600-block batch cost 2.4× the tokens and 6× the wall-clock of the same corpus
at N=256.

### The payload is id-keyed JSON

The reply is mapped back by **segment id**, not by position, and kapi accepts only
the ids it sent. A dropped segment is therefore a detectable absence, retried
individually — rather than an off-by-one that silently shifts every translation
after it.

The payload is **JSON**, not `[1] text` lines and not XML tags, for two reasons.
Content containing `[2]` or `</segment>` could forge a boundary in a
delimiter-framed payload, and that needs no attacker — a numbered list is enough.
And the text may carry inline markup as literal placeholder tags (`<ph id="1"/>`)
that the model must reproduce verbatim; XML-escaping the payload would mangle
precisely what the tag-fidelity constraint demands be preserved. JSON escaping is
lossless and cannot be broken out of from inside a string.

Ids are batch-local (`s1`, `s2`, …) rather than the block's content key, because
content keys are content-derived: two blocks with identical source text share one,
which in a batch is a collision, not an identifier.

### Prompt injection is contained structurally, not by asking nicely

The text kapi translates is frequently not written by the person running kapi. The
prompt states the data/instruction boundary as an attributed `constraint` — visible
in `--explain-prompts` and the reference — but that is **defence in depth and
nothing more**. No prompt-level rule reliably stops a model from following an
instruction embedded in its input, and this AD does not claim otherwise.

What contains the damage:

- **No capability.** The call has no tools, no filesystem, no network. An injected
  instruction cannot act; at worst it produces a wrong translation. The blast
  radius is content, not capability.
- **Constrained decoding.** The reply is held to a JSON schema by the provider's
  grammar, so it can fill a shape but not escape it.
- **Id validation.** An injected omission is caught, not silently absorbed.
- **An unforgeable frame.** See the payload decision above.

The residual exposure — an injected instruction corrupting the *text* of a
translation — is what the checks and review passes exist to catch. That is the
honest boundary, and it is the one we document.

## Consequences

- Adding a prompt means adding it to `prompt.Catalog()`; a guard test fails
  otherwise, so a prompt cannot ship undocumented.
- Adding a provider means declaring `Limits()`. A provider that doesn't is treated
  as unknown and packed conservatively — smaller batches, not truncated replies.
- Rewording any prompt moves the fingerprint (re-translating affected blocks) and
  fails the reference drift gate until the docs are regenerated. Both are intended.
- `MaxBlocksPerCall` is now set from a measurement of translation itself
  (`scripts/batcheval`, published at **/batch-eval**) rather than inferred from
  adjacent tasks — and is re-measured as models move, because a ceiling checked once
  decays into folklore.

## Measuring the ceiling

`make batch-eval` (`scripts/batcheval`) sweeps blocks-per-call over a fixed corpus
and scores each N. It scores **structural integrity** — does every segment come
back, under the id it was sent, with its placeholders and inline tags intact? —
rather than wording, for two reasons: that is the failure the literature attributes
to batching, and in a localization pipeline it is a correctness failure rather than
a style complaint. A translation missing its `{0}` cannot be written back into the
source file at all.

Scoring structure needs no reference translations, so the curve can be measured on
any corpus, in any language pair, for the price of the calls.

The corpus deliberately carries what batching is documented to break: long prose
(degradation is worst when items are long), placeholders and inline markup, and the
same source text under two different keys — the case positional batch mapping used
to corrupt silently.

A run against the demo stub exercises the plumbing and measures nothing about any
model. Such runs are marked `"simulated": true` and the dashboard excludes them
from every chart — a stub's flawless curve is exactly the number someone would
otherwise quote.

### The measurement is kept, not taken once

`make batch-eval-publish` sweeps the real models and appends to a committed history
(`web/src/pages/batch-eval/_batcheval.json`), published at **/batch-eval**. Three
choices in that record are what make it worth keeping:

- **Runs are stamped with the corpus digest.** Change the corpus and the digest
  moves, and runs measured on the old one stop being comparable. The dashboard
  says so rather than drawing one line through two different experiments.
- **A same-day re-run corrects its entry** instead of appending a second point, so
  pressing enter twice cannot masquerade as change over time.
- **Transient failures are retried before being recorded.** An overloaded API or a
  dropped socket, published as "the model could not answer at this batch size",
  would be a fabricated cliff — and the most damaging kind, because it is the exact
  shape of the finding we are looking for and would therefore be believed.

### The dashboard plots cost, not quality

The obvious chart — structural integrity against N — is the one this page led with,
and it was the wrong one. Every point sits between 98.8% and 100%, so on a 0–100%
axis it renders as a flat line at the ceiling: a null result drawn as if it were the
finding, and the small real variation (batches of 8 break *more* than batches of 128)
squeezed into invisibility.

So the lead chart is **cost per 1,000 source words against N**, on a log axis, which
is the thing that actually moves: a slope down on the left where the per-call overhead
is amortised (steep on Bedrock, nearly flat on Gemini — it is a property of the
provider's overhead, not of batching), a broad shallow minimum, and a wall on the
right where the output ceiling forces a truncate-and-split and you pay for the same
words twice. Throughput gets the same treatment.

Integrity is still plotted, because a model can regress behind a stable alias and
nobody would be told — but as **breaks per 1,000 blocks**, an axis on which a
regression would be visible, and framed as the guard it is rather than the headline
it is not.

The record exists because model aliases are not stable artefacts. `sonnet` and
`gemini-3.5-flash` point at different weights over time; a ceiling measured once
and never re-checked decays into folklore. Re-run the sweep when the models move.

### The corpus is part of the experiment

The first sweep used a 30-block corpus and reported a flat 100% line at every batch
size to N=32. That result was an artefact, and a self-inflicted one: with 30 blocks,
N=32 does not test a batch of thirty-two — it tests *the whole document in one
call*, and N=16 tests two calls. The sweep had saturated. The ceiling being measured
was the corpus's, not the models'.

The corpus therefore scales (`CorpusN`), holding the stressor mix roughly constant
and generating every block distinct — duplicate source text would let a model
translate one segment and copy it into the next, manufacturing the exact failure the
eval exists to detect. The authored 30 are kept unchanged, so their digest is stable
and the published runs stay comparable.

The real sweep is 600 blocks (9,990 words), N ∈ {8,16,32,64,128,256,600}.

### What the sweep actually found

**There is no batching cliff.** Across `gemini-3.5-flash`, `gemini-3.1-flash-lite`
and `eu.anthropic.claude-sonnet-4-6` on **AWS Bedrock** (the route the Bowrain
platform runs on, and therefore the only one whose numbers describe production),
structural integrity stays above 99% at every batch size — including N=600, six
hundred segments answered in a single call with nothing dropped, merged, renumbered
or stripped of a placeholder.

The batch-prompting literature's degradation-with-N does not transfer to
translation, and the reason is not mysterious: translating segment 300 does not
require having reasoned correctly about segment 299. The items are independent. The
N≈16 figure was measured on classification and reasoning, where they are not.

**What breaks does not break *more* with N.** These are stochastic models, and a
sweep this size drops a segment somewhere: `gemini-3.5-flash` left 11 of 1,200
blocks untranslated at N=256 in one run, which is the worst point measured anywhere.
The claim worth making is therefore about the trend, not about perfection — and the
trend runs the other way. Measured as breaks per 1,000 blocks, the *small* batches
are the worse ones, and the damage there is the kind that matters: `gemini-3.5-flash`
broke placeholders at N=8 and N=16 (8–14 and 2–10 across runs) and broke none at any
size from 32 up. More calls means more chances to fumble a placeholder and less
context in which to recognise one — the opposite of the effect the ceiling was set to
guard against.

`gemini-3.1-flash-lite` and Sonnet on Bedrock were clean at every size on the latest
sweep, which is worth stating plainly: the residual failures are one model's, not a
property of batching.

(The dashboard computes that comparison from the data rather than restating it in
prose, so it cannot go quietly out of date. A page that hardcodes "100% above N=32"
is falsified by one unlucky run while its actual thesis stands.)

`MaxBlocksPerCall` is accordingly raised 16 → 64, and demoted: it is a backstop, and
the token budget is the decision.

**On Bedrock — the route the platform actually runs on — the binding constraint is
requests, not tokens.** `eu.anthropic.claude-sonnet-4-6` came back 100% intact at
every batch size from 2 to 32. N=1 could not be measured at all: thirty calls per
repeat, even issued one at a time, trip the account's Bedrock rate limit and the
sweep is throttled into a 429. That is an argument for batching that has nothing to
do with quality — a batch of 16 makes one sixteenth of the requests, and on Bedrock
requests are the scarce resource. It is also the sharpest illustration of why a
throttle must never be scored as a failure: throttling punishes *small* batches
hardest, so scoring it would have drawn a curve that appeared to prove batching
rescues a failing model. It proves nothing of the kind. It is our own request rate.

**Whether batching saves money depends on the provider, and the mechanism decides
it.** Every call carries a fixed overhead — the system prompt plus the JSON schema
constraining the reply — paid once per call however many blocks ride along. On the
Anthropic-on-Bedrock path that overhead is ~985 tokens per call; on Gemini, ~106.
Two opposite answers follow:

- **On Bedrock, batching is a large cost lever.** At N=2 the overhead is paid
  fifteen times and dominates the bill; batching amortises it. Cost per 1,000 words
  falls from **$0.25 to $0.09** (−64%) between N=2 and N=32, with no loss of
  structural integrity.
- **On Gemini, batching is not a cost lever at all.** Little overhead to amortise,
  and the batched reply must carry an id and a JSON envelope per segment, so output
  tokens grow — and output is priced ~6× input. The effects cancel; cost per 1,000
  words comes out flat.

What batching buys on *every* provider is **throughput** — 2–3× the words per
second. Where it also saves money, that is a consequence of the provider's per-call
overhead, not a law of batching. Our first reading ("batching is not a cost lever")
was true of Gemini and would have been published as if it were universal; Bedrock
refuted it.

**The constraint that actually binds a batch is the output ceiling, and it bills
rather than breaks.** A blocking call may emit at most `NonStreamingMaxOutputTokens`
(16k) — not a model limit but ours, because asking a synchronous request for a
128k-token reply means holding an HTTP connection open for many minutes. A batch
whose reply would exceed it comes back truncated; under a JSON schema a fragment is
invalid JSON, so `splitAndRetry` halves the batch and redoes the work.

Nothing is corrupted. That is precisely why it stayed invisible: the integrity line
reads a flat 100% while the bill quietly doubles. Bedrock at N=600 needed ~33k output
tokens for the whole corpus, truncated at 16k, split to 300+300 (~16.5k each — still
over), split again to 150, and finished. The arithmetic is exactly what was measured:
16k + 2×16k wasted, plus the 33k of real work ≈ **81.7k output tokens** against 33.4k
at N=256 — 2.4× the tokens and 6× the wall-clock, at an unchanged 100% integrity.

So an oversized batch is not dangerous; it is wasteful, and silently so. The lesson
for the design is that the token budget is load-bearing and the block count is not,
which is how kapi already packs — the eval validated the mechanism while refuting
the number attached to it.

Two things follow, neither yet done:

- **Prompt caching is unused.** The fixed per-call overhead is exactly what prompt
  caching exists to remove — Bedrock reads cached input at $0.33/Mtok against
  $3.30, a tenth. Caching the system-prompt-plus-schema prefix would cut small-batch
  cost sharply and weaken batching's cost argument accordingly.
- **The `eu.` inference profile costs 10% more than `global.`** AWS prices a
  regional/geo profile at exactly 1.1× the global one ($3.30/$16.50 against
  $3.00/$15.00 for Sonnet 4.6). If EU data residency is not a requirement for a
  given workload, `global.anthropic.claude-sonnet-4-6` is the same model 10%
  cheaper.

And four bugs in kapi, which is the more useful outcome and the reason to build the
instrument before trusting the intuition.

**Inline tags were reaching the model as escape sequences.** `encoding/json`
escapes `<`, `>` and `&` by default (a defence for embedding JSON in a `<script>`
tag, irrelevant to a prompt), so every `<ph id="1"/>` was sent as
`<ph id="1"/>`. Anthropic's models decoded it; `gemini-3.5-flash` echoed
back a literal `3cph id="1"/3e`, silently corrupting the markup of every tagged
segment it translated. Batching was never the cause — the batch payload was. Fixed
by turning HTML escaping off: the model is asked to reproduce a tag, so it must be
shown a tag.

**No Gemini pro model worked at all.** kapi sent `thinkingBudget: 0` (translation
is a transformation, not a reasoning problem), and the pro models reject that
outright — "Budget 0 is invalid. This model only works in thinking mode". Every
call, at every batch size, was a 400. Fixed by omitting the thinking config for
that family.

**Thinking models were being truncated mid-answer.** The batch packer sizes an
output budget for the *answer* (`need*2+512`), but Gemini draws thoughts from the
same `maxOutputTokens` allowance. The model thought its way through the budget and
emitted a translation cut off mid-tag (`<ph id="2`), which the harness scored as
the model mangling markup. Fixed: a thinking model gets the ceiling. You are billed
for what is emitted, not for what is permitted, so the cap costs nothing and only
removes a way to corrupt output silently.

**The requested output cap never paid for the reply's shape.** The per-call budget
was `sum(source_tokens)*2 + 512` — the translation's *words*, and nothing for the
JSON scaffolding the schema wraps around each of them (`{"id":"s123","text":"…"},`),
which is a cost per *item* rather than per word. On a batch of many short UI strings
the scaffolding is most of the reply, so kapi asked for a cap smaller than the reply
it had just requested and truncated itself. Fixed (`batchOutputBudget`): the budget
now charges for the id and the envelope as well as the text. It is not what caused
the N=600 splits above — the 16k ceiling did that — but it would have caused its own,
on exactly the catalogs kapi is most often pointed at.

A fifth, found while wiring the model-availability check: **the provider registry
advertised retired models as defaults.** `ProviderInfo.DefaultModel` said
`gemini-3-flash-preview` (which the API now answers 404 "no longer available" for)
and `claude-sonnet-4-20250514` (retired), while the constructors had moved on. Its
doc comment claimed the two were the same value; nothing checked it, so `kapi
models` was advertising a default that would 404 on a user's first call. The
constructor constant is now the single source of truth, with a test.

All five were invisible to the unit tests, and three of them would have been
published as *model* findings by a less suspicious harness — degradation curves for
weaknesses the models did not have. That is why a break must be inspectable
(`-dump`) rather than merely counted, and why a transient failure is retried before
it is recorded as a cliff.

The harness had a matching bug of its own, worth recording because it is the same
class: the history keyed a run on (date, model, target) and *not* on the corpus, so
the 600-block sweep silently overwrote the 30-block sweep from the same morning —
the file whose entire purpose is to enforce "different corpora are not comparable"
was itself conflating them. The digest is now part of the key.

## Open

- **Prompt caching is unused on every provider.** It targets precisely the fixed
  per-call overhead that dominates small-batch cost, at a tenth of the input rate.
  This is the largest unexploited saving the eval has surfaced.
- **No cost figure for the claude-code models.** They run on the Claude
  subscription, which is not billed per token and whose token counts do not describe
  an API call — the CLI bills its own agent system prompt as cache creation,
  reporting 240 input tokens across sixty calls. Costing them needs a sweep against
  the metered Anthropic API.
- **Context is limited to the key and immediate neighbours.** TM matches, the file
  path, and prior translations of the same key are all things the evidence says
  would help and that kapi already holds, and none of them reach the prompt.
- **Streaming would lift the 16k output ceiling**, which is the only thing now
  bounding a batch. Whether that is worth wanting is an open question, not an
  obvious yes: the eval shows the gain from batching flattening out well before the
  ceiling is reached, so the honest reading is that the ceiling is not currently
  costing us anything — it is simply the wall an oversized batch hits.
- **The measurement is de-only.** German runs long, which stresses the output
  budget, and that was the point. Whether a language that expands further (Finnish)
  or one that tokenizes badly (Japanese, Thai) moves the curve is unmeasured, and
  the harness takes `-target` precisely so it can be.
