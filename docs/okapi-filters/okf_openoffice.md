# okf_openoffice - OpenOffice Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_openoffice` |
| Java Class | `net.sf.okapi.filters.openoffice.OpenOfficeFilter` |
| MIME Types | `application/x-openoffice` |
| Extensions | `.odt, .ods, .odp, .odg, .ott, .ots, .otp, .otg, .sxd, .sxi, .swx, .swc` |
| Okapi Module | `openoffice` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/openoffice/src/test/java/`

Note: This module is shared with `okf_odf` (ODFFilter). See also `okf_odf.md`.

#### OpenOfficeFilterTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Non-null parameters, name, non-empty configurations | P2 |
| 2 | `testFirstTextUnit` | First TU from TestDocument01.odt is "Heading 1" | P1 |
| 3 | `testMetadataExtraction` | Data-driven: extractMetadata=true extracts title/keywords/description/custom props; false extracts only body text (TestDocumentWithMetadata.odt) | P1 |
| 4 | `testNumberTag` | Data-driven: encodeCharacterEntityReferenceGlyphs true/false affects page-number rendering in .odp (TestDocumentWithNumberTag.odp) | P2 |
| 5 | `testFormulaResultExtraction` | Spreadsheet formula results extracted (TestDocumentWithFormulaResults.ods), 17 text units including sheet names | P1 |
| 6 | `testBookmarkReferencesHandling` | Data-driven: extractReferences/encodeCharacterEntityReferenceGlyphs affect bookmark-ref code data (bookmark-reference.odt) | P2 |
| 7 | `testStartDocument` | StartDocument event from TestDocument01.odt | P2 |
| 8 | `testDoubleExtraction` | Double extraction roundtrip for 14 files (.odt, .ods, .odp, .odg) | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripOpenOfficeIT` | `integration-tests/okapi/src/test/java/.../RoundTripOpenOfficeIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `OpenOfficeXliffCompareIT` | `integration-tests/okapi/src/test/java/.../OpenOfficeXliffCompareIT.java` | N/A |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyOpenOfficeIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyOpenOfficeIT.java` | N/A |

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/openoffice/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `TestDocument01.odt` | `testFirstTextUnit`, `testStartDocument`, `testDoubleExtraction` | Basic ODT document |
| `TestDocument02.odt` | `testDoubleExtraction` | Additional ODT test |
| `TestDocument03.odt` | `testDoubleExtraction` | Additional ODT test |
| `TestDocument04.odt` | `testDoubleExtraction` | Additional ODT test |
| `TestDocument05.odt` | `testDoubleExtraction` | Additional ODT test |
| `TestDocument06.odt` | `testDoubleExtraction` | Additional ODT test |
| `TestSpreadsheet01.ods` | `testDoubleExtraction` | ODS spreadsheet |
| `TestDrawing01.odg` | `testDoubleExtraction` | ODG drawing |
| `TestPresentation01.odp` | `testDoubleExtraction` | ODP presentation |
| `TestDocument_WithITS.odt` | `testDoubleExtraction` | ODT with ITS annotations |
| `TestDocumentWithMetadata.odt` | `testMetadataExtraction`, `testDoubleExtraction` | ODT with metadata properties |
| `TestDocumentWithNumberTag.odp` | `testNumberTag`, `testDoubleExtraction` | ODP with number/page-number tags |
| `TestDocumentWithFormulaResults.ods` | `testFormulaResultExtraction`, `testDoubleExtraction` | ODS with formula results |
| `TestDocumentWithTableWrappingAboutTable.odt` | `testDoubleExtraction` | ODT with table wrapping |
| `bookmark-reference.odt` | `testBookmarkReferencesHandling` | ODT with bookmark references |

### Synthetic test data to create

None needed - good coverage across formats.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/TestDocument*.odt okapi-testdata/okf_openoffice/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/TestSpreadsheet*.ods okapi-testdata/okf_openoffice/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/TestDrawing*.odg okapi-testdata/okf_openoffice/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/TestPresentation*.odp okapi-testdata/okf_openoffice/
cp okapi/filters/openoffice/src/test/resources/net/sf/okapi/filters/openoffice/bookmark-reference.odt okapi-testdata/okf_openoffice/

# Integration test resources
cp integration-tests/okapi/src/test/resources/openoffice/*.odt okapi-testdata/okf_openoffice/roundtrip/
cp integration-tests/okapi/src/test/resources/openoffice/*.ods okapi-testdata/okf_openoffice/roundtrip/
cp integration-tests/okapi/src/test/resources/openoffice/*.odp okapi-testdata/okf_openoffice/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/openoffice`

Build tag: `//go:build integration`

#### openoffice_test.go - Extraction Tests

```go
func TestExtract_BasicDocument(t *testing.T) {
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
            input: "TestDocument01.odt",
            wantTexts: []string{"Heading 1"},
            javaRef: "OpenOfficeFilterTest#testFirstTextUnit",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_MetadataExtraction(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "metadata_enabled",
            params: map[string]any{"extractMetadata": true},
            input:  "TestDocumentWithMetadata.odt",
            javaRef: "OpenOfficeFilterTest#testMetadataExtraction",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "TestDocument01.odt", "TestDocument02.odt", "TestSpreadsheet01.ods",
        "TestDrawing01.odg", "TestPresentation01.odp",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/openoffice/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/openoffice/
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
  - OpenOffice files are ZIP packages with XML content (content.xml, meta.xml, styles.xml)
  - Supports ODT (text), ODS (spreadsheet), ODP (presentation), ODG (drawing) and template variants
  - Parameters: extractMetadata, extractReferences, encodeCharacterEntityReferenceGlyphs
  - This is the legacy filter; see okf_odf for the newer ODF filter from the same module
  - Formula results in ODS are extracted as text (not the formulas themselves)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/openoffice/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `OpenOfficeFilterTest.java` | `okapi/filters/openoffice/src/test/java/.../` | 8 |
