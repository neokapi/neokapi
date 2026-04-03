package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "build-versions":
		cmdBuildVersions(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `PseudoBench — neokapi performance benchmarks

Usage:
  pseudobench run [flags]         Run benchmarks
  pseudobench build-versions      Build kapi from git tags

Flags for 'run':
  -kapi string          Path to kapi binary
  -okapi string         Path to tikal binary
  -bridge-jar string    Path to bridge JAR (enables daemon mode)
  -iterations int       Measurement iterations (default 5)
  -warmup int           Warmup iterations (default 1)
  -testdata string      Test data directory (default "testdata")
  -okapi-testdata string Path to okapi-testdata root (copies files to testdata)
  -output string        Output directory for preserved files (default "output")
  -results string       Results directory for JSON/HTML (default "results")
  -html string          HTML report path (default "results/pseudobench.html")

Flags for 'build-versions':
  -versions string  Comma-separated git tags (e.g. v0.1.0,v0.2.0)
  -repo string      Git repo root (default "../../")
  -output string    Output directory for binaries (default "versions")
`)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	kapiBin := fs.String("kapi", "", "path to kapi binary")
	okapiBin := fs.String("okapi", "", "path to tikal binary")
	bridgeJar := fs.String("bridge-jar", "", "path to bridge JAR (enables daemon mode)")
	iterations := fs.Int("iterations", 5, "measurement iterations")
	warmup := fs.Int("warmup", 1, "warmup iterations")
	testdata := fs.String("testdata", "testdata", "test data directory")
	okapiTestdata := fs.String("okapi-testdata", "", "path to okapi-testdata root")
	output := fs.String("output", "output", "output directory for preserved files")
	results := fs.String("results", "results", "results directory")
	htmlFile := fs.String("html", "results/pseudobench.html", "HTML report path")
	traceDir := fs.String("trace-dir", "", "directory for batch trace JSON files (enables internal tracing)")
	fs.Parse(args)

	if *kapiBin == "" && *okapiBin == "" {
		fmt.Fprintf(os.Stderr, "Error: specify at least -kapi or -okapi\n")
		os.Exit(1)
	}

	cfg := &Config{
		KapiBin:       *kapiBin,
		OkapiBin:      *okapiBin,
		BridgeJar:     *bridgeJar,
		Iterations:    *iterations,
		Warmup:        *warmup,
		TestdataDir:   *testdata,
		OkapiTestdata: *okapiTestdata,
		OutputDir:     *output,
		ResultsDir:    *results,
		HTMLFile:      *htmlFile,
		TraceDir:      *traceDir,
	}

	// Copy test data from okapi-testdata if specified.
	if cfg.OkapiTestdata != "" {
		fmt.Println("Copying test data from okapi-testdata...")
		if err := copyTestData(cfg.OkapiTestdata, cfg.TestdataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error copying test data: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Copied %d files to %s\n\n", len(testFiles), cfg.TestdataDir)
	}

	// Verify testdata exists.
	if _, err := os.Stat(cfg.TestdataDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: testdata directory %s not found. Use -okapi-testdata to copy files.\n", cfg.TestdataDir)
		os.Exit(1)
	}

	fmt.Println("Running PseudoBench (mixed experiment)")
	fmt.Printf("  Files:      %d\n", len(testFiles))
	fmt.Printf("  Iterations: %d (warmup: %d)\n", cfg.Iterations, cfg.Warmup)
	if cfg.KapiBin != "" {
		fmt.Printf("  kapi:       %s\n", cfg.KapiBin)
	}
	if cfg.OkapiBin != "" {
		fmt.Printf("  okapi:      %s\n", cfg.OkapiBin)
	}
	if cfg.BridgeJar != "" {
		fmt.Printf("  bridge-jar: %s\n", cfg.BridgeJar)
	}

	report, err := runBenchmarks(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Write JSON report.
	if err := os.MkdirAll(cfg.ResultsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating results dir: %v\n", err)
		os.Exit(1)
	}

	jsonPath := filepath.Join(cfg.ResultsDir, "pseudobench.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling report: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nJSON results: %s\n", jsonPath)

	// Write HTML report.
	if err := generateHTMLReport(report, cfg.HTMLFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing HTML report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("HTML report:  %s\n", cfg.HTMLFile)

	printSummary(report)
}

func cmdBuildVersions(args []string) {
	fs := flag.NewFlagSet("build-versions", flag.ExitOnError)
	versions := fs.String("versions", "", "comma-separated git tags")
	repo := fs.String("repo", "../../", "git repo root")
	output := fs.String("output", "versions", "output directory")
	fs.Parse(args)

	if *versions == "" {
		fmt.Fprintf(os.Stderr, "Error: -versions is required\n")
		os.Exit(1)
	}

	tags := strings.Split(*versions, ",")
	if err := buildVersions(*repo, *output, tags); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
