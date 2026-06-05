//go:build parity

package parity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// Outcome is one row written to the parity report.
type Outcome struct {
	// Kind discriminates filter / step / other.
	Kind string `json:"kind"`

	// ID identifies the row (filter class for formats, step class for steps).
	ID string `json:"id"`

	// Name is the human-readable name (test name).
	Name string `json:"name"`

	// Status is "pass" | "fail" | "skip" | "error".
	Status string `json:"status"`

	// Mode is "head-to-head" | "bridge-only" | "byte" — describes
	// what the test actually verified.
	Mode string `json:"mode,omitempty"`

	// Detail is a short failure or skip reason. Empty on pass.
	Detail string `json:"detail,omitempty"`

	// DurationMS is the test duration in milliseconds.
	DurationMS int64 `json:"duration_ms"`

	// Timestamp is when the test ran (RFC 3339).
	Timestamp string `json:"timestamp"`
}

var (
	reportMu sync.Mutex
	report   []Outcome
)

// Report records one row in the parity report. Tests typically call this
// from a t.Cleanup so the row reflects the post-test state. Calling
// Report twice with the same Kind+ID overwrites.
func Report(t *testing.T, o Outcome) {
	t.Helper()
	if o.Timestamp == "" {
		o.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if o.Status == "" {
		o.Status = statusFromTest(t)
	}
	reportMu.Lock()
	defer reportMu.Unlock()
	for i, existing := range report {
		if existing.Kind == o.Kind && existing.ID == o.ID {
			report[i] = o
			return
		}
	}
	report = append(report, o)
}

// FlushReport writes the accumulated rows to the path given by
// $KAPI_PARITY_REPORT (default: $KAPI_PARITY_SANDBOX/test-comparison.json).
// Called from TestMain in each parity test package after m.Run().
func FlushReport() error {
	reportMu.Lock()
	defer reportMu.Unlock()
	if len(report) == 0 {
		return nil
	}

	dest := os.Getenv("KAPI_PARITY_REPORT")
	if dest == "" {
		root := os.Getenv("KAPI_PARITY_SANDBOX")
		if root == "" {
			return errors.New("FlushReport: neither $KAPI_PARITY_REPORT nor $KAPI_PARITY_SANDBOX is set")
		}
		dest = filepath.Join(root, "test-comparison.json")
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("FlushReport: mkdir %s: %w", filepath.Dir(dest), err)
	}

	// Merge with any existing report so multiple `go test ./...`
	// packages each contribute. We key on Kind+ID; later writers win.
	existing := loadExisting(dest)
	merged := mergeOutcomes(existing, report)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Kind != merged[j].Kind {
			return merged[i].Kind < merged[j].Kind
		}
		return merged[i].ID < merged[j].ID
	})

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("FlushReport: marshal: %w", err)
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("FlushReport: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("FlushReport: rename %s: %w", dest, err)
	}
	report = report[:0]
	return nil
}

func loadExisting(path string) []Outcome {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rows []Outcome
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil
	}
	return rows
}

func mergeOutcomes(existing, fresh []Outcome) []Outcome {
	idx := map[string]int{}
	for i, o := range existing {
		idx[o.Kind+"|"+o.ID] = i
	}
	for _, o := range fresh {
		if i, ok := idx[o.Kind+"|"+o.ID]; ok {
			existing[i] = o
			continue
		}
		idx[o.Kind+"|"+o.ID] = len(existing)
		existing = append(existing, o)
	}
	return existing
}

func statusFromTest(t *testing.T) string {
	switch {
	case t.Skipped():
		return "skip"
	case t.Failed():
		return "fail"
	default:
		return "pass"
	}
}
