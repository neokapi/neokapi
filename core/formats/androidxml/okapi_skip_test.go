package androidxml_test

// The native Android string-resources reader and writer are not mapped
// against an Okapi filter in the contract-audit dashboard. Okapi has no
// Android filter, only the generic XML filter, which knows nothing of
// <plurals>, <string-array>, Android escapes or <xliff:g> spans. A row against
// it would report each Android behaviour as a divergence. This package's own
// round-trip and extraction tests (roundtrip_test.go, harvest_test.go) cover
// the format.
//
// The file names no Okapi test: an `okapi-skip:` marker must name a real one,
// and scripts/contract-audit fails on one that does not exist.
