# Demo cell contract

A **demo cell** shows what AI output looks like **WITHOUT kapi** vs **WITH kapi**,
across many asset formats. Each cell is one self-contained directory owned by one
author, reproducible against the real `kapi` binary.

## The story every cell tells

> AI alone drifts off-brand, uses inconsistent terminology, and ships one language.
> AI **+ kapi** stays on-brand, keeps terminology consistent, and publishes in every
> language and format.

## Directory layout

```
web/demos/{demo-id}/          # demo-id = {asset|topic}-{stage}-{domain}
  demo.yaml                        # the cell contract (metadata + expectations)
  run.sh                           # regenerates without/ + with/ + scorecard.json
  fixtures/                        # real input assets (small, committed)
  without/   output.*  score.json  # the AI-alone baseline + its brand score
  with/      output.*  score.json  # the kapi-enforced result + its brand score
  scorecard.json                   # derived: {without, with, delta}
```

## `demo.yaml`

```yaml
id: brand-rewrite-marketing
kind: brand              # brand | formats | translate
asset_type: markdown
loop_stage: brand-rewrite
domain: marketing
brand_pack: marketing-blog
reader: native           # native | okapi  (which format reader the cell exercises)
signal: high
summary: One-line description of what the cell demonstrates.
expect:                  # the delta gate — a cell must show a real difference
  without_overall_max: 80
  with_overall_min: 95
  delta_min: 10
```

## Reproducibility

- `run.sh` regenerates everything from `fixtures/` using the real `kapi` binary.
- Committed cells are **deterministic**: rule-based brand checks/rewrites and
  pseudo-translation need no LLM and no network, so the gallery always builds and
  the delta is stable. Cells that opt into `--ai` document the credential they need.
- The numbers in `scorecard.json` come from real `kapi brand check --json` output —
  the same scorer the product ships.

## Native AND okapi

Format cells set `reader: native` or `reader: okapi`. The okapi cells exercise the
okapi-bridge filters (`--map '*.ext=okf_*'`) so the campaign demonstrates kapi's
format coverage both ways. `run.sh` skips okapi cells gracefully if the
okapi-bridge plugin is not installed.

## Parallelization

`registry.yaml` is the matrix/ledger (one row per cell: `status`, `owner`, `delta`).
Each agent claims a `planned` row, builds only its own `web/demos/{id}/`
directory, runs `run.sh`, confirms the `expect:` gate, and reports its delta. The
coordinator owns `registry.yaml` writes to avoid contention.
