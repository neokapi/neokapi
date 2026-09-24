# Format Ops: Overview

This overview explains how neokapi adopts formats, assesses their support and
schedules maintenance. The detailed runbooks are linked below.

Format operations combines measured capabilities with declared support tiers.
Audits identify gaps, and recurring maintenance tasks improve implementations
as formats and models change.

## Support tiers and measured scores

| | **Support commitment** (a tier) | **The score** (a vector) |
|---|---|---|
| What | What users may rely on | Measured capabilities |
| Values | **Supported** · **Maintained** · **Available** | one small ladder per axis |
| Changes | only by a human-approved event | recomputed every audit, automatically |
| Backed by | a CI gate | deterministic file evidence |

The headline tier is the **minimum over the gating axes**, never an average. A
format can score high on a non-gating axis and still qualify only as "Maintained."

## The axes, in three families

Each axis is a ladder (L0–L4, V0–V3, …). They group by the question they answer:

**Comprehension: how deeply we read it**
| Axis | | Measures | The ladder, roughly |
|---|---|---|---|
| Engine | L0–L4 | parse / round-trip / parity fidelity | reads → round-trips → spec'd → parity-verified → highest verified level |
| Vocabulary | V0–V3 | inline meaning (bold, links, placeholders) survives into the canonical model | opaque → typed reading → bidirectional → loss-proven |
| Structure & Geometry | G0–G4 | how much document structure & layout we recover | opaque → metadata → text → roles/tables/reading-order → +geometry/bboxes |
| Prose | P0–P4 | how much of the comment layer kapi can locate, check and rewrite, per format and per source language | none → located → governed → editable → complete |

**Assurance: how we prove it**
| Axis | | Measures |
|---|---|---|
| Corpus | C0–C3 | real & wild reference files, with provenance |
| Security | S0–S4 | bounded / fuzzed / hostile-hardened parsing |

**Enablement: how we work with it**
| Axis | | Measures |
|---|---|---|
| Knowledge | K0–K3 | the spec & learning assets to work on it (human or AI) |
| Editor | E0–E4 | how close kapi gets to the format's native editor |

> The families are a reading aid. The gating set (Engine ∧ Corpus ∧ Knowledge)
> deliberately spans all three; Security, Structure & Geometry and Prose are
> non-gating display axes.

The **Structure & Geometry** axis is the one that captures depth-of-understanding
the way you'd expect of an image format: extracting only *metadata* (G1) is
shallower than *OCR text* (G2), which is shallower than recognizing *headings,
tables and reading order* (G3), which is shallower than recovering *page geometry
and bounding boxes* (G4).

## How a score is trustworthy

The deterministic audit (`audit-format.py`) derives a capability floor from
format files. A model may demote selected quality dimensions only with cited
file or test evidence. A reproducibility check detects score variation.
See the live [`/format-maturity` dashboard](https://neokapi.github.io/format-maturity).

## How it runs: the runbook

Ask the assistant to run the format-ops skill at `.skills/format-ops/`.
To inspect pending work without executing it:

```bash
node .skills/format-ops/scripts/due.mjs
```

The runbook reads a committed **ledger** + live repo signals, computes what's
**due** since last time, ranks it, does the due work with executable evidence,
and records the run. The rituals: score the fleet, remediate top gaps, watch
upstream specs & Okapi, sweep the corpus, scan for new formats, and, on a
calendar or when a new model lands, recalibrate its own prompts against a
human-graded golden set. Promotions, demotions, and new-format decisions wait in
an approval queue for the maintainer; everything else is autonomous.

## Adding a new format

1. Review the candidate against the adoption evidence requirements and obtain
   maintainer approval.
2. Run the implement-format skill to build the reader, writer and supporting
   specifications, documentation and fixtures.
3. Run triage-score to discover and assess the format.
4. Use tier-review to propose promotion from Available to Maintained or Supported.

## Where the detail lives

| Doc | What |
|---|---|
| [format-maturity.md](./format-maturity.md) | the bar: tiers + all seven axes + the rubric |
| [format-ops.md](./format-ops.md) | the process: rituals, cadences, ledger, runbook, self-improvement |
| [format-spec-cases.md](./format-spec-cases.md) | executable spec cases + AI test generation |
| [format-engineering.md](./format-engineering.md) | how the format engine itself works |
| `format-radar.yaml` · `format-ops-ledger.json` · `research/format-ops/` | the radar, the run ledger, and the design rationale |
