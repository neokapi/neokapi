---
sidebar_position: 0
title: Checks
description: Checks are tests for AI output — read-only verifiers that inspect content against rules and report findings on a shared model, without modifying it. Brand voice, QA, terminology enforcement, and placeholder integrity are all one check family.
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
it reads content against its source, and it reports exactly what broke — a
dropped placeholder, a translated do-not-translate term, an off-voice phrase, an
inconsistent glossary term. `kapi check` runs the family over a file or a
source/target pair and **exits non-zero when the gate fails**, so a regression is
caught in CI — or inside an AI assistant's fix-loop — the same way a failing
test is. That is the role checks play for generated content: the assistant
drafts, the checks tell it what to fix, and the file ships only when the gate is
green.

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
`kapi check` exit code, the Kapi Desktop checks panel, and — on the Bowrain
platform — the [correction-learning
loop](https://neokapi.github.io/web/bowrain/) that turns repeated human
corrections into new checks.

## The check families

- **QA checks** — fast, deterministic rules over each block: whitespace,
  placeholder and inline-code integrity, length, pattern, and cross-block
  inconsistency, plus optional LLM-assisted review. See [QA
  Checks](/framework/checks/qa-checks).
- **Brand voice** — a machine-readable profile of tone, style, and vocabulary,
  run as one checkset and gateable by score. See [Brand
  Voice](/framework/checks/brand-voice).
- **Terminology enforcement** — verifies that the right term was used, drawing
  on the project [termbase](/framework/terminology). Terminology is also a
  translation *resource* (a glossary that feeds AI translation); term-enforce is
  the check side of it.
- **Placeholder and do-not-translate integrity** — part of the QA family: catch
  a dropped `{count}`, a corrupted `<b>`, or a translated DNT term.

## Composing and gating

Checks are tools, so they compose in a [flow](/framework/flows) exactly like
translation or transform stages — typically as the trailing stage after
translation. In CI, gate on the exit code; in an editor or assistant, surface
the findings for one-click fixes. A check never blocks the pipeline by mutating
content; it annotates, and the gate decides.
