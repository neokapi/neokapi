package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// runBenchmarks executes the mixed experiment for all engines.
func runBenchmarks(cfg *Config) (*Report, error) {
	report := &Report{
		Metadata: collectMetadata(),
	}

	engines, daemon, err := buildEngines(cfg)
	if err != nil {
		return nil, err
	}
	if daemon != nil {
		defer daemon.Shutdown()
	}

	if len(engines) == 0 {
		return nil, fmt.Errorf("no engines available")
	}

	for _, engine := range engines {
		fmt.Printf("\n=== %s ===\n", engine.Name())

		result, err := runExperiment(engine, cfg, daemon)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			continue
		}

		report.Experiments = append(report.Experiments, *result)

		successCount := 0
		for _, fr := range result.FileResults {
			if fr.Success {
				successCount++
			}
		}
		fmt.Printf("  Result: %.0fms median, %d/%d files OK\n",
			result.WallTimeMs.Median, successCount, len(result.FileResults))
	}

	return report, nil
}

// runExperiment runs warmup + measurement iterations for one engine.
func runExperiment(engine Engine, cfg *Config, daemon *DaemonProcess) (*ExperimentResult, error) {
	ctx := context.Background()

	// Warmup
	for i := range cfg.Warmup {
		fmt.Printf("  warmup %d/%d...\n", i+1, cfg.Warmup)
		tmpDir, err := os.MkdirTemp("", "pseudobench-warmup-*")
		if err != nil {
			return nil, err
		}
		_, _, _ = engine.ProcessBatch(ctx, cfg.Fixtures, "", tmpDir, "")
		os.RemoveAll(tmpDir)
	}

	// Measurement iterations
	var wallTimes, peakRSSs []float64
	var lastFileResults []FileResult

	// Start RSS sampling for daemon engine.
	isDaemon := engine.Name() == "kapi-bridge-daemon" && daemon != nil
	if isDaemon {
		daemon.StartRSSSampling(500 * time.Millisecond)
	}

	// Prepare trace file path for the final iteration.
	var traceFile string
	if cfg.TraceDir != "" {
		os.MkdirAll(cfg.TraceDir, 0o755)
		traceFile = filepath.Join(cfg.TraceDir, engine.Name()+"-trace.json")
	}

	for i := range cfg.Iterations {
		fmt.Printf("  iteration %d/%d...", i+1, cfg.Iterations)

		// Use a temp dir for all iterations except the last.
		var outputDir string
		isLast := i == cfg.Iterations-1
		if isLast {
			outputDir = filepath.Join(cfg.OutputDir, engine.Name())
			os.MkdirAll(outputDir, 0o755)
		} else {
			tmpDir, err := os.MkdirTemp("", "pseudobench-iter-*")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(tmpDir)
			outputDir = tmpDir
		}

		// Only capture trace on the final iteration.
		iterTraceFile := ""
		if isLast {
			iterTraceFile = traceFile
		}

		result, fileResults, err := engine.ProcessBatch(ctx, cfg.Fixtures, "", outputDir, iterTraceFile)
		if err != nil {
			fmt.Printf(" ERROR: %v\n", err)
			// Record the error but continue with other iterations.
			lastFileResults = fileResults
			continue
		}

		wallTimes = append(wallTimes, float64(result.WallTime.Microseconds())/1000.0)
		peakRSSs = append(peakRSSs, float64(result.PeakRSSKB))
		lastFileResults = fileResults

		fmt.Printf(" %.0fms\n", float64(result.WallTime.Microseconds())/1000.0)
	}

	if len(wallTimes) == 0 {
		return nil, fmt.Errorf("all iterations failed")
	}

	exp := &ExperimentResult{
		Engine:      engine.Name(),
		Version:     engine.Version(),
		Iterations:  len(wallTimes),
		WallTimeMs:  computeStats(wallTimes),
		PeakRssKB:   computeStats(peakRSSs),
		FileResults: lastFileResults,
	}

	// Aggregate verification stats from the last iteration's per-file
	// results. FilesUnverified counts fixtures whose output exists but
	// has zero pseudo-translated runes despite the input containing
	// letters — a strong signal the engine short-circuited pseudo or
	// dropped translatable content. Surfaced in the report so a "fast"
	// engine that does nothing useful is visibly distinct from a "fast"
	// engine that actually pseudo-translates.
	exp.FilesAttempted = len(lastFileResults)
	for _, fr := range lastFileResults {
		if fr.Success {
			exp.FilesSucceeded++
		}
		if fr.Verified {
			exp.FilesVerified++
		} else if fr.Success && fr.PseudoChars == 0 && fr.VerifyNote != "" {
			exp.FilesUnverified++
		}
		exp.TotalPseudoChars += fr.PseudoChars
	}
	fmt.Printf("  verified: %d/%d files have pseudo content (%d total runes)\n",
		exp.FilesVerified, exp.FilesAttempted, exp.TotalPseudoChars)
	if exp.FilesUnverified > 0 {
		fmt.Printf("  ⚠ %d files succeeded but produced zero pseudo runes\n", exp.FilesUnverified)
	}

	// Collect daemon RSS stats.
	if isDaemon {
		samples := daemon.StopRSSSampling()
		if len(samples) > 0 {
			floats := make([]float64, len(samples))
			for i, s := range samples {
				floats[i] = float64(s)
			}
			stats := computeStats(floats)
			exp.DaemonRssKB = &stats
		}
	}

	// Parse batch trace JSON if trace was captured.
	if traceFile != "" {
		data, err := os.ReadFile(traceFile)
		if err == nil {
			var bt BatchTrace
			if json.Unmarshal(data, &bt) == nil && len(bt.FileTraces) > 0 {
				exp.BatchTrace = &bt
				fmt.Printf("  batch trace: %d files, %d lanes\n", len(bt.FileTraces), bt.Concurrency)
			}
		}
	}

	// Per-file trace pass: run each file individually to get per-file timing.
	if fp, ok := engine.(FileProcessor); ok {
		fmt.Print("  per-file trace...")
		timings := runFileTrace(ctx, fp, cfg)
		exp.FileTimings = timings
		fmt.Printf(" %d files\n", len(timings))
	}

	return exp, nil
}

// runFileTrace processes each file individually to collect per-file timing.
func runFileTrace(ctx context.Context, fp FileProcessor, cfg *Config) []FileTiming {
	tmpDir, err := os.MkdirTemp("", "pseudobench-trace-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	traceStart := time.Now()
	var timings []FileTiming

	for _, f := range cfg.Fixtures {
		sizeBytes := f.SizeBytes
		if sizeBytes == 0 {
			if info, serr := os.Stat(f.SourcePath); serr == nil {
				sizeBytes = info.Size()
			}
		}

		fileStart := time.Since(traceStart)
		result, err := fp.ProcessFile(ctx, f, "", tmpDir)
		fileEnd := time.Since(traceStart)

		ft := FileTiming{
			Name:      f.Name,
			Format:    f.Format,
			Category:  f.Category,
			SizeBytes: sizeBytes,
			StartMs:   float64(fileStart.Microseconds()) / 1000.0,
			EndMs:     float64(fileEnd.Microseconds()) / 1000.0,
		}

		if err != nil {
			ft.Success = false
			ft.Error = err.Error()
			ft.WallMs = ft.EndMs - ft.StartMs
		} else {
			ft.Success = true
			ft.WallMs = float64(result.WallTime.Microseconds()) / 1000.0
			ft.PeakRssKB = result.PeakRSSKB
			ft.UserCpuMs = float64(result.UserCPU.Microseconds()) / 1000.0
			ft.SysCpuMs = float64(result.SystemCPU.Microseconds()) / 1000.0
			ft.BlockCount = result.BlockCount
		}

		timings = append(timings, ft)
	}

	return timings
}

func buildEngines(cfg *Config) ([]Engine, *DaemonProcess, error) {
	var engines []Engine
	var daemon *DaemonProcess

	kapiVersion := ""
	if cfg.KapiBin != "" {
		kapiVersion = detectVersion(cfg.KapiBin)
	}

	if cfg.KapiBin != "" {
		engines = append(engines, &KapiNativeEngine{
			BinaryPath: cfg.KapiBin,
			VersionStr: kapiVersion,
		})
		engines = append(engines, &KapiBridgeEngine{
			BinaryPath:  cfg.KapiBin,
			OkapiBridge: cfg.OkapiBridge,
			VersionStr:  kapiVersion,
		})
	}

	// Daemon engine (requires bridge JAR).
	if cfg.KapiBin != "" && cfg.BridgeJar != "" {
		fmt.Print("Starting bridge daemon...")
		d, err := StartDaemon(cfg.BridgeJar)
		if err != nil {
			fmt.Printf(" FAILED: %v\n", err)
		} else {
			daemon = d
			fmt.Printf(" OK (PID %d, address %s)\n", d.PID(), d.Address())
			engines = append(engines, &KapiBridgeDaemonEngine{
				BinaryPath:  cfg.KapiBin,
				OkapiBridge: cfg.OkapiBridge,
				VersionStr:  kapiVersion,
				Daemon:      d,
			})
		}
	}

	if cfg.OkapiBridge != "" {
		// Version detection: kapi-okapi-bridge supports `version` subcommand.
		okapiVersion := detectVersion(cfg.OkapiBridge)
		engines = append(engines, &OkapiPseudoEngine{
			BinaryPath: cfg.OkapiBridge,
			VersionStr: okapiVersion,
		})
	}

	return engines, daemon, nil
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

func printSummary(report *Report) {
	fmt.Println("\n--- Summary ---")
	fmt.Printf("Platform: %s | CPU: %s (%d cores) | Memory: %.0fGB\n",
		report.Metadata.Platform, report.Metadata.CPUModel,
		report.Metadata.CPUCores, report.Metadata.MemoryGB)

	for _, exp := range report.Experiments {
		successCount := 0
		for _, fr := range exp.FileResults {
			if fr.Success {
				successCount++
			}
		}
		fmt.Printf("  %-20s %s  %3d/%d files  %.0fms median  %.0fKB peak RSS",
			exp.Engine, exp.Version, successCount, len(exp.FileResults),
			exp.WallTimeMs.Median, exp.PeakRssKB.Max)
		if exp.DaemonRssKB != nil {
			fmt.Printf("  %.0fKB daemon RSS", exp.DaemonRssKB.Max)
		}
		fmt.Println()
	}
}
