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
derived from the model's declared output limit, with a ceiling on the number of
segments (`MaxBlocksPerCall`) because quality degrades with batch size
independently of length.

**And it is not a decidable question for a user.** The right count depends on the
model's ceiling, on the length of *their* segments, and on a quality-versus-N
curve that has never been published for segment translation. A form field
defaulting to 100 did not give anyone control; it gave them a way to break their
run. The user-facing choice is now `batching: auto | single` — a question they can
actually answer. A numeric pin survives as a hidden override, for recipes that
must reproduce a historical run and for the eval sweep that will measure the curve
we currently take on faith.

**Batching is a cost optimisation with a quality cost, and the cost argument is
weaker than it looks.** Shrinking a batch does not re-send the content — each
block is sent once either way. It re-sends only the *system prefix*, which every
provider caches at ~0.1× after the first call. Meanwhile the batch-prompting
literature finds accuracy degrading measurably by N≈16, and the failure mode at
size is not worse wording but **dropped, merged and renumbered segments** — a
correctness failure, not a quality one.

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
- We ship a `MaxBlocksPerCall` chosen from evidence about *adjacent* tasks, not
  from a measurement of our own. That is a known gap, not a settled answer, and it
  is the first thing the eval should replace.

## Open

- **No quality-versus-N curve exists for segment translation.** Ours should be
  measured, sweeping N and scoring both translation quality *and* segment-id
  integrity — because the literature says the latter is what actually breaks.
- **kapi sends no context beyond the batch** — no key or path, no neighbouring
  strings, no TM matches. The evidence strongly favours *large non-translated
  context, small translation unit*; we currently have neither half. See #1226.
