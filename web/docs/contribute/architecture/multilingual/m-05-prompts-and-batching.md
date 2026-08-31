---
id: m-05-prompts-and-batching
sidebar_position: 5
title: "M-05: Prompts and batching"
description: "Every prompt kapi sends is rendered by a typed builder from attributed sections, batch size is derived from what a model can emit rather than configured as a number, the reply is id-keyed JSON, and a truncated reply is carried as a signal instead of committed as a translation."
keywords: [neokapi, architecture decision, prompts, batching, output tokens, truncation, prompt injection, structured output]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# M-05: Prompts and batching

## The three questions

kapi sends a user's content to a language model on their behalf. Three questions
follow, and answering them by accident produces one defect in three places:

1. **What is in a prompt, and who owns each part of it?** A prompt written
   inline at its call site is invisible to the user, to the docs and to any
   test.
2. **How much content goes into one call?** A batch size exposed as a form field
   asks a question nobody is equipped to answer.
3. **What happens when that content is hostile, or merely looks like it?** A
   numbered text blob mapped back by position has no frame the content cannot
   forge.

## Prompts are rendered by a typed builder

Every prompt kapi sends is rendered by a typed builder in `core/ai/prompt` from
**attributed sections**. Each section carries a kind (task, constraint,
instruction, voice, terminology pinned from the terms store, context, and the
content itself) and an origin saying what produced it. Every call also carries
a **prompt id**, so an unlabelled use of someone's content cannot exist.

<PipelineDiagram
  stages={[
    { label: "typed builder", sub: "core/ai/prompt", role: "tool", note: "attributed sections + origin" },
    {
      label: "one rendering",
      parallelLabel: "three consumers, one source",
      lanes: [
        { label: "the call", sub: "provider-neutral turns" },
        { label: "--explain-prompts", sub: "attributed, per run" },
        { label: "/reference/prompts", sub: "generated + drift-gated" },
      ],
      role: "annotate",
    },
    { label: "config fingerprint", sub: "invalidates cached targets", role: "qa" },
  ]}
  channelLabel=""
  caption="The prompt the binary sends, the prompt the user is shown, and the prompt the docs describe are the same object."
/>

That buys three things at once:

- **`--explain-prompts`** shows the exact prompt of a real run, each section
  attributed to the thing that produced it. The bare flag prints to stderr; given
  a path it writes the exchanges as JSON.
- **The [prompt reference](/reference/prompts) is generated** from the same
  builders the binary uses, and a CI drift gate fails the build when the
  committed reference stops matching. The docs cannot describe a prompt kapi does
  not send. A hand-written page can, and the errors it invites (claiming a
  prompt carries surrounding blocks, or memory matches) are invisible to a
  reader.
- **The prompt is fingerprinted** into the translate config, so rewording a
  prompt invalidates cached targets rather than silently serving a translation
  produced by a prompt that no longer exists.

The corollary is that **prompts never fork per provider**. Providers differ in
*mechanism* (one constrains output by grammar, another by a response-format
parameter) and in *capability* (schema subsets, output ceilings). Those
differences are absorbed by the structured-chat seam and a declared limits
record ([E-07](../engine/e-07-model-providers.md)); they are never absorbed by
rewording a prompt. A provider-specific prompt would escape the drift gate, the
reference and the fingerprint at once: one provider could quietly send a
different translate prompt than every other, and nothing would say so.

## Batch size is derived from the output ceiling

**The binding constraint is output tokens, not the context window.** For
translation, output tracks input: translating N tokens produces roughly N
tokens. Current frontier models read an order of magnitude more than they can
emit, so the ceiling on "how much can we translate in one call" is what the model
can *say*, not what it can *read*. Filling a huge context with source text you
cannot emit translations for is a truncated reply, not a strategy.

**A fixed block count is the wrong unit.** The same hundred blocks are a few
hundred tokens of interface strings or hundreds of thousands of tokens of prose.
kapi packs each call against a token budget derived from the model's declared
output limit, with a backstop on the number of blocks. That backstop is a guard
rail rather than the decision, because the token budget binds first on anything
longer than an interface string.

**And it is not a decidable question for a user.** The right count depends on the
model's ceiling, on the length of *their* segments, and on a quality-versus-N
curve that has never been published for segment translation. A form field
defaulting to a number gives nobody control; it gives them a way to break their
run. The user-facing choice is therefore `batching: auto | single`, a question
they can actually answer, with a numeric pin available as a hidden override for
a recipe that must reproduce a historical run, and for the eval sweep.

One case overrides the setting in the other direction. On-device models
translate far better per block than batched into one long structured
generation (a small local model tends to ignore the "return JSON" instruction
and emit plain numbered text), so a local provider is forced to one block per
call unless the user deliberately asked for something smaller.

## A truncated reply is carried as a signal

A model that stops at its output cap has produced the *beginning* of an answer.
Under a JSON schema that is invalid JSON and fails loudly. In free text, the
single-block path, it is a shorter translation, which is to say it is
indistinguishable from a translation, and committing it corrupts the document
with nothing to notice.

So truncation is **carried, never inferred**. Every provider reads its API's
finish signal on every method it implements (blocking and streaming, free text
and structured) and reports it as a field on the response, or as a distinct
error where the reply shape leaves nothing to return. The translate response
carries the same field, because the translate path never sees the chat response.

Two responses, because there are two situations:

- **A batch** was kapi's choice, so its consequences are kapi's to absorb: halve
  it and ask again, down to a single block.
- **A single block** already has a call to itself and a budget sized for it.
  There is no smaller request left to make, so the run stops and says so.
  Failing a run is recoverable; a plausible-looking half-translation stamped as a
  draft is not.

The budget follows the packer. A block is isolated into a batch of one exactly
when its estimated output alone exceeds the call budget, so the blocks that
reach the single-block path are the ones already known to be large, and that path
derives its own output cap the same way a batch does rather than falling back to
a provider's constructor default. That derivation only ever *raises* the cap: a
reasoning model draws its thoughts from the same allowance as its answer, so
sizing a short block's request down to what its words need would be a new way to
truncate one.

The output budget also charges for the reply's **shape**, not only its words.
The JSON scaffolding the schema wraps around each item costs per *item* rather
than per word, and on a batch of many short interface strings it is most of the
reply. A budget covering the translation's words alone asks for a cap smaller
than the reply it has just requested, and truncates itself on exactly the
catalogs kapi is most often pointed at.

## The payload is id-keyed JSON

The reply is mapped back by **segment id**, not by position, and kapi accepts
only the ids it sent. A dropped segment is therefore a detectable absence,
retried individually, rather than an off-by-one that silently shifts every
translation after it.

The payload is **JSON**, not numbered lines and not XML tags, for two reasons.
Content containing a bracketed number or a closing tag could forge a boundary in
a delimiter-framed payload, and that needs no attacker; a numbered list is
enough. And the text carries inline markup as literal placeholder tags the model
must reproduce verbatim; XML-escaping the payload would mangle precisely what
the tag-fidelity constraint demands be preserved. JSON escaping is lossless and
cannot be broken out of from inside a string.

For the same reason the payload is written **without HTML escaping**. The
standard encoder escapes angle brackets and ampersands by default, which is a
defence for embedding JSON in a script tag and irrelevant to a prompt. With it
on, every placeholder tag reaches the model in escaped form; some models decode
it and some echo the escapes back, corrupting the markup of every tagged segment
they translate. The model is asked to reproduce a tag, so it is shown a tag.

Ids are batch-local (`s1`, `s2`, …) rather than the block's content key, because
content keys are content-derived: two blocks with identical source text share
one, which in a batch is a collision, not an identifier
([F-03](../foundations/f-03-identity.md)).

## What the model is told besides the block

There are two very different things you can put in a translation prompt, and
conflating them produces a worse translation for more money:

- **Segments the model must emit.** Every one is a task competing for attention
  and a distractor for every other. Adding them degrades quality, and at size the
  failure is dropped or renumbered segments rather than clumsier wording.
- **Reference material the model must not translate.** Close to free upside: it
  disambiguates without adding work.

So context is its own configuration axis, `none | key | neighbours`, defaulting
to `key`:

- **`key`** sends the block's key or path. It is the cheapest disambiguation
  there is, it is already in the document, and it is *stable*: a block's key is a
  function of the block, so sending it cannot make a cached translation wrong. A
  bare "Save" is a coin flip between a verb and a noun.
- **`neighbours`** adds the surrounding source blocks as reference. It costs
  tokens, and it makes a block's translation depend on text that is not the
  block, so callers fold a neighbourhood digest into the cache key, and an edit
  to one string re-translates its neighbours. That is correct, and it is not
  free, which is why it is opt-in.

A segment may also carry a **`prior`**: what that block said before, and the
translation approved for it then, drawn from the content memory's version
chain ([C-09](../context/c-09-content-memory.md#version-chains-and-governed-reuse)).
It is reference material of the second kind. The prompt states that the prior
is history rather than a segment to emit and that its wording is not to be
reused where the source has changed; it is included only when the memory's
governance gate says the earlier answer was approved under rules that still
apply, and it is folded into the fingerprint like every other section.

## Prompt injection is contained structurally

The text kapi translates is frequently not written by the person running kapi.
The prompt states the data/instruction boundary as an attributed constraint,
visible in `--explain-prompts` and in the reference, but that is **defence in
depth and nothing more**. No prompt-level rule reliably stops a model from
following an instruction embedded in its input, and this decision does not claim
otherwise.

What contains the damage:

- **No capability.** The call has no tools, no filesystem, no network. An
  injected instruction cannot act; at worst it produces a wrong translation. The
  blast radius is content, not capability
  ([E-06](../engine/e-06-execution-trust.md)).
- **Constrained decoding.** The reply is held to a JSON schema by the provider's
  own mechanism, so it can fill a shape but not escape it.
- **Id validation.** An injected omission is caught, not silently absorbed.
- **An unforgeable frame.** See the payload above.

The residual exposure, an injected instruction corrupting the *text* of a
translation, is what the checks and review passes exist to catch.

## Consequences

- Adding a prompt means adding it to the catalog; a guard test fails otherwise,
  so a prompt cannot ship undocumented.
- Adding a provider means declaring its limits. A provider that does not is
  treated as unknown and packed conservatively: smaller batches, not truncated
  replies.
- Rewording any prompt moves the fingerprint (re-translating affected blocks) and
  fails the reference drift gate until the docs are regenerated. Both are
  intended.
- The block cap is set from a measurement of translation itself and re-measured
  as models move, because a ceiling checked once decays into folklore.

## Measuring the ceiling

`make batch-eval` sweeps blocks-per-call over a fixed corpus and scores each N.
It scores **structural integrity** (does every segment come back, under the id
it was sent, with its placeholders and inline tags intact?) rather than wording,
for two reasons: that is the failure the literature attributes to batching, and
in this pipeline it is a correctness failure rather than a style complaint. A
translation missing its placeholder cannot be written back into the source file
at all.

Scoring structure needs no reference translations, so the curve can be measured
on any corpus, in any language pair, for the price of the calls.

**The corpus is part of the experiment.** A corpus smaller than the batch size
measures nothing about batching: at thirty blocks, N=32 is not a batch of
thirty-two but the whole document in one call, and the sweep reports the
*corpus's* ceiling as the models'. So the corpus scales, holding the stressor mix
roughly constant and generating every block distinct, since duplicate source text
would let a model translate one segment and copy it into the next, manufacturing
the exact failure the eval exists to detect. It deliberately carries what
batching is documented to break: long prose, placeholders and inline markup, and
the same source text under two different keys, which is the case positional batch
mapping corrupts silently. The authored core keeps a stable digest so published
runs stay comparable.

A run against the demo stub exercises the plumbing and measures nothing about
any model. Such runs are marked as simulated and the dashboard excludes them from
every chart; a stub's flawless curve is exactly the number someone would
otherwise quote.

### The measurement is kept

The publishing target sweeps the real models and appends to a committed history
behind the **/batch-eval** dashboard. Three choices in that record are what make
it worth keeping:

- **Runs are stamped with the corpus digest**, which is part of the history key.
  Change the corpus and the digest moves, and runs measured on the old one stop
  being comparable, so a 600-block sweep cannot overwrite a 30-block sweep from
  the same morning. The dashboard says so rather than drawing one line through
  two experiments.
- **A same-day re-run corrects its entry** instead of appending a second point,
  so pressing enter twice cannot masquerade as change over time.
- **Transient failures are retried before being recorded.** An overloaded API
  published as "the model could not answer at this batch size" would be a
  fabricated cliff, and the most damaging kind, because it is the exact shape of
  the finding being looked for and would therefore be believed. A throttle in
  particular punishes *small* batches hardest, so scoring one would draw a curve
  that appeared to prove batching rescues a failing model.

### What the sweep found

**There is no batching cliff.** Structural integrity stays above 99% at every
batch size swept, including six hundred segments answered in a single call with
nothing dropped, merged, renumbered or stripped of a placeholder. The
batch-prompting literature's degradation-with-N does not transfer to
translation, and the reason is plain: translating segment 300 does not
require having reasoned correctly about segment 299. The items are independent.
The small-batch figures in the literature were measured on classification and
reasoning, where they are not.

**What breaks does not break more with N.** These are stochastic models, and a
sweep of this size drops a segment somewhere. Measured as breaks per 1,000
blocks the trend runs the other way: the small batches are the worse ones, and
the damage there is the kind that matters, broken placeholders at the smallest
sizes and none at any size above them. More calls means more chances to fumble a
placeholder and less context in which to recognise one, which is the opposite of
the effect a low block cap guards against.

**Whether batching saves money depends on the provider, and the mechanism
decides it.** Every call carries a fixed overhead, the system prompt plus the
JSON schema constraining the reply, paid once per call however many blocks ride
along. Where that overhead is large, batching amortises it and cost per thousand
words falls steeply with N. Where it is small, there is little to amortise and
the batched reply must carry an id and a JSON envelope per segment, so output
tokens grow while output is priced several times input: the effects cancel and
cost comes out flat. What batching buys on *every* provider is throughput.

**The constraint that actually binds a batch is the output ceiling, and it bills
rather than breaks.** A blocking call may emit at most our own non-streaming
cap, which is not a model limit but the consequence of holding a synchronous
HTTP connection open for minutes. A batch whose reply would exceed it comes back
truncated; under a JSON schema a fragment is invalid JSON, so the batch is halved
and the work redone. Nothing is corrupted, which is precisely why it stays
invisible: the integrity line reads a flat 100% while the bill quietly doubles.
An oversized batch is wasteful rather than dangerous, and silently so.

That is why the dashboard's lead chart is **cost per thousand source words
against N** on a log axis rather than integrity against N. Every integrity point
sits near the ceiling, so on a 0–100% axis it renders as a flat line: a null
result drawn as if it were the finding, with the small real variation squeezed
into invisibility. Cost is the thing that moves: a slope down on the left where
per-call overhead is amortised, a broad shallow minimum, and a wall on the right
where the output ceiling forces a split and the same words are paid for twice.
Integrity is still plotted, because a model can regress behind a stable alias and
nobody would be told, but as **breaks per thousand blocks**, an axis on which a
regression would be visible, and framed as the guard it is rather than the
headline it is not.

The record exists because model aliases are not stable artefacts: the same alias
points at different weights over time. Re-run the sweep when the models move.

## Open

- **Prompt caching is unused on every provider.** It targets precisely the fixed
  per-call overhead that dominates small-batch cost, at a fraction of the input
  rate. This is the largest unexploited saving the eval has surfaced, and taking
  it would weaken batching's cost argument accordingly.
- **Subscription-billed routes have no cost figure.** They are not billed per
  token and their reported token counts do not describe an API call. Costing them
  needs a sweep against the metered API.
- **Streaming would lift the output ceiling**, which is the only thing now
  bounding a batch. Whether that is worth wanting is an open question rather than
  an obvious yes: the gain from batching flattens out well before the ceiling is
  reached, so the honest reading is that the ceiling is not currently costing
  anything. It is the wall an oversized batch hits, and nothing reaches it.
- **The measurement is single-target.** German runs long, which stresses the
  output budget, and that was the point. Whether a language that expands further,
  or one that tokenizes badly, moves the curve is unmeasured; the harness takes a
  target flag precisely so it can be.

## Related

- [F-03: Identity](../foundations/f-03-identity.md): why a content key cannot serve as a batch-local id
- [E-06: Execution trust](../engine/e-06-execution-trust.md): the capability boundary that bounds an injection's blast radius
- [E-07: Model and translation providers](../engine/e-07-model-providers.md): the structured-chat seam, declared limits, and finish signals
- [M-03: Multimodal content](m-03-multimodal-content.md): the per-modality refinement prompts built the same way
- [C-07: Voice profiles](../context/c-07-voice-profiles.md): the voice section of a prompt
- [C-08: Terms](../context/c-08-terms.md): the terminology section of a prompt
- [C-09: Content memory](../context/c-09-content-memory.md): the version chain a `prior` reference is drawn from
- [Prompt reference](/reference/prompts): the generated, drift-gated catalog of every prompt kapi sends
