---
sidebar_position: 6
title: Contextual meaning evaluation
description: Separate structured-data integrity from context-dependent meaning, and measure checker benefit before expanding agent comparisons.
---

# Contextual meaning evaluation

This note describes semantic-review regression fixtures and experimental response
contracts. The [scoped context evaluation](scoped-context-evaluation.md) tests
whether kapi resolves applicable writing guidance and supports review against it.
The [paired runner](paired-agent-evaluation.md) measures integration behavior.

Procedural completeness, source conflicts and quotation integrity provide small
controls for review machinery. They do not define the product's purpose or
establish the value of context selection. Arithmetic and placeholder checks
remain separate integrity controls.

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

The subscription transport accepts a single complete backtick fence labelled
`json` or without a language, provided the enclosed payload passes the selected
protocol's full validation. It records the envelope contract, transformation,
raw and payload hashes, and original validation errors. The host result retains
the exact raw answer. Reports distinguish bare acceptance, acceptance after
unwrapping, and rejection. Surrounding prose, multiple answers, malformed JSON
and invalid evidence remain failures. The framework parser itself requires bare
JSON. Offline replay can test envelope compatibility; saved study outcomes and
their original contracts remain immutable.

The runner is a direct-review capability probe, not an implementation of a
semantic analyzer inside `kapi check`. A useful result supports prototyping and
measuring that analyzer; any claim of kapi benefit still needs the matched
ordinary-review and context-retrieval comparisons below.

## Requirement-aware framework prototype

`core/check/contextual` supplies an opt-in review contract shared by framework
callers and the subscription experiment. It does not start a model, resolve a
project or change the default `kapi check` behavior. The caller supplies the
candidate, reader task, audience, surface, destination, variables, source
passages and explicit requirements. Each requirement identifies the source
passages that justify it. Requirements define necessary reader actions and
decisions; they do not specify where an error should be found.

`BuildPrompt` constructs the review request. `ParseResponse` validates and
interprets a returned response. The response separates:

| Output | Required evidence | Treatment |
| --- | --- | --- |
| Conflicting claim | Exact candidate quote, source IDs and explanation | Advisory finding, whether or not the claim concerns a declared requirement |
| Requirement covered | Declared requirement ID, candidate quote and reasoning | Coverage observation, not proof of a correct judgment |
| Requirement missing | Declared requirement ID, supporting sources and missing action | Advisory omission finding |
| Requirement uncertain | Declared requirement ID and explanation of uncertainty | Retained separately as an abstention |
| Optional suggestion | Candidate/source evidence and rationale | Separate advice; no content-gate penalty |

Exactly one assessment is required for each declared requirement. An omitted or
invented requirement ID, invalid source reference, malformed JSON or fabricated
candidate quotation invalidates the response. A mention of tool options is not
automatically an instruction to perform the required action. The prompt asks the
reviewer to assess that distinction; the parser cannot decide its truth.

Conflicts and missing requirements map to the common `check.Finding` type with
neutral severity and evidence metadata. The prototype does not issue a release
verdict or promote model allegations into blocking errors. Optional suggestions
and uncertain assessments remain separate from those findings. A complete
response can still contain incorrect reasoning or miss a conflict.

The result retains a serialized snapshot of the supplied request and its
fingerprint, including the contract version. This binds recorded findings to
their processing inputs; it does not certify what the model actually considered.
The experiment independently records the model, effort, exact prompt and shared
core code hash. There is no result cache in this prototype, and the caller must
retain those identities before considering evidence reuse.

### Matched comparison with ordinary review

The quote-based protocol uses `contextual-requirements/v2`. Its prompt and
response contract remain separate from the anchored protocol below, allowing
matched comparisons without changing the control's instructions.

The comparison gives both modes identical candidate text, sources and explicit
requirements. `ordinary` uses the existing open finding-list protocol;
`requirements` uses the shared framework prompt and requires a full assessment
of the declared requirements. The intervention includes the prompt, response
structure and validation together. Different output lengths can affect elapsed
time and token use, which remain part of the comparison.

```sh
make meaning-eval-prepare-comparison MEANING_EVAL_INPUTS=harness/out/requirements-inputs
make meaning-eval-preflight MEANING_EVAL_INPUTS=harness/out/requirements-inputs MEANING_EVAL_DIR=harness/out/requirements-review
make meaning-eval-run MEANING_EVAL_INPUTS=harness/out/requirements-inputs MEANING_EVAL_DIR=harness/out/requirements-review
```

Only the last command starts inference. Six fresh sessions cover three selected
development cases twice: the supported release guide, supported editing guide
and faulty destination guide. They target unnecessary omission claims and a
missed post-save action from development work. They are not held-out evidence.
Protocol order alternates by case, and host/model/effort, evidence and resource
limits are fixed. A host/case/protocol combination cannot be repeated within the
study. Shared core changes invalidate study identity as well as runner changes.

Results retain the native framework assessment, neutral findings and a common
view of conflicts, omissions and abstentions. Optional suggestions are displayed
separately. A lower finding count is useful only if required errors remain
detected; requiring a checklist is useful only if its judgments are correct.
Independent source-based adjudication remains necessary. This comparison can
assess the prototype's review contract with supplied evidence; it cannot
establish the benefit of automatic context retrieval or production readiness.

### Readiness before integration

Response reliability and action coverage are separate requirements for a usable
analyzer. Removing an accepted transport envelope addresses formatting alone.
A structurally valid assessment can still mark a necessary action as covered
merely because the candidate describes that tool's options. Neither response
validity nor complete coverage establishes semantic correctness.

The review prompt considers who acts, what they do, the object of the action and
its relevant conditions. A statement of available form fields or tool options
can leave a required instruction absent. Faithful indirect instructions can
cover the action without imperative wording or matching keywords. Ambiguous
coverage requires an uncertain assessment with an explanation.

Before adding a caller-facing analyzer, verify response-format handling with
saved outputs and test action coverage on fresh document families. Include
explicit instructions, faithful indirect instructions and descriptions that
leave a necessary step unstated. Any transport normalization must retain the raw
response and record the transformation separately; it must not reinterpret
already-frozen study results. Continue to supply identical requirements to the
ordinary-review control. These checks precede wider host/model comparisons and
automatic context retrieval.

### Fresh action-coverage cases

`scripts/checkeval/action-coverage.json` defines fictional museum-transfer,
event-equipment handoff and editorial-correction procedures. Each family has
direct instructions, faithful indirect instructions and a variant describing
available options while leaving a necessary action unstated. The governing
policies are authored synthetic evidence, not claims about real organizations.
All variants share their family's task, sources and explicit requirements.

```sh
make meaning-eval-prepare-actions MEANING_EVAL_INPUTS=harness/out/action-inputs
make meaning-eval-preflight MEANING_EVAL_INPUTS=harness/out/action-inputs MEANING_EVAL_DIR=harness/out/action-review
make meaning-eval-run MEANING_EVAL_INPUTS=harness/out/action-inputs MEANING_EVAL_DIR=harness/out/action-review
```

Preparation fixes one case from each family: a direct instruction, an indirect
instruction and a missing action. Each receives ordinary and requirement-based
review under the same subscription limits. The casebook shows all nine
variants; only the selected three enter subject inputs. The other variants
remain unrun development controls. An independent agent assesses the cases
before live review, with disagreements resolved and recorded separately from
the authored labels. This small probe checks behavior on fresh families; it
does not estimate generalization, human acceptance or a product advantage.

### Passage selection and explicit claim comparison

`BuildAnchoredPrompt` and `ParseAnchoredResponse` provide a separate opt-in
contract, `anchored-requirements/v1`. The request retains the original candidate
and derives numbered paragraph spans with exact text and UTF-8 byte offsets.
The reviewer selects passage IDs instead of reproducing quotations. Parsing
resolves those IDs against the request; unknown or repeated IDs fail validation.
The native result and advisory findings retain the selected spans. Separate
passages are never concatenated into a purported contiguous quotation.

Each conflict states the model's interpretation of the candidate claim and the
source claim, together with their evidence and an explanation of the
incompatibility. A wrong actor, object or condition may itself be the conflict;
the two claims need alignment, not identical roles or scope. A heading or field
description alone does not assert that a required action is optional. When the
problem is an absent instruction, it belongs in the requirement assessment.
An additional conflict needs an independently asserted incompatible claim.

The parser validates structure and references. It cannot establish whether the
selected paragraph asserts the model's paraphrase, whether the source supports
the comparison, or whether two findings describe the same defect. It does not
silently discard alleged conflicts based on overlapping references or keywords.
Reports show both model interpretations beside the selected original passages
so those errors remain inspectable.

```sh
make meaning-eval-prepare-evidence MEANING_EVAL_INPUTS=harness/out/evidence-inputs
make meaning-eval-preflight MEANING_EVAL_INPUTS=harness/out/evidence-inputs MEANING_EVAL_DIR=harness/out/evidence-review
make meaning-eval-run MEANING_EVAL_INPUTS=harness/out/evidence-inputs MEANING_EVAL_DIR=harness/out/evidence-review
```

The six-session comparison pairs the unchanged quote-based requirements
protocol with the anchored protocol on the museum transfer, editorial omission
and faulty destination-guidance cases. Both receive identical task evidence and
requirements. Only the anchored prompt adds derived passage IDs and explicit
claim-comparison instructions. This bundled intervention targets known
development failures and preservation of real conflicts; it does not isolate
the effect of evidence selection or establish performance on held-out cases.

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

## Regression-suite policy

Retain the existing procedural cases as bounded regressions for lost evidence,
missed necessary actions and unsupported allegations. Run deterministic
preparation and parser tests when their contracts change. Their labels remain
provisional, and repeated live review of these same cases cannot establish
performance on unseen documents.

Product evaluation uses actual context resolution, source-grounded writing
guidance and legitimate destination changes. A procedural instruction belongs
in that context only when the applicable guidance or authoring brief requires
it. The reviewer must not invent a checklist of everything a guide could cover.
Style departures, factual conflicts and optional editorial suggestions remain
separate judgments. See the [scoped context evaluation](scoped-context-evaluation.md)
for the active workflow and its limits.

Any further live regression run needs a concrete unresolved question, frozen
inputs and independently assessed expectations. Use at most six reserved
subscription attempts, including failures, with no automatic retries or API
fallback. Report every attempt rather than treating a small development sample
as a release threshold or a stable latency estimate. Human acceptance and
review-time claims require actual human evidence.

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
counterexamples are required. Style findings must follow applicable guidance; personal phrase preferences
and guesses about authorship do not establish a departure.
