package androidxml_test

// This file documents why the neokapi native Android string-resources
// reader/writer is deliberately NOT mapped head-to-head against an Okapi (Java)
// filter in the contract-audit dashboard.
//
// The Okapi Framework has no Android-strings filter. The closest thing in the
// pinned okapi-bridge build is the GENERIC okf_xml filter (XMLFilter), driven by
// an external its/itsx parameters file. That generic filter knows nothing about
// the Android string-resources schema or its semantics:
//
//   - It has no notion of <plurals> CLDR quantity items or <string-array> item
//     indices — both are the native reader's first-class translatable units
//     (name[quantity], name[index]).
//   - It does not understand Android's backslash-escape layer (\' \" \n \t \@
//     \uXXXX) which the native reader keeps verbatim in the Block text so the
//     surface a translator edits matches the file, nor the printf %1$s/%% codes
//     the native reader lifts into inline placeholders.
//   - It has no built-in handling of <xliff:g> do-not-translate spans, CDATA
//     embedded HTML, or the translatable="false" / @string-reference skip rules
//     that are intrinsic to Android resources.
//
// Mapping the native package onto okf_xml would therefore measure two different
// contracts: a schema-aware Android reader versus a generic, externally
// configured XML walker. A head-to-head parity row would register every
// Android-specific behavior as a "divergence" that is not a real defect — a
// false negative on the dashboard. The native behavior is fully verified by the
// package's own round-trip and extraction tests (roundtrip_test.go,
// harvest_test.go) instead, so this format is intentionally not parity-mapped.
//
// The marker below is standalone documentation scanned by
// scripts/contract-audit; it intentionally has no Go test body.
//
// okapi-skip: XMLFilter#androidStrings — Okapi has only a generic okf_xml filter with no Android-strings semantics (plurals/arrays/escaping/xliff:g); a head-to-head row would be a false divergence, so this native format is intentionally not parity-mapped
