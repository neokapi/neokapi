# okf_html5 - HTML5/ITS Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_html5` |
| Java Class | `net.sf.okapi.filters.its.html5.HTML5Filter` |
| MIME Types | `text/html` |
| Extensions | `.html, .htm` |
| Okapi Module | `its` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/its/src/test/java/`

#### HTML5DefaultsTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testWinthinText` | Verifies `its-within-text='no'` on `<i>` removes it from inline, checks 2 codes in `<span>` content | P1 |
| 2 | `testTranslateOverrides` | (Ignored) Tests `translate='no'` on `<html>` suppresses all text units | P2 |

#### HTML5FilterTest.java (30 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimpleRead` | Basic extraction of title, paragraph with `<span>`, paragraph with `<i>` inline codes | P1 |
| 2 | `testTranslateLocally` | `translate=no` on inline `<span>` creates placeholder instead of translatable content | P1 |
| 3 | `testTranslateOnAttribute` | `translate='no'` on `<html>` still extracts keywords meta due to default rules; empty translate attr means yes | P1 |
| 4 | `testTranslateAttribute` | Extracts `alt` attribute as separate text unit with type `x-alt` | P1 |
| 5 | `testTranslateOverridenByRule` | External ITS rules in test01.html override default translate behavior | P2 |
| 6 | `testPreserveSpace` | `<pre>` preserves whitespace/tabs; `<p>` normalizes whitespace | P1 |
| 7 | `testDomain` | Extracts domain from `dcterms.subject` and `keywords` meta tags via ITS domain rules | P2 |
| 8 | `testRulesInScripts` | ITS rules embedded in `<script type=application/its+xml>` suppress title extraction | P1 |
| 9 | `testLocaleFilterLocal` | `its-locale-filter-list` filters segments by target locale | P2 |
| 10 | `testIdValueLocal` | `id` attribute mapped to text unit name via ITS idValue rule | P1 |
| 11 | `testAllowedChars` | `its-allowed-characters` annotation extracted on text units | P2 |
| 12 | `testStorageSizeLocal` | `its-storage-size` and `its-storage-encoding` annotations on list items | P2 |
| 13 | `testStorageSizeOnAttribute` | Storage size via global ITS rule on `@title`, plus local on paragraph | P2 |
| 14 | `testExternalResources` | `its:externalResourceRefRule` extracts resource refs on `<video>` attributes | P2 |
| 15 | `testLQRLocal` | `its-loc-quality-rating-score` and `its-loc-quality-rating-vote` annotations | P2 |
| 16 | `testTerminologyLocal` | `its-term`, `its-term-info-ref`, `its-term-confidence` terminology annotations | P2 |
| 17 | `testLocNoteLocal` | `its-loc-note`, `its-loc-note-ref`, `its-loc-note-type` localization note annotations | P2 |
| 18 | `testWithinTextLocal` | `its-within-text='no'` splits inline element into separate text units | P1 |
| 19 | `testGlobalLocQualityIssues` | Global ITS `locQualityIssueRule` via script applies LQI annotation to attributes | P2 |
| 20 | `testLocQualityIssuesExternalXMLStandoff` | External XML standoff LQI references from file lqi-test1.html | P2 |
| 21 | `testStandofftLocQualityIssues` | Standoff LQI via `<script>` with `its:locQualityIssues` and `its-loc-quality-issues-ref` | P2 |
| 22 | `testLocalLocQualityIssues` | Local LQI attributes: type, severity, comment, profile-ref, enabled | P2 |
| 23 | `testProvenanceStandoff` | Standoff provenance records with person/org/tool and rev variants | P2 |
| 24 | `testLink` | Verifies link extraction from test02.html with anchor tag | P1 |
| 25 | `testWhiteSpaces` | File-based whitespace handling: normal vs `<pre>` blocks in testWhiteSpaces.html | P1 |
| 26 | `testSimpleOutput` | Roundtrip output: verifies attributes get quoted, charset normalized | P1 |
| 27 | `testMinimalHTML5Output` | Minimal HTML5 without explicit `<html>/<body>` tags gets normalized output | P1 |
| 28 | `testAddITSAnnotations1` | Adding ITS annotations (ta-class-ref, allowed-characters, terminology) to output | P2 |
| 29 | `testAddITSAnnotations2` | Adding standoff LQI and provenance annotations produces correct script blocks | P2 |
| 30 | `testAddITSAnnotations3` | Hard-wired annotations produce same standoff output as programmatic | P2 |
| 31 | `testMinimalHTMLWithStandoff` | Minimal HTML with standoff annotations wraps in `<html><body>` correctly | P2 |
| 32 | `testOpenTwice` | Filter can be opened, closed, and reopened without error | P1 |
| 33 | `testDATAContentOutput` | `translate="no"` on `<html>` with class-based translate rule; style/script DATA content preserved | P1 |
| 34 | `testEmptyElements2` | Empty `<span></span>` elements preserved as inline codes in roundtrip | P1 |
| 35 | `testTextDirectionClarification` | (DataProvider, 7 cases) RTL/LTR dir attribute adjusted based on target locale | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripHtmlItsIT` | `integration-tests/okapi/src/test/java/.../RoundTripHtmlItsIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/htmlIts/`):
- `test01.html`
- `test2.html`
- `lqi-test1.html`
- `lqi-test1-standoff.html`
- `lqi-test1-standoff.xml`
- `test01-html-rules.xml`

**Known failing files**: `test01.html`, `test2.html`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `HtmlItsXliffCompareIT` | `integration-tests/okapi/src/test/java/.../HtmlItsXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyHtmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyHtmlIT.java` | 2 |

Note: This uses `okf_html` (HtmlFilter), not `okf_html5` (HTML5Filter). Listed for reference only.

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `HtmlMemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../HtmlMemoryLeakTestIT.java` | 1 (main) |

Note: Tests `okf_html` (HtmlFilter), not `okf_html5`. Listed for reference only.

## Test Data Files

### Unit test resources

Source: `okapi/filters/its/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test01.html` | `HTML5FilterTest#testTranslateOverridenByRule`, `#testOpenTwice` | HTML5 with external ITS rules override |
| `test01-html-rules.xml` | (referenced by test01.html) | External ITS rules XML for HTML5 |
| `test02.html` | `HTML5FilterTest#testLink` | HTML5 with link/anchor elements |
| `testWhiteSpaces.html` | `HTML5FilterTest#testWhiteSpaces` | Whitespace handling in normal and pre blocks |
| `lqi-test1.html` | `HTML5FilterTest#testLocQualityIssuesExternalXMLStandoff` | HTML5 with external XML standoff LQI |
| `lqi-test1-standoff.html` | (integration test) | Standoff LQI HTML variant |
| `lqi-test1-standoff.xml` | (referenced by lqi-test1.html) | External XML standoff data for LQI |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/htmlIts/`

| File | Type | Purpose |
|------|------|---------|
| `test01.html` | roundtrip | Basic HTML5/ITS roundtrip (known failing) |
| `test2.html` | roundtrip | HTML5/ITS roundtrip variant (known failing) |
| `lqi-test1.html` | roundtrip | LQI standoff roundtrip |
| `lqi-test1-standoff.html` | roundtrip | LQI standoff variant |
| `lqi-test1-standoff.xml` | roundtrip | External XML data for LQI test |
| `test01-html-rules.xml` | roundtrip | External ITS rules |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.html` | Minimal valid HTML5 for smoke test | `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8><title>Title</title></head><body><p>Hello world</p></body></html>` |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources (html5-specific)
cp okapi/filters/its/src/test/resources/test01.html okapi-testdata/okf_html5/
cp okapi/filters/its/src/test/resources/test01-html-rules.xml okapi-testdata/okf_html5/
cp okapi/filters/its/src/test/resources/test02.html okapi-testdata/okf_html5/
cp okapi/filters/its/src/test/resources/testWhiteSpaces.html okapi-testdata/okf_html5/
cp okapi/filters/its/src/test/resources/lqi-test1.html okapi-testdata/okf_html5/
cp okapi/filters/its/src/test/resources/lqi-test1-standoff.xml okapi-testdata/okf_html5/

# Integration test resources
cp integration-tests/okapi/src/test/resources/htmlIts/test01.html okapi-testdata/okf_html5/roundtrip/
cp integration-tests/okapi/src/test/resources/htmlIts/test2.html okapi-testdata/okf_html5/roundtrip/
cp integration-tests/okapi/src/test/resources/htmlIts/lqi-test1.html okapi-testdata/okf_html5/roundtrip/
cp integration-tests/okapi/src/test/resources/htmlIts/lqi-test1-standoff.html okapi-testdata/okf_html5/roundtrip/
cp integration-tests/okapi/src/test/resources/htmlIts/lqi-test1-standoff.xml okapi-testdata/okf_html5/roundtrip/
cp integration-tests/okapi/src/test/resources/htmlIts/test01-html-rules.xml okapi-testdata/okf_html5/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_html5`

Build tag: `//go:build integration`

#### html5_test.go - Extraction Tests

```go
func TestExtract_simpleRead(t *testing.T) {
    // Table-driven: maps 1:1 to Java HTML5FilterTest
    tests := []struct {
        name       string
        input      string
        wantBlocks int
        wantTexts  []string
        params     map[string]any
        javaRef    string
    }{
        {
            name:       "simple read with inline span and i",
            input:      `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8><title>Title</title></head><body><p>Text in <span>bold</span>.<p>Text in <i>italics</i>.</body></html>`,
            wantTexts:  []string{"Title", "Text in <span>bold</span>.", "Text in <i>italics</i>."},
            javaRef:    "HTML5FilterTest#testSimpleRead",
        },
        {
            name:       "translate=no on inline span creates placeholder",
            input:      `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8><title>Title</title></head><body><p>Text in <span translate=no>code</span>.</p></body></html>`,
            javaRef:    "HTML5FilterTest#testTranslateLocally",
        },
        {
            name:       "alt attribute extracted as separate text unit",
            input:      `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8><title>Title</title></head><body><p>Text <img src=test.png alt=Text>.</p></body></html>`,
            javaRef:    "HTML5FilterTest#testTranslateAttribute",
        },
        {
            name:       "preserve space in pre vs normalize in p",
            input:      `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8><title>Title</title></head><body><pre> text  		 <b>  etc.  </b>	 </pre><p> text  		 <b>  etc.  </b>	 </p></body></html>`,
            javaRef:    "HTML5FilterTest#testPreserveSpace",
        },
        {
            name:       "id attribute as text unit name",
            input:      `<!DOCTYPE html><html lang=en><head><meta charset=utf-8><title>Title</title></head><body><p id='n1'>Text 1</p></body></html>`,
            javaRef:    "HTML5FilterTest#testIdValueLocal",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_itsRulesInScript(t *testing.T) {
    // Maps to Java HTML5FilterTest ITS rules tests
    tests := []struct {
        name    string
        params  map[string]any
        input   string
        want    []string
        javaRef string
    }{
        {
            name:    "ITS rules in script suppress title",
            javaRef: "HTML5FilterTest#testRulesInScripts",
        },
        {
            name:    "translate=no on html with class override",
            javaRef: "HTML5FilterTest#testDATAContentOutput",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripHtmlItsIT
    testFiles := []string{
        "lqi-test1.html",
        "lqi-test1-standoff.html",
    }
    knownFailing := map[string]string{
        "test01.html": "ITS rules override causes event mismatch",
        "test2.html":  "Known failing in Java integration tests",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java HtmlItsXliffCompareIT
    // Verifies Part structure matches expected XLIFF output
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_html5/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_html5/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] XLIFF compare structure matches
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - HTML5 filter uses nu.validator HTML parser, not standard XML parser
  - ITS 2.0 annotations (LQI, provenance, terminology) are HTML5-specific metadata
  - `<span/>` self-closing not supported by nu.parser; must use `<span></span>`
  - Default ITS rules are always applied (translate meta keywords, alt attributes, etc.)
  - Minimal HTML without `<html>/<body>` tags gets auto-wrapped in output
  - RTL/LTR direction is auto-adjusted based on target locale
  - Standoff annotations produce `<script>` blocks appended before `</body>`
  - Schema has only `simplifierRules` and `path` parameters; real configuration is via ITS rules XML

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/its/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `HTML5DefaultsTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/its/html5/` | 2 |
| `HTML5FilterTest.java` | `okapi/filters/its/src/test/java/net/sf/okapi/filters/its/html5/` | 30 |
