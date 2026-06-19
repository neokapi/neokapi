package asciidoc_test

// This file documents why the neokapi native AsciiDoc reader/writer has NO
// Okapi-bridge counterpart filter, so the test-comparison / parity harnesses
// (scripts/contract-audit) have no upstream Java filter test class to map
// against. AsciiDoc is therefore a HARVEST-cohort format (no spec parity,
// native-only): the obligations it carries instead are an invariants test, a
// corpus test, and a NATIVE spec.yaml — see invariants_test.go, corpus_test.go,
// and spec.yaml / spec_test.go.
//
// The Okapi Framework ships no AsciiDoc filter. Its closest lightweight-markup
// filters are okf_markdown (CommonMark via flexmark) and okf_wiki (MediaWiki /
// DokuWiki); there is no okf_asciidoc, no AsciiDoc Java filter test class, and
// no /asciidoc corpus in the Okapi sources. The native neokapi reader/writer is
// authored directly against the AsciiDoc language definition, not ported from
// Okapi.
//
// Ground truth for the format is the Eclipse AsciiDoc Language working group's
// specification effort (https://gitlab.eclipse.org/eclipse/asciidoc-lang/) and
// the Asciidoctor reference documentation it formalizes
// (https://docs.asciidoctor.org/asciidoc/latest/). The spec wins ties; where a
// construct sits outside the faithfully-implemented core set it is round-tripped
// as non-translatable skeleton (the documented coverage boundary), never
// silently dropped.
//
// Consequently this package carries NO `// okapi:` mapping markers and NO
// `// okapi-skip: Class#method` markers: there is simply no Okapi test surface
// to map or to declare not-applicable. The contract-audit scanner matches only
// marker lines containing a `Class#method` payload, so this prose-only file is
// inert documentation and never references a (non-existent) Java test, keeping
// the audit drift-free.
