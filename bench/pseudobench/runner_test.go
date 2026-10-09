package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeEngine answers ProcessBatch with a fixed per-file outcome, so the runner
// can be driven without a kapi binary.
type fakeEngine struct {
	name    string
	success []bool
}

func (e *fakeEngine) Name() string    { return e.name }
func (e *fakeEngine) Version() string { return "test" }

func (e *fakeEngine) ProcessBatch(_ context.Context, files []TestFile, _, _, _ string) (*RunResult, []FileResult, error) {
	results := make([]FileResult, len(files))
	for i, f := range files {
		results[i] = FileResult{Name: f.Name, Format: f.Format, Success: e.success[i]}
		if !e.success[i] {
			results[i].Error = "no output written"
		}
	}
	return &RunResult{WallTime: 5 * time.Millisecond}, results, nil
}

func benchConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		Iterations: 1,
		OutputDir:  t.TempDir(),
		Fixtures: []TestFile{
			{Name: "a.html", Format: "html"},
			{Name: "b.json", Format: "json"},
		},
	}
}

func TestRunEngines_ZeroSuccessfulFilesFailsTheRun(t *testing.T) {
	engine := &fakeEngine{name: "kapi-native", success: []bool{false, false}}

	report, err := runEngines([]Engine{engine}, benchConfig(t), nil)
	if err == nil {
		t.Fatal("a run in which no file succeeded returned a report")
	}
	if report != nil {
		t.Fatalf("a failed run must return no report, got %d experiment(s)", len(report.Experiments))
	}
	var none *NoSuccessfulFilesError
	if !errors.As(err, &none) {
		t.Fatalf("error is %T (%v), want *NoSuccessfulFilesError", err, err)
	}
	if none.Engine != "kapi-native" || none.Attempted != 2 {
		t.Fatalf("error names %q with %d attempted, want kapi-native with 2", none.Engine, none.Attempted)
	}
}

func TestRunEngines_PartialSuccessIsAResult(t *testing.T) {
	engine := &fakeEngine{name: "kapi-native", success: []bool{true, false}}

	report, err := runEngines([]Engine{engine}, benchConfig(t), nil)
	if err != nil {
		t.Fatalf("a run with one successful file failed: %v", err)
	}
	if len(report.Experiments) != 1 {
		t.Fatalf("got %d experiment(s), want 1", len(report.Experiments))
	}
	exp := report.Experiments[0]
	if exp.FilesSucceeded != 1 || exp.FilesAttempted != 2 {
		t.Fatalf("recorded %d/%d successful, want 1/2", exp.FilesSucceeded, exp.FilesAttempted)
	}
}

// A later engine that produced nothing fails the run even when an earlier one
// succeeded: the dataset is a comparison, and a comparison missing an engine
// that silently did nothing would publish the wrong story.
func TestRunEngines_AnyEngineWithoutSuccessFailsTheRun(t *testing.T) {
	good := &fakeEngine{name: "okapi", success: []bool{true, true}}
	empty := &fakeEngine{name: "kapi-bridge", success: []bool{false, false}}

	report, err := runEngines([]Engine{good, empty}, benchConfig(t), nil)
	if err == nil || report != nil {
		t.Fatalf("got report=%v err=%v, want no report and an error", report != nil, err)
	}
	var none *NoSuccessfulFilesError
	if !errors.As(err, &none) || none.Engine != "kapi-bridge" {
		t.Fatalf("error is %v, want *NoSuccessfulFilesError naming kapi-bridge", err)
	}
}
