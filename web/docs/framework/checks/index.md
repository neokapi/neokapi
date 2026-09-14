---
sidebar_position: 0
title: Checks
description: "Checks are tests for AI output: read-only verifiers that inspect content against rules and return one machine-readable Report (pass, score, gate, located findings) without modifying it. A content-first checkset (hygiene, length, patterns, voice) plus opt-in bilingual checks, all one family."
keywords: [checks, content verification, tests for AI, findings, voice profile, terminology, gate, CI]
---

# Checks

A **check** reads content, inspects it against a set of rules, and **reports
findings without modifying it**. neokapi runs every kind of verification through
one engine: deterministic rule-based checks, terminology enforcement, placeholder and
do-not-translate integrity, and [voice profile](/framework/checks/voice) are
check families that share one model rather than separate systems.

In the CLI, `kapi check <path>` checks authored content and `kapi check --ship`
enforces project release gates. Checks also run inside `kapi up`'s loop: each pass runs the project's bound
checks over what was produced. `kapi exec` runs a single check tool (`qa`,
`term-check`, `voice-check`) on its own. See
[Understanding the CLI layers](/kapi/direct-execution-layer).

## Checks are tests for AI output

Exact rule checks behave like tests: for fixed input and context they are
repeatable, and report a specific violation: an over-long string, a forbidden phrase, an
off-brand term, a doubled word. `kapi check` runs a **content-first** checkset
over any file, with no translation needed, and returns one stable, machine-readable
[`kapi.check/v1` Report](#the-report): `pass`, a 0–100 score, a severity gate,
and a finding per **stable rule id** (`length.max-chars-exceeded`,
`hygiene.doubled-word`, …) anchored to the exact **block**.
It **exits non-zero when the gate fails**, so a regression is caught in CI, or
inside an AI assistant's fix-loop, the same way a failing test is. The assistant
drafts, the checks tell it which block and which rule broke, it fixes that block
(through `kapi apply`, or the `apply_edits` MCP tool), and re-checks against the
declared thresholds. Meaning and unsupported guidance remain for review.

Bilingual checks (do-not-translate and placeholder integrity, which
compare a translated target against its source) are an opt-in: pass
`kapi check src.json --target src.de.json --target-lang de`.

## The Report

A completed `kapi check` run produces a `core/check.Report` (versioned
`kapi.check/v1`): a summary
(counts + score), the gate (the thresholds and which tripped), and a list of
**diagnostics**. Each diagnostic carries a stable `rule` id, a `severity`, a
human `message`, an optional `suggestion`, and a `location` (the block, plus a
run-range when the checker pinpointed one). The stable rule id is the loop's
primary key: an assistant tracks it across iterations to confirm a fix and avoid
regressions. `--json` emits the Report verbatim; over MCP, the `check_file` and
`check_text` tools return the same Report, the verifier counterpart to the
`extract_content`/`apply_edits` editing tools, so an assistant can
**author → check → revise → re-check** without leaving the conversation.

For a draft intended for a project file, pass that destination as `context_path`
to `check_text`. kapi applies the destination's voice channel and terms even
before the file exists. For example:

```json
{"text": "Your draft wording", "context_path": "content/en/page.json"}
```

The report describes the supplied text and records its intended destination in
`target.context_path`. After saving, use `check_file` to check the file itself.
An unscoped snippet can use `profile_file` or `profile_pack`; these explicit
profile options cannot be combined with `context_path`.

The optional `execution` object records which analyzers ran for each input,
which were not requested, and why a capability was unsupported. Each entry has
an analyzer ID, execution status, finding count and, when it ran, a duration.
The host also reports context, extraction, analysis, report-construction and
total time in milliseconds; serialization and process startup are excluded.
Reports without `execution` have **unreported coverage**.

The report's `verdict` is `passed`, `failed` or `did_not_run`, and `pass` is
true only for `passed`. Beside the content, each analyzer is given a known-bad
sample, its **canary**, through the same configured checker, and records what it
made of it in `execution.analyzers[].canary`: `caught`, `missed` or `impossible`.
An analyzer that misses its canary has status `invalid`, and the run did not
run, whatever else it reported. An analyzer whose configuration gives it nothing
to catch, such as a voice profile with no deterministic rules, has status
`did_not_run`; when the invocation asked for it by name, the run did not run
either. A run that checked no blocks did not run. The top-level `did_not_run`
field lists the reasons. `kapi check` exits `4` for this verdict, and neither
`--no-fail` nor `--lenient` changes that.

Every `did_not_run` verdict carries a `did_not_run_cause`, and the causes share
exit code `4`, so read the cause before acting on it:

- `checker_invalid`: an analyzer reported nothing on its canary. A checker is
  broken, and nothing this run reported can be trusted, including its findings.
- `nothing_to_check`: no content was in scope, such as an empty file or a diff
  that touches no content block.
- `content_not_checked`: content in scope was not checked, such as a changed
  file whose blocks could not be located, an analyzer the invocation asked for
  with nothing to catch, or declared content in a format with no installed
  reader when the check read nothing else.

`execution.contexts` records the effective guidance for each checked input.
Each entry identifies the file or draft destination, the voice selection
(`project`, `override` or `none`), the loaded profile name and source, and the
project profile and channel when they governed the check. `voice.applied`
distinguishes a loaded profile from a project location with no voice binding.
`terms_applied` states whether terminology was supplied to the checks; it does
not imply that a term matched or produced a finding. Project terminology checks
also run when no voice profile is bound. An omitted `contexts`
field means the producer did not report context selection.

When a project resolved the guidance, each entry carries the `point` it was
resolved at: the `profile` and `channel`, and `comments` for the point a file's
comments sit at. A file whose comments sit apart from its other content has one
entry for each point. Each finding carries the `point` its block was checked at,
and an analyzer that ran once for each point carries its point too.

The optional `warnings` array names problems in the configuration a check ran
under, apart from the findings about content. Each entry has a stable `code`, a
`message`, the `source` it came from (a profile file's path, or `pack:<name>` or
`store:<name>`) and, when the warning concerns one key, its dotted `key`. A run
reports the warnings of each voice profile it loads once, however many files the
profile governs:

- `voice.unknown_key`: a key the voice profile model does not define. The
  profile loads without it, so whatever the key was meant to state applies
  nowhere. `kapi voice validate` refuses the same key.
- `voice.unfamiliar_value`: a tone value outside the usual set, kept and
  rendered into the voice guide as written.
- `voice.preferred_term_dropped`: a channel or persona preferred term that
  resolution drops, because a profile rule already governs the term.
- `voice.override_drops_pattern`: a base style pattern that stops applying where
  a channel or persona supplies its own style.
- `format.no_reader`: a file the project declares as content, in a format no
  installed reader handles, usually one a plugin supplies. `source` is the file,
  and the message names the plugin to install. A check over the project's
  content leaves the file out, checks the rest and prints the same warning on
  stderr. A file named on the command line in such a format fails the check
  instead.

Warnings never change the summary, the score, the gate, the verdict or the exit
code. Fix the configuration they name. `kapi check` prints them after the
verdict, `kapi check --ship` carries the same array, and the MCP check tools
return it in their report.

For a project file, omit MCP `profile_file` and `profile_pack` to retain its
applicable profile and channel. An explicit profile replaces that voice
selection, while project terms still apply. Check the reported scope before
using a passing result to assess the task.

A passing gate and a score of 100 mean no gate-breaking findings in the
configured checks. They do not establish factual accuracy or compliance with
rules that were never encoded. Exact voice rules, advisory example similarity
(`--voice`), and optional LLM review are distinct analyses. `kapi check` does not
run LLM review and records `voice.llm` as `not_requested`. Applicable profile
guidance that exact rules cannot assess is recorded as `voice.guidance` with
status `unsupported`; it does not fail the deterministic content gate.

A requested analyzer that cannot run, or governing context that cannot resolve,
returns an operational error instead of a successful report. `--no-fail` affects
content-gate exits only. MCP uses its existing error response for these failures.
Embedded applications share checker primitives but can retain their own response
shapes and operation error views.

## Checking a change

`kapi check --diff-against <rev>` checks only the content a change touched. kapi
runs `git diff` against the revision, read-only, treats untracked files as
wholly new, and reads each changed file once. `--diff-file <path>` reads a
unified diff you already hold, or standard input with `-`. Named files narrow
the scope.

A diff names lines, and kapi widens each changed line to the content block it
belongs to: a one-line edit inside a seven-line paragraph checks the whole
paragraph. A finding's `location.lines` gives the lines of its block. Rules that
hold over a whole document, such as a voice profile's required patterns, read
the whole changed file.

In a source file read for its comments, such as a Go file, or a TypeScript,
Python or CSS file the sourcecode plugin reads, the unit is the comment. A
changed line inside a comment checks that comment whole, and a change to code or
to directives alone leaves the file `untouched`. Where the language has a
formatter, it compares only the comments the change touched, and it catches its
canary as it does when kapi checks the whole file.
When a recipe declares the comments of a file a reader parses, such as a YAML,
Markdown or PO file with `comments: true`, each comment is a block beside the
reader's blocks, and a changed line takes whichever block it sits in.

Removed lines leave nothing to point at, so a deletion first takes the blocks on
both sides of where the lines were. A block that lost its first or last lines is
then in scope, and so is one the deletion merged with its neighbour. For a
block the deletion only borders, kapi rebuilds the file as it was before the
change from the diff, and drops the block from the scope when the earlier file
holds the same block on the same lines: removing one key of a JSON catalog
checks neither the key before it nor the key after it.

The report's `scope` lists every file the diff names with a status:

- `checked`, with the blocks checked and their lines;
- `untouched`, when the change touched no content block, as with markup, a
  rename or a mode change;
- `no_content`, `no_reader` or `deleted`, where the reason of a `no_reader`
  file the project declares in a plugin's format names the plugin to install;
- `out_of_scope`, for a file a project's recipe does not declare as content, or
  one outside the files named;
- `did_not_run`, with the reason, for a changed file whose touched content
  cannot be located: a format that keeps no record of where its content sits, a
  binary change, or a change on lines where the file's structure leaves a
  block's position ambiguous. The reason names that block and the lines it could
  sit on. A change elsewhere in the same file is checked as usual, and the
  touched blocks kapi can place are checked and listed either way.

A `did_not_run` file leaves the whole check `did_not_run`, and so does a diff
that touches no content block. Over MCP, `check_file` takes `diff` (unified diff
text) or `diff_against` (a revision) for the same scope, and `file` then narrows
it.

## One model: findings

Every check emits the same structured **finding** (the `core/check.Finding`
type): a kind, a severity, the run-index range it points at, and an optional
suggested replacement. A check is a read-only [tool](/framework/tools): it uses
the annotate capability, so it may attach findings but never rewrite content
(see the [immutability model](/framework/tools)). Findings are recorded as
stand-off [overlays](/framework/content-model) anchored to the offending runs,
so a check pass slots into any [flow](/framework/flows) as an ordinary stage and
its results surface uniformly to the CLI, an editor, the MCP tools, or a
downstream gate.

The shared finding model lets the CLI, Kapi Desktop and downstream editors
consume the checks each surface invokes. Matching findings does not imply that
every surface requested the same analysis.

## The check families

**Generic content checks** (source-side, no translation needed; the default
checkset):

- **Text hygiene**: empty content, doubled spaces and words, stray leading/
  trailing whitespace, control characters. Always on.

  Hygiene is judged against the block's **content boundaries**, where an inline
  code counts as content. A leading or trailing placeholder is the edge of the
  content, so the space beside it is a separator, not stray whitespace:
  `{price} each` has no leading whitespace, `Hello {name} world` has no double
  space, and a block that is only a placeholder is not empty. Genuine
  whitespace (` {price} each`) still reports.

  An inline code is likewise a real **boundary** for the adjacency rules: it
  separates what sits either side of it, so `the {name} the cat` holds no doubled
  word. It is a token of its own rather than part of the word beside it, so it
  cannot hide one either: `{name}the the cat` reports. Nothing separates two
  *adjacent text runs*, so a defect spanning their join (`the ` + `the cat`) is
  real and reports. The editor's highlights come from these same rules, so a
  preview and a `kapi check` finding cannot disagree.
- **Length**: flag content over a character or word budget (`--max-chars`/
  `--max-words`).
- **Patterns**: regex that must not appear (`--forbid`) or must appear
  (`--require`) in the content.
- **Voice vocabulary**: forbidden/competitor/preferred-term rules from a bound
  [voice profile](/framework/checks/voice). Optional `--voice` adds an advisory
  similarity comparison with profile examples. LLM voice review is a separate
  tool or an explicit AI voice command.
- **Formatter agreement**: for the comments in source code, a comment the
  language's formatter would rewrite is a `formatter.<formatter>` finding, such
  as `formatter.gofmt`. It fails the gate whatever the severity limits allow;
  `--lenient` reports it without failing. The comment reader and the formatter
  each catch a canary on every run, as every other analyzer does. See
  [Comments in source code](/kapi/recipes/verify-content#comments-in-source-code).
- **Comment limits**: where the voice profile at a comment's point sets
  [comment limits](/reference/serialization/voice-profile#comment-limits), a
  sentence over the minor or major word limit is a `comment.sentence-length`
  finding of that severity, and a comment over the limit for what it documents
  is a major `comment.length` finding. Sentences are split with the UAX #29
  sentence break over the comment's prose: the lines a comment wraps over are
  joined first, a blank line, a divider line, a list item, a heading or a
  documentation tag starts a new sentence, and code spans, references and links
  are not counted as words. A build without the sentence break reports the
  sentence check as not run. In a check scoped to a diff, a change that adds at
  least the minimum number of comment lines to a file, and more comment lines
  for each code line than the ratio allows, is a major `comment.density`
  finding. Lines are counted from where the comment reader places each comment,
  and the package doc comment is not counted. A check over whole files reports
  density as unsupported.

**Bilingual checks** (opt-in, with `--target`: a translated target
against its source):

- **Placeholder integrity**: catch a dropped `{count}` or a corrupted `<b>` in
  the translation.
- **Do-not-translate**: terms that must survive verbatim into the target.
The separate `term-check` tool checks target terminology against rules from
the project [terms store](/framework/terminology).

The full rule-based family (whitespace, inline-code integrity, cross-block
consistency, optional LLM review) is documented under
[Rule-based checks](/framework/checks/rule-checks).

> **Document structure & encoding validity** is a format-reader concern rather
> than a content check: the readers extract leniently by default. Surface it on
> demand with `kapi check --validate report` (or `strict` to gate on it): the
> reader emits located `structure.*` / `encoding.*` findings (malformed
> XML/YAML, invalid UTF-8, charset mismatch, and the JSON faults the parser
> rejects) into the same Report. Coverage tracks each reader's own strictness. Validate source and target files
> separately; reader validation cannot be combined with `--target`.

## Composing and gating

Checks are tools, so they compose in a [flow](/framework/flows) exactly like
translation or transform stages, typically as the trailing stage after
translation. In CI, gate on the exit code; in an editor or assistant, surface
the findings for one-click fixes. A check never blocks the pipeline by mutating
content; it annotates, and the gate decides.

In a project, `kapi check --ship` (and each pass of `kapi up`) runs the bound
gates over what was produced. The project-gate response groups findings by gate:
**voice**
(the compliance score against the bound profile, with `--min-score`),
**terminology**, **qa**, **ship** (the ship gate on target status), **source**
(the source-side checks), and **staleness** (content produced under a context
that has since changed, such as a new voice profile version or new term rules,
must be re-run before its locale ships).

Source-only collections receive authored-content checks, including in a project
that also has translated targets. Naming a source-only file checks that content
directly; translation checks apply to target content with its source. Each gate
carries the same three verdicts as a report: a gate with no content in scope,
or whose checks did not catch their canaries, did not run, and the command
exits `4`. See
[the agent surface design](/contribute/architecture/surfaces/s-03-agent-surfaces)
for the edit loop and its release checks.

For a worked example of gating a pull request on a project's bound checks with
GitHub Actions, see [Ship gates &amp; CI](/kapi/recipes/ship-gates-and-ci).
