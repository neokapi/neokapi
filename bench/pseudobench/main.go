package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var defaultFormats = []string{
	"json", "html", "xml", "xliff", "properties", "po", "yaml", "plaintext",
	"docx", "pptx", "xlsx",
}
var defaultSizes = []string{"small", "medium", "large"}
var defaultCategories = []string{"single", "collection"}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		cmdGenerate(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, `PseudoBench — gokapi performance benchmarks

Usage:
  pseudobench generate [flags]    Generate test data files
  pseudobench run [flags]         Run benchmarks
  pseudobench build-versions      Build kapi from git tags

Flags for 'generate':
  -dir string       Output directory (default "testdata")
  -formats string   Comma-separated formats (default all incl. docx,pptx,xlsx)
  -sizes string     Comma-separated size tiers: small,medium,large

Flags for 'run':
  -kapi string        Path(s) to kapi binary (comma-sep, use name=path)
  -okapi string       Path(s) to tikal (comma-sep, use name=path)
  -bridge             Also benchmark kapi with Okapi bridge formats
  -formats string     Comma-separated formats
  -sizes string       Comma-separated size tiers: small,medium,large
  -categories string  Comma-separated: single,collection (default both)
  -iterations int     Iterations per benchmark (default 10)
  -warmup int         Warmup iterations (default 2)
  -testdata string    Test data directory (default "testdata")
  -output string      Output JSON file (default "results/pseudobench.json")

Flags for 'build-versions':
  -versions string  Comma-separated git tags (e.g. v0.1.0,v0.2.0)
  -repo string      Git repo root (default "../../")
  -output string    Output directory for binaries (default "versions")
`)
}

func cmdGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	dir := fs.String("dir", "testdata", "output directory")
	formats := fs.String("formats", strings.Join(defaultFormats, ","), "formats")
	sizes := fs.String("sizes", strings.Join(defaultSizes, ","), "sizes")
	fs.Parse(args)

	fmts := strings.Split(*formats, ",")
	szs := strings.Split(*sizes, ",")

	fmt.Println("Generating single-file test data...")
	if err := generateTestData(*dir, fmts, szs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nGenerating collection test data...")
	if err := generateCollections(*dir, szs); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating collections: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDone.")
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	kapiBins := fs.String("kapi", "", "kapi binary path(s), use version=path for labels")
	okapiBins := fs.String("okapi", "", "tikal path(s), use version=path for labels")
	bridge := fs.Bool("bridge", false, "also benchmark kapi with bridge formats")
	formats := fs.String("formats", strings.Join(defaultFormats, ","), "formats to benchmark")
	sizes := fs.String("sizes", strings.Join(defaultSizes, ","), "size tiers to benchmark")
	categories := fs.String("categories", strings.Join(defaultCategories, ","), "benchmark categories: single,collection")
	iterations := fs.Int("iterations", 10, "iterations per benchmark")
	warmup := fs.Int("warmup", 2, "warmup iterations")
	testdata := fs.String("testdata", "testdata", "test data directory")
	output := fs.String("output", "results/pseudobench.json", "output JSON file")
	fs.Parse(args)

	cfg := &Config{
		Formats:     strings.Split(*formats, ","),
		Sizes:       strings.Split(*sizes, ","),
		Categories:  strings.Split(*categories, ","),
		Iterations:  *iterations,
		Warmup:      *warmup,
		TestdataDir: *testdata,
		OutputFile:  *output,
		Bridge:      *bridge,
	}

	if *kapiBins != "" {
		cfg.KapiBins = parseVersionedBinaries(*kapiBins)
	}
	if *okapiBins != "" {
		cfg.OkapiBins = parseVersionedBinaries(*okapiBins)
	}

	if len(cfg.KapiBins) == 0 && len(cfg.OkapiBins) == 0 {
		fmt.Fprintf(os.Stderr, "Error: specify at least one engine with -kapi or -okapi\n")
		os.Exit(1)
	}

	fmt.Println("Running PseudoBench...")
	fmt.Printf("  Engines:    kapi=%d, okapi=%d, bridge=%v\n", len(cfg.KapiBins), len(cfg.OkapiBins), cfg.Bridge)
	fmt.Printf("  Categories: %s\n", strings.Join(cfg.Categories, ", "))
	fmt.Printf("  Formats:    %s\n", strings.Join(cfg.Formats, ", "))
	fmt.Printf("  Sizes:      %s\n", strings.Join(cfg.Sizes, ", "))
	fmt.Printf("  Iterations: %d (warmup: %d)\n", cfg.Iterations, cfg.Warmup)
	fmt.Println()

	report, err := runBenchmarks(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Write JSON report
	dir := filepath.Dir(cfg.OutputFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling report: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(cfg.OutputFile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResults written to %s\n", cfg.OutputFile)
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

// parseVersionedBinaries parses "v1=path1,v2=path2" or "path1,path2" format.
func parseVersionedBinaries(s string) []VersionedBinary {
	var bins []VersionedBinary
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.Index(part, "="); i > 0 {
			bins = append(bins, VersionedBinary{
				Version: part[:i],
				Path:    part[i+1:],
			})
		} else {
			// Auto-detect version by running binary
			version := detectVersion(part)
			bins = append(bins, VersionedBinary{
				Version: version,
				Path:    part,
			})
		}
	}
	return bins
}

// detectVersion tries to get the version string from a binary.
func detectVersion(binPath string) string {
	out, err := runCommand(binPath, "version", "--json")
	if err != nil {
		return filepath.Base(binPath)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.Version == "" {
		return filepath.Base(binPath)
	}
	return v.Version
}

func printSummary(report *Report) {
	fmt.Println("\n--- Summary ---")
	fmt.Printf("Platform: %s | CPU: %s (%d cores) | Memory: %.0fGB\n",
		report.Metadata.Platform, report.Metadata.CPUModel,
		report.Metadata.CPUCores, report.Metadata.MemoryGB)
	fmt.Printf("Total benchmarks: %d\n", len(report.Benchmarks))

	// Group by category then engine.
	type key struct{ cat, engine string }
	engineTimes := make(map[key][]float64)
	for _, b := range report.Benchmarks {
		k := key{b.Category, b.Engine}
		engineTimes[k] = append(engineTimes[k], b.Metrics.WallTimeMs.Median)
	}

	for _, cat := range []string{"single", "collection"} {
		hasCat := false
		for k := range engineTimes {
			if k.cat == cat {
				hasCat = true
				break
			}
		}
		if !hasCat {
			continue
		}
		fmt.Printf("\n  [%s]\n", cat)
		for k, times := range engineTimes {
			if k.cat != cat {
				continue
			}
			total := 0.0
			for _, t := range times {
				total += t
			}
			fmt.Printf("    %s: %.1fms total median wall time across %d benchmarks\n",
				k.engine, total, len(times))
		}
	}
}
