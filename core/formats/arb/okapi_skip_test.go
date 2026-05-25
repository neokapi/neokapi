package arb_test

// This file documents why the neokapi native arb (Flutter Application Resource
// Bundle) reader/writer has NO Okapi-bridge counterpart filter, so the
// test-comparison / parity harnesses (scripts/contract-audit) have no upstream
// Java filter test class to map against.
//
// ARB (.arb) is the JSON localization format produced and consumed by Flutter's
// gen_l10n tooling: a flat object of message keys whose values are ICU
// MessageFormat strings, with sibling "@<id>" attribute objects (descriptions,
// placeholders) and "@@<global>" metadata such as "@@locale". The Okapi
// Framework does not ship a filter for it — there is no okf_* filter, no Java
// filter test class, and no /arb corpus in the Okapi sources. The native
// neokapi reader/writer was authored directly against the ARB spec, not ported
// from Okapi.
//
// Consequently this package carries NO `// okapi:` mapping markers and NO
// `// okapi-skip: Class#method` markers: there is simply no Okapi test surface
// to map or to declare not-applicable. The contract-audit scanner matches only
// marker lines containing a `Class#method` payload, so this prose-only file is
// inert documentation and never references a (non-existent) Java test, keeping
// the audit drift-free.
//
// (ICU MessageFormat itself does have a separate native neokapi package,
// messageformat, with its own okapi_skip_test.go documenting the upstream
// MessageFormat filter tests. The arb reader treats each top-level ICU
// construct as an opaque protected placeholder rather than re-implementing that
// filter; see icu.go.)
//
// The format's behavior is fully covered by this package's own unit and
// round-trip tests (roundtrip_test.go, malformed_test.go): byte-faithful
// read→write across pretty and compact layouts, ICU placeholder/plural/select
// protection, @-description notes, @@-global preservation, translation updates,
// and write-from-scratch.
