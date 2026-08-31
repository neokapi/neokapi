---
sidebar_position: 0
title: Checks
description: "Checks are tests for AI output: read-only verifiers that inspect content against rules and return one machine-readable Report (pass, score, gate, located findings) without modifying it. A content-first checkset (hygiene, length, patterns, voice) plus opt-in bilingual checks, all one family."
keywords: [checks, content verification, tests for AI, findings, QA, voice profile, terminology, gate, CI]
---

# Checks

A **check** reads content, inspects it against a set of rules, and **reports
findings without modifying it**. neokapi runs every kind of verification through
one engine: deterministic QA rules, terminology enforcement, placeholder and
do-not-translate integrity, and [voice profile](/framework/checks/voice) are
check families that share one model rather than separate systems.

In the CLI, checks run in `kapi check` (and, project-wide, `kapi check
--ship`), and inside `kapi up`'s loop: each pass runs the project's bound
checks over what was produced. `kapi exec` runs a single check tool (`qa`,
`term-check`, `voice-check`) on its own. See
[Understanding the CLI layers](/kapi/direct-execution-layer).

## Checks are tests for AI output

Run as a gate, a check behaves like a test: it is deterministic and repeatable,
and it reports exactly what broke: an over-long string, a forbidden phrase, an
off-brand term, a doubled word. `kapi check` runs a **content-first** checkset
over any file, with no translation needed, and returns one stable, machine-readable
[`kapi.check/v1` Report](#the-report): `pass`, a 0–100 score, a severity gate,
and a finding per **stable rule id** (`length.max-chars-exceeded`,
`hygiene.doubled-word`, …) anchored to the exact **block**.
It **exits non-zero when the gate fails**, so a regression is caught in CI, or
inside an AI assistant's fix-loop, the same way a failing test is. The assistant
drafts, the checks tell it which block and which rule broke, it fixes that block
(through `kapi apply`, or the `apply_edits` MCP tool), and the file ships only
when the gate is green.

Bilingual checks (do-not-translate and placeholder integrity, which
compare a translated target against its source) are an opt-in: pass
`kapi check src.json --target src.de.json --target-lang de`.

## The Report

Every run produces a `core/check.Report` (versioned `kapi.check/v1`): a summary
(counts + score), the gate (the thresholds and which tripped), and a list of
**diagnostics**. Each diagnostic carries a stable `rule` id, a `severity`, a
human `message`, an optional `suggestion`, and a `location` (the block, plus a
run-range when the checker pinpointed one). The stable rule id is the loop's
primary key: an assistant tracks it across iterations to confirm a fix and avoid
regressions. `--json` emits the Report verbatim; over MCP, the `check_file` and
`check_text` tools return the same Report, the verifier counterpart to the
`extract_content`/`apply_edits` editing tools, so an assistant can
**author → check → revise → re-check** without leaving the conversation.

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

Because the model is shared, a single finding list drives every surface: the
`kapi check` exit code, the Kapi Desktop checks panel, and any downstream gate
or editor that consumes the same finding stream.

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
  [voice profile](/framework/checks/voice) profile; plus an optional
  LLM-judged style/voice check.

**Bilingual checks** (opt-in, with `--target`: a translated target
against its source):

- **Placeholder integrity**: catch a dropped `{count}` or a corrupted `<b>` in
  the translation.
- **Do-not-translate**: terms that must survive verbatim into the target.
- **Terminology enforcement**: verifies the right term was used, drawing on the
  project [terms store](/framework/terminology).

The full QA family (whitespace, inline-code integrity, cross-block consistency,
optional LLM review) is documented under [QA Checks](/framework/checks/qa-checks).

> **Document structure & encoding validity** is a format-reader concern rather
> than a content check: the readers extract leniently by default. Surface it on
> demand with `kapi check --validate report` (or `strict` to gate on it): the
> reader emits located `structure.*` / `encoding.*` findings (malformed
> XML/YAML, invalid UTF-8, charset mismatch, and the JSON faults the parser
> rejects) into the same Report. Coverage tracks each reader's own strictness.

## Composing and gating

Checks are tools, so they compose in a [flow](/framework/flows) exactly like
translation or transform stages, typically as the trailing stage after
translation. In CI, gate on the exit code; in an editor or assistant, surface
the findings for one-click fixes. A check never blocks the pipeline by mutating
content; it annotates, and the gate decides.

In a project, `kapi check --ship` (and each pass of `kapi up`) runs the bound
gates over what was produced, each reporting into the same Report: **voice**
(the compliance score against the bound profile, with `--min-score`),
**terminology**, **qa**, **ship** (the ship gate on target status), **source**
(the source-side checks), and **staleness** (content produced under a context
that has since changed, such as a new voice profile version or new term rules,
must be re-run before its locale ships).

For a worked example of gating a pull request on a project's bound checks with
GitHub Actions, see [Ship gates &amp; CI](/kapi/recipes/ship-gates-and-ci).
