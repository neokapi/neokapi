//go:build parity

package parity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var (
	tikalOnce  sync.Once
	tikalPath  string
	tikalErr   error
	tikalProbe sync.Once
)

// TikalAvailable reports whether a Tikal CLI is reachable. The lookup
// honours $OKAPI_TIKAL (a direct path to the launcher) and
// $OKAPI_HOME/tikal.sh; otherwise PATH is searched.
func TikalAvailable() (string, error) {
	tikalOnce.Do(func() {
		if explicit := os.Getenv("OKAPI_TIKAL"); explicit != "" {
			abs, err := filepath.Abs(explicit)
			if err != nil {
				tikalErr = fmt.Errorf("resolve OKAPI_TIKAL %q: %w", explicit, err)
				return
			}
			if _, err := os.Stat(abs); err != nil {
				tikalErr = fmt.Errorf("OKAPI_TIKAL %s: %w", abs, err)
				return
			}
			tikalPath = abs
			return
		}
		if home := os.Getenv("OKAPI_HOME"); home != "" {
			candidate := filepath.Join(home, "tikal.sh")
			if _, err := os.Stat(candidate); err == nil {
				tikalPath = candidate
				return
			}
		}
		if found, err := exec.LookPath("tikal"); err == nil {
			tikalPath = found
			return
		}
		if found, err := exec.LookPath("tikal.sh"); err == nil {
			tikalPath = found
			return
		}
		tikalErr = errors.New("tikal CLI not found — set $OKAPI_TIKAL or $OKAPI_HOME, or place tikal on PATH")
	})
	return tikalPath, tikalErr
}

// RequireTikal SkipNow's the test if Tikal is unavailable. Most parity
// tests use this rather than failing — Tikal is only needed for the
// byte-level harness, and not every CI runner has the upstream Okapi
// distro installed.
func RequireTikal(t *testing.T) string {
	t.Helper()
	path, err := TikalAvailable()
	if err != nil {
		t.Skipf("Tikal CLI unavailable: %v", err)
	}
	return path
}

// TikalRoundTripRequest configures a Tikal extract+merge cycle.
type TikalRoundTripRequest struct {
	// InputBytes is the source document. The harness writes it under
	// t.TempDir() with Filename and runs Tikal against the path.
	InputBytes []byte

	// Filename is the document's basename — its extension drives
	// Tikal's filter detection.
	Filename string

	// SourceLocale defaults to "en" when empty.
	SourceLocale string
	// TargetLocale defaults to "fr" when empty.
	TargetLocale string

	// ExtraArgs is appended to the extract invocation (e.g.
	// "-fc okf_html@my-config" to pin a filter configuration).
	ExtraArgs []string
}

// TikalRoundTripResult captures the merged output and the intermediate
// XLIFF Tikal produced during extract.
type TikalRoundTripResult struct {
	XLIFF    []byte
	MergedTo string
	Output   []byte
}

// RunTikalRoundTrip performs `tikal -x` then `tikal -m` and returns the
// merged output bytes. Any Tikal stderr is included in failure messages.
func RunTikalRoundTrip(t *testing.T, req TikalRoundTripRequest) TikalRoundTripResult {
	t.Helper()
	path := RequireTikal(t)
	if len(req.InputBytes) == 0 {
		t.Fatal("RunTikalRoundTrip: InputBytes is required")
	}
	if req.Filename == "" {
		t.Fatal("RunTikalRoundTrip: Filename is required (extension drives filter detection)")
	}

	src := defaultStr(req.SourceLocale, "en")
	tgt := defaultStr(req.TargetLocale, "fr")
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, req.Filename)
	if err := os.WriteFile(inputPath, req.InputBytes, 0o644); err != nil {
		t.Fatalf("RunTikalRoundTrip: write input: %v", err)
	}

	extractArgs := []string{"-x", inputPath, "-sl", src, "-tl", tgt}
	extractArgs = append(extractArgs, req.ExtraArgs...)

	if out, err := runWithTimeout(path, extractArgs, 60*time.Second); err != nil {
		t.Fatalf("tikal -x failed: %v\n%s", err, out)
	}

	xliffPath := inputPath + ".xlf"
	xliffData, err := os.ReadFile(xliffPath)
	if err != nil {
		t.Fatalf("RunTikalRoundTrip: read XLIFF %s: %v", xliffPath, err)
	}

	outDir := filepath.Join(tmpDir, "merged")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("RunTikalRoundTrip: mkdir merged: %v", err)
	}

	if out, err := runWithTimeout(path, []string{"-m", xliffPath, "-od", outDir}, 60*time.Second); err != nil {
		t.Fatalf("tikal -m failed: %v\n%s", err, out)
	}

	mergedPath := filepath.Join(outDir, req.Filename)
	mergedData, err := os.ReadFile(mergedPath)
	if err != nil {
		t.Fatalf("RunTikalRoundTrip: read merged %s: %v", mergedPath, err)
	}
	return TikalRoundTripResult{XLIFF: xliffData, MergedTo: mergedPath, Output: mergedData}
}

// runWithTimeout runs a subprocess and returns its combined output. The
// command is killed if it exceeds the timeout.
func runWithTimeout(name string, args []string, d time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	tikalProbe.Do(func() {})
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
