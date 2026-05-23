package designtokens_test

// This file documents why the neokapi native designtokens package has NO Okapi
// (Java) filter counterpart, mirroring the standalone-documentation form of
// messageformat/okapi_skip_test.go that scripts/contract-audit scans.
//
// The Okapi Framework has no design-tokens filter. The W3C Design Tokens
// Community Group (DTCG) interchange format is a comparatively recent web
// standard (first stable revision "2025.10") with no representation in Okapi's
// filter set. There is therefore no `okf_designtokens` filter and no Okapi test
// class to map to or skip.
//
// DTCG files are ordinary JSON, and the native package is deliberately a thin,
// opinionated LAYER over core/formats/json: the inner JSON reader does all the
// tokenizing, key-path naming, and byte-faithful round-trip bookkeeping; this
// package only configures that reader with the design-tokens preset (extract
// ONLY $description values, since every $value is a non-linguistic colour /
// dimension / alias) and relabels the root layer's format. It adds no new
// parsing or serialization behavior of its own.
//
// Consequently:
//
//   - The Okapi JSON contract (okf_json / JSONFilterTest) is verified by the
//     neokapi native `json` package, NOT by this package. Re-pointing those
//     Java tests here would double-count the same delegated behavior.
//   - The DTCG-specific behavior (extract only $description; treat $value,
//     $type, $extensions, $deprecated as non-translatable; DTCG content sniff)
//     has no Okapi analogue, so there is nothing to map or skip from Okapi.
//
// This package is therefore intentionally NOT parity-mapped: it carries no
// `// okapi:` mappings and no `// okapi-skip:` markers, because there is no
// Okapi design-tokens filter or test class to map to or skip. The format's
// contract is the DTCG localization scoping documented and tested here; the
// underlying JSON contract is owned by the `json` package.
