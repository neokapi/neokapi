---
sidebar_position: 0
title: Checks
description: Checks are tests for AI output — read-only verifiers that inspect content against rules and return one machine-readable Report (pass, score, gate, located findings) without modifying it. A content-first checkset (hygiene, length, patterns, brand) plus opt-in bilingual checks, all one family.
keywords: [checks, content verification, tests for AI, findings, QA, brand voice, terminology, gate, CI]
---

# Checks

A **check** reads content, inspects it against a set of rules, and **reports
findings without modifying it**. neokapi runs every kind of verification through
one engine: deterministic QA rules, terminology enforcement, placeholder and
do-not-translate integrity, and [brand voice](/framework/checks/brand-voice) are
not separate systems — they are check families that share one model.

## Checks are tests for AI output

Run as a gate, a check behaves like a test: it is deterministic and repeatable,
and it reports exactly what broke — an over-long string, a forbidden phrase, an
off-brand term, a doubled word. `kapi check` runs a **content-first** checkset
over any file — no translation needed — and returns one stable, machine-readable
[`kapi.check/v1` Report](#the-report): `pass`, a 0–100 score, a severity gate,
and a finding per **stable rule id** (`length.max-chars-exceeded`,
`hygiene.doubled-word`, `brand.vocabulary`, …) anchored to the exact **block**.
It **exits non-zero when the gate fails**, so a regression is caught in CI — or
inside an AI assistant's fix-loop — the same way a failing test is. The assistant
drafts, the checks tell it which block and which rule broke, it fixes that block
(often via the [`rewrite`](/framework/tools) moat), and the file ships only when
the gate is green.

Bilingual localization checks — do-not-translate and placeholder integrity, which
compare a translated target against its source — are an opt-in: pass
`kapi check src.json --target src.de.json --target-lang de`.

## The Report

Every run produces a `core/check.Report` (versioned `kapi.check/v1`): a summary
(counts + score), the gate (the thresholds and which tripped), and a list of
**diagnostics**. Each diagnostic carries a stable `rule` id, a `severity`, a
human `message`, an optional `suggestion`, and a `location` (the block, plus a
run-range when the checker pinpointed one). The stable rule id is the loop's
primary key: an assistant tracks it across iterations to confirm a fix and avoid
regressions. `--json` emits the Report verbatim; over MCP, the `check_file` and
`check_text` tools return the same Report — the verifier counterpart to the
`extract_content`/`apply_edits` editing moat — so an assistant can
**author → check → revise → re-check** without leaving the conversation.

## One model: findings

Every check emits the same structured **finding** (the `core/check.Finding`
type): a kind, a severity, the run-index range it points at, and an optional
suggested replacement. A check is a read-only [tool](/framework/tools) — it uses
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

**Generic content checks** (source-side, no translation needed — the default
checkset):

- **Text hygiene** — empty content, doubled spaces and words, stray leading/
  trailing whitespace, control characters. Always on.
- **Length** — flag content over a character or word budget (`--max-chars`/
  `--max-words`).
- **Patterns** — regex that must not appear (`--forbid`) or must appear
  (`--require`) in the content.
- **Brand vocabulary** — forbidden/competitor/preferred-term rules from a bound
  [brand voice](/framework/checks/brand-voice) profile; plus an optional
  LLM-judged style/voice check.

**Bilingual localization checks** (opt-in, with `--target` — a translated target
against its source):

- **Placeholder integrity** — catch a dropped `{count}` or a corrupted `<b>` in
  the translation.
- **Do-not-translate** — terms that must survive verbatim into the target.
- **Terminology enforcement** — verifies the right term was used, drawing on the
  project [termbase](/framework/terminology).

The full QA family (whitespace, inline-code integrity, cross-block consistency,
optional LLM review) is documented under [QA Checks](/framework/checks/qa-checks).

> **Document structure & encoding validity** is a format-reader concern, not a
> content check — the readers extract leniently by default. Surface it on demand
> with `kapi check --validate report` (or `strict` to gate on it): the reader
> emits located `structure.*` / `encoding.*` findings (malformed XML/YAML,
> invalid UTF-8, charset mismatch, and the JSON faults the parser rejects) into
> the same Report. Coverage tracks each reader's own strictness.

## Composing and gating

Checks are tools, so they compose in a [flow](/framework/flows) exactly like
translation or transform stages — typically as the trailing stage after
translation. In CI, gate on the exit code; in an editor or assistant, surface
the findings for one-click fixes. A check never blocks the pipeline by mutating
content; it annotates, and the gate decides.

For a worked example of gating a pull request on a project's bound checks with
GitHub Actions, see [Gate localization in CI](/kapi/recipes/gate-localization-in-ci).
