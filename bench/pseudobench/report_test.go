package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The HTML report renders from a report that holds an experiment, with every
// field the template reads, and carries the experiment's numbers.
func TestGenerateHTMLReportRendersAnExperiment(t *testing.T) {
	report := &Report{
		Metadata: Metadata{Timestamp: "2026-11-22T00:00:00Z", Platform: "linux/amd64", GoVersion: "go1.27.4"},
		Experiments: []ExperimentResult{{
			Engine:     "kapi",
			Version:    "test",
			Iterations: 1,
			WallTimeMs: Stats{Median: 1234, Stddev: 5},
			PeakRssKB:  Stats{Max: 4321},
			FileTimings: []FileTiming{
				{Name: "a.json", Format: "json", WallMs: 10, PeakRssKB: 100, Success: true},
				{Name: "b.po", Format: "po", WallMs: 20, PeakRssKB: 200},
			},
		}},
	}
	out := filepath.Join(t.TempDir(), "report.html")
	if err := generateHTMLReport(report, out); err != nil {
		t.Fatalf("generateHTMLReport: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"kapi", "1234", "4321", "1/2"} {
		if !strings.Contains(html, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}
