// Package main implements a CLI that parses Okapi surefire XML reports and Go
// test JSON output, then merges them into a single JSON file for the test
// comparison dashboard.
package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// --- Surefire XML structures ---

type xmlTestSuite struct {
	Name      string         `xml:"name,attr"`
	Tests     int            `xml:"tests,attr"`
	Errors    int            `xml:"errors,attr"`
	Skipped   int            `xml:"skipped,attr"`
	Failures  int            `xml:"failures,attr"`
	Time      float64        `xml:"time,attr"`
	TestCases []xmlTestCase  `xml:"testcase"`
}

type xmlTestCase struct {
	Name      string     `xml:"name,attr"`
	ClassName string     `xml:"classname,attr"`
	Time      float64    `xml:"time,attr"`
	Failure   *xmlMarker `xml:"failure"`
	Error     *xmlMarker `xml:"error"`
	Skipped   *xmlMarker `xml:"skipped"`
}

type xmlMarker struct {
	Message string `xml:"message,attr"`
}

// --- Go test JSON event ---

type goTestEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Elapsed float64 `json:"Elapsed"`
}

// --- Output JSON ---

type ComparisonData struct {
	GeneratedAt   string             `json:"generatedAt"`
	OkapiVersion  string             `json:"okapiVersion"`
	GokapiVersion string             `json:"gokapiVersion"`
	Filters       []FilterComparison `json:"filters"`
	Summary       Summary            `json:"summary"`
}

type Summary struct {
	TotalFiltersOkapi  int `json:"totalFiltersOkapi"`
	TotalFiltersGokapi int `json:"totalFiltersGokapi"`
	TotalFiltersBoth   int `json:"totalFiltersBoth"`
	TotalTestsOkapi    int `json:"totalTestsOkapi"`
	TotalTestsGokapi   int `json:"totalTestsGokapi"`
}

type FilterComparison struct {
	FilterName string        `json:"filterName"`
	Okapi      *FilterResult `json:"okapi"`
	Gokapi     *FilterResult `json:"gokapi"`
}

type FilterResult struct {
	Suites  []Suite `json:"suites"`
	Total   int     `json:"total"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Skipped int     `json:"skipped"`
	Errors  int     `json:"errors"`
}

type Suite struct {
	Name       string  `json:"name"`
	Tests      []Test  `json:"tests"`
	Total      int     `json:"total"`
	Passed     int     `json:"passed"`
	Failed     int     `json:"failed"`
	Skipped    int     `json:"skipped"`
	Errors     int     `json:"errors"`
	DurationMs float64 `json:"durationMs"`
}

type Test struct {
	Name       string  `json:"name"`
	ClassName  string  `json:"className,omitempty"`
	Status     string  `json:"status"`
	DurationMs float64 `json:"durationMs"`
}

func main() {
	okapiDir := flag.String("okapi-dir", "", "path to Okapi filters directory")
	gotestJSON := flag.String("gotest-json", "", "path to go test -json JSONL file")
	outFile := flag.String("out", "", "output JSON file path")
	okapiVer := flag.String("okapi-version", "", "Okapi version label")
	gokapiVer := flag.String("gokapi-version", "", "gokapi version label")
	flag.Parse()

	if *okapiDir == "" || *outFile == "" {
		fmt.Fprintln(os.Stderr, "Usage: testcompare -okapi-dir DIR -out FILE [-gotest-json FILE]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	okapi := parseSurefire(*okapiDir)

	var gokapi map[string]*FilterResult
	if *gotestJSON != "" {
		gokapi = parseGoTest(*gotestJSON)
	}

	data := merge(okapi, gokapi, *okapiVer, *gokapiVer)

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	if dir := filepath.Dir(*outFile); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("mkdir: %v", err)
		}
	}
	if err := os.WriteFile(*outFile, b, 0o644); err != nil {
		log.Fatalf("write %s: %v", *outFile, err)
	}
	fmt.Printf("wrote %s (%d filters)\n", *outFile, len(data.Filters))
}

func parseSurefire(dir string) map[string]*FilterResult {
	out := map[string]*FilterResult{}

	matches, err := filepath.Glob(filepath.Join(dir, "*", "target", "surefire-reports", "TEST-*.xml"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}

	for _, path := range matches {
		rel, _ := filepath.Rel(dir, path)
		filter := strings.SplitN(rel, string(filepath.Separator), 2)[0]

		raw, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warn: %v", err)
			continue
		}

		var xs xmlTestSuite
		if err := xml.Unmarshal(raw, &xs); err != nil {
			log.Printf("warn: parse %s: %v", path, err)
			continue
		}

		s := Suite{
			Name:       xs.Name,
			DurationMs: xs.Time * 1000,
		}

		for _, tc := range xs.TestCases {
			st := "pass"
			switch {
			case tc.Error != nil:
				st = "error"
			case tc.Failure != nil:
				st = "fail"
			case tc.Skipped != nil:
				st = "skip"
			}
			s.Tests = append(s.Tests, Test{
				Name:       tc.Name,
				ClassName:  tc.ClassName,
				Status:     st,
				DurationMs: tc.Time * 1000,
			})
			s.Total++
			switch st {
			case "pass":
				s.Passed++
			case "fail":
				s.Failed++
			case "skip":
				s.Skipped++
			case "error":
				s.Errors++
			}
		}

		fr := out[filter]
		if fr == nil {
			fr = &FilterResult{}
			out[filter] = fr
		}
		fr.Suites = append(fr.Suites, s)
		fr.Total += s.Total
		fr.Passed += s.Passed
		fr.Failed += s.Failed
		fr.Skipped += s.Skipped
		fr.Errors += s.Errors
	}

	return out
}

func parseGoTest(path string) map[string]*FilterResult {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	type testResult struct {
		name    string
		status  string
		elapsed float64
	}
	pkgs := map[string][]testResult{}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)

	for sc.Scan() {
		var ev goTestEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Test == "" {
			continue
		}
		var st string
		switch ev.Action {
		case "pass":
			st = "pass"
		case "fail":
			st = "fail"
		case "skip":
			st = "skip"
		default:
			continue
		}
		pkgs[ev.Package] = append(pkgs[ev.Package], testResult{ev.Test, st, ev.Elapsed})
	}

	if err := sc.Err(); err != nil {
		log.Fatalf("scan: %v", err)
	}

	out := map[string]*FilterResult{}
	for pkg, tests := range pkgs {
		filter := filterFromPkg(pkg)
		if filter == "" {
			continue
		}

		s := Suite{Name: lastSegment(pkg)}
		for _, t := range tests {
			s.Tests = append(s.Tests, Test{
				Name:       t.name,
				Status:     t.status,
				DurationMs: t.elapsed * 1000,
			})
			s.Total++
			s.DurationMs += t.elapsed * 1000
			switch t.status {
			case "pass":
				s.Passed++
			case "fail":
				s.Failed++
			case "skip":
				s.Skipped++
			}
		}

		fr := out[filter]
		if fr == nil {
			fr = &FilterResult{}
			out[filter] = fr
		}
		fr.Suites = append(fr.Suites, s)
		fr.Total += s.Total
		fr.Passed += s.Passed
		fr.Failed += s.Failed
		fr.Skipped += s.Skipped
		fr.Errors += s.Errors
	}

	return out
}

func filterFromPkg(pkg string) string {
	seg := lastSegment(pkg)
	if !strings.HasPrefix(seg, "okf_") {
		return ""
	}
	return strings.TrimPrefix(seg, "okf_")
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func merge(okapi, gokapi map[string]*FilterResult, okapiVer, gokapiVer string) *ComparisonData {
	names := map[string]struct{}{}
	for n := range okapi {
		names[n] = struct{}{}
	}
	for n := range gokapi {
		names[n] = struct{}{}
	}

	filters := make([]FilterComparison, 0, len(names))
	for n := range names {
		fc := FilterComparison{FilterName: n, Okapi: okapi[n]}
		if gokapi != nil {
			fc.Gokapi = gokapi[n]
		}
		filters = append(filters, fc)
	}

	sort.Slice(filters, func(i, j int) bool {
		bi := filters[i].Okapi != nil && filters[i].Gokapi != nil
		bj := filters[j].Okapi != nil && filters[j].Gokapi != nil
		if bi != bj {
			return bi
		}
		return filters[i].FilterName < filters[j].FilterName
	})

	var sum Summary
	for _, fc := range filters {
		if fc.Okapi != nil {
			sum.TotalFiltersOkapi++
			sum.TotalTestsOkapi += fc.Okapi.Total
		}
		if fc.Gokapi != nil {
			sum.TotalFiltersGokapi++
			sum.TotalTestsGokapi += fc.Gokapi.Total
		}
		if fc.Okapi != nil && fc.Gokapi != nil {
			sum.TotalFiltersBoth++
		}
	}

	return &ComparisonData{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		OkapiVersion:  okapiVer,
		GokapiVersion: gokapiVer,
		Filters:       filters,
		Summary:       sum,
	}
}
