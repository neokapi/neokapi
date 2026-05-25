package mdx_test

// This file documents why the neokapi native mdx package has NO direct Okapi
// (Java) filter counterpart, mirroring the standalone-documentation form of
// messageformat/okapi_skip_test.go that scripts/contract-audit scans.
//
// Okapi has a Markdown filter (`okf_markdown`), and neokapi's `markdown`
// package is the native counterpart that carries the CommonMark/GFM parity
// mapping against it (see core/formats/markdown okapi mappings). MDX, however,
// is NOT plain Markdown: it is CommonMark extended with ESM (`import`/`export`),
// JSX elements/fragments, and `{expression}` braces. Okapi has no MDX filter,
// and these MDX-only constructs have no Okapi analogue at all.
//
// The native mdx package PRE-SEGMENTS the document body into opaque MDX regions
// (ESM / JSX / expressions / GFM tables, preserved byte-for-byte and never
// translated in v1) and plain-Markdown spans, then delegates each Markdown span
// to the proven `markdown` reader/writer machinery with a self-verifying opaque
// fallback (a span that does not reconstruct exactly is preserved verbatim
// rather than corrupted). So:
//
//   - The CommonMark/GFM prose contract (okf_markdown / Okapi Markdown tests)
//     is owned and parity-mapped by the neokapi native `markdown` package, NOT
//     by this package. Re-pointing those Java tests here would double-count the
//     same delegated behavior.
//   - The MDX-specific behavior (ESM/JSX/expression segmentation, opaque
//     preservation of non-Markdown constructs, the byte-faithful fallback) has
//     no Okapi counterpart, so there is nothing to map or skip from Okapi.
//
// This package is therefore intentionally NOT parity-mapped: it carries no
// `// okapi:` mappings and no `// okapi-skip:` markers, because there is no
// Okapi MDX filter or test class to map to or skip. Markdown parity is covered
// separately by the `markdown` package; this package's contract is the MDX
// segmentation + opaque-preservation behavior documented and tested here.
