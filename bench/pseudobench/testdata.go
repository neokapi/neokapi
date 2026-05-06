package main

import "os"

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

// totalInputSize sums SourcePath byte sizes for all fixtures. Used by
// the report header to print "throughput per MB". Falls back to 0
// silently for any fixture whose file vanished between discovery and
// the report write.
func totalInputSize(fixtures []TestFile) int64 {
	var total int64
	for _, f := range fixtures {
		if f.SourcePath == "" {
			continue
		}
		info, err := os.Stat(f.SourcePath)
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}
