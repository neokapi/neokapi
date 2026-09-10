---
sidebar_position: 7
title: Scoped context evaluation
description: Verify actual context selection and review content against the writing guidance that applies at its destination.
---

# Scoped context evaluation

kapi connects content to applicable guidance through its project context. This
evaluation starts with that resolution and then asks whether a reviewer can
identify departures from the selected voice and style. The
[semantic regression suite](contextual-meaning-evaluation.md) retains separate
controls for evidence integrity and procedural completeness.

## What the comparison measures

The fixture is an isolated local project with distinct support-reply and
service-status channels. It contains a warm reply, a faithful paraphrase and a
neutral status notice expressing the same service facts. Each candidate is
placed at both legitimate destinations. A separate product with different
guidance supplies an offline isolation control, and an unbound location tests
the absence of a voice binding.

For every destination, preparation calls the real `kapi context <path> --json`
and `kapi check <path> --json`. The review receives the guide returned by the
resolver, together with its reported coordinates and voice provenance. The
preparer does not choose a guide based on the expected review outcome. Candidate
labels and assessment expectations remain outside subject sessions.

The comparison separates three questions:

| Question | Evidence |
| --- | --- |
| Did the right context resolve? | Actual product/channel, effective guide, unrelated-product exclusion and unbound result |
| Did the review follow that context? | Same candidate under both destinations, with each departure grounded in selected guidance |
| Did it accept valid variation? | A faithful alternate reply assessed against the same guidance |

There is one fixed review protocol and model across the six attempts. This is
not a comparison of model hosts or access through CLI versus MCP, and it does
not establish superiority over an agent given the same correct guidance.

## Current resolution boundary

Project profiles and collection or content-item `channel: product/channel`
bindings select the effective voice. Channel tone and style replace their
whole corresponding structures. The by-location context surface and file
checks share that resolver. The returned guide renders the effective profile;
it does not include the other channels' style overrides.

Additional coordinates can describe the destination without selecting a voice
override. An `audience` coordinate alone does not activate an audience-specific
profile. This fixture therefore uses supported product/channel bindings and
remains locale-neutral. A project-scope result does not exercise connected
workspace relations or team-wide decision sharing.

## Advisory style review

The experimental runner's `style` protocol receives no procedural requirement
list. It asks whether the content follows the supplied scoped writing guidance.
Substantive style rules appear only in the resolver-supplied guide; generic
task instructions must not repeat them. It must not infer style rules from a
filename, an age category or a model's own prose preferences.

The model selects candidate and guidance paragraph IDs. The runner resolves
both to exact original text and records the request snapshot and fingerprint.
Findings identify an evidenced departure; uncertainty and optional suggestions
remain separate. The assessment is `aligned`, `departures` or
`insufficient_context`. These are advisory model judgments, not factual
conflicts, numeric quality scores or release gates. Reference validation cannot
establish whether the explanation is correct.

Fast deterministic checks are recorded separately. Unsupported semantic
guidance remains explicit even when those checks emit no findings. The model
review does not change the normal `kapi check` execution path or require an API
credential in the fixture.

## Run the comparison

Build kapi, then prepare into a new directory. Preparation also writes a browser
casebook at `review.html`; it makes no model calls.

```sh
make build
make meaning-eval-prepare-scoped MEANING_EVAL_INPUTS=harness/out/scoped-context
make meaning-eval-preflight MEANING_EVAL_INPUTS=harness/out/scoped-context MEANING_EVAL_DIR=harness/out/scoped-style-review
```

After reviewing the cases and preflight record, the subscription run consumes
plan allowance, with at most six started attempts:

```sh
make meaning-eval-run MEANING_EVAL_INPUTS=harness/out/scoped-context MEANING_EVAL_DIR=harness/out/scoped-style-review MEANING_EVAL_MAX_ATTEMPTS=6
python3 scripts/checkeval/report_scoped_context.py --prepared harness/out/scoped-context --study harness/out/scoped-style-review --out harness/out/scoped-style-results.html
```

The renderer accepts an optional `--assessment` JSON file with
`inputs_sha256`, `method`, `summary` and `sessions` (session IDs mapped to lists
of assessment notes). It checks that prepared and frozen inputs match and
retains the raw model output. An invalid response or incomplete host result
does not become an accepted style assessment.

The unbound control is offline only: it verifies absent resolved guidance. The
style runner requires a resolved guide before it can create a review request;
this run does not test model abstention when no guide is available.

## Execution and interpretation

Preparation records the binary identity, fixture hashes, raw resolver and check
results, selected guidance and destination metadata. Personal configuration,
plugins and project discovery are isolated. Subject attempts use the existing
subscription runner's frozen inputs, no-tools restriction, observed-model
verification and six-reservation ceiling. Failed attempts are retained without
retries or API fallback.

Independent agent assessment checks the selected guidance and case expectations
before the live run, then evaluates findings against that same evidence. It is
distinct from human adjudication. These synthetic examples establish bounded
observations about resolution and context-sensitive review, not general writing
quality, human acceptance, review-time savings or the full connected-workspace
vision.
