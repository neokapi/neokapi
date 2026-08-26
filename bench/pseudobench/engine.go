package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// inputPathFor returns the absolute path engines should pass to their
// CLI for fixture f. DiscoverFixtures sets SourcePath; legacy callers
// that hand-curate TestFile lists can still pass an inputDir + Name.
func inputPathFor(f TestFile, inputDir string) string {
	if f.SourcePath != "" {
		return f.SourcePath
	}
	return filepath.Join(inputDir, f.Name)
}

// Engine processes a batch of files and returns metrics.
type Engine interface {
	Name() string
	Version() string
	// ProcessBatch processes all files. When traceFile is non-empty, kapi engines
	// pass --trace to capture internal concurrency data.
	ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error)
}

// benchCWD is a stable empty directory used as the working directory for
// every spawned process. Every fixture and output path the engines pass is
// absolute, so the cwd never carries meaning — pinning it outside the repo
// keeps a kapi invocation from binding the repo's dogfood kapi.yaml via the
// upward project walk (belt-and-suspenders with the KAPI_NO_PROJECT=1 that
// nativeOnlyEnv/bridgeEnv already set).
func benchCWD() string {
	d := filepath.Join(os.TempDir(), "kapi-bench-cwd")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// runProcess executes a command and collects resource usage metrics.
func runProcess(ctx context.Context, env []string, name string, args ...string) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = benchCWD()
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
	cmd.Dir = benchCWD()
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

// KapiNativeEngine runs kapi with no plugins loaded (native formats only).
// It points XDG_DATA_HOME at a scratch dir so the bridge plugin under
// the user's real ~/.local/share/kapi/plugins does not register, and
// the native filter wins format dispatch for every supported format.
type KapiNativeEngine struct {
	BinaryPath string
	VersionStr string
}

func (e *KapiNativeEngine) Name() string    { return "kapi-native" }
func (e *KapiNativeEngine) Version() string { return e.VersionStr }

// nativeOnlyEnv suppresses plugin discovery and isolates the run from the
// developer's config, caches, and the repo's dogfood project — the full
// dogfooding contract (CLAUDE.md, Makefile $(KAPI_ISO_ENV)). KAPI_PLUGINS_DIR
// points at an empty scratch dir and KAPI_PLUGINS_DIR_ONLY pins discovery to
// exactly that dir, so neither the user's XDG root nor the absolute system
// roots the host also walks can leak a Homebrew plugin into the "kapi-native"
// numbers. Without _ONLY, XDG_DATA_HOME alone left the system roots live and
// the comparison "native vs bridge" was not actually native-only.
func nativeOnlyEnv() []string {
	scratch := filepath.Join(os.TempDir(), "kapi-native-no-plugins")
	_ = os.MkdirAll(scratch, 0o755)
	return []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_TELEMETRY=0",
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + scratch,
		"KAPI_CONFIG_DIR=" + filepath.Join(scratch, "config"),
		"XDG_DATA_HOME=" + scratch,
		"XDG_CACHE_HOME=" + filepath.Join(scratch, "cache"),
		"HOME=" + scratch,
	}
}

// bridgePluginsDir derives the plugins directory from the path to the
// jpackage bridge launcher. okapiBridgePath looks like
// "<parity>/plugins/okapi-bridge/Contents/MacOS/kapi-okapi-bridge"; we
// want "<parity>/plugins".
func bridgePluginsDir(okapiBridgePath string) string {
	d := filepath.Dir(okapiBridgePath) // .../Contents/MacOS
	d = filepath.Dir(d)                // .../Contents
	d = filepath.Dir(d)                // .../okapi-bridge
	return filepath.Dir(d)             // .../plugins
}

// bridgeEnv pins kapi's plugin discovery to the parity-built okapi-bridge
// under <parity>/plugins and isolates the run from the developer's config,
// caches, and the dogfood project. KAPI_PLUGINS_DIR_ONLY restricts discovery
// to that one dir, so the user's brew-installed plugin (typically an older
// release JAR) and the system roots can't sneak into the comparison — the
// bench measures exactly what was built into the parity sandbox.
func bridgeEnv(okapiBridgePath string) []string {
	scratch := filepath.Join(os.TempDir(), "kapi-bridge-no-xdg")
	_ = os.MkdirAll(scratch, 0o755)
	return []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_TELEMETRY=0",
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_PLUGINS_DIR=" + bridgePluginsDir(okapiBridgePath),
		"KAPI_CONFIG_DIR=" + filepath.Join(scratch, "config"),
		"XDG_DATA_HOME=" + scratch,
		"XDG_CACHE_HOME=" + filepath.Join(scratch, "cache"),
		"HOME=" + scratch,
	}
}

func (e *KapiNativeEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	return runKapiBatch(ctx, e.BinaryPath, files, inputDir, outputDir, traceFile, nativeOnlyEnv())
}

// runKapiBatch is the shared body for the three kapi engines. It runs
// `kapi pseudo-translate <files...> --fail-on-unknown -o <template>`,
// then verifies that every input produced an output file. If kapi exits
// 0 but didn't actually write the expected output (e.g. the bridge
// silently dropped the file because filter dispatch failed), the
// fixture is marked as failed so the timing for that engine isn't
// silently misreported as success. --fail-on-unknown ensures kapi
// itself fails the run on any format-detection skip.
func runKapiBatch(ctx context.Context, binPath string, files []TestFile, inputDir, outputDir, traceFile string, env []string) (*RunResult, []FileResult, error) {
	args := []string{"pseudo-translate"}
	fileResults := make([]FileResult, len(files))
	inputs := make([]string, len(files))
	for i, f := range files {
		inputs[i] = inputPathFor(f, inputDir)
		args = append(args, inputs[i])
		fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: true}
	}
	outputTemplate := filepath.Join(outputDir, "{name}_{lang}{ext}")
	// Don't pass --fail-on-unknown: some engines legitimately can't
	// handle some formats (no native idml/openxml reader), and we want
	// them to skip rather than fail the whole batch. Per-file success
	// is determined post-hoc by checking output existence below — that
	// catches the silent-skip class of bugs without losing engine-wide
	// timing on partial-coverage formats.
	args = append(args, "--target-lang", "qps", "-q", "-o", outputTemplate)
	if traceFile != "" {
		args = append(args, "--trace", traceFile)
	}

	result, err := runProcess(ctx, env, binPath, args...)
	if err != nil {
		for i := range fileResults {
			fileResults[i].Success = false
			fileResults[i].Error = scrubPaths(err.Error())
		}
		return nil, fileResults, err
	}

	// Sanity check 1: did each input actually produce an output file?
	// kapi may exit 0 even when a filter silently failed downstream
	// (e.g. bridge can't instantiate okf_json) and the file got
	// skipped via --no-warn. This catches the silent-skip bug class.
	written := collectOutputs(outputDir)
	missing := 0
	for i, in := range inputs {
		if hasOutputFor(written, in) {
			continue
		}
		fileResults[i].Success = false
		fileResults[i].Error = "no output written"
		missing++
	}
	// We don't fail the engine when missing > 0 — partial coverage is
	// expected (e.g. native can't handle idml). Per-file Success flags
	// already record exactly which fixtures the engine processed. Only
	// fail if the engine produced literally nothing AND wasn't told it
	// could skip; the runner will continue with the next iteration.
	_ = missing

	// Sanity check 2: does each output ACTUALLY contain pseudo-
	// translated content? kapi may write an output that's byte-identical
	// to the input (engine short-circuited pseudo, filter dropped the
	// translatable units, etc.). Without this check the timing for
	// "I read 5MB and wrote 5MB really fast" looks great but the bench
	// is meaningless. Count SCRIPT_EXT_LATIN destination runes in each
	// output; flag fixtures where input had letters but output has zero
	// pseudo runes.
	for i, in := range inputs {
		if !fileResults[i].Success {
			continue
		}
		outPath := findOutputFor(written, in)
		if outPath == "" {
			continue
		}
		letters := countLetters(in)
		vr, err := verifyPseudoOutput(outPath, letters)
		if err != nil {
			fileResults[i].VerifyNote = err.Error()
			continue
		}
		fileResults[i].PseudoChars = vr.PseudoChars
		fileResults[i].Verified = vr.Verified
		if vr.Reason != "" {
			fileResults[i].VerifyNote = vr.Reason
		}
	}

	result.OutputBytes = sumDirSize(outputDir)
	return result, fileResults, nil
}

// findOutputFor returns the first output path whose basename matches
// the input. Returns "" if none. Used by verifyPseudoOutput to locate
// the bytes to scan.
func findOutputFor(written []string, inputPath string) string {
	base := filepath.Base(inputPath)
	stem := base
	if i := strings.LastIndex(base, "."); i >= 0 {
		stem = base[:i]
	}
	for _, w := range written {
		wb := filepath.Base(w)
		if wb == base {
			return w
		}
		if strings.HasPrefix(wb, stem+"_") || strings.HasPrefix(wb, stem+".") {
			return w
		}
	}
	return ""
}

// collectOutputs returns the set of all files (with absolute paths)
// under outputDir, recursively. Used to verify that an engine actually
// wrote an output for each input fixture.
func collectOutputs(outputDir string) []string {
	var files []string
	_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Size() > 0 {
			files = append(files, path)
		}
		return nil
	})
	return files
}

// hasOutputFor returns true if any file in `written` looks like the
// pseudo-translated output of inputPath. kapi's `{name}_{lang}{ext}`
// template can collapse to a directory containing the original
// filename, so we accept any match by basename — exact path matching
// is too brittle across the kapi/bridge/native engines.
func hasOutputFor(written []string, inputPath string) bool {
	base := filepath.Base(inputPath)
	stem := base
	if i := strings.LastIndex(base, "."); i >= 0 {
		stem = base[:i]
	}
	for _, w := range written {
		wb := filepath.Base(w)
		if wb == base {
			return true
		}
		if strings.HasPrefix(wb, stem+"_") || strings.HasPrefix(wb, stem+".") {
			return true
		}
	}
	return false
}

// --- Kapi Bridge Engine (subprocess per invocation) ---

// KapiBridgeEngine runs kapi with the bridge plugin (subprocess JVM).
type KapiBridgeEngine struct {
	BinaryPath  string // path to kapi
	OkapiBridge string // path to kapi-okapi-bridge launcher (used to derive plugins dir)
	VersionStr  string
}

func (e *KapiBridgeEngine) Name() string    { return "kapi-bridge" }
func (e *KapiBridgeEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	return runKapiBatch(ctx, e.BinaryPath, files, inputDir, outputDir, traceFile, bridgeEnv(e.OkapiBridge))
}

// --- Kapi Bridge Daemon Engine ---

// KapiBridgeDaemonEngine runs kapi attached to a pre-started bridge daemon.
type KapiBridgeDaemonEngine struct {
	BinaryPath  string // path to kapi
	OkapiBridge string // path to kapi-okapi-bridge launcher (used to derive plugins dir)
	VersionStr  string
	Daemon      *DaemonProcess
}

func (e *KapiBridgeDaemonEngine) Name() string    { return "kapi-bridge-daemon" }
func (e *KapiBridgeDaemonEngine) Version() string { return e.VersionStr }

func (e *KapiBridgeDaemonEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, traceFile string) (*RunResult, []FileResult, error) {
	// Tell kapi's plugin host to attach to our pre-started daemon's Unix
	// socket instead of spawning a fresh JVM via the manifest's `daemon`
	// command. This is what makes "kapi-bridge-daemon" actually different
	// from "kapi-bridge": the JVM cold-start cost is paid once at bench
	// startup, not on every kapi invocation.
	env := append(bridgeEnv(e.OkapiBridge),
		fmt.Sprintf("KAPI_DAEMON_SOCKET_OKAPI_BRIDGE=%s", e.Daemon.SocketPath()),
	)
	return runKapiBatch(ctx, e.BinaryPath, files, inputDir, outputDir, traceFile, env)
}

// --- Okapi Pseudo Engine ---

// OkapiPseudoEngine runs Okapi's pseudo-translate pipeline via the
// `kapi-okapi-bridge pseudo --manifest <tsv>` CLI. Files are grouped
// by (filter class, fprm) and each group is processed in a single
// JVM invocation: one cold-start per group, all files in one batch.
//
// This replaces the previous tikal-based engine — same upstream Okapi
// pipeline (RawDocumentToFilterEventsStep → TextModificationStep with
// SCRIPT_EXT_LATIN → FilterEventsToRawDocumentStep) but routed through
// our maintained bridge launcher instead of the legacy tikal jar that
// shipped without batch support per filter.
type OkapiPseudoEngine struct {
	BinaryPath string // kapi-okapi-bridge launcher (jpackage native)
	VersionStr string
}

func (e *OkapiPseudoEngine) Name() string    { return "okapi" }
func (e *OkapiPseudoEngine) Version() string { return e.VersionStr }

// pseudoGroupKey shards files into (filter class, fprm) buckets so each
// JVM invocation can pin a single --filter and one shared --fprm.
type pseudoGroupKey struct {
	FilterClass string
	Fprm        string
}

func (e *OkapiPseudoEngine) ProcessBatch(ctx context.Context, files []TestFile, inputDir, outputDir, _ string) (*RunResult, []FileResult, error) {
	fileResults := make([]FileResult, len(files))

	// Group files by (filter class, fprm). Files lacking a FilterClass
	// (legacy hand-curated lists) are skipped with an explicit error so
	// the report shows what the engine couldn't run.
	type indexed struct {
		i int
		f TestFile
	}
	groups := map[pseudoGroupKey][]indexed{}
	for i, f := range files {
		fileResults[i] = FileResult{Name: f.Name, Format: f.Format, Success: true}
		if f.FilterClass == "" {
			fileResults[i].Success = false
			fileResults[i].Error = "no filter class — fixture not in parity corpus"
			continue
		}
		key := pseudoGroupKey{FilterClass: f.FilterClass, Fprm: f.OkapiFprm}
		groups[key] = append(groups[key], indexed{i: i, f: f})
	}

	if len(groups) == 0 {
		return &RunResult{}, fileResults, nil
	}

	tmpDir, err := os.MkdirTemp("", "pseudobench-okapi-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Aggregate timings: wall = sum of group wall times (sequential),
	// peak RSS = max across groups. Each group is one JVM cold-start
	// + one PipelineDriver.processBatch() invocation.
	agg := &RunResult{}
	groupNo := 0
	for key, items := range groups {
		groupNo++

		// Stage outputs in a per-group sub-tmpdir so a basename clash
		// between formats (rare but possible) doesn't overwrite.
		groupOutDir := filepath.Join(outputDir, fmt.Sprintf("%s-%d", strings.ReplaceAll(key.FilterClass, "okf_", ""), groupNo))
		if err := os.MkdirAll(groupOutDir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("mkdir group out: %w", err)
		}

		// Build manifest: <input>\t<output> per line.
		var manifest strings.Builder
		for _, it := range items {
			src := inputPathFor(it.f, inputDir)
			dst := filepath.Join(groupOutDir, it.f.Name)
			fmt.Fprintf(&manifest, "%s\t%s\n", src, dst)
		}
		manifestPath := filepath.Join(tmpDir, fmt.Sprintf("manifest-%d.tsv", groupNo))
		if err := os.WriteFile(manifestPath, []byte(manifest.String()), 0o644); err != nil {
			return nil, nil, fmt.Errorf("write manifest: %w", err)
		}

		args := []string{
			"pseudo",
			"--filter", key.FilterClass,
			"--manifest", manifestPath,
			"--src-lang", "en",
			"--tgt-lang", "qps",
		}
		if key.Fprm != "" {
			args = append(args, "--fprm", key.Fprm)
		}

		result, runErr := runProcess(ctx, nil, e.BinaryPath, args...)
		if runErr != nil {
			for _, it := range items {
				fileResults[it.i].Success = false
				fileResults[it.i].Error = scrubPaths(runErr.Error())
			}
			continue
		}

		agg.WallTime += result.WallTime
		agg.UserCPU += result.UserCPU
		agg.SystemCPU += result.SystemCPU
		if result.PeakRSSKB > agg.PeakRSSKB {
			agg.PeakRSSKB = result.PeakRSSKB
		}
	}

	// Per-file pseudo-output verification (same as runKapiBatch).
	written := collectOutputs(outputDir)
	for i, f := range files {
		if !fileResults[i].Success {
			continue
		}
		in := inputPathFor(f, inputDir)
		outPath := findOutputFor(written, in)
		if outPath == "" {
			continue
		}
		letters := countLetters(in)
		vr, err := verifyPseudoOutput(outPath, letters)
		if err != nil {
			fileResults[i].VerifyNote = err.Error()
			continue
		}
		fileResults[i].PseudoChars = vr.PseudoChars
		fileResults[i].Verified = vr.Verified
		if vr.Reason != "" {
			fileResults[i].VerifyNote = vr.Reason
		}
	}

	agg.OutputBytes = sumDirSize(outputDir)
	return agg, fileResults, nil
}

// --- Per-file processing (for timeline trace) ---

// FileProcessor processes a single file. Used for per-file timing trace.
type FileProcessor interface {
	ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error)
}

func (e *KapiNativeEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"--output-format", "json", "pseudo-translate",
		inputPathFor(file, inputDir), "--target-lang", "qps",
		"--json", "-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	stdout, result, err := runProcessWithOutput(ctx, nativeOnlyEnv(), e.BinaryPath, args...)
	if err != nil {
		return nil, err
	}
	result.BlockCount = parseBlockCount(stdout)
	return result, nil
}

func (e *KapiBridgeEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"pseudo-translate",
		inputPathFor(file, inputDir), "--target-lang", "qps", "-q",
		"-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	return runProcess(ctx, bridgeEnv(e.OkapiBridge), e.BinaryPath, args...)
}

func (e *KapiBridgeDaemonEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	args := []string{"pseudo-translate",
		inputPathFor(file, inputDir), "--target-lang", "qps", "-q",
		"-o", filepath.Join(outputDir, "{name}_{lang}{ext}")}
	env := append(bridgeEnv(e.OkapiBridge),
		fmt.Sprintf("KAPI_DAEMON_SOCKET_OKAPI_BRIDGE=%s", e.Daemon.SocketPath()),
	)
	return runProcess(ctx, env, e.BinaryPath, args...)
}

func (e *OkapiPseudoEngine) ProcessFile(ctx context.Context, file TestFile, inputDir, outputDir string) (*RunResult, error) {
	if file.FilterClass == "" {
		return nil, fmt.Errorf("file %s has no filter class", file.Name)
	}
	src := inputPathFor(file, inputDir)
	dst := filepath.Join(outputDir, file.Name)
	args := []string{
		"pseudo",
		"--filter", file.FilterClass,
		"--input", src,
		"--output", dst,
		"--src-lang", "en",
		"--tgt-lang", "qps",
	}
	if file.OkapiFprm != "" {
		args = append(args, "--fprm", file.OkapiFprm)
	}
	return runProcess(ctx, nil, e.BinaryPath, args...)
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

// detectVersion tries to get the version string from a binary.
// Both kapi and kapi-okapi-bridge expose a `version` subcommand; the
// bridge prints a bare version string ("0.0.0-dev"), kapi prints JSON
// when given --json. Never invoke the binary with no args — the bridge
// launcher treats that as "start gRPC daemon" and blocks forever.
func detectVersion(binPath string) string {
	// kapi-style first: `binary version --json` → {"version": "..."}
	if out, err := runCommand(binPath, "version", "--json"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, `"version"`) {
				if _, after, ok := strings.Cut(line, ":"); ok {
					v := strings.TrimSpace(after)
					v = strings.Trim(v, `",`)
					if v != "" {
						return v
					}
				}
			}
		}
		// Bridge case: `version --json` ignores the flag and prints the
		// bare string anyway. First non-empty stripped line wins.
		for _, line := range strings.Split(out, "\n") {
			clean := strings.TrimSpace(stripANSI(line))
			if clean != "" && !strings.HasPrefix(clean, "[") {
				return clean
			}
		}
	}

	// Last resort: `binary version` with no flag.
	if out, err := runCommand(binPath, "version"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			clean := strings.TrimSpace(stripANSI(line))
			if clean != "" && !strings.HasPrefix(clean, "[") {
				return clean
			}
		}
	}

	return filepath.Base(binPath)
}

// scrubPaths removes absolute paths from anything that reaches the committed
// dataset.
//
// The results are checked in, and scripts/check-abs-paths.sh holds every
// tracked file to zero of them: /Users/<name>/... resolves on exactly one
// laptop. Error strings are the way they get in, because a failing engine
// reports the binary and the fixture by full path, and a run where nothing
// fails carries none — so the guard stays quiet until the first bad run and
// then fails a build that looks unrelated.
func scrubPaths(msg string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		msg = strings.ReplaceAll(msg, home, "~")
	}
	return absPathPattern.ReplaceAllString(msg, "<path>")
}

// absPathPattern matches the absolute paths that survive the home-directory
// substitution: a temp dir, a system root, another checkout.
var absPathPattern = regexp.MustCompile(`(?:/(?:Users|home|private|var|tmp|opt)/)[^\s"']*`)
