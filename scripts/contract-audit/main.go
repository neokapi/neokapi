// Command contract-audit produces the JSON that powers the
// /test-comparison docs page. It treats Okapi's own *Test.java methods
// as the canonical contract list (per pinned Okapi version) and joins
// them with native Go test results so the dashboard shows where
// neokapi's behavioural coverage sits relative to upstream Okapi.
//
// Inputs:
//
//   -okapi-surefire <dir>   Directory containing surefire-reports/TEST-*.xml
//                           from a `mvn test` of one or more Okapi filter
//                           modules. Walked recursively. Each XML maps to
//                           one Okapi test class, each <testcase/> inside
//                           it to one contract row.
//
//   -native-gotest <path>   Output of `go test -json ./core/formats/<f>/...`
//                           (a JSONL stream). Optional — when omitted, the
//                           native column is left empty (every Okapi method
//                           shows as `unmapped`).
//
//   -okapi-version <ver>    Pinned Okapi version (e.g. 1.47.0). Surfaced
//                           in the dashboard header.
//
//   -okapi-tag <tag>        Git tag for source links (e.g. v1.47.0).
//
//   -go-commit <sha>        neokapi git SHA for source links.
//
//   -out <path>             Output JSON path. Defaults to
//                           web/docs/static/data/contract-audit.json so
//                           the legacy /test-comparison.json stays intact
//                           during the MVP.
//
// Filter scope: each Surefire XML's package prefix
// (net.sf.okapi.filters.<name>.*) selects the filter row it belongs to.
// One FilterComparison per <name>.
//
// Annotation joining: Go test functions can carry one of two comment
// markers immediately above the `func TestXxx(...)` line:
//
//	// okapi: HtmlSnippetsTest#testEscapes
//	func TestSnippets_EscapedEntities(t *testing.T) { ... }
//
// or, for tests that are deliberately not applicable in neokapi:
//
//	// okapi-skip: HtmlSnippetsTest#testFoo — config subsystem differs
//	// (free-text reason after an em-dash)
//
// The generator joins these annotations with the per-test status from
// `go test -json` and the per-method status from Surefire to produce a
// 4-state model per Okapi method:
//
//   - implemented — annotation present, Go test passes.
//   - pending     — annotation present, Go test is t.Skip()'d
//                   (or Java method is `@Ignore`d).
//   - skipped     — `// okapi-skip:` declares the test not-applicable
//                   to neokapi by design; reason carried verbatim.
//   - unmapped    — Java method exists, no Go counterpart found.
//
// The dashboard renders this directly: every Okapi @Test method is one
// row, the state drives the colour, and the skip reason surfaces as a
// tooltip on the row.
package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ── Surefire XML ────────────────────────────────────────────────────────────

type sfTestSuite struct {
	XMLName  xml.Name     `xml:"testsuite"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Time     string       `xml:"time,attr"`
	TestCase []sfTestCase `xml:"testcase"`
}

type sfTestCase struct {
	Name      string     `xml:"name,attr"`
	ClassName string     `xml:"classname,attr"`
	Time      string     `xml:"time,attr"`
	Failure   *sfFailure `xml:"failure"`
	Error     *sfFailure `xml:"error"`
	Skipped   *sfFailure `xml:"skipped"`
}

type sfFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
}

// ── go test -json ───────────────────────────────────────────────────────────

type goTestEvent struct {
	Action  string  `json:"Action"` // "run", "pass", "fail", "skip", "output", …
	Package string  `json:"Package"`
	Test    string  `json:"Test,omitempty"`
	Output  string  `json:"Output,omitempty"`
	Elapsed float64 `json:"Elapsed,omitempty"`
}

// ── Dashboard wire schema (mirrors web/docs/src/pages/test-comparison/_types.ts) ──

type testComparisonData struct {
	GeneratedAt    string             `json:"generatedAt"`
	OkapiVersion   string             `json:"okapiVersion"`
	NeokapiVersion string             `json:"neokapiVersion"`
	GoCommitSHA    string             `json:"goCommitSHA,omitempty"`
	OkapiTag       string             `json:"okapiTag,omitempty"`
	Filters        []filterComparison `json:"filters"`
	Summary        summary            `json:"summary"`
}

type summary struct {
	TotalFiltersOkapi  int     `json:"totalFiltersOkapi"`
	TotalFiltersBridge int     `json:"totalFiltersBridge"`
	TotalFiltersNative int     `json:"totalFiltersNative"`
	TotalFiltersBoth   int     `json:"totalFiltersBoth"`
	TotalTestsOkapi    int     `json:"totalTestsOkapi"`
	TotalTestsBridge   int     `json:"totalTestsBridge"`
	TotalTestsNative   int     `json:"totalTestsNative"`
	CoveragePct        float64 `json:"coveragePct"`
}

type filterComparison struct {
	FilterName       string          `json:"filterName"`
	NativeFilterName string          `json:"nativeFilterName,omitempty"`
	Okapi            *filterResult   `json:"okapi"`
	Bridge           *filterResult   `json:"bridge"`
	Native           *filterResult   `json:"native"`
	TestCases        []testCaseMatch `json:"testCases"`
	Coverage         *coverage       `json:"coverage"`
}

type testCaseMatch struct {
	JavaClass     string `json:"javaClass"`
	JavaMethod    string `json:"javaMethod"`
	OkapiStatus   string `json:"okapiStatus"`
	OkapiFile     string `json:"okapiFile,omitempty"`
	BridgeTest    string `json:"bridgeTest,omitempty"`
	BridgeStatus  string `json:"bridgeStatus,omitempty"`
	BridgeFile    string `json:"bridgeFile,omitempty"`
	BridgeLine    int    `json:"bridgeLine,omitempty"`
	NativeTest    string `json:"nativeTest,omitempty"`
	NativeStatus  string `json:"nativeStatus,omitempty"`
	NativeFile    string `json:"nativeFile,omitempty"`
	NativeLine    int    `json:"nativeLine,omitempty"`
	SkipReason    string `json:"skipReason,omitempty"`
	TestState     string `json:"testState,omitempty"` // implemented | pending | skipped | unmapped
	SkipCategory  string `json:"skipCategory,omitempty"`
}

type filterResult struct {
	Suites  []testSuite `json:"suites"`
	Total   int         `json:"total"`
	Passed  int         `json:"passed"`
	Failed  int         `json:"failed"`
	Skipped int         `json:"skipped"`
	Errors  int         `json:"errors"`
}

type testSuite struct {
	Name       string     `json:"name"`
	Tests      []testCase `json:"tests"`
	Total      int        `json:"total"`
	Passed     int        `json:"passed"`
	Failed     int        `json:"failed"`
	Skipped    int        `json:"skipped"`
	Errors     int        `json:"errors"`
	DurationMS int64      `json:"durationMs"`
}

type testCase struct {
	Name       string `json:"name"`
	ClassName  string `json:"className,omitempty"`
	Status     string `json:"status"` // pass | fail | skip | error
	DurationMS int64  `json:"durationMs"`
}

type coverage struct {
	TotalOkapi    int     `json:"totalOkapi"`
	BridgeMapped  int     `json:"bridgeMapped"`
	BridgePassing int     `json:"bridgePassing"`
	NativeMapped  int     `json:"nativeMapped"`
	NativePassing int     `json:"nativePassing"`
	CoveragePct   float64 `json:"coveragePct"`
}

// ── main ────────────────────────────────────────────────────────────────────

func main() {
	surefireDir := flag.String("okapi-surefire", "", "Directory containing surefire-reports/ (walked recursively)")
	nativeJSON := flag.String("native-gotest", "", "go test -json output for native side (optional)")
	nativeSrc := flag.String("native-src", "", "Comma-separated list of native test source dirs to scan for // okapi: annotations")
	parityReport := flag.String("parity-report", "", "Path to .parity/test-comparison.json (optional). Populates the per-filter Bridge column with the head-to-head parity outcome.")
	okapiVersion := flag.String("okapi-version", "1.47.0", "Pinned Okapi version, surfaced in the dashboard header")
	okapiTag := flag.String("okapi-tag", "", "Okapi git tag for source links (e.g. v1.47.0)")
	goCommit := flag.String("go-commit", "", "neokapi git SHA for source links")
	out := flag.String("out", "web/docs/static/data/contract-audit.json", "Output JSON path")
	flag.Parse()

	if *surefireDir == "" {
		die("must set -okapi-surefire")
	}

	okapiByFilter, err := parseSurefireDir(*surefireDir)
	if err != nil {
		die("parse surefire: %v", err)
	}
	if len(okapiByFilter) == 0 {
		die("no surefire XMLs found under %s", *surefireDir)
	}

	var nativeByFilter map[string]*filterResult
	if *nativeJSON != "" {
		nativeByFilter, err = parseGoTestJSON(*nativeJSON)
		if err != nil {
			die("parse native gotest: %v", err)
		}
	}

	var nativeAnnotations []annotation
	if *nativeSrc != "" {
		for _, dir := range strings.Split(*nativeSrc, ",") {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			anns, err := scanAnnotations(dir)
			if err != nil {
				die("scan annotations in %s: %v", dir, err)
			}
			nativeAnnotations = append(nativeAnnotations, anns...)
		}
	}

	var bridgeByFilter map[string]parityRow
	if *parityReport != "" {
		bridgeByFilter, err = parseParityReport(*parityReport)
		if err != nil {
			die("parse parity report: %v", err)
		}
	}

	doc := buildDoc(okapiByFilter, nativeByFilter, bridgeByFilter, nativeAnnotations, *okapiVersion, *okapiTag, *goCommit)

	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		die("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		die("mkdir %s: %v", filepath.Dir(*out), err)
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		die("write %s: %v", *out, err)
	}
	fmt.Fprintf(os.Stderr, "contract-audit: %d filters → %s\n", len(doc.Filters), *out)
}

// nativeFilterAliases maps an Okapi filter id to the neokapi package
// name when they differ. The dashboard then surfaces both names so a
// reviewer can navigate either side. Only one direction is needed
// because the generator keys all maps by Okapi filter id.
var nativeFilterAliases = map[string]string{
	"php":       "phpcontent",
	"xmlstream": "xml",
	"table":     "csv",
	// neokapi splits Okapi's `subtitles` filter into `vtt`+`ttml`+`srt`.
	// We keep the per-format Okapi ids and rely on the per-class join
	// in scanAnnotations to match them.
}

// parseSurefireDir walks surefireDir and returns one filterResult per
// Okapi filter (e.g. "html", "json"). The filter name is derived from
// the package prefix net.sf.okapi.filters.<name>.*.
func parseSurefireDir(surefireDir string) (map[string]*filterResult, error) {
	pkgRE := regexp.MustCompile(`^net\.sf\.okapi\.filters\.([^.]+)`)
	out := map[string]*filterResult{}
	err := filepath.WalkDir(surefireDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasPrefix(filepath.Base(path), "TEST-") || !strings.HasSuffix(path, ".xml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var ts sfTestSuite
		if err := xml.Unmarshal(data, &ts); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		m := pkgRE.FindStringSubmatch(ts.Name)
		if m == nil {
			return nil // not a per-filter suite (could be a core/* test); skip
		}
		filterName := m[1]
		fr, ok := out[filterName]
		if !ok {
			fr = &filterResult{}
			out[filterName] = fr
		}
		suite := testSuite{
			Name:       ts.Name,
			DurationMS: parseSecondsToMs(ts.Time),
		}
		for _, tc := range ts.TestCase {
			status := "pass"
			switch {
			case tc.Failure != nil:
				status = "fail"
			case tc.Error != nil:
				status = "error"
			case tc.Skipped != nil:
				status = "skip"
			}
			suite.Tests = append(suite.Tests, testCase{
				Name:       tc.Name,
				ClassName:  tc.ClassName,
				Status:     status,
				DurationMS: parseSecondsToMs(tc.Time),
			})
			suite.Total++
			fr.Total++
			switch status {
			case "pass":
				suite.Passed++
				fr.Passed++
			case "fail":
				suite.Failed++
				fr.Failed++
			case "skip":
				suite.Skipped++
				fr.Skipped++
			case "error":
				suite.Errors++
				fr.Errors++
			}
		}
		fr.Suites = append(fr.Suites, suite)
		return nil
	})
	return out, err
}

// parseGoTestJSON consumes a go test -json stream and returns one
// filterResult per Go package, keyed by the package's last path
// segment (e.g. "html" for ".../core/formats/html"). Subtests are
// reported as separate cases under a synthetic suite named after the
// parent test.
func parseGoTestJSON(path string) (map[string]*filterResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type key struct{ pkg, test string }
	results := map[key]string{}
	durations := map[key]int64{}
	pkgs := map[string]struct{}{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ev goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Test == "" {
			continue
		}
		pkgs[ev.Package] = struct{}{}
		k := key{pkg: ev.Package, test: ev.Test}
		switch ev.Action {
		case "pass", "fail", "skip":
			results[k] = ev.Action
			if ev.Elapsed > 0 {
				durations[k] = int64(ev.Elapsed * 1000)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := map[string]*filterResult{}
	pkgKeys := make([]string, 0, len(pkgs))
	for p := range pkgs {
		pkgKeys = append(pkgKeys, p)
	}
	sort.Strings(pkgKeys)
	for _, pkg := range pkgKeys {
		short := lastPathSegment(pkg)
		fr, ok := out[short]
		if !ok {
			fr = &filterResult{}
			out[short] = fr
		}
		// One synthetic suite per package.
		suite := testSuite{Name: pkg}
		var rows []testCase
		for k, status := range results {
			if k.pkg != pkg {
				continue
			}
			rows = append(rows, testCase{
				Name:       k.test,
				Status:     statusFromGo(status),
				DurationMS: durations[k],
			})
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		for _, r := range rows {
			suite.Tests = append(suite.Tests, r)
			suite.Total++
			fr.Total++
			switch r.Status {
			case "pass":
				suite.Passed++
				fr.Passed++
			case "fail":
				suite.Failed++
				fr.Failed++
			case "skip":
				suite.Skipped++
				fr.Skipped++
			}
		}
		fr.Suites = append(fr.Suites, suite)
	}
	return out, nil
}

// ── Parity report (head-to-head bridge↔native) ──────────────────────────────

// parityRow mirrors one entry in .parity/test-comparison.json. The
// generator only consumes `kind: "format"` rows; step-level rows are
// out of scope for the per-filter dashboard.
type parityRow struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"` // okf_<filterName>
	Name     string `json:"name"`
	Status   string `json:"status"` // pass | skip
	Mode     string `json:"mode"`   // bridge-only | head-to-head
	Detail   string `json:"detail,omitempty"`
	Duration int64  `json:"duration_ms,omitempty"`
}

// parseParityReport reads the parity JSON and returns a map keyed by
// filter id (with `okf_` stripped) so the bridge column can join
// against the same names the rest of the generator already uses.
func parseParityReport(path string) (map[string]parityRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []parityRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	out := map[string]parityRow{}
	for _, r := range rows {
		if r.Kind != "format" {
			continue
		}
		out[strings.TrimPrefix(r.ID, "okf_")] = r
	}
	return out, nil
}

// bridgeFilterAliases bridges naming differences between the parity
// report's `okf_<id>` ids and the Okapi surefire-derived filter names.
// Only divergences are listed.
var bridgeFilterAliases = map[string]string{
	"phpcontent": "php", // parity uses phpcontent; surefire/native use php
}

// buildDoc joins the per-filter Okapi and native maps with the
// scanned annotations into a single dashboard document, deterministic
// in iteration order.
func buildDoc(okapiByFilter, nativeByFilter map[string]*filterResult, bridgeByFilter map[string]parityRow, annotations []annotation, okapiVersion, okapiTag, goCommit string) testComparisonData {
	// Index annotations by Java FQN#method for O(1) joins.
	annByOkapi := map[string]annotation{}
	for _, a := range annotations {
		key := a.JavaClass + "#" + a.JavaMethod
		annByOkapi[key] = a
	}
	// Index native go-test status by func name (last segment of pkg + "/" + Test).
	// We just store status by Test name; collisions across packages are rare
	// enough that filter scoping resolves them.
	nativeStatus := map[string]string{}
	for _, fr := range nativeByFilter {
		for _, suite := range fr.Suites {
			for _, tc := range suite.Tests {
				nativeStatus[tc.Name] = tc.Status
			}
		}
	}

	names := map[string]struct{}{}
	for n := range okapiByFilter {
		names[n] = struct{}{}
	}
	for n := range nativeByFilter {
		names[n] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)

	doc := testComparisonData{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		OkapiVersion:   okapiVersion,
		NeokapiVersion: "main",
		OkapiTag:       okapiTag,
		GoCommitSHA:    goCommit,
	}
	var sum summary
	for _, name := range ordered {
		fc := filterComparison{FilterName: name, TestCases: []testCaseMatch{}}
		if r := okapiByFilter[name]; r != nil {
			fc.Okapi = r
			sum.TotalFiltersOkapi++
			sum.TotalTestsOkapi += r.Total
		}
		// Native lookup — try the Okapi id first, then the alias.
		nativeName := name
		if alias, ok := nativeFilterAliases[name]; ok {
			if _, present := nativeByFilter[alias]; present {
				nativeName = alias
				fc.NativeFilterName = alias
			}
		}
		if r := nativeByFilter[nativeName]; r != nil {
			fc.Native = r
			sum.TotalFiltersNative++
			sum.TotalTestsNative += r.Total
		}
		if fc.Okapi != nil && fc.Native != nil {
			sum.TotalFiltersBoth++
		}
		// Bridge column: synthesize a single "bridge parity" suite from the
		// head-to-head parity row, if one exists. The dashboard renders
		// this as one badge per filter — per-test bridge granularity is
		// out of scope until okapi-bridge exposes per-test results.
		if br, ok := lookupParity(bridgeByFilter, name); ok {
			fc.Bridge = parityToFilterResult(br)
			sum.TotalFiltersBridge++
			sum.TotalTestsBridge += fc.Bridge.Total
		}
		// Build one row per Okapi @Test method, joined with annotations.
		fc.TestCases = buildRows(fc.Okapi, annByOkapi, nativeStatus)
		fc.Coverage = computeCoverageFromRows(fc.Okapi, fc.TestCases)
		doc.Filters = append(doc.Filters, fc)
	}
	// Coverage % = implemented / totalOkapi summed across filters.
	// Also count unmapped (empty testState) for parity with the
	// dashboard's per-card convention.
	var implemented, unmapped int
	for _, f := range doc.Filters {
		for _, r := range f.TestCases {
			switch r.TestState {
			case "implemented":
				implemented++
			case "":
				unmapped++
			}
		}
	}
	_ = unmapped
	if sum.TotalTestsOkapi > 0 {
		sum.CoveragePct = round1(float64(implemented) / float64(sum.TotalTestsOkapi) * 100)
	}
	doc.Summary = sum
	return doc
}

// buildRows produces one TestCaseMatch per Okapi @Test method, joining
// against annotations and native go-test status.
func buildRows(okapi *filterResult, annByOkapi map[string]annotation, nativeStatus map[string]string) []testCaseMatch {
	rows := []testCaseMatch{}
	if okapi == nil {
		return rows
	}
	for _, suite := range okapi.Suites {
		for _, tc := range suite.Tests {
			javaClass := tc.ClassName
			if javaClass == "" {
				javaClass = suite.Name
			}
			row := testCaseMatch{
				JavaClass:   javaClass,
				JavaMethod:  tc.Name,
				OkapiStatus: tc.Status,
			}
			ann, ok := annByOkapi[javaClass+"#"+tc.Name]
			if !ok {
				// Try short-class match (Surefire uses FQN; annotations
				// usually use short class).
				ann, ok = annByOkapi[shortClass(javaClass)+"#"+tc.Name]
			}
			switch {
			case !ok:
				// Dashboard convention: unmapped rows leave testState
				// empty (so the "no annotation" filter at
				// _TestCaseTable.tsx#186 matches them).
			case ann.Skip:
				row.TestState = "skipped"
				row.SkipReason = ann.Reason
				row.SkipCategory = classifySkip(ann.Reason)
				row.NativeTest = ann.GoFunc
				row.NativeFile = ann.File
				row.NativeLine = ann.Line
			default:
				row.NativeTest = ann.GoFunc
				row.NativeFile = ann.File
				row.NativeLine = ann.Line
				gs := nativeStatus[ann.GoFunc]
				row.NativeStatus = gs
				switch gs {
				case "pass":
					row.TestState = "implemented"
				case "skip":
					row.TestState = "pending"
				case "fail":
					row.TestState = "implemented" // implemented but failing
				default:
					// Annotation present but Go test wasn't found in the
					// JSON — treat as pending (likely the test isn't in
					// scope of the gotest package set).
					row.TestState = "pending"
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func computeCoverageFromRows(okapi *filterResult, rows []testCaseMatch) *coverage {
	if okapi == nil {
		return nil
	}
	c := &coverage{TotalOkapi: okapi.Total}
	for _, r := range rows {
		if r.NativeTest != "" {
			c.NativeMapped++
			if r.NativeStatus == "pass" {
				c.NativePassing++
			}
		}
	}
	if c.TotalOkapi > 0 {
		c.CoveragePct = round1(float64(c.NativeMapped) / float64(c.TotalOkapi) * 100)
	}
	return c
}

// lookupParity finds the parity row for a filter, falling back to the
// alias map for the small set of names that diverge between the parity
// id space (`okf_<id>`) and the surefire-derived names.
func lookupParity(bridgeByFilter map[string]parityRow, name string) (parityRow, bool) {
	if r, ok := bridgeByFilter[name]; ok {
		return r, true
	}
	if alias, ok := bridgeFilterAliases[name]; ok {
		if r, ok := bridgeByFilter[alias]; ok {
			return r, true
		}
	}
	// Reverse: a few filters key by the surefire short id but the parity
	// row uses the longer alias (e.g. surefire `php` → parity `phpcontent`).
	for k, v := range bridgeFilterAliases {
		if v == name {
			if r, ok := bridgeByFilter[k]; ok {
				return r, true
			}
		}
	}
	return parityRow{}, false
}

// parityToFilterResult turns one parity row into a single-suite,
// single-case filterResult — one synthetic "bridge parity" badge that
// the dashboard renders next to the Okapi/native columns.
func parityToFilterResult(r parityRow) *filterResult {
	status := "skip"
	if r.Status == "pass" {
		status = "pass"
	}
	caseName := r.Mode
	if caseName == "" {
		caseName = "parity"
	}
	suite := testSuite{
		Name: "bridge parity",
		Tests: []testCase{{
			Name:       caseName,
			Status:     status,
			DurationMS: r.Duration,
		}},
		Total:      1,
		DurationMS: r.Duration,
	}
	fr := &filterResult{Total: 1}
	switch status {
	case "pass":
		suite.Passed = 1
		fr.Passed = 1
	case "skip":
		suite.Skipped = 1
		fr.Skipped = 1
	}
	fr.Suites = []testSuite{suite}
	return fr
}

// classifySkip bins a free-text skip reason into a SkipCategory the
// dashboard recognises. Matches are case-insensitive substring lookups
// against the most distinctive keyword.
func classifySkip(reason string) string {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "subfilter"):
		return "subfilter"
	case strings.Contains(r, "vendor"), strings.Contains(r, "sdl"), strings.Contains(r, "iws"):
		return "vendor"
	case strings.Contains(r, "roundtrip"), strings.Contains(r, "round-trip"):
		return "roundtrip"
	case strings.Contains(r, "testdata"), strings.Contains(r, "test data"):
		return "testdata"
	case strings.Contains(r, "java api"), strings.Contains(r, "java-api"):
		return "java-api"
	case strings.Contains(r, "regex"):
		return "regex"
	case strings.Contains(r, "config"):
		return "config"
	case strings.Contains(r, "format"):
		return "format"
	case strings.Contains(r, "dita"):
		return "dita"
	case strings.Contains(r, "feature"):
		return "feature"
	case strings.Contains(r, "not implemented"), strings.Contains(r, "not-implemented"):
		return "not-implemented"
	default:
		return "other"
	}
}

// ── Annotation scanning ─────────────────────────────────────────────────────

type annotation struct {
	JavaClass  string // short class name (e.g. HtmlSnippetsTest) or FQN
	JavaMethod string
	Skip       bool
	Reason     string // free-text reason for // okapi-skip:
	GoFunc     string // the Go func it sits above (e.g. TestSnippets_Foo)
	File       string // path relative to the scan root, then prefixed with the package dir
	Line       int    // 1-based line of the func declaration
}

var (
	okapiCommentRE     = regexp.MustCompile(`^\s*//\s*okapi:\s*([^#\s]+)#(\S+)\s*$`)
	okapiSkipCommentRE = regexp.MustCompile(`^\s*//\s*okapi-skip:\s*([^#\s]+)#(\S+)(?:\s*[—\-]\s*(.+))?\s*$`)
	funcDeclRE         = regexp.MustCompile(`^func\s+(Test\w+)\s*\(`)
)

// scanAnnotations walks dir and extracts all // okapi: and // okapi-skip:
// annotations from *.go test files. The annotation must immediately
// precede a func TestXxx declaration (allowing other comment lines in
// between, since godoc-style multi-line comments are common).
func scanAnnotations(dir string) ([]annotation, error) {
	var out []annotation
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		anns, err := scanFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, anns...)
		return nil
	})
	return out, err
}

func scanFile(path string) ([]annotation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	rel := relPath(path)

	// Walk forward, accumulating comment lines, then attaching them to
	// the next func TestXxx declaration we see.
	var pending []annotation
	var out []annotation
	for i, line := range lines {
		if m := okapiCommentRE.FindStringSubmatch(line); m != nil {
			pending = append(pending, annotation{JavaClass: m[1], JavaMethod: m[2]})
			continue
		}
		if m := okapiSkipCommentRE.FindStringSubmatch(line); m != nil {
			reason := ""
			if len(m) > 3 {
				reason = strings.TrimSpace(m[3])
			}
			pending = append(pending, annotation{
				JavaClass:  m[1],
				JavaMethod: m[2],
				Skip:       true,
				Reason:     reason,
			})
			continue
		}
		// Line that's neither okapi comment nor func decl: keep
		// accumulating as long as it's a comment or blank line.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || trimmed == "" {
			continue
		}
		if m := funcDeclRE.FindStringSubmatch(line); m != nil {
			for _, a := range pending {
				a.GoFunc = m[1]
				a.File = rel
				a.Line = i + 1
				out = append(out, a)
			}
		}
		pending = nil
	}
	return out, nil
}

// relPath converts an absolute path to the repo-relative form the
// dashboard uses for source links. We assume the scan root is inside
// the neokapi repo and just strip everything before "core/" / "cli/" /
// "kapi/" / "bowrain/" (the four module roots).
func relPath(p string) string {
	for _, prefix := range []string{"/core/", "/cli/", "/kapi/", "/bowrain/", "/apps/"} {
		if idx := strings.Index(p, prefix); idx >= 0 {
			return p[idx+1:]
		}
	}
	return filepath.Base(p)
}

func parseSecondsToMs(s string) int64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return int64(f * 1000)
}

func lastPathSegment(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

func shortClass(fqn string) string {
	idx := strings.LastIndex(fqn, ".")
	if idx < 0 {
		return fqn
	}
	return fqn[idx+1:]
}

func statusFromGo(s string) string {
	switch s {
	case "pass", "fail", "skip":
		return s
	default:
		return "skip"
	}
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "contract-audit: "+format+"\n", args...)
	os.Exit(1)
}
