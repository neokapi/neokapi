// Command testcompare reads .parity/test-comparison.json (the raw
// parity report written by the cli/parity/ test packages) and emits
// the docs-site shape at web/static/data/parity-report.json.
//
// The published shape is intentionally narrower than the raw report —
// it includes only what the /parity dashboard page renders:
//
//	{
//	  "generated_at": "2026-04-29T...",
//	  "totals": {
//	    "format": {"pass": 51, "fail": 1, "skip": 0, "total": 52},
//	    "step":   {"pass": 119, "fail": 0, "skip": 3, "total": 122}
//	  },
//	  "rows": [
//	    {"kind": "format", "id": "okf_html", "status": "pass", "mode": "head-to-head", "duration_ms": 412, "detail": ""},
//	    ...
//	  ]
//	}
//
// The output path is deliberately separate from the legacy
// /test-comparison page's data file so the two dashboards can coexist
// during the transition.
//
// Usage:
//
//	go run ./scripts/testcompare \
//	    -in .parity/test-comparison.json \
//	    -out web/static/data/parity-report.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type rawRow struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Mode       string `json:"mode,omitempty"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Timestamp  string `json:"timestamp"`
}

type publishedRow struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Status     string `json:"status"`
	Mode       string `json:"mode,omitempty"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type totals struct {
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
	Error int `json:"error"`
	Total int `json:"total"`
}

type published struct {
	GeneratedAt string             `json:"generated_at"`
	Totals      map[string]*totals `json:"totals"`
	Rows        []publishedRow     `json:"rows"`
}

func main() {
	in := flag.String("in", ".parity/test-comparison.json", "input parity report path")
	out := flag.String("out", "web/static/data/parity-report.json", "output dashboard JSON path")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcompare: read %s: %v\n", *in, err)
		os.Exit(1)
	}
	var rows []rawRow
	if err := json.Unmarshal(data, &rows); err != nil {
		fmt.Fprintf(os.Stderr, "testcompare: parse %s: %v\n", *in, err)
		os.Exit(1)
	}

	out_, totals_ := transform(rows)
	doc := published{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Totals:      totals_,
		Rows:        out_,
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcompare: marshal: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "testcompare: mkdir %s: %v\n", filepath.Dir(*out), err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "testcompare: write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "testcompare: %d rows → %s\n", len(rows), *out)
}

// transform reduces the raw rows to the published shape and computes
// per-kind totals. Rows are sorted by (kind, id) so the dashboard
// output is deterministic across runs.
func transform(rows []rawRow) ([]publishedRow, map[string]*totals) {
	out := make([]publishedRow, 0, len(rows))
	tot := map[string]*totals{}
	for _, r := range rows {
		out = append(out, publishedRow{
			Kind:       r.Kind,
			ID:         r.ID,
			Status:     r.Status,
			Mode:       r.Mode,
			Detail:     r.Detail,
			DurationMS: r.DurationMS,
		})
		t, ok := tot[r.Kind]
		if !ok {
			t = &totals{}
			tot[r.Kind] = t
		}
		t.Total++
		switch r.Status {
		case "pass":
			t.Pass++
		case "fail":
			t.Fail++
		case "skip":
			t.Skip++
		case "error":
			t.Error++
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out, tot
}
