// Package main implements a CLI that parses Okapi surefire XML reports and Go
// test JSON output, then merges them into a single JSON file for the test
// comparison dashboard. It also parses // okapi: annotations from Go source
// files to build per-test-case mappings between Java and Go tests.
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
	"regexp"
	"sort"
	"strings"
	"time"
)

// --- Surefire XML structures ---

type xmlTestSuite struct {
	Name      string        `xml:"name,attr"`
	Tests     int           `xml:"tests,attr"`
	Errors    int           `xml:"errors,attr"`
	Skipped   int           `xml:"skipped,attr"`
	Failures  int           `xml:"failures,attr"`
	Time      float64       `xml:"time,attr"`
	TestCases []xmlTestCase `xml:"testcase"`
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
	Output  string  `json:"Output"`
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
	TotalFiltersOkapi  int     `json:"totalFiltersOkapi"`
	TotalFiltersBridge int     `json:"totalFiltersBridge"`
	TotalFiltersNative int     `json:"totalFiltersNative"`
	TotalFiltersBoth   int     `json:"totalFiltersBoth"`
	TotalTestsOkapi    int     `json:"totalTestsOkapi"`
	TotalTestsBridge   int     `json:"totalTestsBridge"`
	TotalTestsNative   int     `json:"totalTestsNative"`
	CoveragePct        float64 `json:"coveragePct"`
}

type FilterComparison struct {
	FilterName string          `json:"filterName"`
	Okapi      *FilterResult   `json:"okapi"`
	Bridge     *FilterResult   `json:"bridge"`
	Native     *FilterResult   `json:"native"`
	TestCases  []TestCaseMatch `json:"testCases"`
	Coverage   CoverageStats   `json:"coverage"`
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

type TestCaseMatch struct {
	JavaClass    string `json:"javaClass"`
	JavaMethod   string `json:"javaMethod"`
	OkapiStatus  string `json:"okapiStatus"`
	BridgeTest   string `json:"bridgeTest,omitempty"`
	BridgeStatus string `json:"bridgeStatus"`
	NativeTest   string `json:"nativeTest,omitempty"`
	NativeStatus string `json:"nativeStatus"`
	SkipReason   string `json:"skipReason,omitempty"`
	TestState    string `json:"testState"` // "implemented" | "pending" | "skipped" | "unmapped"
}

type CoverageStats struct {
	TotalOkapi     int     `json:"totalOkapi"`
	BridgeMapped   int     `json:"bridgeMapped"`
	BridgePassing  int     `json:"bridgePassing"`
	NativeMapped   int     `json:"nativeMapped"`
	NativePassing  int     `json:"nativePassing"`
	CoveragePct    float64 `json:"coveragePct"`
	SkippedCount   int     `json:"skippedCount"`
	PendingCount   int     `json:"pendingCount"`
	ImplementedPct float64 `json:"implementedPct"`
}

// annotation maps a Go test function to its Java test counterpart.
type annotation struct {
	JavaClass  string
	JavaMethod string
	GoTest     string
	Filter     string // normalized filter name (e.g. "html", "json")
}

// skipAnnotation marks a Java test as not applicable to Go.
type skipAnnotation struct {
	JavaClass  string
	JavaMethod string
	Reason     string
	Filter     string
}

var annotationRe = regexp.MustCompile(`^//\s*okapi:\s+(\w+)#(\w+)\s*$`)
var skipAnnotRe = regexp.MustCompile(`^//\s*okapi-skip:\s+(\w+)#(\w+)\s*[\x{2014}\x{2013}\-]\s*(.+)$`)
var funcTestRe = regexp.MustCompile(`^func\s+(Test\w+)\s*\(`)

func main() {
	okapiDir := flag.String("okapi-dir", "", "path to Okapi filters directory")
	// New flags
	bridgeJSON := flag.String("gotest-bridge-json", "", "path to bridge go test -json JSONL file")
	nativeJSON := flag.String("gotest-native-json", "", "path to native go test -json JSONL file")
	bridgeSrc := flag.String("bridge-src", "", "path to bridge test source (e.g. core/plugin/bridge/filters)")
	nativeSrc := flag.String("native-src", "", "path to native format source (e.g. core/formats)")
	// Legacy flag alias
	gotestJSON := flag.String("gotest-json", "", "alias for -gotest-bridge-json (deprecated)")

	outFile := flag.String("out", "", "output JSON file path")
	okapiVer := flag.String("okapi-version", "", "Okapi version label")
	gokapiVer := flag.String("gokapi-version", "", "gokapi version label")
	flag.Parse()

	if *okapiDir == "" || *outFile == "" {
		fmt.Fprintln(os.Stderr, "Usage: testcompare -okapi-dir DIR -out FILE [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Handle legacy flag
	if *bridgeJSON == "" && *gotestJSON != "" {
		*bridgeJSON = *gotestJSON
	}

	okapi := parseSurefire(*okapiDir)

	var bridgeResults, nativeResults goTestResults
	if *bridgeJSON != "" {
		bridgeResults = parseGoTestResults(*bridgeJSON, bridgeFilterFromPkg)
	}
	if *nativeJSON != "" {
		nativeResults = parseGoTestResults(*nativeJSON, nativeFilterFromPkg)
	}

	// Parse annotations from source
	var bridgeAR, nativeAR annotationResult
	if *bridgeSrc != "" {
		bridgeAR = parseAnnotations(*bridgeSrc, "bridge")
	}
	if *nativeSrc != "" {
		nativeAR = parseAnnotations(*nativeSrc, "native")
	}

	// Build Go test status maps from results
	bridgeTestStatus := buildTestStatusMap(bridgeResults.filters)
	nativeTestStatus := buildTestStatusMap(nativeResults.filters)

	data := merge(okapi, bridgeResults.filters, nativeResults.filters,
		bridgeAR.annotations, nativeAR.annotations,
		bridgeAR.skips, nativeAR.skips,
		bridgeTestStatus, nativeTestStatus,
		bridgeResults.skipMsgs, nativeResults.skipMsgs,
		*okapiVer, *gokapiVer)

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

	// Support two directory layouts:
	// 1. Nested (local Okapi checkout): {dir}/{filter}/target/surefire-reports/TEST-*.xml
	// 2. Flat (fetched from release):   {dir}/{filter}/TEST-*.xml
	matches, err := filepath.Glob(filepath.Join(dir, "*", "target", "surefire-reports", "TEST-*.xml"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}
	flatMatches, err := filepath.Glob(filepath.Join(dir, "*", "TEST-*.xml"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}
	matches = append(matches, flatMatches...)

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

// goTestResults holds parsed Go test results along with skip message data.
type goTestResults struct {
	filters  map[string]*FilterResult
	skipMsgs map[string]string // "pkg/TestName" → skip output message
}

// parseGoTestResults parses Go test JSON output using a filter extraction function.
func parseGoTestResults(path string, filterFn func(string) string) goTestResults {
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

	// Capture output lines for skip detection. Key: "pkg/TestName"
	outputBuf := map[string]string{}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)

	for sc.Scan() {
		var ev goTestEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Test == "" {
			continue
		}

		// Capture output for skip message detection
		if ev.Action == "output" && ev.Output != "" {
			key := ev.Package + "/" + ev.Test
			outputBuf[key] += ev.Output
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

	// Build skip messages map keyed by "filter/TestName"
	skipMsgs := map[string]string{}
	for key, output := range outputBuf {
		pkg := key[:strings.LastIndex(key, "/")]
		testName := key[strings.LastIndex(key, "/")+1:]
		filter := filterFn(pkg)
		if filter == "" {
			continue
		}
		skipMsgs[filter+"/"+testName] = output
	}

	out := map[string]*FilterResult{}
	for pkg, tests := range pkgs {
		filter := filterFn(pkg)
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

	return goTestResults{filters: out, skipMsgs: skipMsgs}
}

// bridgeFilterFromPkg extracts the filter name from a bridge test package path.
// e.g. ".../bridge/filters/okf_html" → "html"
func bridgeFilterFromPkg(pkg string) string {
	seg := lastSegment(pkg)
	if !strings.HasPrefix(seg, "okf_") {
		return ""
	}
	return strings.TrimPrefix(seg, "okf_")
}

// nativeFilterFromPkg extracts the filter name from a native format package path.
// e.g. ".../formats/json" → "json"
func nativeFilterFromPkg(pkg string) string {
	// Check the package path contains "/formats/"
	if !strings.Contains(pkg, "/formats/") {
		return ""
	}
	return lastSegment(pkg)
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// annotationResult holds both regular and skip annotations from source scanning.
type annotationResult struct {
	annotations []annotation
	skips       []skipAnnotation
}

// parseAnnotations scans Go source files for // okapi: and // okapi-skip: annotations.
// srcDir is the root directory to scan (e.g. "core/plugin/bridge/filters" or "core/formats").
// kind is "bridge" or "native" (used for debug logging).
func parseAnnotations(srcDir, kind string) annotationResult {
	var pattern string
	switch kind {
	case "bridge":
		pattern = filepath.Join(srcDir, "okf_*", "*_test.go")
	default:
		pattern = filepath.Join(srcDir, "*", "*_test.go")
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("warn: annotation glob %s: %v", pattern, err)
		return annotationResult{}
	}

	var result annotationResult
	for _, path := range matches {
		ar := parseFileAnnotations(path, kind)
		result.annotations = append(result.annotations, ar.annotations...)
		result.skips = append(result.skips, ar.skips...)
	}

	fmt.Printf("parsed %d %s annotations + %d skips from %d files\n",
		len(result.annotations), kind, len(result.skips), len(matches))
	return result
}

// parseFileAnnotations parses a single Go test file for // okapi: and // okapi-skip: annotations.
func parseFileAnnotations(path, kind string) annotationResult {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("warn: read %s: %v", path, err)
		return annotationResult{}
	}

	filter := filterFromPath(path, kind)
	if filter == "" {
		return annotationResult{}
	}

	lines := strings.Split(string(data), "\n")
	var result annotationResult
	var pending []struct{ class, method string }

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for // okapi-skip: annotation
		if m := skipAnnotRe.FindStringSubmatch(trimmed); m != nil {
			result.skips = append(result.skips, skipAnnotation{
				JavaClass:  m[1],
				JavaMethod: m[2],
				Reason:     strings.TrimSpace(m[3]),
				Filter:     filter,
			})
			continue
		}

		// Check for // okapi: annotation
		if m := annotationRe.FindStringSubmatch(trimmed); m != nil {
			pending = append(pending, struct{ class, method string }{m[1], m[2]})
			continue
		}

		// Check for func Test...
		if m := funcTestRe.FindStringSubmatch(trimmed); m != nil && len(pending) > 0 {
			goTestName := m[1]
			for _, p := range pending {
				result.annotations = append(result.annotations, annotation{
					JavaClass:  p.class,
					JavaMethod: p.method,
					GoTest:     goTestName,
					Filter:     filter,
				})
			}
			pending = nil
			continue
		}

		// Non-annotation, non-func line clears pending annotations
		// (only if it's not blank or another comment)
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			pending = nil
		}
	}

	return result
}

// filterFromPath extracts the filter name from a file path.
func filterFromPath(path, kind string) string {
	dir := filepath.Dir(path)
	seg := filepath.Base(dir)

	switch kind {
	case "bridge":
		if !strings.HasPrefix(seg, "okf_") {
			return ""
		}
		return strings.TrimPrefix(seg, "okf_")
	default:
		return seg
	}
}

// buildTestStatusMap builds a map of "filter/TestName" → status from FilterResult maps.
func buildTestStatusMap(results map[string]*FilterResult) map[string]string {
	out := map[string]string{}
	if results == nil {
		return out
	}
	for filter, fr := range results {
		for _, s := range fr.Suites {
			for _, t := range s.Tests {
				key := filter + "/" + t.Name
				out[key] = t.Status
			}
		}
	}
	return out
}

func merge(
	okapi, bridge, native map[string]*FilterResult,
	bridgeAnns, nativeAnns []annotation,
	bridgeSkips, nativeSkips []skipAnnotation,
	bridgeTestStatus, nativeTestStatus map[string]string,
	bridgeSkipMsgs, nativeSkipMsgs map[string]string,
	okapiVer, gokapiVer string,
) *ComparisonData {
	// Build annotation lookup: filter → javaClass#method → []GoTestName
	type annKey struct{ filter, class, method string }
	bridgeAnnMap := map[annKey][]string{}
	for _, a := range bridgeAnns {
		k := annKey{a.Filter, a.JavaClass, a.JavaMethod}
		bridgeAnnMap[k] = append(bridgeAnnMap[k], a.GoTest)
	}
	nativeAnnMap := map[annKey][]string{}
	for _, a := range nativeAnns {
		k := annKey{a.Filter, a.JavaClass, a.JavaMethod}
		nativeAnnMap[k] = append(nativeAnnMap[k], a.GoTest)
	}

	// Build skip annotation lookup: annKey → reason
	skipMap := map[annKey]string{}
	for _, s := range bridgeSkips {
		skipMap[annKey{s.Filter, s.JavaClass, s.JavaMethod}] = s.Reason
	}
	for _, s := range nativeSkips {
		skipMap[annKey{s.Filter, s.JavaClass, s.JavaMethod}] = s.Reason
	}

	// Collect all filter names
	names := map[string]struct{}{}
	for n := range okapi {
		names[n] = struct{}{}
	}
	for n := range bridge {
		names[n] = struct{}{}
	}
	for n := range native {
		names[n] = struct{}{}
	}

	filters := make([]FilterComparison, 0, len(names))
	var sum Summary

	for n := range names {
		fc := FilterComparison{
			FilterName: n,
			Okapi:      okapi[n],
		}
		if bridge != nil {
			fc.Bridge = bridge[n]
		}
		if native != nil {
			fc.Native = native[n]
		}

		// Build TestCaseMatch rows from Okapi test cases
		if fc.Okapi != nil {
			var testCases []TestCaseMatch
			for _, suite := range fc.Okapi.Suites {
				for _, tc := range suite.Tests {
					className := shortClassName(tc.ClassName)
					tcm := TestCaseMatch{
						JavaClass:   className,
						JavaMethod:  tc.Name,
						OkapiStatus: tc.Status,
					}

					// Check for skip annotation
					k := annKey{n, className, tc.Name}
					if reason, ok := skipMap[k]; ok {
						tcm.TestState = "skipped"
						tcm.SkipReason = reason
						testCases = append(testCases, tcm)
						continue
					}

					// Look up bridge annotation
					if goTests := bridgeAnnMap[k]; len(goTests) > 0 {
						tcm.BridgeTest = goTests[0]
						if st, ok := bridgeTestStatus[n+"/"+goTests[0]]; ok {
							tcm.BridgeStatus = st
						}
					}

					// Look up native annotation
					if goTests := nativeAnnMap[k]; len(goTests) > 0 {
						tcm.NativeTest = goTests[0]
						if st, ok := nativeTestStatus[n+"/"+goTests[0]]; ok {
							tcm.NativeStatus = st
						}
					}

					// Determine testState
					tcm.TestState = determineTestState(tcm, n,
						bridgeTestStatus, nativeTestStatus,
						bridgeSkipMsgs, nativeSkipMsgs)

					testCases = append(testCases, tcm)
				}
			}

			// Sort test cases by class then method
			sort.Slice(testCases, func(i, j int) bool {
				if testCases[i].JavaClass != testCases[j].JavaClass {
					return testCases[i].JavaClass < testCases[j].JavaClass
				}
				return testCases[i].JavaMethod < testCases[j].JavaMethod
			})

			fc.TestCases = testCases

			// Compute coverage stats
			fc.Coverage = computeCoverage(testCases)
		}

		filters = append(filters, fc)

		// Accumulate summary
		if fc.Okapi != nil {
			sum.TotalFiltersOkapi++
			sum.TotalTestsOkapi += fc.Okapi.Total
		}
		if fc.Bridge != nil {
			sum.TotalFiltersBridge++
			sum.TotalTestsBridge += fc.Bridge.Total
		}
		if fc.Native != nil {
			sum.TotalFiltersNative++
			sum.TotalTestsNative += fc.Native.Total
		}
		if fc.Okapi != nil && (fc.Bridge != nil || fc.Native != nil) {
			sum.TotalFiltersBoth++
		}
	}

	// Sort: filters with both sides first, then alphabetically
	sort.Slice(filters, func(i, j int) bool {
		bi := filters[i].Okapi != nil && (filters[i].Bridge != nil || filters[i].Native != nil)
		bj := filters[j].Okapi != nil && (filters[j].Bridge != nil || filters[j].Native != nil)
		if bi != bj {
			return bi
		}
		return filters[i].FilterName < filters[j].FilterName
	})

	// Overall coverage
	if sum.TotalTestsOkapi > 0 {
		totalMapped := 0
		for _, fc := range filters {
			totalMapped += fc.Coverage.BridgeMapped
		}
		sum.CoveragePct = float64(totalMapped) / float64(sum.TotalTestsOkapi) * 100
	}

	return &ComparisonData{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		OkapiVersion:  okapiVer,
		GokapiVersion: gokapiVer,
		Filters:       filters,
		Summary:       sum,
	}
}

// determineTestState classifies a test case into one of: implemented, pending, skipped, unmapped.
func determineTestState(
	tcm TestCaseMatch, filter string,
	bridgeTestStatus, nativeTestStatus map[string]string,
	bridgeSkipMsgs, nativeSkipMsgs map[string]string,
) string {
	// Already handled: "skipped" is set before calling this

	hasBridge := tcm.BridgeTest != ""
	hasNative := tcm.NativeTest != ""

	if !hasBridge && !hasNative {
		return "unmapped"
	}

	// Check if any mapped test is pending (skip with "pending" message)
	if hasBridge {
		if tcm.BridgeStatus == "skip" {
			if output := bridgeSkipMsgs[filter+"/"+tcm.BridgeTest]; strings.Contains(output, "pending") {
				return "pending"
			}
		}
	}
	if hasNative {
		if tcm.NativeStatus == "skip" {
			if output := nativeSkipMsgs[filter+"/"+tcm.NativeTest]; strings.Contains(output, "pending") {
				return "pending"
			}
		}
	}

	return "implemented"
}

// computeCoverage calculates coverage stats from test case matches.
func computeCoverage(testCases []TestCaseMatch) CoverageStats {
	cs := CoverageStats{
		TotalOkapi: len(testCases),
	}
	for _, tc := range testCases {
		switch tc.TestState {
		case "skipped":
			cs.SkippedCount++
		case "pending":
			cs.PendingCount++
		}

		if tc.BridgeTest != "" {
			cs.BridgeMapped++
			if tc.BridgeStatus == "pass" {
				cs.BridgePassing++
			}
		}
		if tc.NativeTest != "" {
			cs.NativeMapped++
			if tc.NativeStatus == "pass" {
				cs.NativePassing++
			}
		}
	}
	if cs.TotalOkapi > 0 {
		cs.CoveragePct = float64(cs.BridgeMapped) / float64(cs.TotalOkapi) * 100
		implemented := cs.BridgeMapped + cs.NativeMapped - cs.PendingCount
		if implemented < 0 {
			implemented = 0
		}
		cs.ImplementedPct = float64(implemented) / float64(cs.TotalOkapi) * 100
	}
	return cs
}

// shortClassName extracts the short class name from a fully qualified Java class.
// e.g. "net.sf.okapi.filters.html.HtmlSnippetsTest" → "HtmlSnippetsTest"
func shortClassName(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}
