package applestrings_test

// The native Apple Strings reader and writer (.strings tables and .stringsdict
// plural dictionaries) are not mapped against an Okapi filter in the
// contract-audit dashboard. The okapi-bridge build has no Apple Strings
// filter, so there is no Okapi test to map them to. This package's own
// round-trip and extraction tests (roundtrip_test.go, harvest_test.go) cover
// the format.
//
// The file names no Okapi test: an `okapi-skip:` marker must name a real one,
// and scripts/contract-audit fails on one that does not exist.
