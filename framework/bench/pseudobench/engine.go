package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Engine processes a batch of files and returns metrics.
type Engine interface {
	Name() string
	Version() string
	// ProcessBatch processes all files. When traceFile is non-empty, kapi engines
	// pass --trace to capture internal concurrency data.
	ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error)
}

// runProcess executes a command and collects resource usage metrics.
func runProcess(ctx context.Context, env []string, name string, args ...string) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	start := time.Now()
	if err := cmd.Run(); err != nil {
		errMsg := stderrBuf.String()
		if errMsg != "" {
			return nil, fmt.Errorf("command %s failed: %w\nstderr: %s", name, err, errMsg)
		}
		return nil, fmt.Errorf("command %s failed: %w", name, err)
	}
	wallTime := time.Since(start)

	var userCPU, sysCPU time.Duration
	var peakRSS int64

	if ps := cmd.ProcessState; ps != nil {
		userCPU = ps.UserTime()
		sysCPU = ps.SystemTime()
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
			peakRSS = ru.Maxrss
			if runtime.GOOS == "darwin" {
				peakRSS /= 1024
			}
		}
	}

	return &RunResult{
		WallTime:  wallTime,
		UserCPU:   userCPU,
		SystemCPU: sysCPU,
		PeakRSSKB: peakRSS,
	}, nil
}

// runProcessWithOutput executes a command, captures stdout, and collects resource usage metrics.
func runProcessWithOutput(ctx context.Context, env []string, name string, args ...string) ([]byte, *RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdoutBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	start := time.Now()
	if err := cmd.Run(); err != nil {
		errMsg := stderrBuf.String()
		if errMsg != "" {
			return nil, nil, fmt.Errorf("command %s failed: %w\nstderr: %s", name, err, errMsg)
		}
		return nil, nil, fmt.Errorf("command %s failed: %w", name, err)
	}
	wallTime := time.Since(start)

	var userCPU, sysCPU time.Duration
	var peakRSS int64

	if ps := cmd.ProcessState; ps != nil {
		userCPU = ps.UserTime()
		sysCPU = ps.SystemTime()
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
			peakRSS = ru.Maxrss
			if runtime.GOOS == "darwin" {
				peakRSS /= 1024
			}
		}
	}

	return []byte(stdoutBuf.String()), &RunResult{
		WallTime:  wallTime,
		UserCPU:   userCPU,
		SystemCPU: sysCPU,
		PeakRSSKB: peakRSS,
	}, nil
}

// parseBlockCount extracts the block count from kapi JSON output with --stats.
func parseBlockCount(stdout []byte) int64 {
	var result struct {
		Stats *struct {
			BlockCount int64 `json:"block_count"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil || result.Stats == nil {
		return 0
	}
	return result.Stats.BlockCount
}

// --- Kapi Native Engine ---

// KapiNativeEngine runs kapi with --disable-plugins okapi (native formats only).
type KapiNativeEngine struct {
	BinaryPath string
	VersionStr string
}

func (e *KapiNativeEngine) Name() string    { return "kapi-native" }
func (e *KapiNativeEngine) Version() string { return e.VersionStr }

func (e *KapiNativeEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	var inputArgs []string
	fileResults := make([]FileResult, len(files))

	for i, f := range files {
		inputArgs = append(inputArgs, "-i", filepath.Join(inputDir, f.Name))
		fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: true}
	}

	outputTemplate := filepath.Join(outputDir, "{name}_{lang}{ext}")
	args := []string{"--disable-plugins", "okapi", "flow", "run", "pseudo-translate"}
	args = append(args, inputArgs...)
	args = append(args, "--target-lang", "qps", "-q", "-o", outputTemplate)
	if traceFile != "" {
		args = append(args, "--trace", traceFile)
	}

	result, err := runProcess(ctx, nil, e.BinaryPath, args...)
	if err != nil {
		for i := range fileResults {
			fileResults[i].Success = false
			fileResults[i].Error = err.Error()
		}
		return nil, fileResults, err
	}

	result.OutputBytes = sumDirSize(outputDir)
	return result, fileResults, nil
}

// --- Kapi Bridge Engine (subprocess per invocation) ---

// KapiBridgeEngine runs kapi with the bridge plugin (subprocess JVM).
type KapiBridgeEngine struct {
	BinaryPath string
	VersionStr string
}

func (e *KapiBridgeEngine) Name() string    { return "kapi-bridge" }
func (e *KapiBridgeEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	var inputArgs []string
	fileResults := make([]FileResult, len(files))

	for i, f := range files {
		inputArgs = append(inputArgs, "-i", filepath.Join(inputDir, f.Name))
		fileResults[i] = FileResult{
			Name:    f.Name,
			Format:  f.Format,
			Success: true,
		}
	}

	args := []string{"flow", "run", "pseudo-translate"}
	args = append(args, inputArgs...)
	outputTemplate := filepath.Join(outputDir, "{name}_{lang}{ext}")
	args = append(args, "--target-lang", "qps", "-q", "-o", outputTemplate)
	if traceFile != "" {
		args = append(args, "--trace", traceFile)
	}

	result, err := runProcess(ctx, nil, e.BinaryPath, args...)
	if err != nil {
		for i := range fileResults {
			fileResults[i].Success = false
			fileResults[i].Error = err.Error()
		}
		return nil, fileResults, err
	}

	result.OutputBytes = sumDirSize(outputDir)
	return result, fileResults, nil
}

// --- Kapi Bridge Daemon Engine ---

// KapiBridgeDaemonEngine runs kapi with NEOKAPI_BRIDGE_ADDRS pointing to a pre-started daemon.
type KapiBridgeDaemonEngine struct {
	BinaryPath string
	VersionStr string
	Daemon     *DaemonProcess
}

func (e *KapiBridgeDaemonEngine) Name() string    { return "kapi-bridge-daemon" }
func (e *KapiBridgeDaemonEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeDaemonEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	var inputArgs []string
	fileResults := make([]FileResult, len(files))

	for i, f := range files {
		inputArgs = append(inputArgs, "-i", filepath.Join(inputDir, f.Name))
		fileResults[i] = FileResult{
			Name:    f.Name,
			Format:  f.Format,
			Success: true,
		}
	}

	outputTemplate := filepath.Join(outputDir, "{name}_{lang}{ext}")
	args := []string{"flow", "run", "pseudo-translate"}
	args = append(args, inputArgs...)
	args = append(args, "--target-lang", "qps", "-q", "-o", outputTemplate)
	if traceFile != "" {
		args = append(args, "--trace", traceFile)
	}

	env := []string{fmt.Sprintf("NEOKAPI_BRIDGE_ADDRS=%s", e.Daemon.Address())}

	result, err := runProcess(ctx, env, e.BinaryPath, args...)
	if err != nil {
		for i := range fileResults {
			fileResults[i].Success = false
			fileResults[i].Error = err.Error()
		}
		return nil, fileResults, err
	}

	result.OutputBytes = sumDirSize(outputDir)
	return result, fileResults, nil
}

// --- Okapi Tikal Engine ---

// OkapiTikalEngine runs Okapi's tikal CLI.
type OkapiTikalEngine struct {
	TikalPath  string
	VersionStr string
}

func (e *OkapiTikalEngine) Name() string    { return "okapi" }
func (e *OkapiTikalEngine) Version() string { return e.VersionStr }

func (e *OkapiTikalEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, _ string) (*RunResult, []FileResult, error) {
	// Copy files to a temp dir so tikal writes output there.
	tmpDir, err := os.MkdirTemp("", "pseudobench-okapi-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmpDir)

	fileResults := make([]FileResult, len(files))
	var tmpPaths []string

	for i, f := range files {
		src := filepath.Join(inputDir, f.Name)
		dst := filepath.Join(tmpDir, f.Name)
		data, err := os.ReadFile(src)
		if err != nil {
			fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: false, Error: err.Error()}
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: false, Error: err.Error()}
			continue
		}
		tmpPaths = append(tmpPaths, dst)
		fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: true}
	}

	if len(tmpPaths) == 0 {
		return &RunResult{}, fileResults, nil
	}

	args := []string{"-t"}
	args = append(args, tmpPaths...)
	args = append(args, "-sl", "en", "-tl", "qps", "-od", outputDir)

	result, err := runProcess(ctx, nil, e.TikalPath, args...)
	if err != nil {
		for i := range fileResults {
			if fileResults[i].Success {
				fileResults[i].Success = false
				fileResults[i].Error = err.Error()
			}
		}
		return nil, fileResults, err
	}

	result.OutputBytes = sumDirSize(outputDir)
	return result, fileResults, nil
}

// --- Per-file processing (for timeline trace) ---

// FileProcessor processes a single file. Used for per-file timing trace.
type FileProcessor interface {
	ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error)
}

func (e *KapiNativeEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"--disable-plugins", "okapi", "--output-format", "json",
		"flow", "run", "pseudo-translate",
		"-i", filepath.Join(inputDir, file.Name), "--target-lang", "qps",
		"--stats", "-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	stdout, result, err := runProcessWithOutput(ctx, nil, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	result.BlockCount = parseBlockCount(stdout)
	return result, nil
}

func (e *KapiBridgeEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"flow", "run", "pseudo-translate",
		"-i", filepath.Join(inputDir, file.Name), "--target-lang", "qps", "-q",
		"-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	return runProcess(ctx, nil, e.BinaryPath, args...)
}

func (e *KapiBridgeDaemonEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"flow", "run", "pseudo-translate",
		"-i", filepath.Join(inputDir, file.Name), "--target-lang", "qps", "-q",
		"-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	env := []string{fmt.Sprintf("NEOKAPI_BRIDGE_ADDRS=%s", e.Daemon.Address())}
	return runProcess(ctx, env, e.BinaryPath, args...)
}

func (e *OkapiTikalEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	tmpDir, err := os.MkdirTemp("", "pseudobench-okapi-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	data, err := os.ReadFile(filepath.Join(inputDir, file.Name))
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(tmpDir, file.Name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return nil, err
	}
	return runProcess(ctx, nil, e.TikalPath, "-t", dst, "-sl", "en", "-tl", "qps", "-od", outputDir)
}

// sumDirSize totals the sizes of all files in a directory (non-recursive).
func sumDirSize(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			info, err := e.Info()
			if err == nil {
				total += info.Size()
			}
		}
	}
	return total
}

// runCommand runs a command and returns stdout as a string.
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// runCommandCombined runs a command and returns combined stdout+stderr.
func runCommandCombined(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// detectVersion tries to get the version string from a binary.
func detectVersion(binPath string) string {
	// Try kapi-style: kapi version --json → {"version": "..."}
	out, err := runCommand(binPath, "version", "--json")
	if err == nil {
		// Simple line-based parse to avoid importing encoding/json.
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, `"version"`) {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					v := strings.TrimSpace(parts[1])
					v = strings.Trim(v, `",`)
					if v != "" {
						return v
					}
				}
			}
		}
	}

	// Try tikal-style: prints "Version: X.Y.Z".
	out, err = runCommandCombined(binPath)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Version: ") {
				return strings.TrimPrefix(line, "Version: ")
			}
		}
	}

	return filepath.Base(binPath)
}
