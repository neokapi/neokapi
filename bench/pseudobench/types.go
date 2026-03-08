package main

import "time"

// Config holds the benchmark configuration.
type Config struct {
	KapiBins    []VersionedBinary // kapi binaries to benchmark (native formats)
	OkapiBins   []VersionedBinary // okapi tikal paths to benchmark
	Formats     []string          // formats to test (e.g. json, html, xliff, openxml)
	Sizes       []string          // size tiers (small, medium, large)
	Categories  []string          // benchmark categories ("single", "collection")
	Iterations  int               // number of iterations per benchmark
	Warmup      int               // warmup iterations (discarded)
	TestdataDir string            // directory containing generated test data
	OutputFile  string            // path to write JSON results
	Bridge      bool              // also benchmark kapi with okf_* bridge formats
}

// VersionedBinary is a path to a binary with a version label.
type VersionedBinary struct {
	Path    string
	Version string
}

// Report is the top-level benchmark report.
type Report struct {
	Metadata   Metadata          `json:"metadata"`
	Benchmarks []BenchmarkResult `json:"benchmarks"`
}

// Metadata describes the benchmark environment.
type Metadata struct {
	Timestamp string  `json:"timestamp"`
	Platform  string  `json:"platform"`
	GoVersion string  `json:"goVersion"`
	CPUModel  string  `json:"cpuModel"`
	CPUCores  int     `json:"cpuCores"`
	MemoryGB  float64 `json:"memoryGB"`
}

// BenchmarkResult holds results for a single benchmark.
type BenchmarkResult struct {
	Category        string `json:"category"`        // "single" or "collection"
	Engine          string `json:"engine"`           // "kapi-native", "kapi-bridge", "okapi"
	Format          string `json:"format"`           // format name (single) or "mixed" (collection)
	FileSize        string `json:"fileSize"`         // size tier: "small", "medium", "large"
	FileSizeBytes   int64  `json:"fileSizeBytes"`    // actual input bytes (single file)
	FileCount       int    `json:"fileCount"`        // number of files (collection only)
	TotalInputBytes int64  `json:"totalInputBytes"`  // total input bytes across all files
	UnitCount       int    `json:"unitCount"`        // translatable unit count
	Version         string `json:"version"`          // engine version
	Iterations      int    `json:"iterations"`       // measurement iterations
	Metrics         Metrics `json:"metrics"`
}

// Metrics holds all collected measurement statistics.
type Metrics struct {
	WallTimeMs  Stats `json:"wallTimeMs"`
	UserCpuMs   Stats `json:"userCpuMs"`
	SysCpuMs    Stats `json:"sysCpuMs"`
	PeakRssKB   Stats `json:"peakRssKB"`
	OutputBytes Stats `json:"outputBytes"`
}

// Stats holds descriptive statistics for a series of measurements.
type Stats struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P95    float64 `json:"p95"`
	Stddev float64 `json:"stddev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

// RunResult holds raw measurements from a single run.
type RunResult struct {
	WallTime    time.Duration
	UserCPU     time.Duration
	SystemCPU   time.Duration
	PeakRSSKB   int64
	OutputBytes int64
}

// CollectionFile describes one file in a collection test set.
type CollectionFile struct {
	Format string
	Path   string
	Units  int
}
