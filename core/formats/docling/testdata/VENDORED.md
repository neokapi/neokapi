# Vendored DoclingDocument conformance assets

These files are **vendored unmodified from upstream** for neokapi's Docling
ingestion + cross-implementation parity tests. They are not neokapi's own work.

- **Source:** https://github.com/docling-project/docling-core (commit `e3360a0`)
- **License:** MIT (© 2024 International Business Machines) — the upstream
  license text is vendored verbatim as `UPSTREAM-LICENSE.txt`.

File contents are byte-identical to upstream `test/data/doc/`. One file is
renamed for the `*.json` glob: `corpus/page_without_pic.json` is upstream's
`page_without_pic.dt.json` (content unmodified). `parity/polymers.{json,gt.md}`
are upstream's `polymers.json` + `polymers.gt.md`.

## Contents

- `corpus/*.json` — real `DoclingDocument` JSON from upstream's
  `test/data/doc/`, chosen for size + construct coverage (picture+caption,
  page-header furniture, headings, lists, a table, key-value/form fields).
  `corpus_test.go` asserts our reader ingests each into the expected roles.
- `parity/polymers.json` + `parity/polymers.gt.md` — a `DoclingDocument` and
  **Docling's own `export_to_markdown` groundtruth** for it. `parity_test.go`
  renders the same document through neokapi's Markdown projection and compares
  the extracted content against Docling's, with a documented divergence ledger
  (Docling reserves H1 for the title, preserves source list markers, indents
  nested lists, and emits image/page-break comments; neokapi normalizes these).

All vendored fixtures here are free of embedded base64 image data (kept small).
To refresh: re-copy from the upstream repo and re-run the tests.
