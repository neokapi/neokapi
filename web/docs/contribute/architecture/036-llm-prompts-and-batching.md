---
title: "AD-036: LLM prompts, batching, and the model boundary"
sidebar_label: "036: LLM prompts & batching"
description: How kapi builds the prompts it sends to a language model, how much it sends at once, and what contains the damage when the content is hostile.
---

# AD-036: LLM prompts, batching, and the model boundary

## Context

kapi sends the user's content to a language model on their behalf. Three questions
follow:

1. **What is in a prompt, and who owns each part of it?**
2. **How much content goes into one call?**
3. **What happens when that content is hostile — or merely looks like it?**

Answered by accident, they are one defect wearing three hats. A prompt written
inline at its call site is invisible to the user, to the docs and to any test. A
batch size exposed as a form field asks a question nobody is equipped to answer.
And a numbered text blob mapped back by position has no frame the content cannot
forge.

## Decision

### Prompts are built, not written

Every prompt kapi sends is rendered by a typed builder in `core/ai/prompt` from
**attributed sections** — each section carries a `Kind` (task, constraint,
instruction, voice, terminology — pinned from the terms store, still spelled
`glossary` on the wire — and content) and an `Origin` saying what produced it.

This buys three things at once:

- **`--explain-prompts`** can show the exact prompt of a real run, with each
  section attributed to the thing that produced it.
- **The [Prompt Reference](/reference/prompts) is generated** from the same
  builders the binary uses, and a CI drift gate fails the build if the committed
  reference stops matching. The docs cannot describe a prompt kapi does not send
  — a hand-written page can, and the errors it invites (claiming a prompt carries
  surrounding blocks, or memory matches) are invisible to a reader.
- **The prompt is fingerprinted** into the translate config, so rewording a prompt
  invalidates cached targets rather than silently serving a translation produced
  by a prompt that no longer exists.

The corollary is that **prompts never fork per provider**. Providers differ in
*mechanism* (Anthropic constrains output by grammar, OpenAI by `response_format`,
Gemini by `response_format`, Ollama by `format`) and in *capability* (schema
subsets, output ceilings), and those differences are absorbed by the
`ChatStructured(messages, JSONSchema)` seam and a declared `Limits()`. They are
never absorbed by rewording a prompt. A provider-specific prompt would escape the
drift gate, the reference and the fingerprint: one provider could quietly send a
different translate prompt than every other, and nothing would say so.

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

The block cap is 64. The sweep below finds no structural degradation at any batch
size up to 600, and the token budget binds first on anything longer than a UI
string, so the cap is a backstop rather than the decision.

**And it is not a decidable question for a user.** The right count depends on the
model's ceiling, on the length of *their* segments, and on a quality-versus-N
curve that has never been published for segment translation. A form field
defaulting to a number gives nobody control; it gives them a way to break their
run. The user-facing choice is `batching: auto | single` — a question they can
actually answer. A numeric pin is a hidden override, for recipes that
must reproduce a historical run and for the eval sweep that will measure the curve
we currently take on faith.

**Batching is commonly taken to be a cost optimisation with a quality cost.
Measurement inverts both halves.** The quality cost does not materialise:
structural integrity is perfect from N=32 to N=600 on every model swept, and the
*only* breaks measured anywhere are at N=8 and N=16. The cost saving is real but
provider-dependent — it is the per-call fixed overhead (system prompt + JSON
schema) being amortised, so it is large where that overhead is large (Bedrock,
~985 tokens per call) and negligible where it is small (Gemini, ~106).

What batching genuinely costs is bounded by the **output ceiling**: a batch whose
reply would exceed `NonStreamingMaxOutputTokens` (16k) comes back truncated,
`splitAndRetry` halves it, and the work is redone. That is billed, not broken —
which is why a flat 100% integrity line hides it until the tokens are looked at. A
600-block batch costs 2.4× the tokens and 6× the wall-clock of the same corpus at
N=256.

### A truncated reply is a signal, never a target

A model that stops at its output cap has produced the *beginning* of an answer.
Under a JSON schema that is invalid JSON and fails loudly. In free text — the
single-block path — it is a shorter translation, which is to say it is
indistinguishable from a translation, and committing it corrupts the document with
nothing to notice.

So truncation is carried, not inferred. Every provider reads its API's finish
signal (`finish_reason: "length"`, `stop_reason: "max_tokens"`,
`finishReason: "MAX_TOKENS"`) on every method it implements — blocking and
streaming, free-text and structured — and reports it as `ChatResponse.Truncated`,
or as `ErrOutputTruncated` where the reply shape leaves nothing to return.
`TranslateResponse` carries the same field, because the translate path never sees
the `ChatResponse`.

Two responses, because there are two situations:

- **A batch** was kapi's choice, so its consequences are kapi's to absorb:
  `splitAndRetry` halves it and asks again, down to a single block.
- **A single block** already has a call to itself and a budget sized for it. There
  is no smaller request left to make, so the run stops and says so. Failing a run
  is recoverable; a plausible-looking half-translation stamped as a draft is not.

The budget follows the packer. `packBlocks` isolates a block into a batch of one
exactly when its estimated output alone exceeds the call budget — so the blocks
that reach the single-block path are the ones already known to be large, and that
path derives its own `max_tokens` the same way a batch does (`blockOutputBudget`)
rather than falling back to a provider's constructor default. That derivation only
ever *raises* the cap: a reasoning model draws its thoughts from the same
allowance as its answer, so sizing a short block's request down to what its words
need would be a new way to truncate one.

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
- `MaxBlocksPerCall` is set from a measurement of translation itself
  (`scripts/batcheval`, published at **/batch-eval**), and re-measured as models
  move, because a ceiling checked once decays into folklore.

## Measuring the ceiling

`make batch-eval` (`scripts/batcheval`) sweeps blocks-per-call over a fixed corpus
and scores each N. It scores **structural integrity** — does every segment come
back, under the id it was sent, with its placeholders and inline tags intact? —
rather than wording, for two reasons: that is the failure the literature attributes
to batching, and in this pipeline it is a correctness failure rather than
a style complaint. A translation missing its `{0}` cannot be written back into the
source file at all.

Scoring structure needs no reference translations, so the curve can be measured on
any corpus, in any language pair, for the price of the calls.

The corpus deliberately carries what batching is documented to break: long prose
(degradation is worst when items are long), placeholders and inline markup, and the
same source text under two different keys — the case positional batch mapping
corrupts silently.

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

The obvious chart — structural integrity against N — is the wrong one. Every point
sits between 98.8% and 100%, so on a 0–100% axis it renders as a flat line at the
ceiling: a null result drawn as if it were the finding, with the small real
variation (batches of 8 break *more* than batches of 128) squeezed into
invisibility.

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

A corpus smaller than the batch size measures nothing about batching. With 30
blocks, N=32 does not test a batch of thirty-two — it tests *the whole document in
one call*, and N=16 tests two calls. A saturated sweep reports a flat 100% line at
every size, and the ceiling it has measured is the corpus's, not the models'.

The corpus therefore scales (`CorpusN`), holding the stressor mix roughly constant
and generating every block distinct — duplicate source text would let a model
translate one segment and copy it into the next, manufacturing the exact failure the
eval exists to detect. The authored 30 keep a stable digest, so published runs stay
comparable.

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
context in which to recognise one — the opposite of the effect a low block cap
guards against.

`gemini-3.1-flash-lite` and Sonnet on Bedrock were clean at every size on the latest
sweep, which is worth stating plainly: the residual failures are one model's, not a
property of batching.

(The dashboard computes that comparison from the data rather than restating it in
prose, so it cannot go quietly out of date. A page that hardcodes "100% above N=32"
is falsified by one unlucky run while its actual thesis stands.)

`MaxBlocksPerCall` is accordingly a backstop at 64, not the decision — the token
budget is.

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

And five defects in the request path, which is the more useful outcome and the
reason to build the instrument before trusting the intuition. Each is a rule the
design holds to, and none of them is visible to a unit test.

**The batch payload is written without HTML escaping.** `encoding/json`
escapes `<`, `>` and `&` by default — a defence for embedding JSON in a `<script>`
tag, irrelevant to a prompt. With it on, every `<ph id="1"/>` reaches the model as
`&lt;ph id="1"/&gt;`. Anthropic's models decode it, but `gemini-3.5-flash` echoes
back a literal `\x3cph id="1"/\x3e` and corrupts the markup of every tagged
segment it translates. The model is asked to reproduce a tag, so it is shown a tag.

**The thinking config is omitted for the Gemini pro family.** kapi otherwise sends
`thinkingBudget: 0` — translation is a transformation, not a reasoning problem —
which those models reject outright ("Budget 0 is invalid. This model only works in
thinking mode"), failing every call at every batch size with a 400.

**A thinking model gets the output ceiling rather than a sized budget.** Gemini
draws thoughts from the same `maxOutputTokens` allowance as the answer, so a budget
sized for the answer alone (`need*2+512`) is spent thinking and the translation
comes back cut off mid-tag (`<ph id="2`) — which reads as the model mangling
markup. Billing is for what is emitted, not for what is permitted, so the ceiling
costs nothing and removes a way to corrupt output silently.

**The output budget charges for the reply's shape, not only its words**
(`batchOutputBudget`). The JSON scaffolding the schema wraps around each item
(`{"id":"s123","text":"…"},`) costs per *item* rather than per word, and on a batch
of many short UI strings it is most of the reply. A budget covering the
translation's words alone (`sum(source_tokens)*2 + 512`) asks for a cap smaller
than the reply it has just requested and truncates itself — on exactly the catalogs
kapi is most often pointed at. (It is not what caused the N=600 splits above; the
16k ceiling did that.)

**A provider's constructor constant is the single source of truth for its default
model**, with a test. A second copy in `ProviderInfo.DefaultModel` drifts from it
silently — naming a `gemini-3-flash-preview` the API answers 404 "no longer
available" for, or a retired `claude-sonnet-4-20250514` — and `kapi models` then
advertises a default that 404s on a user's first call.

Three of the five would be published as *model* findings by a less suspicious
harness — degradation curves for weaknesses the models do not have. That is why a
break must be inspectable (`-dump`) rather than merely counted, and why a transient
failure is retried before it is recorded as a cliff.

The harness earns the same scrutiny, and the corpus digest is part of its history
key for the same reason. Keying a run on (date, model, target) alone lets a
600-block sweep overwrite a 30-block sweep from the same morning — the file whose
entire purpose is to enforce "different corpora are not comparable" conflating them.

## Open

- **Prompt caching is unused on every provider.** It targets precisely the fixed
  per-call overhead that dominates small-batch cost, at a tenth of the input rate.
  This is the largest unexploited saving the eval has surfaced.
- **No cost figure for the claude-code models.** They run on the Claude
  subscription, which is not billed per token and whose token counts do not describe
  an API call — the CLI bills its own agent system prompt as cache creation,
  reporting 240 input tokens across sixty calls. Costing them needs a sweep against
  the metered Anthropic API.
- **Context is limited to the key and immediate neighbours.** Memory matches, the file
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
