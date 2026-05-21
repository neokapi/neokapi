package yaml_test

// ---- File-based roundtrip iteration tests ----
// RoundTripYamlIT iterates over all .yaml/.yml test resource files using
// EventComparator. The native equivalent is TestRoundTrip_YamlIT in
// skeleton_test.go (extract→write→re-extract text-unit stability over a
// representative corpus), which carries the RoundTripYamlIT#yamlFiles and
// YamlXliffCompareIT#yamlXliffCompareFiles contract annotations. The
// per-construct double-extraction tests in reader_test.go and the byte-exact
// skeleton roundtrip suite cover the same fidelity at finer granularity.
