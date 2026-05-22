# formats-native-vs-okapi

Demonstrates that kapi reads and round-trips the **same asset** two ways — through
its built-in **native** reader and through the **okapi-bridge** `okf_html` filter.
Same translatable content, your choice of engine.

```bash
KAPI=./bin/kapi ./run.sh
```

`run.sh` extracts the translatable word count with each reader and pseudo-
translates the asset (offline, no LLM), writing `parity.json` and two round-trip
outputs under `out/`.

## What this proves

kapi has native readers and writers for localization, document, data, subtitle,
and office formats, plus more through the okapi-bridge — selectable per file with
`--map`. Both readers extract and round-trip the same asset (`both_ok: true`).
They segment HTML
slightly differently — in this demo the okapi `okf_html` filter also localizes
the `<button>` and `<a>` text (18 words) that the native reader leaves alone
(14 words), which you can see in `out/page.native.html` vs `out/page.okapi.html`.
The authoritative, normalized head-to-head native↔okapi comparison across every
shared format is the **parity suite** (`cli/parity`, `make parity`), run
continuously in CI.

## Requirements

The okapi side needs the `okapi-bridge` plugin (`kapi plugin install okapi-bridge`)
and a JRE. When the bridge is absent, `run.sh` still produces the native side and
records the okapi `status`.
