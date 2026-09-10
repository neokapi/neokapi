---
sidebar_position: 8
title: Context transfer
description: Compare adapting an independent documentation project with raw references, learned guidance and kapi.
---

# Context transfer

This proof of concept asks whether reusable writing context produces a useful
transformation in a project created independently of that context. It compares
three ways to adapt the same four documents. It does not score writing by
counting edits or prohibited words.

## Source and target

The source corpus contains pinned Astro documentation, its writing guide and
its recipe guidance. Preparation saves the original Markdown or MDX, source
URLs, repository revisions, SHA-256 hashes and both repositories' MIT licenses.
The source projects do not endorse this experiment.

A separate fictional developer tool, Pageglass, supplies the target. Its brief
fixes the supported commands, local preview behavior, sharing access, expiry,
revocation and recovery actions. The four deliverables are an overview,
getting-started tutorial, preview-link explanation and troubleshooting page.
The tool and command examples are fictional; this experiment does not install
or execute them.

The context learner sees only the source corpus. It produces attributed
principles with explicit or inferred status, applicable document surfaces,
non-transferable details and uncertainties. The result is frozen before the
initial writer is staged. That writer sees only the factual brief and a request
for normal, useful documentation. There is no instruction to write poorly or
imitate a source project.

## Three adaptation approaches

| Approach | Supplied context | Editing route |
| --- | --- | --- |
| References | Raw source corpus, including its explicit writing guide | Ordinary agent tools |
| Guidance | Frozen learned guide and exact per-document kapi resolver answers as files | Ordinary agent tools |
| kapi | Same learned guide compiled into a project profile, with supported product/channel bindings | Shipped CLI skill, scoped retrieval, inspection, edits and checks |

All approaches start from byte-identical draft documents and the same factual
brief. The guide compiler preserves the learner's statements and qualifications;
it does not introduce hand-written semantic rules. Actual `kapi context`
responses are saved before adaptation and copied to the plain-guidance approach.
Additional coordinates describe the destination. Supported channel bindings
select the applicable constraints.

The kapi approach can use ordinary authoring tools for structural changes that
the supported edit route cannot express. Such fallbacks remain visible in the
transcript and the author's notes. This is a comparison of complete workflows,
not a claim that every Markdown transformation uses `apply`. No semantic-check
provider is configured. Unsupported semantic guidance needs the writing
agent's judgment and independent assessment; a clean deterministic check does
not establish that the prose follows the guide.

## Prepare and run

The run uses five staged subscription attempts: learning, initial drafting and
one adaptation per approach. Every stage uses Claude Sonnet 5 at high effort.
The persistent ledger reserves at most six starts, including failed or
interrupted attempts. A started stage cannot be retried. The spare reservation
is not an instruction to run another attempt. There is no API fallback.

Credential preflight rejects a known expired subscription token before launch.
An authentication failure that occurs after launch remains a started attempt,
even when the provider reports zero model tokens. An explicitly resumed study
retains that failed reservation within the shared ceiling and records its
recovery provenance separately from the successful learning stage. It does not
rewrite the failed stage's inputs, outcome or runner identity.

Build kapi and prepare a fresh output directory:

```sh
make build
make context-transfer-prepare CONTEXT_TRANSFER_DIR=harness/out/context-transfer
make context-transfer-preflight CONTEXT_TRANSFER_DIR=harness/out/context-transfer CONTEXT_TRANSFER_STAGE=learn
make context-transfer-run CONTEXT_TRANSFER_DIR=harness/out/context-transfer CONTEXT_TRANSFER_STAGE=learn
```

Inspect the learner's `workspaces/learn/context.json` against its sources. Freeze
that unchanged result, then stage and run the independent writer:

```sh
python3 scripts/checkeval/prepare_context_transfer.py freeze-context --out harness/out/context-transfer
python3 scripts/checkeval/prepare_context_transfer.py draft --out harness/out/context-transfer
make context-transfer-preflight CONTEXT_TRANSFER_STAGE=draft
make context-transfer-run CONTEXT_TRANSFER_STAGE=draft
```

Inspect the draft without editing it to create convenient defects. Prepare the
three adaptation workspaces and verify the saved resolver answers:

```sh
python3 scripts/checkeval/prepare_context_transfer.py adapt --out harness/out/context-transfer --kapi-bin bin/kapi
make context-transfer-preflight CONTEXT_TRANSFER_STAGE=references
make context-transfer-preflight CONTEXT_TRANSFER_STAGE=guidance
make context-transfer-preflight CONTEXT_TRANSFER_STAGE=kapi
```

Run each stage once with `make context-transfer-run` and the corresponding
`CONTEXT_TRANSFER_STAGE`. Use the same `CONTEXT_TRANSFER_DIR` throughout if it
differs from the default. Preparation and preflight make no model calls.
Preflight snapshots the prompt, declared inputs, runner and applicable skill
and binary identities. Runtime state is separate from the document inventory.

## Read the result

The report shows the factual brief, full reference sources and attribution,
learned guidance, original documents and all three adaptations. A document
selector supports either a paired view or all four versions. Exact Markdown
diffs remain available beside rendered prose. Missing outputs remain explicit.
Every displayed source or document is verified against its saved hash.

The independent assessment is a JSON object with `method`, `summary` and a
`notes` list. It should discuss concrete changes in reader orientation,
explanation order, actionable instructions and example use, while checking
that product facts survive. It must distinguish useful refinement from a
required correction and identify losses as well as improvements.

```sh
python3 scripts/checkeval/bundle_context_transfer.py --run harness/out/context-transfer --assessment assessment.json --out harness/out/context-transfer-report
python3 scripts/checkeval/report_context_transfer.py --index harness/out/context-transfer-report/index.json --out harness/out/context-transfer-report/review.html --repo .
```

Open the resulting `review.html` directly or serve its directory with a local
HTTP server. No remote scripts, images or styles are required.

## What the evidence establishes

The learner's setup cost is separate from adaptation time. Host completion,
observed model identity, tool use, retained artifacts and semantic quality are
different observations. Agent assessment is not human acceptance or measured
review-time savings.

One run per approach can demonstrate feasibility and expose limitations. An
already adequate draft may need only modest changes. Equal results would
suggest that reusable guidance is useful while leaving kapi's incremental
benefit unproved. A weaker kapi result or slower workflow is equally reportable.
This small demonstration does not compare model families, skill versus MCP,
long-term context maintenance or Bowrain's connected workspace.

## Recorded demonstration

The September 10, 2026 demonstration retained five completed Sonnet 5 stages
and one earlier authentication failure, reaching the six-start ceiling. The
learner took 87.8 seconds and the independent draft took 63.8 seconds. The
draft already followed much of the source guidance and covered the important
product behavior accurately.

| Adaptation | Elapsed time | Observed changes |
| --- | --- | --- |
| Raw references | 155.6 seconds | A preposition correction and consistent configured-workspace wording |
| Guidance as files | 244.5 seconds | More precise local-access wording, several third-person rewrites, and an over-broad troubleshooting instruction |
| Guidance through kapi | 270.4 seconds | The preposition correction and a useful expected-behavior section in troubleshooting |

The plain-guidance adaptation tells readers to try the same command against a
smaller input directory for an unknown error. Only the preview command accepts
a directory; sharing and revocation take IDs. An inferred troubleshooting
pattern became an instruction with broader scope than the facts support.
Several second-person removals also rested on an overstated distinction
between tutorial and explanation guidance. The learned guide itself retained
qualifications, but those qualifications did not reliably govern adaptation.

The kapi agent retrieved all four contexts. It used inspection, a diff preview
and `apply` for the preposition correction. It used ordinary editing to add
the new troubleshooting section, then inspected and checked the result. Its
deterministic checks passed; semantic guidance remained unsupported and no
model-backed check ran. The useful section therefore demonstrates an agent
applying guidance, not structured block insertion or automated semantic
verification.

Independent agent review found localized improvements, no compelling
transformation and no established reduction in human review work. The kapi
result retained the original access overstatement, ambiguous update-link
wording and broadly stated account prerequisite. There is one attempt per
approach; these timings and outcomes do not establish a general ranking.

Transcript audit also retains execution differences: the plain-guidance run
recovered from one denied shell command by reading the supplied files directly.
The kapi host advertised additional skills, although only kapi was invoked.
No outside-workspace content access was observed. These are whole-workflow
observations, not an isolated causal measurement of the CLI.

A useful next demonstration applies a genuine scoped editorial decision across
existing documents and shows which pages change and which remain unaffected.
That exercises the coordination benefit of reusable context more directly than
imitation of a style the starting draft already follows.
