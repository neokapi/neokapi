//go:build parity

package roundtrip_test

// Per-fixture skip directives live in per-format YAML at
// core/formats/<format>/parity-annotations.yaml, so one source of truth
// feeds both the parity harness (via roundtrip.LookupSkip) and the
// /parity/fixtures dashboard.
