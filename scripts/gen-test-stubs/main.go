// Package main generates Go test stub files from Okapi Surefire XML reports.
// For each filter, it produces okapi_stubs_test.go with t.Skip("pending")
// stubs for unmapped Java tests, and // okapi-skip: comments for tests that
// are not applicable to Go.
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// --- Surefire XML structures ---

type xmlTestSuite struct {
	Name      string        `xml:"name,attr"`
	Tests     int           `xml:"tests,attr"`
	TestCases []xmlTestCase `xml:"testcase"`
}

type xmlTestCase struct {
	Name      string `xml:"name,attr"`
	ClassName string `xml:"classname,attr"`
}

// --- Filter configuration ---

type filterConfig struct {
	Name        string // e.g. "html"
	SurefireDir string
	BridgePkg   string // e.g. "core/plugin/bridge/filters/okf_html"
	NativePkg   string // e.g. "core/formats/html" (empty if no native)
}

// phase1Filters defines all Phase 1 filters and their packages.
var phase1Filters = []filterConfig{
	{Name: "html", BridgePkg: "core/plugin/bridge/filters/okf_html", NativePkg: "core/formats/html"},
	{Name: "markdown", BridgePkg: "core/plugin/bridge/filters/okf_markdown", NativePkg: "core/formats/markdown"},
	{Name: "xliff", BridgePkg: "core/plugin/bridge/filters/okf_xliff"},
	{Name: "xmlstream", BridgePkg: "core/plugin/bridge/filters/okf_xmlstream"},
	{Name: "json", BridgePkg: "core/plugin/bridge/filters/okf_json", NativePkg: "core/formats/json"},
	{Name: "po", BridgePkg: "core/plugin/bridge/filters/okf_po", NativePkg: "core/formats/po"},
	{Name: "plaintext", BridgePkg: "core/plugin/bridge/filters/okf_plaintext", NativePkg: "core/formats/plaintext"},
	{Name: "yaml", BridgePkg: "core/plugin/bridge/filters/okf_yaml"},
	{Name: "xliff2", BridgePkg: "core/plugin/bridge/filters/okf_xliff2", NativePkg: "core/formats/xliff2"},
	{Name: "properties", BridgePkg: "core/plugin/bridge/filters/okf_properties", NativePkg: "core/formats/properties"},
}

// javaTest represents a single test method from Surefire XML.
type javaTest struct {
	ClassName string // short class name (e.g. "HtmlSnippetsTest")
	Method    string // method name (e.g. "testPWithInlines")
}

// classified holds a Java test with its classification result.
type classified struct {
	test   javaTest
	action string // "stub" or "skip"
	reason string // skip reason
	goName string // generated Go test name (for stubs)
}

// --- Regex patterns ---

var (
	annotationRe = regexp.MustCompile(`^//\s*okapi:\s+(\w+)#(\w+)\s*$`)
	skipAnnotRe  = regexp.MustCompile(`^//\s*okapi-skip:\s+(\w+)#(\w+)`)
	funcTestRe   = regexp.MustCompile(`^func\s+(Test\w+)\s*\(`)
)

func main() {
	var (
		filter       = flag.String("filter", "", "filter name (e.g. html) or 'all' for all Phase 1 filters")
		surefireBase = flag.String("surefire-dir", "", "base surefire directory (e.g. okapi-surefire/1.48.0-v1)")
		repoRoot     = flag.String("repo-root", ".", "repository root directory")
		dryRun       = flag.Bool("dry-run", false, "print what would be generated without writing files")
	)
	flag.Parse()

	if *surefireBase == "" {
		fmt.Fprintln(os.Stderr, "Usage: gen-test-stubs -surefire-dir DIR [-filter NAME] [-repo-root DIR] [-dry-run]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	filters := phase1Filters
	if *filter != "" && *filter != "all" {
		filters = nil
		for _, f := range phase1Filters {
			if f.Name == *filter {
				filters = append(filters, f)
				break
			}
		}
		if len(filters) == 0 {
			log.Fatalf("unknown filter: %s", *filter)
		}
	}

	totalFiles := 0
	totalStubs := 0
	totalSkips := 0

	for i := range filters {
		filters[i].SurefireDir = filepath.Join(*surefireBase, filters[i].Name)

		bs, bk, err := generateForFilter(filters[i], *repoRoot, *dryRun, "bridge")
		if err != nil {
			log.Fatalf("filter %s (bridge): %v", filters[i].Name, err)
		}
		if bs+bk > 0 {
			totalFiles++
			totalStubs += bs
			totalSkips += bk
		}

		if filters[i].NativePkg != "" {
			ns, nk, err := generateForFilter(filters[i], *repoRoot, *dryRun, "native")
			if err != nil {
				log.Fatalf("filter %s (native): %v", filters[i].Name, err)
			}
			if ns+nk > 0 {
				totalFiles++
				totalStubs += ns
				totalSkips += nk
			}
		}
	}

	fmt.Printf("generated %d files (%d stubs, %d skips)\n", totalFiles, totalStubs, totalSkips)
}

// generateForFilter generates okapi_stubs_test.go for a single filter+kind.
// Returns (stubCount, skipCount, error).
func generateForFilter(fc filterConfig, repoRoot string, dryRun bool, kind string) (int, int, error) {
	pkg := fc.BridgePkg
	if kind == "native" {
		pkg = fc.NativePkg
	}
	if pkg == "" {
		return 0, 0, nil
	}

	// Parse surefire XML to get all Java tests
	javaTests, err := parseSurefireDir(fc.SurefireDir)
	if err != nil {
		return 0, 0, fmt.Errorf("parse surefire: %w", err)
	}
	if len(javaTests) == 0 {
		fmt.Printf("  %s/%s: no surefire tests found\n", fc.Name, kind)
		return 0, 0, nil
	}

	// Scan existing Go test files for annotations and test names
	pkgDir := filepath.Join(repoRoot, pkg)
	existing := scanExistingTests(pkgDir)

	// Group by class, preserving order
	byClass := map[string][]javaTest{}
	var classOrder []string
	for _, jt := range javaTests {
		if _, seen := byClass[jt.ClassName]; !seen {
			classOrder = append(classOrder, jt.ClassName)
		}
		byClass[jt.ClassName] = append(byClass[jt.ClassName], jt)
	}

	var stubs, skips []classified
	usedNames := map[string]bool{} // track generated names to avoid duplicates

	for _, className := range classOrder {
		tests := byClass[className]
		skipReason := classSkipReason(className)

		for _, jt := range tests {
			key := jt.ClassName + "#" + jt.Method

			// Also check base method name (without parameterized suffix)
			baseMethod := jt.Method
			if idx := strings.Index(baseMethod, "["); idx >= 0 {
				baseMethod = baseMethod[:idx]
			}
			baseKey := jt.ClassName + "#" + baseMethod

			// Already mapped by annotation? (check both full and base key)
			if existing.hasAnnotation(key) || existing.hasAnnotation(baseKey) {
				continue
			}

			// Class-level skip?
			if skipReason != "" {
				// For parameterized tests, only emit skip for the base method once
				if baseMethod != jt.Method {
					skipKey := "skip:" + baseKey
					if usedNames[skipKey] {
						continue
					}
					usedNames[skipKey] = true
					skips = append(skips, classified{
						test:   javaTest{ClassName: jt.ClassName, Method: baseMethod},
						action: "skip",
						reason: skipReason,
					})
					continue
				}
				skips = append(skips, classified{
					test:   jt,
					action: "skip",
					reason: skipReason,
				})
				continue
			}

			// Method-level skip?
			if reason := methodSkipReason(jt.Method); reason != "" {
				skips = append(skips, classified{
					test:   jt,
					action: "skip",
					reason: reason,
				})
				continue
			}

			// Generate Go test name
			goName := goTestName(jt, kind)

			// Skip if name already exists in existing tests or already generated
			if existing.hasFunc(goName) || usedNames[goName] {
				continue
			}
			usedNames[goName] = true

			stubs = append(stubs, classified{
				test:   javaTest{ClassName: jt.ClassName, Method: baseMethod},
				action: "stub",
				goName: goName,
			})
		}
	}

	if len(stubs) == 0 && len(skips) == 0 {
		fmt.Printf("  %s/%s: all %d tests already mapped\n", fc.Name, kind, len(javaTests))
		return 0, 0, nil
	}

	// Determine package name for the stub file
	pkgName := determinePackageName(pkgDir, kind)

	// Generate file content
	content := generateStubFile(pkgName, stubs, skips)

	outPath := filepath.Join(pkgDir, "okapi_stubs_test.go")

	if dryRun {
		fmt.Printf("  %s/%s: would write %s (%d stubs, %d skips)\n",
			fc.Name, kind, outPath, len(stubs), len(skips))
		fmt.Println(content)
		return len(stubs), len(skips), nil
	}

	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return 0, 0, fmt.Errorf("write %s: %w", outPath, err)
	}
	fmt.Printf("  %s/%s: wrote %s (%d stubs, %d skips)\n",
		fc.Name, kind, outPath, len(stubs), len(skips))

	return len(stubs), len(skips), nil
}

// parseSurefireDir reads all TEST-*.xml files in a directory.
func parseSurefireDir(dir string) ([]javaTest, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "TEST-*.xml"))
	if err != nil {
		return nil, err
	}

	var result []javaTest
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warn: read %s: %v", path, err)
			continue
		}

		var suite xmlTestSuite
		if err := xml.Unmarshal(raw, &suite); err != nil {
			log.Printf("warn: parse %s: %v", path, err)
			continue
		}

		for _, tc := range suite.TestCases {
			result = append(result, javaTest{
				ClassName: shortClassName(tc.ClassName),
				Method:    tc.Name,
			})
		}
	}

	// Sort by class then method for stable output
	sort.Slice(result, func(i, j int) bool {
		if result[i].ClassName != result[j].ClassName {
			return result[i].ClassName < result[j].ClassName
		}
		return result[i].Method < result[j].Method
	})

	return result, nil
}

// shortClassName extracts the short class name from a fully qualified Java class.
func shortClassName(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// existingTests tracks what's already in Go test files.
type existingTests struct {
	annotations map[string]bool // "ClassName#method" → true
	funcs       map[string]bool // "TestFoo" → true
}

func (e *existingTests) hasAnnotation(key string) bool {
	return e.annotations[key]
}

func (e *existingTests) hasFunc(name string) bool {
	return e.funcs[name]
}

// scanExistingTests reads all *_test.go files in a package directory.
func scanExistingTests(dir string) *existingTests {
	et := &existingTests{
		annotations: map[string]bool{},
		funcs:       map[string]bool{},
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	for _, path := range matches {
		// Skip the generated stubs file — it's the file we'll overwrite
		if filepath.Base(path) == "okapi_stubs_test.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			if m := annotationRe.FindStringSubmatch(trimmed); m != nil {
				et.annotations[m[1]+"#"+m[2]] = true
			}

			if m := skipAnnotRe.FindStringSubmatch(trimmed); m != nil {
				et.annotations[m[1]+"#"+m[2]] = true
			}

			if m := funcTestRe.FindStringSubmatch(trimmed); m != nil {
				et.funcs[m[1]] = true
			}
		}
	}

	return et
}

// determinePackageName figures out the Go package name for the stub file.
func determinePackageName(dir string, kind string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	for _, path := range matches {
		if filepath.Base(path) == "okapi_stubs_test.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "package ") {
				return strings.Fields(trimmed)[1]
			}
		}
	}

	// Fallback: derive from directory name
	base := filepath.Base(dir)
	if kind == "native" {
		return base + "_test"
	}
	return base
}

// --- Naming rules ---

// goTestName generates a Go test function name from a Java test method.
func goTestName(jt javaTest, kind string) string {
	prefix := testPrefix(jt, kind)
	name := cleanMethodName(jt.Method)
	return prefix + name
}

// testPrefix determines the Go test name prefix based on Java class pattern.
func testPrefix(jt javaTest, kind string) string {
	cls := jt.ClassName

	// RoundTrip integration tests
	if strings.Contains(cls, "RoundTrip") || strings.HasSuffix(cls, "IT") ||
		strings.Contains(jt.Method, "RoundTrip") || strings.Contains(jt.Method, "DoubleExtraction") {
		return "TestRoundTrip_"
	}

	// Writer tests
	if strings.Contains(cls, "Writer") {
		return "TestWrite_"
	}

	// Configuration tests (non-skipped ones)
	if strings.Contains(cls, "ConfigurationTest") && !strings.Contains(cls, "ConfigurationSupport") {
		return "TestConfig_"
	}

	// Parser tests
	if strings.Contains(cls, "Parser") {
		return "TestParse_"
	}

	// Snippet/Filter/Event/Extraction tests → extract/read
	if kind == "native" {
		return "TestRead_"
	}
	return "TestExtract_"
}

// cleanMethodName converts a Java method name to a Go test name suffix.
func cleanMethodName(method string) string {
	name := method

	// Strip parameterized test suffix: "testFoo[0: data...]" → "testFoo"
	if idx := strings.Index(name, "["); idx >= 0 {
		name = name[:idx]
	}

	// Strip "test" prefix (only if followed by uppercase, underscore, or digit)
	if strings.HasPrefix(name, "test") && len(name) > 4 {
		next := rune(name[4])
		if unicode.IsUpper(next) || next == '_' || unicode.IsDigit(next) {
			name = name[4:]
		}
	}

	// Strip leading underscore
	name = strings.TrimLeft(name, "_")

	// Convert underscore-separated segments to PascalCase
	if strings.Contains(name, "_") {
		name = underscoreToPascal(name)
	}

	// Ensure first character is uppercase
	if len(name) > 0 && !unicode.IsUpper(rune(name[0])) {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}

// underscoreToPascal converts "GLOBAL_PRESERVE_WHITESPACE" to "GlobalPreserveWhitespace".
func underscoreToPascal(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if isAllUpper(part) {
			result.WriteString(strings.ToUpper(part[:1]))
			result.WriteString(strings.ToLower(part[1:]))
		} else {
			result.WriteString(strings.ToUpper(part[:1]))
			result.WriteString(part[1:])
		}
	}
	return result.String()
}

func isAllUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// --- Skip heuristics ---

// classSkipReason returns a skip reason if the entire class should be skipped.
func classSkipReason(className string) string {
	if strings.HasSuffix(className, "ConfigurationSupportTest") {
		return "Java YAML config subsystem"
	}
	return ""
}

// methodSkipReason returns a skip reason for individual methods.
func methodSkipReason(method string) string {
	skipPatterns := []struct {
		pattern string
		reason  string
	}{
		{"Clone", "Java clone API"},
		{"Serialize", "Java serialization"},
		{"InputStream", "Java I/O stream API"},
		{"OutputStream", "Java I/O stream API"},
		{"getProperty", "Java properties API"},
		{"setProperty", "Java properties API"},
	}

	for _, sp := range skipPatterns {
		if strings.Contains(method, sp.pattern) {
			return sp.reason
		}
	}

	return ""
}

// --- File generation ---

func generateStubFile(pkgName string, stubs, skips []classified) string {
	var b strings.Builder

	b.WriteString("//go:build integration\n\n")
	b.WriteString("package ")
	b.WriteString(pkgName)
	b.WriteString("\n\n")
	b.WriteString("// Code generated by gen-test-stubs from Surefire XML. DO NOT EDIT.\n")
	b.WriteString("// To implement a test, move it to the appropriate test file and\n")
	b.WriteString("// replace t.Skip(\"pending\") with test logic.\n")

	if len(stubs) > 0 {
		b.WriteString("\nimport \"testing\"\n")
	}

	// Write skipped tests as comments
	if len(skips) > 0 {
		b.WriteString("\n// ---- Skipped (not applicable to Go) ----\n//\n")

		byClass := groupByClass(skips)
		for _, cls := range byClass.order {
			for _, item := range byClass.items[cls] {
				fmt.Fprintf(&b, "// okapi-skip: %s#%s \u2014 %s\n",
					item.test.ClassName, item.test.Method, item.reason)
			}
		}
	}

	// Write test stubs grouped by class
	if len(stubs) > 0 {
		byClass := groupByClass(stubs)
		for _, cls := range byClass.order {
			items := byClass.items[cls]
			fmt.Fprintf(&b, "\n// ---- %s (%d tests) ----\n", cls, len(items))

			for _, item := range items {
				b.WriteString("\n")
				fmt.Fprintf(&b, "// okapi: %s#%s\n", item.test.ClassName, item.test.Method)
				fmt.Fprintf(&b, "func %s(t *testing.T) {\n", item.goName)
				b.WriteString("\tt.Skip(\"pending\")\n")
				b.WriteString("}\n")
			}
		}
	}

	return b.String()
}

type classGroup struct {
	order []string
	items map[string][]classified
}

func groupByClass(items []classified) classGroup {
	g := classGroup{items: map[string][]classified{}}
	for _, item := range items {
		cls := item.test.ClassName
		if _, seen := g.items[cls]; !seen {
			g.order = append(g.order, cls)
		}
		g.items[cls] = append(g.items[cls], item)
	}
	return g
}
