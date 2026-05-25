package i18next_test

// This file documents why the neokapi native i18next package has NO direct
// Okapi (Java) filter counterpart, mirroring the standalone-documentation form
// of messageformat/okapi_skip_test.go that scripts/contract-audit scans.
//
// There is no `okf_i18next` filter in the Okapi Framework. i18next resource
// bundles are ordinary JSON, and Okapi localizes them with its generic JSON
// filter (`okf_json`) — which in neokapi maps to the `json` package, not this
// one. The native i18next package is deliberately a thin, opinionated LAYER
// over `core/formats/json`: the inner JSON reader does all the tokenizing,
// key-path naming, subfiltering, inline-code detection, and byte-faithful
// round-trip bookkeeping; this package only configures that reader with the
// i18next preset and post-processes the emitted block stream to attach i18next
// plural/context metadata (baseKey, pluralCategory, pluralGroup, context,
// legacy-form flags). It adds no new parsing or serialization behavior of its
// own.
//
// Consequently:
//
//   - The Okapi JSON contract (okf_json / JSONFilterTest) is verified by the
//     neokapi native `json` package (see core/formats/json okapi mappings),
//     NOT by this package. Re-pointing those Java tests here would double-count
//     the same delegated behavior.
//   - i18next-specific behavior (CLDR-style plural sibling keys, context keys,
//     {{interpolation}} / $t() nesting protection, the opt-in _html subfilter)
//     has no Okapi analogue at all: Okapi has no i18next-aware plural/context
//     grouping. These are neokapi extensions verified by this package's own
//     tests (i18next_test.go), so there is nothing to map or skip from Okapi.
//
// This package is therefore intentionally NOT parity-mapped: it carries no
// `// okapi:` mappings and no `// okapi-skip:` markers, because there is no
// Okapi i18next filter or test class to map to or skip. The format's contract
// is the i18next layering documented and tested here; the underlying JSON
// contract is owned by the `json` package.
