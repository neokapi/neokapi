# Vendored DocLang conformance assets

These files are **vendored unmodified from upstream** for neokapi's DocLang
conformance tests. They are not neokapi's own work.

- **Source:** https://github.com/doclang-project/doclang (commit `1395de6`)
- **License:** Apache-2.0 (© the DocLang authors / LF AI & Data) — the upstream
  license text is vendored verbatim as `UPSTREAM-LICENSE.txt` (Apache-2.0 §4(a)).
  Upstream ships no NOTICE file. All files here are byte-identical copies.
- **Pinned to:** DocLang spec v0.6 (the version neokapi targets)

## Contents

- `conformance/doclang.xsd` — the official DocLang XML Schema (v0.6.0).
  `conformance_test.go` validates our writer's output against it with `xmllint`.
- `corpus/*.dclg.xml` — a curated subset of upstream's `tests/data/valid/`
  fixtures, chosen to exercise constructs our reader supports (headings, lists,
  OTSL tables, code, geometry, layer/furniture, and one canonical real-Docling
  export). `corpus_test.go` asserts our reader ingests each into the expected
  roles. Fixtures dominated by not-yet-mapped constructs (span continuations,
  refs/threads, forms, nested pictures) were deliberately excluded.

To refresh: re-copy from the upstream repo at the same (or a newer, re-pinned)
commit and re-run the tests.
