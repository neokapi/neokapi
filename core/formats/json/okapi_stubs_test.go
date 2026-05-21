package json_test

// ---- Java integration roundtrip test ----
// RoundTripJsonIT iterates over all .json test resource files using
// EventComparator. The native equivalent is TestSnippets_DoubleExtraction in
// snippets_test.go (extract→write→re-extract text-unit stability), which
// carries the RoundTripJsonIT#jsonFiles and JsonXliffCompareIT#jsonXliffCompareFiles
// contract annotations. TestRoundTrip, TestRoundTripFileSimple, and the
// TestSnippets_ExactRoundtrip_* suite cover the same fidelity at finer
// granularity.
