# formats-native-vs-okapi

Demonstrates that kapi reads and round-trips the **same asset** two ways — through
its built-in **native** reader and through the **okapi-bridge** `okf_html` filter.
Same translatable content, your choice of engine.

```bash
KAPI=./bin/kapi ./run.sh
```

`run.sh` extracts the translatable word count with each reader and pseudo-
translates the asset (offline, no LLM), writing `parity.json`.

## What this proves

kapi is the localization Swiss-army-knife: 30+ native format readers/writers
**plus** 57+ okapi-bridge filters, selectable per file with `--map`. The
authoritative head-to-head native↔okapi comparison across every shared format is
the **parity suite** (`cli/parity`, `make parity`), run continuously in CI — see
the parity dashboard for per-format faithfulness.

## Environment note

`parity.json` records the okapi side's `status`. The okapi-bridge requires the
plugin to be installed and a working JRE; in environments where the bridge is not
healthy, the native numbers are still produced and the okapi side reports
`bridge unavailable`. The cell is correct and regenerates both sides on any host
with a healthy okapi-bridge.
