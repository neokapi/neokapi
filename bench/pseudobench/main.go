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
	fmt.Fprintf(os.Stderr, `PseudoBench — neokapi pseudo-translate performance benchmarks

Usage:
  pseudobench run [flags]         Run benchmarks
  pseudobench build-versions      Build kapi from git tags

Flags for 'run':
  -kapi string                Path to kapi binary
  -okapi-bridge string        Path to kapi-okapi-bridge launcher (jpackage native binary)
  -bridge-jar string          Path to bridge JAR (enables daemon mode)
  -okapi-testdata string      Path to okapi-testdata root (REQUIRED for fixture discovery)
  -full                       Run on every discovered fixture (default samples ~10%%)
  -sample float               Override default sample fraction (0,1] (default 0.10)
  -iterations int             Measurement iterations (default 5)
  -warmup int                 Warmup iterations (default 1)
  -output string              Output directory for preserved files (default "output")
  -results string             Results directory for JSON/HTML (default "results")
  -html string                HTML report path (default "results/pseudobench.html")
  -trace-dir string           Directory for batch trace JSON files (enables internal tracing)

Flags for 'build-versions':
  -versions string  Comma-separated git tags (e.g. v0.1.0,v0.2.0)
  -repo string      Git repo root (default "../../")
  -output string    Output directory for binaries (default "versions")
`)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	kapiBin := fs.String("kapi", "", "path to kapi binary")
	okapiBridge := fs.String("okapi-bridge", "", "path to kapi-okapi-bridge launcher")
	bridgeJar := fs.String("bridge-jar", "", "path to bridge JAR (enables daemon mode)")
	okapiTestdata := fs.String("okapi-testdata", "", "path to okapi-testdata root")
	full := fs.Bool("full", false, "run all discovered fixtures (default samples ~10%)")
	sample := fs.Float64("sample", 0.10, "sample fraction in (0,1]")
	iterations := fs.Int("iterations", 5, "measurement iterations")
	warmup := fs.Int("warmup", 1, "warmup iterations")
	output := fs.String("output", "output", "output directory for preserved files")
	results := fs.String("results", "results", "results directory")
	htmlFile := fs.String("html", "results/pseudobench.html", "HTML report path")
	traceDir := fs.String("trace-dir", "", "directory for batch trace JSON files (enables internal tracing)")
	fs.Parse(args)

	if *kapiBin == "" && *okapiBridge == "" {
		fmt.Fprintf(os.Stderr, "Error: specify at least -kapi or -okapi-bridge\n")
		os.Exit(1)
	}
	if *okapiTestdata == "" {
		fmt.Fprintf(os.Stderr, "Error: -okapi-testdata is required (path to okapi-testdata root)\n")
		os.Exit(1)
	}

	fraction := *sample
	if *full {
		fraction = 1.0
	}
	if fraction <= 0 || fraction > 1 {
		fmt.Fprintf(os.Stderr, "Error: -sample must be in (0,1]\n")
		os.Exit(1)
	}

	all, err := DiscoverFixtures(*okapiTestdata)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering fixtures: %v\n", err)
		os.Exit(1)
	}
	if len(all) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no fixtures discovered under %s\n", *okapiTestdata)
		os.Exit(1)
	}
	fixtures := Sample(all, fraction)

	cfg := &Config{
		KapiBin:       *kapiBin,
		OkapiBridge:   *okapiBridge,
		BridgeJar:     *bridgeJar,
		Iterations:    *iterations,
		Warmup:        *warmup,
		OkapiTestdata: *okapiTestdata,
		OutputDir:     *output,
		ResultsDir:    *results,
		HTMLFile:      *htmlFile,
		TraceDir:      *traceDir,
		Fixtures:      fixtures,
		Sample:        fraction,
	}

	fmt.Println("Running PseudoBench")
	fmt.Printf("  Discovered: %d fixtures across %d formats\n", len(all), countFormats(all))
	fmt.Printf("  Sampled:    %d fixtures (%.0f%%)\n", len(fixtures), fraction*100)
	fmt.Printf("  Iterations: %d (warmup: %d)\n", cfg.Iterations, cfg.Warmup)
	if cfg.KapiBin != "" {
		fmt.Printf("  kapi:         %s\n", cfg.KapiBin)
	}
	if cfg.OkapiBridge != "" {
		fmt.Printf("  okapi-bridge: %s\n", cfg.OkapiBridge)
	}
	if cfg.BridgeJar != "" {
		fmt.Printf("  bridge-jar:   %s\n", cfg.BridgeJar)
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

	if err := generateHTMLReport(report, cfg.HTMLFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing HTML report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("HTML report:  %s\n", cfg.HTMLFile)

	printSummary(report)
}

func countFormats(fixtures []TestFile) int {
	seen := map[string]bool{}
	for _, f := range fixtures {
		seen[f.Format] = true
	}
	return len(seen)
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
