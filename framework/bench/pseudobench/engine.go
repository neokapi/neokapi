package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// Engine can pseudo-translate a file and return metrics.
type Engine interface {
	Name() string
	Version() string
	Available() bool
	PseudoTranslate(ctx context.Context, input, output, format string) (*RunResult, error)
}

// BatchEngine can process a collection of files in a single invocation.
type BatchEngine interface {
	Engine
	PseudoTranslateBatch(ctx context.Context, files []CollectionFile, outputDir string) (*RunResult, error)
	SupportsBatch() bool
}

// runProcess executes a command and collects resource usage metrics.
func runProcess(ctx context.Context, name string, args ...string) (*RunResult, error) {
	return runProcessWithEnv(ctx, nil, name, args...)
}

// runProcessWithEnv executes a command with extra env vars and collects metrics.
func runProcessWithEnv(ctx context.Context, env []string, name string, args ...string) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	// Discard child output to keep benchmark output clean.
	cmd.Stdout = nil
	cmd.Stderr = nil

	start := time.Now()
	if err := cmd.Run(); err != nil {
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
			// macOS reports Maxrss in bytes; Linux in KB.
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

// --- Kapi Native Engine ---

type KapiEngine struct {
	BinaryPath string
	VersionStr string
}

func (e *KapiEngine) Name() string    { return "kapi-native" }
func (e *KapiEngine) Version() string { return e.VersionStr }

func (e *KapiEngine) Available() bool {
	_, err := os.Stat(e.BinaryPath)
	return err == nil
}

// kapiFormatFlag returns the kapi -f flag value for a benchmark format name.
// Returns empty string if format should be auto-detected from extension.
func kapiFormatFlag(format string) string {
	switch format {
	case "docx", "pptx", "xlsx":
		return "openxml" // all OpenXML subtypes use the same format
	default:
		return format // json, html, xml, etc. match kapi format names
	}
}

func (e *KapiEngine) PseudoTranslate(ctx context.Context, input, output, format string) (*RunResult, error) {
	args := []string{
		"flow", "run", "pseudo-translate",
		"-i", input,
		"-o", output,
		"--target-lang", "qps",
		"-q",
	}
	if f := kapiFormatFlag(format); f != "" {
		args = append(args, "-f", f)
	}
	result, err := runProcess(ctx, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	if info, serr := os.Stat(output); serr == nil {
		result.OutputBytes = info.Size()
	}
	return result, nil
}

func (e *KapiEngine) SupportsBatch() bool { return true }

func (e *KapiEngine) PseudoTranslateBatch(ctx context.Context, files []CollectionFile, outputDir string) (*RunResult, error) {
	args := []string{
		"--disable-plugins", "okapi",
		"flow", "run", "pseudo-translate",
		"--target-lang", "qps",
		"-q",
	}
	for _, f := range files {
		args = append(args, "-i", f.Path)
	}
	result, err := runProcess(ctx, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	// Sum output sizes in the output directory.
	result.OutputBytes = sumDirSize(outputDir)
	return result, nil
}

// --- Kapi Bridge Engine ---

type KapiBridgeEngine struct {
	BinaryPath string
	VersionStr string
}

func (e *KapiBridgeEngine) Name() string    { return "kapi-bridge" }
func (e *KapiBridgeEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeEngine) Available() bool {
	_, err := os.Stat(e.BinaryPath)
	return err == nil
}

// bridgeFormatName maps native format names to Okapi bridge filter IDs.
var bridgeFormatName = map[string]string{
	"json":       "okf_json",
	"html":       "okf_html",
	"xml":        "okf_xmlstream",
	"xliff":      "okf_xliff",
	"xliff2":     "okf_xliff",
	"properties": "okf_properties",
	"po":         "okf_po",
	"yaml":       "okf_yaml",
	"plaintext":  "okf_plaintext",
	"markdown":   "okf_markdown",
	"docx":       "okf_openxml",
	"pptx":       "okf_openxml",
	"xlsx":       "okf_openxml",
}

func (e *KapiBridgeEngine) PseudoTranslate(ctx context.Context, input, output, format string) (*RunResult, error) {
	bridgeFmt, ok := bridgeFormatName[format]
	if !ok {
		return nil, fmt.Errorf("no bridge format mapping for %q", format)
	}
	args := []string{
		"flow", "run", "pseudo-translate",
		"-i", input,
		"-o", output,
		"--target-lang", "qps",
		"-f", bridgeFmt,
		"-q",
	}
	result, err := runProcess(ctx, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	if info, serr := os.Stat(output); serr == nil {
		result.OutputBytes = info.Size()
	}
	return result, nil
}

func (e *KapiBridgeEngine) SupportsBatch() bool { return true }

func (e *KapiBridgeEngine) PseudoTranslateBatch(ctx context.Context, files []CollectionFile, outputDir string) (*RunResult, error) {
	// Bridge formats have higher default priority (100) than built-in (50),
	// so auto-detection picks bridge formats (okf_*) for all extensions.
	args := []string{
		"flow", "run", "pseudo-translate",
		"--target-lang", "qps",
		"-q",
	}
	for _, f := range files {
		args = append(args, "-i", f.Path)
	}
	result, err := runProcess(ctx, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	result.OutputBytes = sumDirSize(outputDir)
	return result, nil
}

// --- Kapi Bridge Daemon Engine ---

// KapiBridgeDaemonEngine runs kapi with KAPI_BRIDGE_DAEMON=1 so the JVM
// persists across invocations. The first call pays JVM startup; subsequent
// calls connect to the warm JVM via address file.
type KapiBridgeDaemonEngine struct {
	BinaryPath string
	VersionStr string
	warmedUp   bool
}

func (e *KapiBridgeDaemonEngine) Name() string    { return "kapi-bridge-daemon" }
func (e *KapiBridgeDaemonEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeDaemonEngine) Available() bool {
	_, err := os.Stat(e.BinaryPath)
	return err == nil
}

// ensureWarm re-warms the daemon if it has died (e.g., idle timeout).
func (e *KapiBridgeDaemonEngine) ensureWarm(ctx context.Context) error {
	// Run a quick no-op to check if daemon is alive and re-start if needed.
	input := filepath.Join("testdata", "json", "small", "input.json")
	output := filepath.Join(os.TempDir(), "pseudobench-daemon-check.json")
	defer os.Remove(output)
	_, err := runProcessWithEnv(ctx, daemonEnv, e.BinaryPath,
		"flow", "run", "pseudo-translate",
		"-i", input, "-o", output,
		"--target-lang", "qps", "-f", "okf_json", "-q")
	return err
}

var daemonEnv = []string{"KAPI_BRIDGE_DAEMON=1", "KAPI_BRIDGE_IDLE_TIMEOUT=600s"}

// WarmUp starts the daemon JVM by running a trivial file through it.
// This ensures the first benchmark iteration doesn't pay startup cost.
func (e *KapiBridgeDaemonEngine) WarmUp(ctx context.Context, testdataDir string) error {
	if e.warmedUp {
		return nil
	}
	// Use a small JSON file to trigger JVM startup.
	input := filepath.Join(testdataDir, "json", "small", "input.json")
	output := filepath.Join(os.TempDir(), "pseudobench-daemon-warmup.json")
	defer os.Remove(output)
	_, err := runProcessWithEnv(ctx, daemonEnv, e.BinaryPath,
		"flow", "run", "pseudo-translate",
		"-i", input, "-o", output,
		"--target-lang", "qps", "-f", "okf_json", "-q")
	if err != nil {
		return fmt.Errorf("daemon warmup: %w", err)
	}
	e.warmedUp = true
	return nil
}

func (e *KapiBridgeDaemonEngine) PseudoTranslate(ctx context.Context, input, output, format string) (*RunResult, error) {
	bridgeFmt, ok := bridgeFormatName[format]
	if !ok {
		return nil, fmt.Errorf("no bridge format mapping for %q", format)
	}
	args := []string{
		"flow", "run", "pseudo-translate",
		"-i", input,
		"-o", output,
		"--target-lang", "qps",
		"-f", bridgeFmt,
		"-q",
	}
	result, err := runProcessWithEnv(ctx, daemonEnv, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	if info, serr := os.Stat(output); serr == nil {
		result.OutputBytes = info.Size()
	}
	return result, nil
}

func (e *KapiBridgeDaemonEngine) SupportsBatch() bool { return true }

func (e *KapiBridgeDaemonEngine) PseudoTranslateBatch(ctx context.Context, files []CollectionFile, outputDir string) (*RunResult, error) {
	args := []string{
		"flow", "run", "pseudo-translate",
		"--target-lang", "qps",
		"-q",
	}
	for _, f := range files {
		args = append(args, "-i", f.Path)
	}
	result, err := runProcessWithEnv(ctx, daemonEnv, e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	result.OutputBytes = sumDirSize(outputDir)
	return result, nil
}

// --- Okapi Tikal Engine ---

type OkapiEngine struct {
	TikalPath  string
	VersionStr string
}

func (e *OkapiEngine) Name() string    { return "okapi" }
func (e *OkapiEngine) Version() string { return e.VersionStr }

func (e *OkapiEngine) Available() bool {
	_, err := os.Stat(e.TikalPath)
	return err == nil
}

// okapiExtension maps format names to file extensions Okapi expects.
var okapiExtension = map[string]string{
	"json":       ".json",
	"html":       ".html",
	"xml":        ".xml",
	"xliff":      ".xlf",
	"xliff2":     ".xlf",
	"properties": ".properties",
	"po":         ".po",
	"yaml":       ".yml",
	"plaintext":  ".txt",
	"markdown":   ".md",
	"docx":       ".docx",
	"pptx":       ".pptx",
	"xlsx":       ".xlsx",
}

func (e *OkapiEngine) PseudoTranslate(ctx context.Context, input, output, format string) (*RunResult, error) {
	// Tikal -t (translate) with no translation resource = full read → write
	// pipeline through the Okapi filter, equivalent to kapi's pseudo-translate
	// flow (the pseudo-translate step itself is trivially cheap compared to
	// format I/O, so this is a fair comparison of filter performance).
	//
	// Output is written as <name>.out.<ext> next to the input file.

	tmpDir, err := os.MkdirTemp("", "pseudobench-okapi-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	ext := okapiExtension[format]
	if ext == "" {
		ext = ".txt"
	}

	tmpInput := tmpDir + "/input" + ext
	data, err := os.ReadFile(input)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(tmpInput, data, 0o644); err != nil {
		return nil, err
	}

	args := []string{"-t", tmpInput, "-sl", "en", "-tl", "qps"}
	result, err := runProcess(ctx, e.TikalPath, args...)
	if err != nil {
		return nil, err
	}

	// Tikal writes output as input.out.<ext>.
	outFile := tmpDir + "/input.out" + ext
	if outData, rerr := os.ReadFile(outFile); rerr == nil {
		result.OutputBytes = int64(len(outData))
		_ = os.WriteFile(output, outData, 0o644)
	}

	return result, nil
}

func (e *OkapiEngine) SupportsBatch() bool { return true }

func (e *OkapiEngine) PseudoTranslateBatch(ctx context.Context, files []CollectionFile, outputDir string) (*RunResult, error) {
	// tikal -t file1 file2 ... -sl en -tl qps -od outputDir
	args := []string{"-t"}
	for _, f := range files {
		args = append(args, f.Path)
	}
	args = append(args, "-sl", "en", "-tl", "qps", "-od", outputDir)

	result, err := runProcess(ctx, e.TikalPath, args...)
	if err != nil {
		return nil, err
	}
	result.OutputBytes = sumDirSize(outputDir)
	return result, nil
}

// sumDirSize totals the sizes of all files in a directory.
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
