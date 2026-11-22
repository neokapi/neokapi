package main

// TestFile describes a single fixture to benchmark.
type TestFile struct {
	Name       string // basename
	Format     string // format id (e.g. "xliff", "openxml", "html")
	Category   string // "large", "medium", "small", "tiny" (size bucket)
	SourcePath string // absolute path on disk (DiscoverFixtures populates this)

	// SizeBytes is the source file size, used for size-bucketing in
	// reports and for evenly-spread sampling in Sample().
	SizeBytes int64

	// FilterClass is the Okapi filter class (e.g. "okf_html") used by
	// the OkapiPseudoEngine to invoke `kapi-okapi-bridge pseudo`.
	FilterClass string

	// OkapiFprm is the optional raw .fprm content forwarded to the
	// pseudo CLI as `--fprm <content>`. Used for filters that need
	// per-fixture parameter overrides (VTT/TTML caption merging).
	OkapiFprm string
}
