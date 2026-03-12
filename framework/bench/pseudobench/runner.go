package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runBenchmarks executes all benchmarks according to the config.
func runBenchmarks(cfg *Config) (*Report, error) {
	report := &Report{
		Metadata: collectMetadata(),
	}

	engines := buildEngines(cfg)
	if len(engines) == 0 {
		return nil, fmt.Errorf("no engines available")
	}

	// Count total benchmarks for progress display.
	total := countBenchmarks(cfg, engines)
	current := 0

	for _, cat := range cfg.Categories {
		switch cat {
		case "single":
			for _, engine := range engines {
				if !engine.Available() {
					fmt.Printf("  SKIP %s (not available)\n", engine.Name())
					continue
				}
				for _, format := range cfg.Formats {
					for _, size := range cfg.Sizes {
						current++
						label := fmt.Sprintf("[%d/%d] %s / %s / %s", current, total, engine.Name(), format, size)
						fmt.Printf("  %s ...", label)

						result, err := benchmarkOne(engine, cfg, format, size)
						if err != nil {
							fmt.Printf(" ERROR: %v\n", err)
							continue
						}

						report.Benchmarks = append(report.Benchmarks, *result)
						fmt.Printf(" %.1fms (median)\n", result.Metrics.WallTimeMs.Median)
					}
				}
			}

		case "collection":
			for _, engine := range engines {
				if !engine.Available() {
					fmt.Printf("  SKIP %s (not available)\n", engine.Name())
					continue
				}
				be, ok := engine.(BatchEngine)
				if !ok || !be.SupportsBatch() {
					current += len(cfg.Sizes)
					fmt.Printf("  SKIP %s (no batch support)\n", engine.Name())
					continue
				}
				for _, size := range cfg.Sizes {
					current++
					label := fmt.Sprintf("[%d/%d] collection / %s / %s", current, total, engine.Name(), size)
					fmt.Printf("  %s ...", label)

					result, err := benchmarkCollection(be, cfg, size)
					if err != nil {
						fmt.Printf(" ERROR: %v\n", err)
						continue
					}

					report.Benchmarks = append(report.Benchmarks, *result)
					fmt.Printf(" %.1fms (median, %d files)\n", result.Metrics.WallTimeMs.Median, result.FileCount)
				}
			}
		}
	}

	return report, nil
}

func countBenchmarks(cfg *Config, engines []Engine) int {
	total := 0
	for _, cat := range cfg.Categories {
		switch cat {
		case "single":
			for _, e := range engines {
				if e.Available() {
					total += len(cfg.Formats) * len(cfg.Sizes)
				}
			}
		case "collection":
			for _, e := range engines {
				if e.Available() {
					total += len(cfg.Sizes)
				}
			}
		}
	}
	return total
}

// benchmarkOne runs warmup + iterations for a single engine/format/size.
func benchmarkOne(engine Engine, cfg *Config, format, size string) (*BenchmarkResult, error) {
	inputPath := findInputFile(cfg.TestdataDir, format, size)
	if inputPath == "" {
		return nil, fmt.Errorf("no test data for %s/%s", format, size)
	}

	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}

	units := unitCounts[size]

	tmpDir, err := os.MkdirTemp("", "pseudobench-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Warmup
	for i := 0; i < cfg.Warmup; i++ {
		outputPath := filepath.Join(tmpDir, fmt.Sprintf("warmup_%d%s", i, filepath.Ext(inputPath)))
		_, _ = engine.PseudoTranslate(ctx, inputPath, outputPath, format)
		os.Remove(outputPath)
	}

	// Collect measurements
	var wallTimes, userCpus, sysCpus, peakRSSs, outputBytesArr []float64

	for i := 0; i < cfg.Iterations; i++ {
		outputPath := filepath.Join(tmpDir, fmt.Sprintf("iter_%d%s", i, filepath.Ext(inputPath)))

		result, err := engine.PseudoTranslate(ctx, inputPath, outputPath, format)
		if err != nil {
			return nil, fmt.Errorf("iteration %d: %w", i, err)
		}

		wallTimes = append(wallTimes, float64(result.WallTime.Microseconds())/1000.0)
		userCpus = append(userCpus, float64(result.UserCPU.Microseconds())/1000.0)
		sysCpus = append(sysCpus, float64(result.SystemCPU.Microseconds())/1000.0)
		peakRSSs = append(peakRSSs, float64(result.PeakRSSKB))
		outputBytesArr = append(outputBytesArr, float64(result.OutputBytes))

		os.Remove(outputPath)
	}

	return &BenchmarkResult{
		Category:        "single",
		Engine:          engine.Name(),
		Format:          format,
		FileSize:        size,
		FileSizeBytes:   inputInfo.Size(),
		FileCount:       1,
		TotalInputBytes: inputInfo.Size(),
		UnitCount:       units,
		Version:         engine.Version(),
		Iterations:      cfg.Iterations,
		Metrics: Metrics{
			WallTimeMs:  computeStats(wallTimes),
			UserCpuMs:   computeStats(userCpus),
			SysCpuMs:    computeStats(sysCpus),
			PeakRssKB:   computeStats(peakRSSs),
			OutputBytes: computeStats(outputBytesArr),
		},
	}, nil
}

// benchmarkCollection runs warmup + iterations for a collection benchmark.
func benchmarkCollection(engine BatchEngine, cfg *Config, size string) (*BenchmarkResult, error) {
	collDir := filepath.Join(cfg.TestdataDir, "collection", size)
	files, totalUnits, err := loadCollectionFiles(collDir)
	if err != nil {
		return nil, fmt.Errorf("load collection %s: %w", size, err)
	}

	var totalInput int64
	for _, f := range files {
		if info, serr := os.Stat(f.Path); serr == nil {
			totalInput += info.Size()
		}
	}

	ctx := context.Background()

	// Warmup
	for i := 0; i < cfg.Warmup; i++ {
		tmpDir, _ := os.MkdirTemp("", "pseudobench-coll-warmup-*")
		_, _ = engine.PseudoTranslateBatch(ctx, files, tmpDir)
		os.RemoveAll(tmpDir)
		cleanCollectionOutputs(collDir)
	}

	// Collect measurements
	var wallTimes, userCpus, sysCpus, peakRSSs, outputBytesArr []float64

	for i := 0; i < cfg.Iterations; i++ {
		tmpDir, _ := os.MkdirTemp("", "pseudobench-coll-*")

		result, err := engine.PseudoTranslateBatch(ctx, files, tmpDir)
		if err != nil {
			os.RemoveAll(tmpDir)
			cleanCollectionOutputs(collDir)
			return nil, fmt.Errorf("iteration %d: %w", i, err)
		}

		wallTimes = append(wallTimes, float64(result.WallTime.Microseconds())/1000.0)
		userCpus = append(userCpus, float64(result.UserCPU.Microseconds())/1000.0)
		sysCpus = append(sysCpus, float64(result.SystemCPU.Microseconds())/1000.0)
		peakRSSs = append(peakRSSs, float64(result.PeakRSSKB))
		outputBytesArr = append(outputBytesArr, float64(result.OutputBytes))

		os.RemoveAll(tmpDir)
		cleanCollectionOutputs(collDir)
	}

	return &BenchmarkResult{
		Category:        "collection",
		Engine:          engine.Name(),
		Format:          "mixed",
		FileSize:        size,
		FileSizeBytes:   0,
		FileCount:       len(files),
		TotalInputBytes: totalInput,
		UnitCount:       totalUnits,
		Version:         engine.Version(),
		Iterations:      cfg.Iterations,
		Metrics: Metrics{
			WallTimeMs:  computeStats(wallTimes),
			UserCpuMs:   computeStats(userCpus),
			SysCpuMs:    computeStats(sysCpus),
			PeakRssKB:   computeStats(peakRSSs),
			OutputBytes: computeStats(outputBytesArr),
		},
	}, nil
}

// loadCollectionFiles reads the collection directory and returns file info.
// cleanCollectionOutputs removes output files (_qps, .out.) that kapi/tikal
// write next to input files in the collection directory between iterations.
func cleanCollectionOutputs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, "_qps") || strings.Contains(name, ".out.") {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

func loadCollectionFiles(dir string) ([]CollectionFile, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}

	var files []CollectionFile
	totalUnits := 0

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Skip output files from previous runs (e.g. app-strings_qps.json).
		if strings.Contains(e.Name(), "_qps") {
			continue
		}
		format := extensionToFormat(filepath.Ext(e.Name()))
		if format == "" {
			continue
		}

		// Estimate units from the collection spec or file size.
		units := estimateUnits(dir, e.Name())
		files = append(files, CollectionFile{
			Format: format,
			Path:   filepath.Join(dir, e.Name()),
			Units:  units,
		})
		totalUnits += units
	}

	if len(files) == 0 {
		return nil, 0, fmt.Errorf("no files found in %s", dir)
	}

	return files, totalUnits, nil
}

func extensionToFormat(ext string) string {
	switch strings.ToLower(ext) {
	case ".json":
		return "json"
	case ".html":
		return "html"
	case ".xml":
		return "xml"
	case ".xlf":
		return "xliff"
	case ".properties":
		return "properties"
	case ".po":
		return "po"
	case ".yml", ".yaml":
		return "yaml"
	case ".txt":
		return "plaintext"
	case ".docx":
		return "docx"
	case ".pptx":
		return "pptx"
	case ".xlsx":
		return "xlsx"
	default:
		return ""
	}
}

// estimateUnits returns a rough unit count for a collection file.
// For collections we look up the spec; if not found, estimate from file size.
func estimateUnits(dir, name string) int {
	// Try to match against known collection specs.
	base := filepath.Base(dir) // size tier name
	specs, ok := collectionSpecs[base]
	if ok {
		nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))
		for _, s := range specs {
			if s.Name == nameNoExt {
				return s.Units
			}
		}
	}
	// Fallback: rough estimate.
	return 50
}

func findInputFile(testdataDir, format, size string) string {
	dir := filepath.Join(testdataDir, format, size)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "input") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func buildEngines(cfg *Config) []Engine {
	var engines []Engine

	for _, bin := range cfg.KapiBins {
		engines = append(engines, &KapiEngine{
			BinaryPath: bin.Path,
			VersionStr: bin.Version,
		})
		if cfg.Bridge {
			engines = append(engines, &KapiBridgeEngine{
				BinaryPath: bin.Path,
				VersionStr: bin.Version,
			})
		}
	}

	for _, bin := range cfg.OkapiBins {
		engines = append(engines, &OkapiEngine{
			TikalPath:  bin.Path,
			VersionStr: bin.Version,
		})
	}

	return engines
}

func collectMetadata() Metadata {
	return Metadata{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Platform:  fmt.Sprintf("%s/%s", osName(), archName()),
		GoVersion: goVersion(),
		CPUModel:  cpuModel(),
		CPUCores:  cpuCores(),
		MemoryGB:  memoryGB(),
	}
}
