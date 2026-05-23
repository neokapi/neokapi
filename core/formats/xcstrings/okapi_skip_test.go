package xcstrings_test

// This file documents why the neokapi native xcstrings (Apple String Catalog)
// reader/writer has NO Okapi-bridge counterpart filter, so the
// test-comparison / parity harnesses (scripts/contract-audit) have no upstream
// Java filter test class to map against.
//
// Apple introduced the String Catalog (.xcstrings) format with Xcode 15 in
// 2023 as a single JSON catalog unifying .strings/.stringsdict plus
// device/plural/substitution variations. The Okapi Framework does not ship a
// filter for it — there is no okf_* filter, no Java filter test class, and no
// /xcstrings corpus in the Okapi sources. The native neokapi reader/writer was
// authored directly against Apple's published format, not ported from Okapi.
//
// Consequently this package carries NO `// okapi:` mapping markers and NO
// `// okapi-skip: Class#method` markers: there is simply no Okapi test surface
// to map or to declare not-applicable. The contract-audit scanner matches only
// marker lines containing a `Class#method` payload, so this prose-only file is
// inert documentation and never references a (non-existent) Java test, keeping
// the audit drift-free.
//
// The format's behavior is fully covered by this package's own unit and
// round-trip tests (roundtrip_test.go, malformed_test.go): byte-faithful
// read→write, plural/device/substitution variation modeling, placeholder
// protection, state/extraction handling, translation updates, and
// write-from-scratch.
