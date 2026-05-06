//go:build parity

package roundtrip

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/parity"
)

// OkapiBatchCache holds per-fixture pseudo-translated bytes produced
// by a single `kapi-okapi-bridge pseudo --manifest ...` invocation. It
// lets OkapiEngine.RoundTrip return cached bytes in O(1) instead of
// shelling out to a fresh JVM per fixture — the per-fixture cost
// dominates the parity suite at scale (~840 fixtures × JVM cold-start
// blew the 30m CI budget; one batch per format is ~30 cold starts
// total).
//
// Outputs are keyed by absolute input path. Errors are recorded the
// same way: when the batch fails for an entry (driver exception,
// missing output file, empty output) the failure is stored and
// surfaced by RoundTrip via t.Fatal so the test still fails cleanly.
type OkapiBatchCache struct {
	mu      sync.RWMutex
	outputs map[string][]byte
	errors  map[string]string
}

// Get returns the cached output for inputPath, if any.
func (c *OkapiBatchCache) Get(inputPath string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out, ok := c.outputs[inputPath]
	return out, ok
}

// GetError returns the recorded batch error for inputPath, if any.
func (c *OkapiBatchCache) GetError(inputPath string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.errors[inputPath]
	return e, ok
}

// RunOkapiBatch invokes `kapi-okapi-bridge pseudo --manifest` once for
// every fixture in inputs, sharing one JVM startup across the whole
// batch. Outputs land in a t.TempDir; the returned cache maps each
// input absolute path to its merged bytes (or to a failure reason).
//
// inputs MUST be absolute paths. The bridge's filter resolves
// companion files (DTD subsets, ITS rules, …) from the input's parent
// directory on disk — staging them in tmpDir is only required when
// the harness does single-call subprocess RoundTrips.
//
// When filterClass is empty (some test scans only exercise the native
// engine) the function returns an empty cache and the harness falls
// back to the per-fixture subprocess path inside OkapiEngine.RoundTrip.
func RunOkapiBatch(t *testing.T, filterClass, paramConfig, srcLang, tgtLang string, inputs []string) *OkapiBatchCache {
	t.Helper()
	cache := &OkapiBatchCache{
		outputs: map[string][]byte{},
		errors:  map[string]string{},
	}
	if filterClass == "" || len(inputs) == 0 {
		return cache
	}
	if srcLang == "" {
		srcLang = "en"
	}
	if tgtLang == "" {
		tgtLang = "fr"
	}

	s, err := parity.LoadSandbox()
	if err != nil {
		t.Fatalf("RunOkapiBatch: %v", err)
	}

	tmpDir := t.TempDir()

	// Build manifest. Output basenames are prefixed so collisions
	// between fixtures of the same name in different source dirs
	// stay distinct.
	var manifest strings.Builder
	outputPaths := make(map[string]string, len(inputs))
	for i, in := range inputs {
		out := filepath.Join(tmpDir, fmt.Sprintf("%04d-%s", i, filepath.Base(in)))
		outputPaths[in] = out
		fmt.Fprintf(&manifest, "%s\t%s\n", in, out)
	}
	manifestPath := filepath.Join(tmpDir, "manifest.tsv")
	if err := os.WriteFile(manifestPath, []byte(manifest.String()), 0o644); err != nil {
		t.Fatalf("RunOkapiBatch: write manifest: %v", err)
	}

	args := []string{
		"pseudo",
		"--filter", filterClass,
		"--manifest", manifestPath,
		"--src-lang", srcLang,
		"--tgt-lang", tgtLang,
	}
	if paramConfig != "" {
		args = append(args, "--fprm", paramConfig)
	}

	// 5-minute ceiling for one format's batch — openxml's 353-fixture
	// run is the worst case; everything else fits comfortably.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.OkapiBridgeBinary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	out, runErr := cmd.CombinedOutput()

	// Read whichever outputs landed; record an error when one didn't.
	// A non-zero exit doesn't necessarily mean every fixture failed —
	// the driver processes items sequentially and one bad item kills
	// the rest, but the prior items' outputs are already on disk.
	for in, outPath := range outputPaths {
		bytes, readErr := os.ReadFile(outPath)
		if readErr != nil {
			cache.errors[in] = fmt.Sprintf("batch produced no output: %v", readErr)
			continue
		}
		if len(bytes) == 0 {
			cache.errors[in] = "batch produced empty output"
			continue
		}
		cache.outputs[in] = bytes
	}

	// If the batch exited non-zero AND we have inputs with no output,
	// attribute the runner-level failure to those inputs so RoundTrip
	// surfaces it. Fixtures whose output landed are considered fine —
	// the failure was downstream of them in the driver loop.
	if runErr != nil {
		for in := range outputPaths {
			if _, ok := cache.outputs[in]; ok {
				continue
			}
			if existing, ok := cache.errors[in]; ok {
				cache.errors[in] = existing + "\n--- batch exit: " + runErr.Error() + "\n--- output:\n" + string(out)
			}
		}
	}

	return cache
}
