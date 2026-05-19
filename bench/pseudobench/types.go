package main

import "time"

// Config holds the benchmark configuration.
type Config struct {
	KapiBin       string // path to kapi binary
	OkapiBridge   string // path to kapi-okapi-bridge binary (jpackage launcher)
	BridgeJar     string // path to bridge JAR for daemon mode
	Iterations    int    // measurement iterations per engine
	Warmup        int    // warmup iterations (discarded)
	OkapiTestdata string // path to okapi-testdata root (DiscoverFixtures input)
	OutputDir     string // directory for preserved output files
	ResultsDir    string // directory for JSON + HTML results
	HTMLFile      string // path to HTML report
	TraceDir      string // directory for trace JSON files (empty = disabled)

	// Fixtures is the discovered + sampled set the bench runs. Populated
	// in main() by DiscoverFixtures() + Sample(); SourcePath on each
	// entry is an absolute path so the engines don't need a staging dir.
	Fixtures []TestFile

	// Sample is the fraction of discovered fixtures to bench (0,1].
	// Default 0.10 (10%); main sets it to 1.0 when -full is passed.
	Sample float64
}

// VersionedBinary is a path to a binary with a version label.
type VersionedBinary struct {
	Path    string
	Version string
}

// Report is the top-level benchmark report.
type Report struct {
	Metadata    Metadata           `json:"metadata"`
	Experiments []ExperimentResult `json:"experiments"`
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

// ExperimentResult holds results for a single engine across all files.
type ExperimentResult struct {
	Engine      string       `json:"engine"`
	Version     string       `json:"version"`
	Iterations  int          `json:"iterations"`
	WallTimeMs  Stats        `json:"wallTimeMs"`
	PeakRssKB   Stats        `json:"peakRssKB"`
	DaemonRssKB *Stats       `json:"daemonRssKB,omitempty"`
	FileResults []FileResult `json:"fileResults"`
	FileTimings []FileTiming `json:"fileTimings,omitempty"`
	BatchTrace  *BatchTrace  `json:"batchTrace,omitempty"`

	// Verification aggregates (populated from the last iteration's
	// FileResults at report-write time). Lets readers see at a glance
	// whether the engine's outputs actually contain pseudo-translated
	// content — a no-op engine that exits 0 quickly would look great
	// in WallTimeMs but show 0 verified files here.
	FilesAttempted   int `json:"filesAttempted,omitempty"`
	FilesSucceeded   int `json:"filesSucceeded,omitempty"`
	FilesVerified    int `json:"filesVerified,omitempty"`
	FilesUnverified  int `json:"filesUnverified,omitempty"`
	TotalPseudoChars int `json:"totalPseudoChars,omitempty"`
}

// BatchTrace mirrors flow.BatchFlowTrace for JSON parsing.
// Pseudobench is a standalone module and cannot import core/flow.
type BatchTrace struct {
	Name        string      `json:"name"`
	Concurrency int         `json:"concurrency"`
	FileTraces  []FileTrace `json:"fileTraces"`
	DurationUs  int64       `json:"durationUs"`
}

// FileTrace holds per-file timing from a batch flow trace.
type FileTrace struct {
	File       string `json:"file"`
	Format     string `json:"format"`
	StartUs    int64  `json:"startUs"`
	EndUs      int64  `json:"endUs"`
	Lane       int    `json:"lane"`
	DurationUs int64  `json:"durationUs"`
}

// FileResult tracks per-file success/failure within an experiment.
type FileResult struct {
	Name        string `json:"name"`
	Format      string `json:"format"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	PseudoChars int    `json:"pseudoChars,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
	VerifyNote  string `json:"verifyNote,omitempty"`
}

// FileTiming holds per-file timing from a sequential trace pass.
type FileTiming struct {
	Name       string  `json:"name"`
	Format     string  `json:"format"`
	Category   string  `json:"category"`
	SizeBytes  int64   `json:"sizeBytes"`
	StartMs    float64 `json:"startMs"`
	EndMs      float64 `json:"endMs"`
	WallMs     float64 `json:"wallMs"`
	PeakRssKB  int64   `json:"peakRssKB"`
	UserCpuMs  float64 `json:"userCpuMs"`
	SysCpuMs   float64 `json:"sysCpuMs"`
	BlockCount int64   `json:"blockCount,omitempty"`
	Success    bool    `json:"success"`
	Error      string  `json:"error,omitempty"`
}

// Stats holds descriptive statistics for a series of measurements.
type Stats struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P5     float64 `json:"p5"`
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
	BlockCount  int64
}
