# kapi demo campaign — WITH vs WITHOUT

A growing gallery of small, reproducible demos that show what AI output looks
like **without kapi** vs **with kapi**, across many asset formats. Each demo is a
self-contained directory; `run.sh` regenerates its artifacts against the real
`kapi` binary.

- **The contract:** [CONTRACT.md](./CONTRACT.md) — what one demo cell is.
- **The ledger:** [registry.yaml](./registry.yaml) — the (asset × stage × domain)
  matrix and each cell's status + delta. Fan out one agent per `planned` row.

## Built cells

| Cell | Shows | Delta |
|---|---|---|
| [`brand-rewrite-marketing`](./brand-rewrite-marketing/) | A generic marketing draft scores **70/100** on voice compliance; `kapi voice rewrite` lifts it to **95/100**. | **+25** |

## Run a cell

```bash
make build                                   # build ./bin/kapi
KAPI=./bin/kapi web/demos/brand-rewrite-marketing/run.sh
```

Cells are **deterministic** (rule-based voice checks/rewrites, pseudo-translation)
so the gallery always builds and the numbers are stable — no LLM or network
required. Cells that opt into `--ai` document the credential they need.

## Readers

Format cells run against kapi's own readers and writers. A cell that needs a
format only a plugin provides declares that plugin in its `demo.yaml`; the
gallery never assumes a plugin is installed.
