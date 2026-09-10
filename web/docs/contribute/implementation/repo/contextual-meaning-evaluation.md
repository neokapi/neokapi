---
sidebar_position: 6
title: Contextual meaning evaluation
description: Separate structured-data integrity from context-dependent meaning, and measure checker benefit before expanding agent comparisons.
---

# Contextual meaning evaluation

The primary question is whether kapi helps preserve consequential meaning during
content work. Arithmetic, placeholder preservation and exact approved strings
are useful workflow controls, but they are weak evidence of this contribution.
The [paired runner](paired-agent-evaluation.md) measures integration behavior;
contextual evaluation first isolates what an analyzer can detect and explain.

## What belongs in the evaluation

| Concern | Preferred owner | Evaluation role |
| --- | --- | --- |
| Calculate a reminder dispatch day | Application logic or structured configuration | Secondary deterministic regression |
| Carry a supplied day or limit across surfaces | Variables, templates and content-edit integrity | Secondary propagation and preservation control |
| Associate a limit with the correct object | Content plus governing source | Primary meaning criterion, even when all placeholders survive |
| Preserve a prerequisite, permission or exception | Content plus scoped guidance | Primary meaning criterion |
| Preserve certainty and participant roles | Content plus source evidence | Primary meaning criterion |
| Improve voice, rhythm or concision | Audience-specific writing guidance | Separate suitability study with an explicit rubric |

For example, both of these candidates preserve exactly the same variables:

```text
Each file can use up to {file_limit}; a workspace can store up to {workspace_limit} in total.
Each file can use up to {workspace_limit}; a workspace can store up to {file_limit} in total.
```

If the source binds the variables as their names suggest, the second candidate
reverses the limits. A placeholder check cannot establish the binding. Variable
names are also not authoritative evidence: legacy identifiers can have different
documented bindings. Context determines whether the sentence is supported.

Moving calculations into variables reduces avoidable errors. It does not remove
the need to check the claims around those variables. Where an author genuinely
must derive a value, retain the calculation as an explicit task requirement and
report it separately from semantic performance.

## Development contrasts

`scripts/checkeval/meaning-seed.json` contains six synthetic capability groups:
prerequisites, exceptions, permissions, certainty, participant roles and variable
binding. Each group has a supported candidate, a valid paraphrase, a conflicting
candidate and the identical conflicting wording supported by different source
evidence. Two additional cases lack enough evidence for a definite answer.

These 26 cases specify development behavior. They are deliberately small, with
provisional authored labels; they are neither independent documents nor an
adjudicated benchmark. They need no preference ballot. Their purpose is to make
the desired analyzer behavior inspectable and expose shortcuts such as treating
every guarantee, permission or unusual variable name as an error.

The method draws on capability-based behavioral tests in
[CheckList](https://aclanthology.org/2020.acl-main.442/) and controlled changes in
[contrast sets](https://aclanthology.org/2020.findings-emnlp.117/). Applying those
methods here does not establish product benefit.

Prepare the inputs and a browser-readable casebook without inference:

```sh
python3 scripts/checkeval/prepare_meaning.py --out harness/out/meaning-development
```

Open `harness/out/meaning-development/review.html` in a browser. It shows the
reader task, variables, governing source, candidate and expected reasoning
together. The command refuses an existing output directory. The manifest records
corpus and preparer hashes, development status and unmeasured performance.

Only `inputs.jsonl` and `instructions.txt` belong in a future subject workspace.
The preparer gives inputs neutral IDs and ordering; `labels.json`, the source
corpus and `review.html` stay with the evaluator. Related variants must be run in
separate fresh contexts so a subject cannot infer an answer from its paired
candidate. Input preparation does not implement a model runner or a semantic
scorer. The existing `checkeval` regression gate and dashboard are unaffected.

## Whole-document development probe

`scripts/checkeval/contextual-documents` contains three repository-grounded
families. Each has a supported guide and a controlled faulty variant, written
for a specific reader, surface and task:

| Family | Reader decision | Consequential defects |
| --- | --- | --- |
| Destination guidance | How to check a draft and the saved project file | Wrong profile/channel relationship, disabled-term claim, omitted saved-file check |
| Concurrent content editing | How to apply a content-only batch and recover blocked edits | False whole-batch rollback guarantee, omitted fresh inspection and hashes |
| Release evidence | What a completed release check establishes | Wrong source-only scope, unsupported assurance of factual accuracy, missing operational-failure handling |

The guides are roughly 265–346 words, with 693–839 words of source evidence per
family across several passages. Excerpts retain repository paths, Git blob
identities and line ranges. They include useful adjacent detail, so finding a
relevant claim requires selecting evidence. The preparer verifies those excerpts
against the recorded blobs of the current source files; a changed source requires
an explicit evidence review and a fresh preparation.

These are synthetic adaptations of product documentation, not naturally
occurring customer errors or held-out proof of generalization. An independent
agent review can check the author's labels before a probe; its agreement remains
agent evidence, not human adjudication. All variants stay in development.

Prepare and inspect the casebook, then verify subscription readiness:

```sh
make meaning-eval-prepare
make meaning-eval-preflight
```

Open `harness/out/contextual-documents/review.html`. The subject subdirectory
contains the manifest, neutral-ID inputs and review instructions. Labels,
construction notes and the casebook stay outside subject sessions. Preparation
refuses an existing output directory. `MEANING_EVAL_INPUTS` and
`MEANING_EVAL_DIR` select alternative input and evidence directories.

Record current deterministic coverage separately, using an already-built kapi:

```sh
make build
make meaning-eval-checks
```

The checker receives each candidate as Markdown and the exact governing passages
as guidance constraints in an explicit profile. No planted answer is converted
to a regex. Configuration, project discovery and plugins are isolated. Saved
reports include the binary and input hashes, raw findings, analyzer coverage,
exit status and elapsed process time. Unsupported semantic guidance is not
reported as successful semantic analysis. This is a coverage baseline, not a
comparison of context retrieval.

The live target consumes the signed-in subscription:

```sh
make meaning-eval-run
```

The prepared manifest uses the existing `claude-sonnet-5` model at `high` effort
for six fixed-document reviews, one per candidate in a fresh session. This probe
does not rank agent hosts or compare CLI and MCP. It bounds each attempt to
three host turns and 180 seconds. Tools are prohibited; Claude's tool set is
disabled and the transcript audit rejects observed tool use. No rewriting or
repair loop runs inside the attempt.

The runner reuses the paired evaluator's subscription authentication, redaction
and observed-model verification. It freezes the input files, exact prompts and
runner code before execution. A persistent reservation counts even when a
launch is interrupted, and resumed runs never retry it. The ceiling is at most
six reservations, including failures; a rate-limit stop prevents further
launches in that study. No API fallback is enabled. Reported tokens describe
consumption, not remaining subscription allowance.

Each result retains the raw host outcome and a separate integrity result. The
integrity check requires valid JSON, existing source IDs and exact candidate
quotes where applicable; it does not establish that a finding's reasoning is
correct. Conflicts, omissions and abstentions are separate. Semantic review of
saved findings compares their explanations to the source and considers findings
outside the planted inventory. An empty finding list is not a publishing verdict.

Render saved outcomes beside their candidate, source evidence and provisional
labels without launching another review:

```sh
python3 scripts/checkeval/report_documents.py --prepared harness/out/contextual-documents --study harness/out/contextual-review --checks harness/out/contextual-review-checks/coverage.json --out harness/out/contextual-results.html
```

The renderer requires matching input hashes. It retains unparsed answers as
unparsed, with their raw text and protocol errors available for inspection.
Optional separately recorded agent adjudication can explain disputed or
unjustified findings; it never rewrites the saved result into an accepted one.

The runner is a direct-review capability probe, not an implementation of a
semantic analyzer inside `kapi check`. A useful result supports prototyping and
measuring that analyzer; any claim of kapi benefit still needs the matched
ordinary-review and context-retrieval comparisons below.

## What counts as evidence

Use three outcomes: supported, contradicted, and insufficient context. Supported
means supported for the stated claim and reader task, not overall writing
quality. An unsupported claim may need attention without its opposite being
proven. The analyzer must identify missing evidence rather than invent a repair.

Claim support is separate from completeness. A true sentence can omit the
exception or next step the reader needs. Longer document cases therefore label
required guidance and consequential omissions separately; an omission must not
be forced into the contradiction category. The seed's claim verdicts do not
establish that a document is complete or ready to publish.

A useful contradiction finding identifies the affected passage, cites the
applicable source and explains the actual conflict. A correct category with an
incorrect explanation does not count as a correct finding. Source citation
validity and output schema can be validated automatically; whether the cited
source entails the explanation needs independent adjudication. A model judge is
a fallible measurement instrument and needs calibration against independently
reviewed labels before its scores support quality claims.

Report missed consequential errors, false alarms on valid alternatives,
context-sensitive consistency, appropriate abstention, explanation correctness
and operational failures separately. Unexpected findings are reviewed rather
than automatically counted as false positives: planted errors are not an
exhaustive inventory of possible problems. Unsupported analyzers remain
unmeasured, even when their output contains no findings.

## Implementation priorities

1. **Establish meaningful source-grounded cases.** Keep the seed as development
   material. Build a separate set from longer, coherent documents and actual
   content decisions, with an explicit source of truth, audience, surface and
   destination scope. Include conditions spread across sections, a scoped
   exception to a general rule, missing action instructions, and plausible but
   irrelevant guidance. Record
   which information is required for each finding. Have a reviewer independent
   of case construction adjudicate labels and disagreements once, before runs.
   Keep related original, mutated and repaired documents in the same partition.
   Synthetic extensions remain labelled as synthetic.
2. **Measure detection before rewriting.** Freeze candidate texts and compare
   current deterministic checks with source-grounded review. Record exact
   inputs, effective evidence, raw findings and analyzer coverage. A direct
   model review is a candidate capability baseline; it is not evidence that
   kapi implements semantic analysis. This removes authoring variability while
   revealing whether a new analyzer could add useful coverage.
3. **Prototype one contextual analyzer.** Start with prerequisites and scoped
   exceptions. Input includes the candidate, destination context and relevant
   source passages; output includes evidence, conflict and uncertainty. Required
   reader actions are explicit inputs distinct from reference material. An
   omission finding must identify the required action the reader cannot complete;
   it cannot promote every available reference detail into mandatory content.
   Keep optional improvements advisory and distinguish claim conflicts from
   incomplete instructions. Reuse
   the common finding/report path, retain deterministic checks for exact
   constraints, and report unsupported or failed analysis explicitly. Measure
   context resolution, inference and total elapsed time independently. Evaluate
   missing or wrong context as well as the correct-context condition.
   Keep model analysis outside the immediate deterministic loop until latency
   and finding usefulness justify it. A later cache must bind candidate, source,
   task requirements, effective context and analyzer identity; changing any of
   them invalidates its evidence.
4. **Compare with an ordinary extra review.** Within each fixed host/model,
   compare the same saved draft reviewed without kapi and through kapi, under
   a matched total resource budget. Give both access to the same source corpus.
   A second condition supplies the same resolved passages directly to isolate
   analyzer benefit from retrieval benefit. Record context acquisition costs;
   do not compare a two-pass kapi workflow only against a first draft.
5. **Return to authoring integration comparisons after useful coverage appears.**
   Use held-out document families and report repair success, introduced errors,
   discovery failures and observed CLI/MCP use separately. If the baseline
   reliably handles a case already, keep it as a regression rather than spending
   more grading effort on it. Human acceptance or review-time claims still need
   actual human evidence.

Before the next live experiment, fix the document partition, label status,
analyzer version, checker model, prompt, effort and reporting criteria. Use a
bounded subscription allowance of at most six started host attempts, including
failures, with the number of documents and maximum review work per attempt
declared too. A small number of sessions must not hide an unbounded inner loop.
No automatic retries or API fallback are part of this plan. The development
preparer itself makes no model calls and changes no allowance.

Six attempts are a feasibility probe. Continue only if the analyzer provides
evidence-correct findings beyond existing checks and the matched ordinary review
on consequential cases, without systematic false alarms on clean or changed-
context controls. Otherwise revise the capability or defer it. Numerical quality
thresholds for a release need the adjudicated document set and cannot be inferred
from six toy families. Keep latency observations per attempt; such a small probe
cannot support a stable tail-latency claim.

## NER, structure and style

`core/ai/ner` already defines a batched provider interface and local-provider
registration. `core/ai/tools/entity_extract.go` consumes it; the browser has a
JavaScript bridge for a local model. Native availability must be verified for
the binary under test. An absent provider is not a successful empty extraction.

Entity tagging can supply spans and identity candidates for later analysis.
It does not establish who acts on whom: swapping Mira and Leon preserves both
entities. Structure detection can locate conditions and headings, but the
semantic decision still needs their relationship to the claim. Reuse existing
interfaces if a measured extraction bottleneck justifies a native model. Compare
entity accuracy, memory, cold initialization and warm batch latency separately
from downstream meaning accuracy before selecting a model or adding a dependency.

Style analysis has a separate rubric: for example, unsupported superlatives,
repetition that obscures the requested action, or tone that conflicts with the
intended audience. A label such as “Opus-style AI slop” is neither a stable
criterion nor evidence of authorship. Context-specific examples and acceptable
counterexamples are required. A general detector of disliked phrases should not
take priority over demonstrable losses of meaning.
