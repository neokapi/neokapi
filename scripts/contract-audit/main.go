// Command contract-audit produces the JSON that powers the
// /test-comparison docs page. It treats Okapi's own *Test.java methods
// as the canonical contract list (per pinned Okapi version) and joins
// them with native Go test results so the dashboard shows where
// neokapi's behavioural coverage sits relative to upstream Okapi.
//
// Inputs:
//
//	-okapi-surefire <dir>   Directory containing surefire-reports/TEST-*.xml
//	                        from a `mvn test` of one or more Okapi filter
//	                        modules. Walked recursively. Each XML maps to
//	                        one Okapi test class, each <testcase/> inside
//	                        it to one contract row.
//
//	-native-gotest <path>   Output of `go test -json ./core/formats/<f>/...`
//	                        (a JSONL stream). Optional — when omitted, the
//	                        native column is left empty (every Okapi method
//	                        shows as `unmapped`).
//
//	-okapi-version <ver>    Pinned Okapi version (e.g. 1.47.0). Surfaced
//	                        in the dashboard header.
//
//	-okapi-tag <tag>        Git tag for source links (e.g. v1.47.0).
//
//	-go-commit <sha>        neokapi git SHA for source links.
//
//	-out <path>             Output JSON path. Defaults to
//	                        web/docs/static/data/contract-audit.json so
//	                        the legacy /test-comparison.json stays intact
//	                        during the MVP.
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
//     (or Java method is `@Ignore`d).
//   - skipped     — `// okapi-skip:` declares the test not-applicable
//     to neokapi by design; reason carried verbatim.
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

	"github.com/neokapi/neokapi/core/format/spec"
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
	// FilterName is the canonical row id: the neokapi native format
	// directory name (e.g. "csv", "fixedwidth", "plaintext"). Rows are
	// keyed by native format so each format gets exactly one row, with
	// the Okapi filter id(s) carried as a sublabel in OkapiFilterIDs.
	FilterName string `json:"filterName"`
	// NativeFilterName mirrors FilterName when it differs from the row id
	// for backward compat; it now equals FilterName and is kept only so
	// older dashboard JSON consumers don't break.
	NativeFilterName string `json:"nativeFilterName,omitempty"`
	// OkapiFilterIDs lists the Okapi filter package id(s) whose @Test
	// classes route to this native format (e.g. csv ← ["table"]). Shown
	// as a secondary label so a user can navigate the Okapi side. Empty
	// for genuinely neokapi-only formats (jsx, mo, exec, formats,
	// memorytest, versifiedtext — no Okapi equivalent).
	OkapiFilterIDs []string `json:"okapiFilterIds,omitempty"`
	// SpecKind mirrors spec.Spec.Kind. Empty for filters with no
	// spec.yaml; "top_level" or "subfilter" for filters that have one.
	// The dashboard groups subfilters (layer formats) into their own
	// section rather than counting them against top-level coverage.
	SpecKind        string                 `json:"specKind,omitempty"`
	Okapi           *filterResult          `json:"okapi"`
	Bridge          *filterResult          `json:"bridge"`
	Native          *filterResult          `json:"native"`
	TestCases       []testCaseMatch        `json:"testCases"`
	Coverage        *coverage              `json:"coverage"`
	Spec            *specSummary           `json:"spec,omitempty"`
	SpecDrift       []specDriftEntry       `json:"specDrift,omitempty"`
	SpecConfigDrift []specConfigDriftEntry `json:"specConfigDrift,omitempty"`
}

// specDriftEntry records one okapi_refs entry in spec.yaml that no
// longer matches a test in the pinned Okapi version. The dashboard
// surfaces these as warnings on the Spec section so spec authors know
// to either repoint the ref or remove it.
type specDriftEntry struct {
	FeatureID string `json:"featureId"`
	OkapiRef  string `json:"okapiRef"` // ClassName#methodName
	Reason    string `json:"reason"`   // currently always "missing-from-okapi"
}

// specConfigDriftEntry records one spec.config[].key that doesn't
// correspond to a property in the bridge composite JSON Schema for the
// pinned Okapi version. Either the spec invented a key the bridge
// can't accept, or the bridge schema renamed/removed the property.
type specConfigDriftEntry struct {
	Key        string `json:"key"`        // spec.config[].key
	OkapiParam string `json:"okapiParam"` // spec.config[].okapi_param (Java field), if set
	Reason     string `json:"reason"`     // "missing-from-bridge-schema"
}

// specSummary aggregates the spec runner's per-feature outcomes for
// one filter. Features list each with their examples; totals roll up
// per status so the dashboard can render a feature-coverage badge
// alongside the existing per-test count.
type specSummary struct {
	Features []specFeature `json:"features"`
	// Status totals across all examples in all features.
	Pass         int `json:"pass"`
	Fail         int `json:"fail"`
	Skip         int `json:"skip"`
	ParityWarn   int `json:"parityWarn"`
	ExpectedFail int `json:"expectedFail"`
}

type specFeature struct {
	ID       string        `json:"id"`
	Examples []specExample `json:"examples"`
}

type specExample struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Mode   string `json:"mode,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Divergence attributes which side is at fault for an
	// expected_fail / parity_warn example so the dashboard can colour by
	// severity. One of: native-bug, bridge-gap, okapi-bug, scope-diff,
	// default-diff, missing-filter, fixture, contract. Empty for
	// pass/skip examples. Computed by classifyDivergence from the detail
	// text, or taken verbatim from the spec.yaml example's
	// divergence_kind when the author set one (the override wins).
	Divergence string `json:"divergence,omitempty"`
}

type testCaseMatch struct {
	JavaClass    string `json:"javaClass"`
	JavaMethod   string `json:"javaMethod"`
	OkapiStatus  string `json:"okapiStatus"`
	OkapiFile    string `json:"okapiFile,omitempty"`
	BridgeTest   string `json:"bridgeTest,omitempty"`
	BridgeStatus string `json:"bridgeStatus,omitempty"`
	BridgeFile   string `json:"bridgeFile,omitempty"`
	BridgeLine   int    `json:"bridgeLine,omitempty"`
	NativeTest   string `json:"nativeTest,omitempty"`
	NativeStatus string `json:"nativeStatus,omitempty"`
	NativeFile   string `json:"nativeFile,omitempty"`
	NativeLine   int    `json:"nativeLine,omitempty"`
	SkipReason   string `json:"skipReason,omitempty"`
	TestState    string `json:"testState,omitempty"` // implemented | pending | skipped | unmapped
	SkipCategory string `json:"skipCategory,omitempty"`
	Params       int    `json:"params,omitempty"` // >1 when this @Test collapses N JUnit parameterized invocations
	// CoveredBy* link a not-applicable / skipped row to the native Go test
	// that actually verifies the equivalent behaviour, when the skip reason
	// names one (e.g. "… covered by TestRoundTrip_DoubleExtraction"). This
	// makes a "covered elsewhere" claim verifiable on the dashboard rather
	// than an unprovable assertion (#611). Empty when the reason makes no
	// coverage claim (genuinely not-implemented).
	CoveredByTest string `json:"coveredByTest,omitempty"`
	CoveredByFile string `json:"coveredByFile,omitempty"`
	CoveredByLine int    `json:"coveredByLine,omitempty"`
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
	Params     int    `json:"params,omitempty"` // >1 when this row collapses N JUnit parameterized invocations
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
	failsafeDir := flag.String("okapi-failsafe", "", "Directory containing Maven Failsafe reports for Okapi *IT integration tests (e.g. integration-tests/okapi/target/failsafe-reports). Walked recursively. Optional — when set, RoundTrip*IT / *XliffCompareIT contracts join their filter's rows.")
	nativeJSON := flag.String("native-gotest", "", "go test -json output for native side (optional)")
	nativeSrc := flag.String("native-src", "", "Comma-separated list of native test source dirs to scan for // okapi: annotations")
	parityReport := flag.String("parity-report", "", "Path to .parity/test-comparison.json (optional). Populates the per-filter Bridge column with the head-to-head parity outcome.")
	formatsDir := flag.String("formats-dir", "core/formats", "Directory containing per-format packages (each with an optional spec.yaml). Used for spec.okapi_refs drift detection.")
	bridgeSchemas := flag.String("bridge-schemas", "../okapi-bridge/schemas", "Path to the okapi-bridge/schemas directory (containing versions.json and filters/composite/). Used for spec.config[].key drift detection. Empty disables the check.")
	failOnDrift := flag.Bool("fail-on-drift", false, "Exit non-zero if any // okapi: annotation or spec drift entry references a Java class/method not present in the pinned Okapi Surefire output.")
	okapiVersion := flag.String("okapi-version", "1.47.0", "Pinned Okapi version, surfaced in the dashboard header")
	okapiTag := flag.String("okapi-tag", "", "Okapi git tag for source links (e.g. v1.47.0)")
	goCommit := flag.String("go-commit", "", "neokapi git SHA for source links")
	out := flag.String("out", "web/docs/static/data/contract-audit.json", "Output JSON path")
	flag.Parse()

	if *surefireDir == "" {
		die("must set -okapi-surefire")
	}

	okapiByFilter, okapiIDsByNative, err := parseSurefireDir(*surefireDir)
	if err != nil {
		die("parse surefire: %v", err)
	}
	if len(okapiByFilter) == 0 {
		die("no surefire XMLs found under %s", *surefireDir)
	}

	// Failsafe (*IT integration tests). Okapi's roundtrip and xliff-compare
	// integration tests live in the integration-tests/okapi module and run
	// under Maven Failsafe, not Surefire — so they're absent from the
	// per-filter surefire-reports. Scan them separately and merge each IT
	// class into its filter's contract rows (keyed by class name, not
	// package, since the IT package is roundtrip.integration /
	// xliffcompare.integration).
	if *failsafeDir != "" {
		itByFilter, itOkapiIDs, err := parseFailsafeDir(*failsafeDir)
		if err != nil {
			die("parse failsafe: %v", err)
		}
		mergeOkapiResults(okapiByFilter, itByFilter)
		mergeOkapiIDs(okapiIDsByNative, itOkapiIDs)
	}

	var nativeByFilter map[string]*filterResult
	if *nativeJSON != "" {
		nativeByFilter, err = parseGoTestJSON(*nativeJSON)
		if err != nil {
			die("parse native gotest: %v", err)
		}
	}

	var nativeAnnotations []annotation
	nativeFuncIndex := map[string]funcLoc{}
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
			funcs, err := scanTestFuncs(dir)
			if err != nil {
				die("scan test funcs in %s: %v", dir, err)
			}
			for name, loc := range funcs {
				if _, seen := nativeFuncIndex[name]; !seen {
					nativeFuncIndex[name] = loc
				}
			}
		}
	}

	var bridgeByFilter map[string]*bridgeRows
	if *parityReport != "" {
		bridgeByFilter, err = parseParityReport(*parityReport)
		if err != nil {
			die("parse parity report: %v", err)
		}
	}

	var specByFilter map[string]*spec.Spec
	if *formatsDir != "" {
		specByFilter, err = loadSpecsForFilters(*formatsDir)
		if err != nil {
			die("load specs from %s: %v", *formatsDir, err)
		}
	}

	var bridgeSchemaProps map[string]map[string]bool
	if *bridgeSchemas != "" && len(specByFilter) > 0 {
		bridgeSchemaProps, err = loadBridgeSchemaProps(*bridgeSchemas, *okapiVersion, specByFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "contract-audit: warning: bridge schema load failed (%v); spec.config drift skipped\n", err)
			bridgeSchemaProps = nil
		}
	}

	doc := buildDoc(okapiByFilter, nativeByFilter, bridgeByFilter, specByFilter, bridgeSchemaProps, nativeAnnotations, nativeFuncIndex, okapiIDsByNative, *okapiVersion, *okapiTag, *goCommit)

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

	// Drift report runs only when both annotations and surefire are
	// present — otherwise the comparison is meaningless (no annotations
	// → no drift; no surefire → everything looks like drift).
	driftFound := false
	if len(nativeAnnotations) > 0 && len(okapiByFilter) > 0 {
		drift := detectAnnotationDrift(nativeAnnotations, okapiByFilter)
		if len(drift) > 0 {
			driftFound = true
			fmt.Fprintf(os.Stderr, "contract-audit: %d annotation(s) reference Okapi tests not present in %s:\n", len(drift), *okapiVersion)
			for _, a := range drift {
				marker := "okapi"
				if a.Kind != "" && a.Kind != "map" {
					marker = "okapi-" + a.Kind
				}
				fmt.Fprintf(os.Stderr, "  %s:%d  // %s: %s#%s  (Go func: %s)\n", a.File, a.Line, marker, a.JavaClass, a.JavaMethod, a.GoFunc)
			}
		}
	}
	// Spec ref drift: same shape as annotation drift but sourced from
	// spec.yaml okapi_refs. Reported per-filter so the location in the
	// spec is unambiguous.
	if len(specByFilter) > 0 && len(okapiByFilter) > 0 {
		var specDrift, configDrift int
		for _, fc := range doc.Filters {
			if len(fc.SpecDrift) > 0 {
				specDrift += len(fc.SpecDrift)
				fmt.Fprintf(os.Stderr, "contract-audit: %s spec.yaml has %d okapi_ref(s) not present in Okapi %s:\n", fc.FilterName, len(fc.SpecDrift), *okapiVersion)
				for _, d := range fc.SpecDrift {
					fmt.Fprintf(os.Stderr, "  feature %s: %s\n", d.FeatureID, d.OkapiRef)
				}
			}
			if len(fc.SpecConfigDrift) > 0 {
				configDrift += len(fc.SpecConfigDrift)
				fmt.Fprintf(os.Stderr, "contract-audit: %s spec.yaml has %d config key(s) not in bridge schema for Okapi %s:\n", fc.FilterName, len(fc.SpecConfigDrift), *okapiVersion)
				for _, d := range fc.SpecConfigDrift {
					if d.OkapiParam != "" {
						fmt.Fprintf(os.Stderr, "  key %q (okapi_param: %s)\n", d.Key, d.OkapiParam)
					} else {
						fmt.Fprintf(os.Stderr, "  key %q\n", d.Key)
					}
				}
			}
		}
		if specDrift > 0 || configDrift > 0 {
			driftFound = true
		}
	}
	if driftFound && *failOnDrift {
		os.Exit(1)
	}
}

// nativeFilterAliases maps an Okapi filter package id to the neokapi
// native package name when a whole Okapi package maps 1:1 to a single
// neokapi format under a different name. The dashboard then surfaces
// both names so a reviewer can navigate either side.
//
// This handles the SINGLE-format case (one Okapi package → one native
// format). Okapi packages that bundle SEVERAL filters which neokapi
// splits into separate native formats (table, plaintext) are routed at
// the test-CLASS level by classToNativeFormat instead, so each native
// format gets exactly one row.
var nativeFilterAliases = map[string]string{
	"php":        "phpcontent",
	"xmlstream":  "xml",
	"openoffice": "odf", // Okapi's openoffice module (ODF + legacy OpenOffice) ↔ neokapi odf reader
	// neokapi splits Okapi's `subtitles` filter into `vtt`+`ttml`+`srt`.
	// We keep the per-format Okapi ids and rely on the per-class join
	// in scanAnnotations to match them.
}

// classToNativeFormat routes an individual Okapi *Test class (short name)
// to the neokapi native format that implements it. This is the
// CLASS-LEVEL split for Okapi packages that bundle several distinct
// filters under one Maven module:
//
//   - net.sf.okapi.filters.table holds the comma/tab/fixed-width/generic
//     table filters; neokapi splits the fixed-width filter into its own
//     `fixedwidth` reader while the comma/tab/generic table cases are all
//     handled by the `csv` reader (which reads TSV via a delimiter
//     config and the generic table base behaviour).
//   - net.sf.okapi.filters.plaintext holds the base/regex/para/spliced
//     plaintext filters; neokapi keeps the base + regex modes in
//     `plaintext` but splits the paragraph and spliced-line variants into
//     their own `paraplaintext` and `splicedlines` readers.
//
// Without this map every class in such a package buckets into one row
// (keyed by the package name), which left native `fixedwidth`,
// `paraplaintext`, and `splicedlines` as 0-okapi phantom rows and folded
// their Okapi contract tests into the wrong format's row (#616). Classes
// not listed here fall through to package-level aliasing.
var classToNativeFormat = map[string]string{
	// net.sf.okapi.filters.table
	"CommaSeparatedValuesFilterTest": "csv",
	"TabSeparatedValuesFilterTest":   "csv", // csv reader handles TSV via delimiter config
	"TableFilterTest":                "csv", // generic table base filter → csv reader
	"FixedWidthColumnsFilterTest":    "fixedwidth",
	// net.sf.okapi.filters.plaintext
	"PlainTextFilterTest":      "plaintext",
	"RegexPlainTextFilterTest": "plaintext", // regex mode of the plaintext reader
	"ParaPlainTextFilterTest":  "paraplaintext",
	"SplicedLinesFilterTest":   "splicedlines",
}

// multiFilterPackages names Okapi Maven modules that bundle several
// filters which neokapi splits into separate native formats. Suites from
// these packages are routed by classToNativeFormat (test-class level);
// a class with no explicit entry falls back to packageDefaultNative.
var multiFilterPackages = map[string]bool{
	"table":     true,
	"plaintext": true,
}

// packageDefaultNative is the native format an unlisted class from a
// multi-filter package routes to (the "generic" reader for that family).
var packageDefaultNative = map[string]string{
	"table":     "csv",
	"plaintext": "plaintext",
}

// okapiFilterForNative is the reverse of the canonical mapping: the
// Okapi filter package id(s) whose @Test classes route to a given native
// format. Computed once at startup from nativeFilterAliases and
// classToNativeFormat so the dashboard can show the Okapi filter id(s) as
// a sublabel on each row (users navigate either side). A native format
// may aggregate more than one Okapi package (none do today, but csv
// aggregates classes from the single `table` package).
func resolveNativeFormat(okapiPkg, shortClass string) string {
	if multiFilterPackages[okapiPkg] {
		if nf, ok := classToNativeFormat[shortClass]; ok {
			return nf
		}
		if def, ok := packageDefaultNative[okapiPkg]; ok {
			return def
		}
	}
	if alias, ok := nativeFilterAliases[okapiPkg]; ok {
		return alias
	}
	return okapiPkg
}

// noNativeFilters lists Okapi filters that neokapi deliberately does not
// implement as a native Go reader — proprietary/vendor containers, Okapi
// composite meta-filters, and abstract base classes. Their behaviour is
// covered by the okapi-bridge (the Java filter runs in-process via gRPC),
// so every bare-unmapped @Test for these filters is classified
// not-applicable-to-native with an honest reason rather than counted as a
// gap (#611). When a native reader is later added, drop the entry and the
// tests become real mapping targets.
var noNativeFilters = map[string]string{
	"xini":            "no native reader — XINI (Wordbee interchange) is bridge-only",
	"sdlpackage":      "no native reader — SDL Trados package (proprietary) is bridge-only",
	"rainbowkit":      "no native reader — Okapi Rainbow translation kit is bridge-only",
	"wsxzpackage":     "no native reader — WorldServer WSXZ package is bridge-only",
	"autoxliff":       "no native reader — Okapi auto-XLIFF detection wrapper is bridge-only",
	"multiparsers":    "no native reader — Okapi multi-parsers composite filter is bridge-only",
	"cascadingfilter": "no native reader — Okapi cascading composite filter is bridge-only",
	"archive":         "no native reader — generic archive (zip) container is bridge-only",
	// abstractmarkup's tests are SimplifierRulesTest, which exercises Okapi's
	// CodeSimplifier (an inline-code merge/reduce utility) — NOT the markup
	// readers. neokapi has no native code simplifier (it preserves inline
	// codes verbatim), so this is a genuine not-implemented, not coverage
	// "exercised via concrete readers" (that earlier claim was false, #611).
	"abstractmarkup": "Okapi CodeSimplifier (inline-code merge/reduce) — no native code simplifier; neokapi preserves inline codes verbatim",
	"its":            "no standalone native ITS filter — inline ITS attributes are honored by the xml/html readers (core/its), but ITS global-rule (.fprm) processing is bridge-only",
	// NOTE: dita/docbook/resx are now NATIVE (config presets on the xml
	// reader — see core/formats/xml/presets.go), so they are intentionally
	// NOT listed here; their IT contracts map to the preset tests.
}

// noNativeCategory bins a no-native filter into a skip category for the
// dashboard's breakdown.
func noNativeCategory(filter string) string {
	switch filter {
	case "abstractmarkup":
		return "abstract"
	case "sdlpackage", "rainbowkit", "wsxzpackage":
		return "vendor"
	default:
		return "no-native"
	}
}

// parseSurefireDir walks surefireDir and returns one filterResult per
// neokapi NATIVE format, plus a side map recording the Okapi filter
// package id(s) each native format aggregates (for the dashboard's
// "Okapi filter" sublabel).
//
// Each Surefire XML maps to one Okapi @Test class. The class is routed
// to its canonical native format via resolveNativeFormat: most Okapi
// packages map 1:1 to a native format (possibly under an alias), but the
// multi-filter `table` and `plaintext` packages split per class so each
// native format (csv, fixedwidth, plaintext, paraplaintext, splicedlines)
// gets exactly one row instead of leaving phantom 0-okapi duplicates
// (#616).
func parseSurefireDir(surefireDir string) (map[string]*filterResult, map[string]map[string]bool, error) {
	pkgRE := regexp.MustCompile(`^net\.sf\.okapi\.filters\.([^.]+)`)
	out := map[string]*filterResult{}
	okapiIDs := map[string]map[string]bool{}
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
		okapiPkg := m[1]
		filterName := resolveNativeFormat(okapiPkg, shortClass(ts.Name))
		fr, ok := out[filterName]
		if !ok {
			fr = &filterResult{}
			out[filterName] = fr
		}
		addSuiteToResult(fr, collapseSuite(ts))
		recordOkapiID(okapiIDs, filterName, okapiPkg)
		return nil
	})
	return out, okapiIDs, err
}

// recordOkapiID notes that native format `native` aggregates @Test
// classes from Okapi filter package `okapiPkg`.
func recordOkapiID(m map[string]map[string]bool, native, okapiPkg string) {
	set := m[native]
	if set == nil {
		set = map[string]bool{}
		m[native] = set
	}
	set[okapiPkg] = true
}

// collapseSuite turns one parsed JUnit test-suite XML into a testSuite,
// collapsing JUnit parameterized invocations (method[0: a], method[1: b], …)
// into one logical contract row per base method. Each @Test method is one
// behavioural contract regardless of how many fixtures it iterates, so the
// dashboard counts methods, not fixture×method cells (#611). Status
// aggregates pessimistically: fail dominates error dominates pass dominates
// skip, mirroring "the method passes for every parameter".
func collapseSuite(ts sfTestSuite) testSuite {
	suite := testSuite{Name: ts.Name, DurationMS: parseSecondsToMs(ts.Time)}
	type acc struct {
		className                string
		anyFail, anyErr, anyPass bool
		allSkip                  bool
		dur                      int64
		params                   int
	}
	order := make([]string, 0, len(ts.TestCase))
	byBase := map[string]*acc{}
	for _, tc := range ts.TestCase {
		base := stripParamSuffix(tc.Name)
		a := byBase[base]
		if a == nil {
			a = &acc{className: tc.ClassName, allSkip: true}
			byBase[base] = a
			order = append(order, base)
		}
		a.params++
		a.dur += parseSecondsToMs(tc.Time)
		switch {
		case tc.Failure != nil:
			a.anyFail = true
			a.allSkip = false
		case tc.Error != nil:
			a.anyErr = true
			a.allSkip = false
		case tc.Skipped != nil:
			// keep allSkip
		default:
			a.anyPass = true
			a.allSkip = false
		}
	}
	for _, base := range order {
		a := byBase[base]
		status := "skip"
		switch {
		case a.anyFail:
			status = "fail"
		case a.anyErr:
			status = "error"
		case a.anyPass:
			status = "pass"
		}
		params := 0
		if a.params > 1 {
			params = a.params
		}
		suite.Tests = append(suite.Tests, testCase{
			Name:       base,
			ClassName:  a.className,
			Status:     status,
			DurationMS: a.dur,
			Params:     params,
		})
		suite.Total++
		switch status {
		case "pass":
			suite.Passed++
		case "fail":
			suite.Failed++
		case "skip":
			suite.Skipped++
		case "error":
			suite.Errors++
		}
	}
	return suite
}

// addSuiteToResult appends a suite to a filterResult and rolls its
// per-status counts into the result totals.
func addSuiteToResult(fr *filterResult, suite testSuite) {
	fr.Suites = append(fr.Suites, suite)
	fr.Total += suite.Total
	fr.Passed += suite.Passed
	fr.Failed += suite.Failed
	fr.Skipped += suite.Skipped
	fr.Errors += suite.Errors
}

// parseFailsafeDir walks a Maven Failsafe report directory and returns one
// filterResult per neokapi NATIVE format, plus a side map of the Okapi
// filter id(s) each native format aggregates. The integration tests live
// in net.sf.okapi.{roundtrip,xliffcompare}.integration, so the package
// prefix can't be used — itClassToFilter derives the Okapi filter id from
// the IT class stem, then resolveNativeFormat routes it to the canonical
// native format (so RoundTripTableIT joins the `csv` row, not a `table`
// one). Unmappable IT suites (memory-leak, pipeline, conversion) are
// skipped.
func parseFailsafeDir(dir string) (map[string]*filterResult, map[string]map[string]bool, error) {
	out := map[string]*filterResult{}
	okapiIDs := map[string]map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
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
		// Only the per-filter IT families map to a filter contract: the
		// roundtrip (extract→merge over a file corpus) and xliff-compare
		// (extract→XLIFF vs gold) integration tests. The simplifier,
		// memory-leak, pipeline, and conversion IT suites are not filter
		// contracts and are skipped.
		if !strings.HasPrefix(ts.Name, "net.sf.okapi.roundtrip.integration.") &&
			!strings.HasPrefix(ts.Name, "net.sf.okapi.xliffcompare.integration.") {
			return nil
		}
		okapiFilter := itClassToFilter(shortClass(ts.Name))
		if okapiFilter == "" {
			return nil
		}
		filterName := resolveNativeFormat(okapiFilter, shortClass(ts.Name))
		suite := collapseSuite(ts)
		dropDebugTests(&suite) // debug/debug2 @Test methods are dev helpers, not contracts
		if suite.Total == 0 {
			return nil
		}
		fr, ok := out[filterName]
		if !ok {
			fr = &filterResult{}
			out[filterName] = fr
		}
		addSuiteToResult(fr, suite)
		recordOkapiID(okapiIDs, filterName, okapiFilter)
		return nil
	})
	return out, okapiIDs, err
}

// dropDebugTests removes the "debug"/"debug2" helper @Test methods some
// Okapi roundtrip IT classes carry — they aren't behavioural contracts.
func dropDebugTests(suite *testSuite) {
	kept := suite.Tests[:0]
	suite.Total, suite.Passed, suite.Failed, suite.Skipped, suite.Errors = 0, 0, 0, 0, 0
	for _, tc := range suite.Tests {
		if tc.Name == "debug" || tc.Name == "debug2" {
			continue
		}
		kept = append(kept, tc)
		suite.Total++
		switch tc.Status {
		case "pass":
			suite.Passed++
		case "fail":
			suite.Failed++
		case "skip":
			suite.Skipped++
		case "error":
			suite.Errors++
		}
	}
	suite.Tests = kept
}

// mergeOkapiIDs folds the per-native-format Okapi-id sets from src into
// dst so the Failsafe IT contracts contribute their Okapi filter ids to
// the same native-format sublabel the Surefire unit tests produced.
func mergeOkapiIDs(dst, src map[string]map[string]bool) {
	for native, set := range src {
		for okapiPkg := range set {
			recordOkapiID(dst, native, okapiPkg)
		}
	}
}

// mergeOkapiResults folds the per-filter results from src into dst,
// appending suites and summing totals so Failsafe IT contracts join the
// same filter rows the Surefire unit tests produced.
func mergeOkapiResults(dst, src map[string]*filterResult) {
	for name, sfr := range src {
		dfr, ok := dst[name]
		if !ok {
			dfr = &filterResult{}
			dst[name] = dfr
		}
		for _, suite := range sfr.Suites {
			addSuiteToResult(dfr, suite)
		}
	}
}

// itClassToFilter maps an Okapi integration-test class (short name) to the
// filter id it exercises. Recognises the two filter-scoped IT families:
//
//	RoundTrip<Name>IT      → <name>   (roundtrip.integration)
//	<Name>XliffCompareIT   → <name>   (xliffcompare.integration)
//
// Returns "" for IT classes that don't map to a single filter (memory-leak,
// pipeline, conversion). The PascalCase <Name> is normalised to the filter
// id, with the same aliases the rest of the audit uses.
func itClassToFilter(short string) string {
	var name string
	switch {
	case strings.HasPrefix(short, "RoundTrip") && strings.HasSuffix(short, "IT"):
		name = strings.TrimSuffix(strings.TrimPrefix(short, "RoundTrip"), "IT")
	case strings.HasSuffix(short, "XliffCompareIT"):
		name = strings.TrimSuffix(short, "XliffCompareIT")
	default:
		return ""
	}
	if name == "" {
		return ""
	}
	if alias, ok := itFilterAliases[strings.ToLower(name)]; ok {
		return alias
	}
	return strings.ToLower(name)
}

// itFilterAliases normalises IT class-name stems whose lower-cased form
// doesn't equal the filter id.
var itFilterAliases = map[string]string{
	"openxm":            "openxml",
	"wik":               "wiki",
	"property":          "properties",
	"jsonmessageformat": "messageformat",
	"yamlmessageformat": "messageformat",
	"htmlits":           "its",
	"openoffice":        "openoffice",
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

// bridgeRows holds the parity outcomes for one filter so the dashboard
// can render them as distinct test cases inside the synthetic "bridge
// parity" suite. Tikal isn't strictly the bridge — it's a third
// reference corner — but rendering it in the same column keeps the
// per-filter parity story in one place.
//
// Fixtures keys per-Java-test fixture parity rows by their
// "ClassName#methodName" ref (in either FQN or short class form). The
// per-row lookup against the surefire join produces true per-test
// bridge granularity instead of one filter-level badge.
type bridgeRows struct {
	Read      *parityRow            // Kind="format" (head-to-head reader comparison)
	RoundTrip *parityRow            // Kind="format-roundtrip" (reader+writer byte parity, native vs bridge)
	Tikal     *parityRow            // Kind="format-tikal" (native round-trip vs Okapi tikal CLI)
	Fixtures  map[string]*parityRow // Kind="format-fixture" keyed by JavaClass#JavaMethod (short class form)
	Spec      []*specRow            // Kind="format-spec-feature" — one row per feature × example
}

// specRow carries one Feature × Example outcome from the spec runner,
// flattened from the parity Outcome. The dashboard groups these by
// feature and shows the per-example status alongside it.
type specRow struct {
	FeatureID string `json:"featureId"`
	Example   string `json:"example"`
	Status    string `json:"status"` // pass | fail | skip | expected_fail | parity_warn
	Mode      string `json:"mode"`   // head-to-head | bridge-only
	Detail    string `json:"detail,omitempty"`
}

// parseParityReport reads the parity JSON and returns a map keyed by
// filter id (with `okf_` stripped) so the bridge column can join
// against the same names the rest of the generator already uses. Both
// `format` and `format-roundtrip` rows are aggregated per filter.
func parseParityReport(path string) (map[string]*bridgeRows, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []parityRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	out := map[string]*bridgeRows{}
	for _, r := range rows {
		key := strings.TrimPrefix(r.ID, "okf_")
		entry, ok := out[key]
		if !ok {
			entry = &bridgeRows{}
			out[key] = entry
		}
		row := r // local copy so &row is stable per loop iteration
		switch r.Kind {
		case "format":
			entry.Read = &row
		case "format-roundtrip":
			entry.RoundTrip = &row
		case "format-tikal":
			entry.Tikal = &row
		case "format-fixture":
			// Composite ID: "okf_<filter>::ClassName#methodName".
			// Re-key the entry under the bare filter name and store
			// per-test rows in Fixtures.
			parts := strings.SplitN(r.ID, "::", 2)
			if len(parts) != 2 {
				continue
			}
			filterKey := strings.TrimPrefix(parts[0], "okf_")
			testRef := parts[1] // ClassName#methodName
			fxEntry, ok := out[filterKey]
			if !ok {
				fxEntry = &bridgeRows{}
				out[filterKey] = fxEntry
			}
			if fxEntry.Fixtures == nil {
				fxEntry.Fixtures = map[string]*parityRow{}
			}
			fxEntry.Fixtures[testRef] = &row
			// The composite-key entry built above (out[key]) is now
			// stale (its key still has the ::ref suffix). Drop it.
			delete(out, key)
			continue
		case "format-spec-feature":
			// Composite ID: "okf_<filter>::<featureId>::<exampleName>".
			parts := strings.SplitN(r.ID, "::", 3)
			if len(parts) != 3 {
				continue
			}
			filterKey := strings.TrimPrefix(parts[0], "okf_")
			featureID := parts[1]
			exampleName := parts[2]
			specEntry, ok := out[filterKey]
			if !ok {
				specEntry = &bridgeRows{}
				out[filterKey] = specEntry
			}
			specEntry.Spec = append(specEntry.Spec, &specRow{
				FeatureID: featureID,
				Example:   exampleName,
				Status:    r.Status,
				Mode:      r.Mode,
				Detail:    r.Detail,
			})
			delete(out, key)
			continue
		}
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
func buildDoc(okapiByFilter, nativeByFilter map[string]*filterResult, bridgeByFilter map[string]*bridgeRows, specByFilter map[string]*spec.Spec, bridgeSchemaProps map[string]map[string]bool, annotations []annotation, nativeFuncIndex map[string]funcLoc, okapiIDsByNative map[string]map[string]bool, okapiVersion, okapiTag, goCommit string) testComparisonData {
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
	// native format dir → parity-report key (the spec's okf_<id> with the
	// prefix stripped). The parity report keys by the spec format id, which
	// can diverge from the native dir name (csv → commaseparatedvalues).
	nativeToParityKey := map[string]string{}
	for native, s := range specByFilter {
		if pk := strings.TrimPrefix(s.Format, "okf_"); pk != "" {
			nativeToParityKey[native] = pk
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
		// Okapi-side sublabel: the Okapi filter package id(s) whose @Test
		// classes route to this native format (e.g. csv ← table). Empty
		// for native-only formats with no Okapi equivalent.
		if ids := sortedKeys(okapiIDsByNative[name]); len(ids) > 0 {
			fc.OkapiFilterIDs = ids
		}
		// Native lookup — the row is keyed by the canonical native format
		// id, so nativeByFilter joins directly on `name`. NativeFilterName
		// is set to the same id for backward compat with older consumers.
		if r := nativeByFilter[name]; r != nil {
			fc.Native = r
			fc.NativeFilterName = name
			sum.TotalFiltersNative++
			sum.TotalTestsNative += r.Total
		}
		if fc.Okapi != nil && fc.Native != nil {
			sum.TotalFiltersBoth++
		}
		// Bridge column: synthesize a single "bridge parity" suite from
		// the per-filter outcomes (read / round-trip / tikal). Per-test
		// bridge granularity flows separately into testCaseMatch.Bridge*
		// fields below via fixture rows in the parity report.
		var brEntry *bridgeRows
		if br, ok := lookupParity(bridgeByFilter, nativeToParityKey, name); ok {
			brEntry = br
			fc.Bridge = parityToFilterResult(br)
			sum.TotalFiltersBridge++
		}
		// Build one row per Okapi @Test method, joined with annotations
		// and per-fixture bridge outcomes.
		fc.TestCases = buildRows(fc.Okapi, annByOkapi, nativeStatus, brEntry, nativeFuncIndex)
		// No-native classification: for filters neokapi deliberately does
		// not implement natively, mark every still-bare-unmapped row
		// not-applicable (bridge-only) with an honest reason, so they are
		// not counted as gaps. Rows already mapped/skipped by an annotation
		// are left untouched.
		if reason, ok := noNativeFilters[name]; ok {
			cat := noNativeCategory(name)
			for i := range fc.TestCases {
				r := &fc.TestCases[i]
				if r.TestState == "" && r.SkipReason == "" {
					r.TestState = "skipped"
					r.SkipReason = reason
					r.SkipCategory = cat
				}
			}
		}
		// Per-test bridge coverage: count rows with a bridge status
		// populated (i.e. a fixture flowed through the bridge for that
		// @Test). This is the honest signal of bridge-test reach.
		for _, r := range fc.TestCases {
			if r.BridgeStatus != "" {
				sum.TotalTestsBridge++
			}
		}
		fc.Coverage = computeCoverageFromRows(fc.Okapi, fc.TestCases)
		// Spec features (when the filter has a spec.yaml driving the
		// parity runner): group rows by feature, tally totals.
		if brEntry != nil && len(brEntry.Spec) > 0 {
			fc.Spec = buildSpecSummary(brEntry.Spec, specByFilter[name])
		}
		// Spec ref drift: each spec.yaml feature.okapi_refs entry must
		// resolve against the pinned Okapi @Test set. Mismatches surface
		// per-filter so reviewers can see exactly which refs went stale.
		if s, ok := specByFilter[name]; ok {
			fc.SpecKind = string(s.Kind)
			if fc.SpecKind == "" {
				fc.SpecKind = string(spec.KindTopLevel)
			}
			fc.SpecDrift = detectSpecRefDrift(s, fc.Okapi)
			// Spec config drift: each spec.config[].key must match a
			// property in the bridge composite schema for the pinned
			// Okapi version. Filters without a loaded schema are
			// skipped (the map miss is benign). Subfilters have no
			// top-level bridge JSON Schema (they're invoked through a
			// parent), so the drift check is skipped for them.
			if !s.IsSubfilter() {
				if props, ok := bridgeSchemaProps[name]; ok {
					fc.SpecConfigDrift = detectSpecConfigDrift(s, props)
				}
			}
		}
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
// against annotations, native go-test status, and per-fixture bridge
// outcomes from the parity report.
func buildRows(okapi *filterResult, annByOkapi map[string]annotation, nativeStatus map[string]string, br *bridgeRows, nativeFuncIndex map[string]funcLoc) []testCaseMatch {
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
				Params:      tc.Params,
			}
			// Per-test bridge join. Fixtures key on short-class form
			// (HtmlSnippetsTest#testFoo); accept either FQN or short
			// for safety.
			if br != nil && br.Fixtures != nil {
				if fx, ok := br.Fixtures[shortClass(javaClass)+"#"+tc.Name]; ok {
					row.BridgeStatus = bridgeStatusFromParity(fx.Status)
					row.BridgeTest = fx.Name
				} else if fx, ok := br.Fixtures[javaClass+"#"+tc.Name]; ok {
					row.BridgeStatus = bridgeStatusFromParity(fx.Status)
					row.BridgeTest = fx.Name
				}
			}
			ann, ok := annByOkapi[javaClass+"#"+tc.Name]
			if !ok {
				// Try short-class match (Surefire uses FQN; annotations
				// usually use short class).
				ann, ok = annByOkapi[shortClass(javaClass)+"#"+tc.Name]
			}
			switch {
			case !ok:
				// Dashboard convention: bare-unmapped rows (no annotation
				// of any kind) leave testState empty — the only true gap.
			case ann.Skip():
				// okapi-skip / okapi-unmapped / okapi-deferred all resolve
				// to a reviewed not-applicable state with a reason. The
				// marker kind feeds the category so the dashboard can tell
				// "won't port (design)" from "covered indirectly".
				row.TestState = "skipped"
				row.SkipReason = ann.Reason
				row.SkipCategory = classifySkipKind(ann.Kind, ann.Reason)
				row.NativeTest = ann.GoFunc
				row.NativeFile = ann.File
				row.NativeLine = ann.Line
				// If the reason claims the behaviour is covered by a named
				// native test, resolve it to a verifiable source link so the
				// "covered elsewhere" claim isn't just an assertion (#611).
				if name, loc, ok := resolveCover(ann.Reason, nativeFuncIndex); ok {
					row.CoveredByTest = name
					row.CoveredByFile = loc.File
					row.CoveredByLine = loc.Line
				}
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

// buildSpecSummary groups raw spec rows by feature (preserving the
// order they appeared in the parity report so spec authors see their
// declared sequence) and tallies status totals across all examples. For
// each expected_fail / parity_warn example it attributes a divergence
// category: an explicit divergence_kind on the matching spec.yaml example
// wins; otherwise the kind is inferred from the detail text by
// classifyDivergence. `s` is the loaded spec for this filter (may be nil
// when only the parity report carries the rows).
func buildSpecSummary(rows []*specRow, s *spec.Spec) *specSummary {
	if len(rows) == 0 {
		return nil
	}
	overrides := divergenceOverrides(s)
	out := &specSummary{}
	byFeature := map[string]int{} // feature id → index into out.Features
	for _, r := range rows {
		idx, ok := byFeature[r.FeatureID]
		if !ok {
			out.Features = append(out.Features, specFeature{ID: r.FeatureID})
			idx = len(out.Features) - 1
			byFeature[r.FeatureID] = idx
		}
		var divergence string
		if r.Status == "expected_fail" || r.Status == "parity_warn" {
			if k, ok := overrides[r.FeatureID+"\x00"+r.Example]; ok && k != "" {
				divergence = k // explicit author override wins
			} else {
				divergence = classifyDivergence(r.Detail)
			}
		}
		out.Features[idx].Examples = append(out.Features[idx].Examples, specExample{
			Name:       r.Example,
			Status:     r.Status,
			Mode:       r.Mode,
			Detail:     r.Detail,
			Divergence: divergence,
		})
		switch r.Status {
		case "pass":
			out.Pass++
		case "fail":
			out.Fail++
		case "skip":
			out.Skip++
		case "expected_fail":
			out.ExpectedFail++
		case "parity_warn":
			out.ParityWarn++
		}
	}
	return out
}

// divergenceOverrides indexes the explicit divergence_kind set on each
// spec.yaml example by "featureID\x00exampleName" so buildSpecSummary can
// let an author-set kind win over the detail-text heuristic.
func divergenceOverrides(s *spec.Spec) map[string]string {
	if s == nil {
		return nil
	}
	out := map[string]string{}
	for _, f := range s.Features {
		for _, ex := range f.Examples {
			if ex.DivergenceKind != "" {
				out[f.ID+"\x00"+ex.Name] = ex.DivergenceKind
			}
		}
	}
	return out
}

// classifyDivergence infers a divergence category from an expected_fail /
// parity_warn detail string using deterministic keyword rules. The
// reasons authored across the spec corpus consistently open with a small
// set of phrases; the ordering below matters (most specific first). When
// nothing matches, the example is attributed "scope-diff" — a neutral,
// native-is-correct default — rather than the alarming "native-bug".
func classifyDivergence(detail string) string {
	d := strings.ToLower(detail)
	switch {
	case d == "":
		return "scope-diff"
	// native-bug FIRST so a "native bug" phrase isn't shadowed by the
	// broader "bridge"/"bug" checks below.
	case strings.Contains(d, "native bug"), strings.Contains(d, "native-bug"):
		return "native-bug"
	case strings.Contains(d, "does not ship the okf_"),
		strings.Contains(d, "doesn't ship the okf_"),
		strings.Contains(d, "missing filter"),
		strings.Contains(d, "no okf_"):
		return "missing-filter"
	case strings.Contains(d, "synthetic-fixture"),
		strings.Contains(d, "synthetic fixture"),
		strings.Contains(d, "test-infra"),
		strings.Contains(d, "fixture artefact"),
		strings.Contains(d, "fixture artifact"):
		return "fixture"
	case strings.Contains(d, "parse error"),
		strings.Contains(d, "parse-error"),
		strings.Contains(d, "no blocks"),
		strings.Contains(d, "by design"),
		strings.Contains(d, "by-design"):
		return "contract"
	case strings.Contains(d, "default-config"),
		strings.Contains(d, "default config"),
		strings.Contains(d, "bridge default"),
		strings.Contains(d, "default differs"),
		strings.Contains(d, "differing default"):
		return "default-diff"
	case strings.Contains(d, "transport gap"),
		strings.Contains(d, "transport-gap"),
		strings.Contains(d, "bridge config"),
		strings.Contains(d, "config divergence"),
		strings.Contains(d, "over grpc"),
		strings.Contains(d, "cannot receive"),
		strings.Contains(d, "can't receive"),
		strings.Contains(d, "bridge gap"),
		strings.Contains(d, "bridge-gap"),
		// Bridge runtime failures: the okapi-bridge daemon couldn't
		// instantiate/run the filter or receive the input over gRPC.
		// The native reader handles the input correctly.
		strings.Contains(d, "bridge error"),
		strings.Contains(d, "cannot instantiate filter"),
		strings.Contains(d, "cannot invoke"),
		strings.Contains(d, "drain stream"),
		strings.Contains(d, "daemon emits 0"),
		strings.Contains(d, "daemon emits zero"),
		strings.Contains(d, "emits zero blocks"):
		return "bridge-gap"
	case strings.Contains(d, "okapi bug"),
		strings.Contains(d, "okapi-bug"),
		strings.Contains(d, "upstream bug"),
		strings.Contains(d, "bridge bug"):
		// "bridge bug" here means the bridge/upstream Okapi filter is
		// wrong (native is correct), distinct from a neokapi native bug.
		return "okapi-bug"
	case strings.Contains(d, "feature difference"),
		strings.Contains(d, "feature-difference"),
		strings.Contains(d, "different feature scope"),
		strings.Contains(d, "feature scope"),
		strings.Contains(d, "trados"):
		return "scope-diff"
	default:
		// Unmatched divergences are correct-by-design until proven a bug
		// — default to the neutral scope-diff, never native-bug.
		return "scope-diff"
	}
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

// lookupParity finds the parity rows for a native format. The parity
// report keys rows by the spec's `okf_<format-id>` (e.g. csv declares
// okf_commaseparatedvalues), which can diverge from the native directory
// name. nativeToParityKey (built from the loaded specs) bridges that gap;
// the static bridgeFilterAliases handles the few formats with no spec
// (php↔phpcontent).
func lookupParity(bridgeByFilter map[string]*bridgeRows, nativeToParityKey map[string]string, name string) (*bridgeRows, bool) {
	if r, ok := bridgeByFilter[name]; ok {
		return r, true
	}
	// Native dir → parity key from the spec format id (csv →
	// commaseparatedvalues, fixedwidth → fixedwidthcolumns, …).
	if pk, ok := nativeToParityKey[name]; ok && pk != name {
		if r, ok := bridgeByFilter[pk]; ok {
			return r, true
		}
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
	return nil, false
}

// parityToFilterResult turns the parity rows for one filter into a
// synthetic "bridge parity" suite. Read parity (`format`) is always
// emitted; round-trip parity (`format-roundtrip`) is appended only
// when the harness produced a row, so the dashboard distinguishes
// "round-trip not exercised" from "round-trip failed".
func parityToFilterResult(b *bridgeRows) *filterResult {
	if b == nil {
		return nil
	}
	suite := testSuite{Name: "bridge parity"}
	fr := &filterResult{}
	if b.Read != nil {
		appendParityCase(&suite, fr, b.Read, "read")
	}
	if b.RoundTrip != nil {
		appendParityCase(&suite, fr, b.RoundTrip, "round-trip")
	}
	if b.Tikal != nil {
		appendParityCase(&suite, fr, b.Tikal, "tikal")
	}
	if suite.Total == 0 {
		return nil
	}
	fr.Suites = []testSuite{suite}
	return fr
}

func appendParityCase(suite *testSuite, fr *filterResult, r *parityRow, label string) {
	status := "skip"
	switch r.Status {
	case "pass":
		status = "pass"
	case "fail":
		status = "fail"
	}
	suite.Tests = append(suite.Tests, testCase{
		Name:       label,
		Status:     status,
		DurationMS: r.Duration,
	})
	suite.Total++
	suite.DurationMS += r.Duration
	fr.Total++
	switch status {
	case "pass":
		suite.Passed++
		fr.Passed++
	case "skip":
		suite.Skipped++
		fr.Skipped++
	case "fail":
		suite.Failed++
		fr.Failed++
	}
}

// ── Spec loading & drift detection ──────────────────────────────────────────

// loadSpecsForFilters walks formatsDir for `<filter>/spec.yaml` files
// and returns one Spec per filter, keyed by the NATIVE format directory
// name (e.g. "csv", "fixedwidth") so it joins directly against the
// canonical native-format row keys the rest of the generator uses.
// (The spec's own `format:` id can diverge from the dir name —
// core/formats/csv declares okf_commaseparatedvalues — so keying by the
// format id would miss the row; the bridge-schema lookup still uses
// s.Format internally.) Directories without a spec.yaml are skipped
// silently — the spec model is opt-in per format.
func loadSpecsForFilters(formatsDir string) (map[string]*spec.Spec, error) {
	out := map[string]*spec.Spec{}
	entries, err := os.ReadDir(formatsDir)
	if err != nil {
		// Missing dir is not fatal — the spec model is optional.
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read %s: %w", formatsDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(formatsDir, e.Name(), "spec.yaml")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		s, err := spec.Load(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		out[e.Name()] = s
	}
	return out, nil
}

// detectSpecRefDrift returns one entry per spec.okapi_refs ref that no
// longer matches any test in the pinned Okapi surefire result for this
// filter. Ref matching mirrors the annotation join: both FQN+method and
// shortClass+method are accepted. Returns nil when there is no Okapi
// data (drift would be meaningless against an empty set).
func detectSpecRefDrift(s *spec.Spec, okapi *filterResult) []specDriftEntry {
	if s == nil || okapi == nil {
		return nil
	}
	valid := map[string]struct{}{}
	for _, suite := range okapi.Suites {
		for _, tc := range suite.Tests {
			valid[tc.ClassName+"#"+tc.Name] = struct{}{}
			valid[shortClass(tc.ClassName)+"#"+tc.Name] = struct{}{}
		}
	}
	var out []specDriftEntry
	for _, f := range s.Features {
		for _, ref := range f.OkapiRefs {
			if _, ok := valid[ref]; ok {
				continue
			}
			out = append(out, specDriftEntry{
				FeatureID: f.ID,
				OkapiRef:  ref,
				Reason:    "missing-from-okapi",
			})
		}
	}
	return out
}

// detectSpecConfigDrift returns one entry per spec.config[].key that
// doesn't exist as a property anywhere in the bridge composite schema
// for the pinned Okapi version. The bridge schema's nested layout
// (general/word/excel for openxml, extraction/output for json …) is
// flattened to a single set of property names; the spec author writes
// flat keys, so flat membership matching is the right join.
//
// Each leaf in the schema may carry an `x-flattenPath` annotation
// naming the legacy Java parameter (e.g. `extractAll` carries
// `extractAllPairs`). Both the schema leaf name and the flatten-path
// alias are accepted as valid spec keys (#518): the bridge runtime's
// ParameterApplier accepts the legacy names while the schema documents
// the new leaves; either form is a real parameter and shouldn't drift.
func detectSpecConfigDrift(s *spec.Spec, props map[string]bool) []specConfigDriftEntry {
	if s == nil || len(props) == 0 {
		return nil
	}
	var out []specConfigDriftEntry
	for _, c := range s.Config {
		if props[c.Key] {
			continue
		}
		out = append(out, specConfigDriftEntry{
			Key:        c.Key,
			OkapiParam: c.OkapiParam,
			Reason:     "missing-from-bridge-schema",
		})
	}
	return out
}

// ── Bridge composite schema loading ─────────────────────────────────────────

// bridgeVersionsFile mirrors the subset of okapi-bridge/schemas/versions.json
// that the drift check needs: per filter id, a list of versions each with
// the Okapi releases it covers.
type bridgeVersionsFile struct {
	Filters map[string]struct {
		Versions []struct {
			Version       int      `json:"version"`
			OkapiVersions []string `json:"okapiVersions"`
		} `json:"versions"`
	} `json:"filters"`
}

// loadBridgeSchemaProps reads versions.json and, for every filter that
// has a spec.yaml in specByFilter, picks the highest schema version
// whose okapiVersions includes the pinned okapiVersion, loads its
// composite JSON Schema, and returns the flat set of property names
// found anywhere in the schema. The returned map is keyed by the bare
// filter id (no okf_ prefix) so it joins against the rest of the
// generator's filter-name space.
//
// Filters without a matching schema version are skipped silently —
// the spec model is opt-in and a brand-new filter may not have a
// bridge counterpart yet. The whole load is best-effort; partial
// failure (one unreadable schema file) is logged but doesn't abort
// the audit.
func loadBridgeSchemaProps(schemasDir, okapiVersion string, specByFilter map[string]*spec.Spec) (map[string]map[string]bool, error) {
	versionsPath := filepath.Join(schemasDir, "versions.json")
	data, err := os.ReadFile(versionsPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", versionsPath, err)
	}
	var v bridgeVersionsFile
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", versionsPath, err)
	}
	out := map[string]map[string]bool{}
	for filterKey, s := range specByFilter {
		// Subfilters have no top-level bridge JSON Schema — they're
		// invoked through their parent filter's content path.
		if s.IsSubfilter() {
			continue
		}
		// versions.json keys filters by their okf_ id (e.g. okf_openxml).
		fullID := s.Format
		if fullID == "" {
			fullID = "okf_" + filterKey
		}
		entry, ok := v.Filters[fullID]
		if !ok {
			continue
		}
		// Pick the highest version whose okapiVersions includes the
		// pinned okapiVersion. Multiple bridge versions can target one
		// Okapi release (the bridge schema can iterate independently);
		// the latest is the one a fresh build would produce.
		bestVersion := -1
		for _, ver := range entry.Versions {
			for _, ov := range ver.OkapiVersions {
				if ov == okapiVersion && ver.Version > bestVersion {
					bestVersion = ver.Version
				}
			}
		}
		if bestVersion < 0 {
			continue
		}
		schemaPath := filepath.Join(schemasDir, "filters", "composite", fmt.Sprintf("%s.v%d.schema.json", fullID, bestVersion))
		props, err := loadSchemaPropertyNames(schemaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "contract-audit: warning: load %s: %v\n", schemaPath, err)
			continue
		}
		out[filterKey] = props
	}
	return out, nil
}

// loadSchemaPropertyNames opens a JSON Schema file and returns the set
// of every property name reachable through nested `properties` blocks
// at any depth. Walks `properties.<key>.properties.<sub>...` recursively
// so the openxml layout (general/word/excel groupings) collapses to a
// single membership set the spec's flat keys can be checked against.
func loadSchemaPropertyNames(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	out := map[string]bool{}
	collectSchemaProperties(raw, out)
	return out, nil
}

// collectSchemaProperties walks a JSON Schema fragment and adds every
// property name it sees under any "properties" block. Handles nested
// objects, oneOf/anyOf/allOf branches, and array `items` schemas so a
// single pass covers the formats the bridge actually emits.
//
// Also harvests `x-flattenPath` annotations: the bridge composite
// schema marks each leaf with the legacy Java parameter name the
// filter's runtime ParameterApplier accepts (e.g. `extractAll` carries
// `"x-flattenPath": "extractAllPairs"`). Spec authors may use either
// form as the spec.config[].key — both are recognized as the same
// parameter, so both must satisfy the drift check (#518).
func collectSchemaProperties(node any, out map[string]bool) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	if props, ok := m["properties"].(map[string]any); ok {
		for key, sub := range props {
			out[key] = true
			collectSchemaProperties(sub, out)
		}
	}
	if fp, ok := m["x-flattenPath"].(string); ok && fp != "" {
		out[fp] = true
	}
	for _, k := range []string{"oneOf", "anyOf", "allOf"} {
		if arr, ok := m[k].([]any); ok {
			for _, sub := range arr {
				collectSchemaProperties(sub, out)
			}
		}
	}
	if items, ok := m["items"]; ok {
		collectSchemaProperties(items, out)
	}
	if ap, ok := m["additionalProperties"].(map[string]any); ok {
		collectSchemaProperties(ap, out)
	}
}

// ── Annotation drift detection ──────────────────────────────────────────────

// detectAnnotationDrift returns annotations whose JavaClass#JavaMethod
// no longer matches any test case in the pinned Okapi Surefire output.
// These are the annotations that quietly orphan — the dashboard would
// otherwise lose the row entirely without telling anyone, because the
// surefire join produces no row for a method that doesn't exist.
//
// Matching mirrors buildRows: the annotation matches if its key is
// present either as FQN+method (e.g.
// `net.sf.okapi.filters.html.HtmlSnippetsTest#testFoo`) or as
// shortClass+method (`HtmlSnippetsTest#testFoo`). Annotations always
// use short class form in practice, but we accept both so users have
// the option to disambiguate.
func detectAnnotationDrift(annotations []annotation, okapiByFilter map[string]*filterResult) []annotation {
	valid := map[string]struct{}{}
	for _, fr := range okapiByFilter {
		for _, suite := range fr.Suites {
			for _, tc := range suite.Tests {
				valid[tc.ClassName+"#"+tc.Name] = struct{}{}
				valid[shortClass(tc.ClassName)+"#"+tc.Name] = struct{}{}
			}
		}
	}
	var drift []annotation
	for _, a := range annotations {
		if _, ok := valid[a.JavaClass+"#"+a.JavaMethod]; ok {
			continue
		}
		drift = append(drift, a)
	}
	sort.SliceStable(drift, func(i, j int) bool {
		if drift[i].File != drift[j].File {
			return drift[i].File < drift[j].File
		}
		return drift[i].Line < drift[j].Line
	})
	return drift
}

// bridgeStatusFromParity normalises parity-report statuses ("pass" /
// "fail" / "skip") to the dashboard's status vocabulary which mirrors
// the Go-test convention.
func bridgeStatusFromParity(s string) string {
	switch s {
	case "pass", "fail", "skip":
		return s
	default:
		return ""
	}
}

// classifySkipKind bins a not-applicable annotation into a SkipCategory,
// preferring the marker kind where it carries meaning the free-text
// reason might not. okapi-deferred always means "behavior covered
// indirectly", so it gets its own category regardless of wording; the
// reason text classifies the rest.
func classifySkipKind(kind, reason string) string {
	if kind == "deferred" {
		return "deferred"
	}
	if c := classifySkip(reason); c != "other" {
		return c
	}
	if kind == "unmapped" {
		// Reviewed gap with a reason that didn't match a keyword bucket.
		return "acknowledged"
	}
	return "other"
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
	// Kind classifies the annotation marker:
	//   "map"      — // okapi:           (a native test verifies this contract)
	//   "skip"     — // okapi-skip:      (not applicable to neokapi native by design)
	//   "unmapped" — // okapi-unmapped:  (reviewed gap, not ported, reason given)
	//   "deferred" — // okapi-deferred:  (behavior covered indirectly / corpus-gated)
	// Any kind other than "map" resolves to a reviewed not-applicable
	// state: the test has been looked at and a reason recorded, so it is
	// NOT a bare gap.
	Kind   string
	Reason string // free-text reason after an em-dash / hyphen / parenthetical
	GoFunc string // the Go func it sits above (e.g. TestSnippets_Foo)
	File   string // path relative to the scan root, then prefixed with the package dir
	Line   int    // 1-based line of the func declaration
}

// Skip reports whether this annotation marks a reviewed, not-to-be-mapped
// contract (any marker other than a live // okapi: mapping).
func (a annotation) Skip() bool { return a.Kind != "map" }

var (
	// One regex matches every per-test okapi annotation marker; the
	// optional first group captures the marker suffix
	// (-skip/-unmapped/-deferred), the second the "Class#method …"
	// payload. splitOkapiPayload then parses the payload so trailing
	// notes, em-dash reasons, and parentheticals after the method name
	// no longer silently break the join (a recurring authoring footgun,
	// #611). `okapi-filter:` file headers never match (the payload must
	// contain a '#').
	okapiAnyRE = regexp.MustCompile(`^\s*//\s*okapi(-skip|-unmapped|-deferred)?:\s*([^#\s]+#\S.*)$`)
	funcDeclRE = regexp.MustCompile(`^func\s+(Test\w+)\s*\(`)
)

// markerKind maps the regex suffix group to an annotation Kind.
func markerKind(suffix string) string {
	switch suffix {
	case "-skip":
		return "skip"
	case "-unmapped":
		return "unmapped"
	case "-deferred":
		return "deferred"
	default:
		return "map"
	}
}

// splitOkapiPayload parses "Class#method <optional note>" into its parts.
// The method name ends at the first whitespace, em-dash, or '(' so a
// trailing " — reason" or " (note)" is captured as the reason rather than
// breaking the join. Returns ok=false when there is no '#'.
func splitOkapiPayload(payload string) (class, method, reason string, ok bool) {
	hash := strings.IndexByte(payload, '#')
	if hash < 0 {
		return "", "", "", false
	}
	class = strings.TrimSpace(payload[:hash])
	rest := payload[hash+1:]
	end := len(rest)
	for i, r := range rest {
		if r == ' ' || r == '\t' || r == '(' || r == '—' || r == '-' {
			end = i
			break
		}
	}
	method = rest[:end]
	reason = strings.TrimSpace(rest[end:])
	reason = strings.TrimLeft(reason, "—-( \t")
	reason = strings.TrimRight(reason, ")")
	reason = strings.TrimSpace(reason)
	return class, method, reason, class != "" && method != ""
}

// scanAnnotations walks dir and extracts all
// // okapi[-skip|-unmapped|-deferred]: annotations from *.go test files.
// The annotation must immediately precede a func TestXxx declaration
// (allowing other comment lines in between, since godoc-style multi-line
// comments are common).
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

// funcLoc is the source location of a Go test function.
type funcLoc struct {
	File string // repo-relative
	Line int    // 1-based line of the func declaration
}

// coverFuncDeclRE matches a top-level test/helper func declaration so the
// cover-link index can record its location.
var coverFuncDeclRE = regexp.MustCompile(`^func\s+(Test\w+)\s*\(`)

// scanTestFuncs walks dir and indexes every `func TestXxx` → its source
// location, so a skip reason that names a covering native test (e.g.
// "… covered by TestRoundTrip_DoubleExtraction") can be turned into a
// verifiable source link on the dashboard. On duplicate names across
// packages the first seen wins (collisions are rare and the per-filter
// reason text disambiguates in practice).
func scanTestFuncs(dir string) (map[string]funcLoc, error) {
	out := map[string]funcLoc{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel := relPath(path)
		for i, line := range strings.Split(string(data), "\n") {
			if m := coverFuncDeclRE.FindStringSubmatch(line); m != nil {
				if _, seen := out[m[1]]; !seen {
					out[m[1]] = funcLoc{File: rel, Line: i + 1}
				}
			}
		}
		return nil
	})
	return out, err
}

// coverRefRE extracts candidate native test-function names from a skip
// reason. Only names present in the native-func index become links, so Java
// references like "PropertiesFilterTest#testFoo" never resolve here.
var coverRefRE = regexp.MustCompile(`\bTest[A-Za-z0-9_]+`)

// resolveCover finds the first native test func named in reason that exists
// in the index, returning its name + location for a "covered by" link.
func resolveCover(reason string, index map[string]funcLoc) (name string, loc funcLoc, ok bool) {
	if index == nil || reason == "" {
		return "", funcLoc{}, false
	}
	for _, ref := range coverRefRE.FindAllString(reason, -1) {
		if loc, found := index[ref]; found {
			return ref, loc, true
		}
	}
	return "", funcLoc{}, false
}

func scanFile(path string) ([]annotation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	rel := relPath(path)

	// Walk forward accumulating annotation comments. A live `// okapi:`
	// mapping must sit above a `func TestXxx` (it claims that test verifies
	// the contract), so it is dropped if some other declaration interrupts
	// it. The reviewed not-applicable markers (skip/unmapped/deferred) are
	// standalone documentation — a whole block of them commonly sits at the
	// top of a file or above a helper — so they are kept regardless of what
	// follows. Each annotation records its own comment line for source links.
	var pending []annotation
	var out []annotation

	// flush emits the pending annotations at a boundary. When the boundary
	// is a Test func, every pending annotation links to it; otherwise only
	// the standalone (non-map) markers survive, linked to their own comment.
	flush := func(testFunc string, funcLine int) {
		for _, a := range pending {
			switch {
			case testFunc != "":
				a.GoFunc = testFunc
				a.Line = funcLine
				a.File = rel
				out = append(out, a)
			case a.Kind != "map":
				// Standalone reviewed-not-applicable marker: keep it,
				// linked to the comment line (GoFunc stays empty).
				a.File = rel
				out = append(out, a)
				// a.Line already holds the comment line.
			default:
				// Orphaned `// okapi:` map not above a Test func — drop.
			}
		}
		pending = nil
	}

	for i, line := range lines {
		if m := okapiAnyRE.FindStringSubmatch(line); m != nil {
			class, method, reason, ok := splitOkapiPayload(m[2])
			if ok {
				pending = append(pending, annotation{
					JavaClass:  class,
					JavaMethod: method,
					Kind:       markerKind(m[1]),
					Reason:     reason,
					Line:       i + 1, // comment line; overridden for map annotations on attach
				})
				continue
			}
		}
		// Comment / blank lines keep the block accumulating.
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || trimmed == "" {
			continue
		}
		if m := funcDeclRE.FindStringSubmatch(line); m != nil {
			flush(m[1], i+1)
		} else {
			// Any other declaration (package, import, var, helper func):
			// keep standalone skip markers, drop orphaned maps.
			flush("", 0)
		}
	}
	// EOF: flush any trailing standalone markers.
	flush("", 0)
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

// stripParamSuffix removes the JUnit parameterized-invocation suffix
// (`method[3: fixture.docx]`) so every invocation of one @Test method
// collapses to a single base-method contract row.
func stripParamSuffix(name string) string {
	if i := strings.IndexByte(name, '['); i > 0 {
		return name[:i]
	}
	return name
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

// sortedKeys returns the keys of a set, sorted, for deterministic output.
func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "contract-audit: "+format+"\n", args...)
	os.Exit(1)
}
