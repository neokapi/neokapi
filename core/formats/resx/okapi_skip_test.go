package resx_test

// This file documents why the neokapi native resx (.NET RESX / .resw)
// reader/writer is intentionally NOT given a dedicated row in the
// test-comparison / parity dashboard, even though Okapi DOES handle RESX.
//
// Okapi has no bespoke RESX filter class. Instead it processes RESX via its
// generic XML filter (okf_xml) driven by a RESX-specific configuration
// (okf_xml-resx), i.e. an ITSEngine/parameters file rather than a Java filter
// implementation with its own filter test class. In neokapi, that generic-XML
// path is exercised through the native xml package's parity mapping, which is
// where the okf_xml + okf_xml-resx contract is verified against the bridge.
//
// This dedicated native resx package exists for byte-faithful, RESX-aware
// round-tripping (a lossless tokenizer plus splice-only value rewriting and
// .NET composite-format placeholder protection), but it is deliberately NOT
// separately parity-mapped: doing so would create a duplicate dashboard row for
// a contract already covered via the xml package's okf_xml / okf_xml-resx path.
//
// Consequently this package carries NO `// okapi:` mapping markers and NO
// `// okapi-skip: Class#method` markers. The contract-audit scanner matches
// only marker lines containing a `Class#method` payload, so this prose-only
// file is inert documentation that adds no audit row and references no Java
// test, keeping the audit drift-free.
//
// The native reader/writer's behavior is fully covered by this package's own
// unit and round-trip tests (roundtrip_test.go, malformed_test.go):
// byte-faithful read→write for both .resx and .resw, string-vs-typed/binary
// data discrimination, comment notes, entity decoding, composite-format
// placeholder protection, translation updates with XML escaping, and
// write-from-scratch.
