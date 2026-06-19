# Format Ops — Overview

The one-page front door to how neokapi adopts and matures document formats.
This is the *map*; the detailed docs are linked at the bottom.

neokapi's edge is faithful, deep processing of many native formats. The
**format-ops framework** is how we keep that edge honest as formats and models
change: it **measures** how good our support for each format is, **promises** a
support level users can rely on, and **drives** the recurring work — with AI — to
improve it.

## Two things, kept separate

| | **The promise** (a tier) | **The score** (a vector) |
|---|---|---|
| What | What users may rely on | How good our support actually is |
| Values | **Supported** · **Maintained** · **Available** | seven axes, each a small ladder |
| Changes | only by a human-approved event | recomputed every audit, automatically |
| Backed by | a CI gate (a tier with no gate is marketing) | deterministic file evidence |

The headline tier is the **minimum over the gating axes** — never an average. A
format can score high on a non-gating axis and still be honestly "Maintained."

## The seven axes, in three families

Each axis is a ladder (L0–L4, V0–V3, …). They group by the question they answer:

**Comprehension — how deeply we read it**
| Axis | | Measures | The ladder, roughly |
|---|---|---|---|
| Engine | L0–L4 | parse / round-trip / parity fidelity | reads → round-trips → spec'd → parity-verified → rock-solid |
| Vocabulary | V0–V3 | inline meaning (bold, links, placeholders) survives into the canonical model | opaque → typed reading → bidirectional → loss-proven |
| Structure & Geometry | G0–G4 | how much document structure & layout we recover | opaque → metadata → text → roles/tables/reading-order → +geometry/bboxes |

**Assurance — how we prove it**
| Axis | | Measures |
|---|---|---|
| Corpus | C0–C3 | real & wild reference files, with provenance |
| Security | S0–S4 | bounded / fuzzed / hostile-hardened parsing |

**Enablement — how we work with it**
| Axis | | Measures |
|---|---|---|
| Knowledge | K0–K3 | the spec & learning assets to work on it (human or AI) |
| Editor | E0–E4 | how close kapi gets to the format's native editor |

> The families are a reading aid. The gating set (Engine ∧ Corpus ∧ Knowledge)
> deliberately spans all three; Security and Structure & Geometry are non-gating
> display axes for now.

The **Structure & Geometry** axis is the one that captures depth-of-understanding
the way you'd expect of an image format: extracting only *metadata* (G1) is
shallower than *OCR text* (G2), which is shallower than recognizing *headings,
tables and reading order* (G3), which is shallower than recovering *page geometry
and bounding boxes* (G4).

## How a score is trustworthy

Scores are **computed, not opinionated**. A deterministic floor (`audit-format.py`
greps each format's files) pins each axis level; a model may only *demote* a few
quality dimensions, and only with a cited file/test as evidence. A reproducibility
check proves the floor alone fixes the level (no model swing). So re-runs — and
new models — produce the same answer. The live results are the
[`/format-maturity` dashboard](https://neokapi.org/format-maturity).

## How it runs — the runbook

The maintainer's whole job is to point Claude at the runbook on a loose cadence:

```
"run the format-ops runbook"      → .skills/format-ops/
node .skills/format-ops/scripts/due.mjs   # zero-cost: what's due, no run
```

The runbook reads a committed **ledger** + live repo signals, computes what's
**due** since last time, ranks it, does the due work with executable evidence,
and records the run. The rituals: score the fleet, remediate top gaps, watch
upstream specs & Okapi, sweep the corpus, scan for new formats, and — on a
calendar or when a new model lands — recalibrate its own prompts against a
human-graded golden set. Promotions, demotions, and new-format decisions wait in
an approval queue for the maintainer; everything else is autonomous.

## Adding a new format

```
radar candidate → adoption-evidence bar → human accept
  → implement-format skill (build it: reader/writer + spec + dossier/vocab/corpus/structure.yaml)
  → triage-score discovers & scores it automatically (no list to edit)
  → tier-review promotes Available → Maintained → Supported
```

## Where the detail lives

| Doc | What |
|---|---|
| [format-maturity.md](./format-maturity.md) | the bar: tiers + all seven axes + the rubric |
| [format-ops.md](./format-ops.md) | the process: rituals, cadences, ledger, runbook, self-improvement |
| [format-spec-cases.md](./format-spec-cases.md) | executable spec cases + AI test generation |
| [format-engineering.md](./format-engineering.md) | how the format engine itself works |
| `format-radar.yaml` · `format-ops-ledger.json` · `research/format-ops/` | the radar, the run ledger, and the design rationale |
