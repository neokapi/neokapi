//go:build parity

package roundtrip_test

// Per-fixture skip directives previously lived here as Go maps. They
// have been migrated to per-format YAML at
// core/formats/<format>/parity-annotations.yaml so the same source of
// truth feeds both the parity harness (via roundtrip.LookupSkip) and
// the /parity/fixtures dashboard.
