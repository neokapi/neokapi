//go:build integration

package regex

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

// --- RoundTrip Integration Tests ---

// okapi: RoundTripRegexIT#regexFiles (dummy.foo)
func TestRoundTrip_DummyFoo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	filePath := tdDir + "/integration-tests/okapi/src/test/resources/regex/dummy.foo"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}
	// dummy.foo may be empty — just verify the roundtrip completes.
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, nil)
}

// okapi: RoundTripRegexIT#regexFiles (meta)
func TestRoundTrip_Meta(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	filePath := tdDir + "/integration-tests/okapi/src/test/resources/regex/meta/test.txt"
	configPath := tdDir + "/integration-tests/okapi/src/test/resources/regex/meta/okf_regex@meta.fprm"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RoundTripRegexIT#regexFiles (meta2)
func TestRoundTrip_Meta2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	filePath := tdDir + "/integration-tests/okapi/src/test/resources/regex/meta2/TestRules05.txt"
	configPath := tdDir + "/integration-tests/okapi/src/test/resources/regex/meta2/okf_regex@TestRules05.fprm"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RoundTripRegexIT#regexFiles (stringInfo)
func TestRoundTrip_StringInfo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	filePath := tdDir + "/integration-tests/okapi/src/test/resources/regex/stringInfo/Test01_stringinfo_en.regex"
	configPath := tdDir + "/integration-tests/okapi/src/test/resources/regex/stringInfo/okf_regex@StringInfo.fprm"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// --- Roundtrip of snippet-level content ---

// okapi: RegexFilterTest#testSimpleRule (roundtrip variant)
func TestRoundTrip_SimpleRule(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@TestRules01.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/TestRules01.txt"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexFilterTest#testConfigurations (SRT roundtrip)
func TestRoundTrip_SRT(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@SRT.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/Test01_srt_en.srt"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexFilterTest#testConfigurations (macStrings roundtrip)
func TestRoundTrip_MacStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@macStrings.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/test.strings"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexFilterTest#testConfigurations (INI roundtrip)
func TestRoundTrip_INI(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@INI.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/TestFrenchISL.isl"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexFilterTest#testConfigurations (SymbianRLS roundtrip)
func TestRoundTrip_SymbianRLS(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@SymbianRLS.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/SymbianRLSSample.rls"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexFilterTest#testConfigurations (StringInfo roundtrip)
func TestRoundTrip_StringInfoUnit(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@StringInfo.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/Test01_stringinfo_en.info"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}

// okapi: RegexXliffCompareIT (roundtrip of semicolon content)
func TestRoundTrip_SemicolonContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okapi/filters/regex/src/test/resources/okf_regex@macStrings.fprm"

	filePath := tdDir + "/okapi/filters/regex/src/test/resources/TestRules07.strings"
	content, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("testdata file not found: %s", filePath)
	}

	params := map[string]any{
		"configFile": configPath,
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, filePath, mimeType, params)
}
