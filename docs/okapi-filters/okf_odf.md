# okf_odf - ODF Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_odf` |
| Java Class | `net.sf.okapi.filters.openoffice.ODFFilter` |
| MIME Types | `text/x-odf` |
| Extensions | `-` (none; works on extracted ODF XML content files) |
| Okapi Module | `openoffice` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/openoffice/src/test/java/`

Note: This module is shared with `okf_openoffice` (OpenOfficeFilter). ODFFilter is the newer version that operates on individual ODF XML files (content.xml, meta.xml, styles.xml) extracted from the ZIP package.

#### ODFFilterTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testFirstTextUnit` | First TU from TestDocument01.odt_content.xml is "Heading 1" | P1 |
| 2 | `testITSMarkup` | ITS (Internationalization Tag Set) annotations: translate='no', localization notes, terminology, locale filter from Content_WithITS.xml | P1 |
| 3 | `testDefaultInfo` | Non-null parameters, name, non-empty configurations | P2 |
| 4 | `testDoubleExtraction` | Double extraction roundtrip for 6 ODF XML files (.odt_content.xml, .odt_meta.xml, .odt_styles.xml, footnote.xml, .ods_content.xml) | P1 |

### Integration Tests

None specific to ODF (integration tests use the OpenOffice filter which wraps ODF).

## Test Data Files

### Unit test resources

Source: `okapi/filters/openoffice/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `TestDocument01.odt_content.xml` | `testFirstTextUnit`, `testDoubleExtraction` | Extracted content.xml from ODT |
| `TestDocument01.odt_meta.xml` | `testDoubleExtraction` | Extracted meta.xml from ODT |
| `TestDocument01.odt_styles.xml` | `testDoubleExtraction` | Extracted styles.xml from ODT |
| `TestDocument02.odt_content.xml` | `testDoubleExtraction` | Additional content.xml |
| `ODFTest_footnote.xml` | `testDoubleExtraction` | Footnote content |
| `TestSpreadsheet01.ods_content.xml` | `testDoubleExtraction` | Spreadsheet content.xml |
| `Content_WithITS.xml` | `testITSMarkup` | ODF content with ITS annotations |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal_content.xml` | Minimal valid ODF content.xml for smoke test | Single paragraph document |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/*.xml okapi-testdata/okf_odf/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/*_content.xml okapi-testdata/okf_odf/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/*_meta.xml okapi-testdata/okf_odf/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/*_styles.xml okapi-testdata/okf_odf/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_odf`

Build tag: `//go:build integration`

#### odf_test.go - Extraction Tests

```go
func TestExtract_BasicContent(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "first_text_unit",
            input: "TestDocument01.odt_content.xml",
            wantTexts: []string{"Heading 1"},
            javaRef: "ODFFilterTest#testFirstTextUnit",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "TestDocument01.odt_content.xml",
        "TestDocument01.odt_meta.xml",
        "TestDocument01.odt_styles.xml",
        "TestDocument02.odt_content.xml",
        "ODFTest_footnote.xml",
        "TestSpreadsheet01.ods_content.xml",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_odf/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_odf/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - ODFFilter works on individual XML files extracted from ODF ZIP packages (content.xml, meta.xml, styles.xml)
  - Does NOT process the ZIP package directly (use okf_openoffice for that)
  - Supports ITS (Internationalization Tag Set) annotations: translate, localization notes, terminology, locale filter
  - Shares the same Java module as okf_openoffice but is a separate filter class
  - Limited test coverage (4 @Test methods) - may need synthetic tests

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/openoffice/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `ODFFilterTest.java` | `okapi/filters/openoffice/src/test/java/.../` | 4 |
