package applestrings_test

// This file documents why the neokapi native Apple Strings reader/writer
// (legacy .strings key/value tables + .stringsdict plist plural dictionaries)
// is deliberately NOT mapped head-to-head against an Okapi (Java) filter in the
// contract-audit dashboard.
//
// The Okapi Framework's pinned okapi-bridge build ships no Apple-strings filter:
// there is no okf_macstrings, no okf_applestrings, and no .stringsdict filter.
// (Okapi's NeXTSTEP/"old plist" handling is not exposed as a bridged filter
// here.) With no counterpart filter in the bridge, there is no Java test suite
// to map this native package's behavior against — a head-to-head parity row has
// no other side.
//
// The native package's behavior — C-style escape decoding (\" \n \t \Uxxxx),
// %@/%lld/%1$@ printf placeholder protection, the .stringsdict
// NSStringLocalizedFormatKey + per-variable CLDR plural-category modeling,
// UTF-16↔UTF-8 transcoding with BOM preservation, and byte-faithful rewrite — is
// verified by the package's own round-trip and extraction tests
// (roundtrip_test.go, harvest_test.go) instead. This format is therefore
// intentionally not parity-mapped.
//
// The marker below is standalone documentation scanned by
// scripts/contract-audit; it intentionally has no Go test body.
//
// okapi-skip: AppleStringsFilter#noBridgeCounterpart — no okf_macstrings/okf_applestrings/.stringsdict filter in the okapi-bridge build, so there is no Okapi counterpart to parity-map against; native behavior is covered by this package's own round-trip tests
