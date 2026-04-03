package yaml_test

// ---- File-based roundtrip iteration tests ----
// RoundTripYamlIT iterates over all .yaml/.yml test resource files using EventComparator.
// The native implementation covers roundtrip via TestExtract_RoundtripFiles,
// TestExtract_RoundTripSubFilterProcessLiteralAsBlock, individual double-extraction tests,
// and the skeleton_test.go byte-exact roundtrip suite.

// okapi-deferred: RoundTripYamlIT — iterates all okf_yaml/*.yaml files; native roundtrip covered by double-extraction tests and skeleton_test.go
// okapi-deferred: RoundTripYamlIT (yml extension) — iterates all okf_yaml/*.yml files; native roundtrip covered by double-extraction tests and skeleton_test.go
