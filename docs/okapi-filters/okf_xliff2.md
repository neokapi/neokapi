# okf_xliff2 - XLIFF 2 Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xliff2` |
| Java Class | `net.sf.okapi.filters.xliff2.XLIFF2Filter` |
| MIME Types | `application/xliff+xml` |
| Extensions | `.xlf, .xlf2, .xliff, .xliff2` |
| Okapi Module | `xliff2` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/xliff2/src/test/java/`

#### XLIFF2FilterTest.java (25 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimple` | Simple XLIFF 2.0 file with source and target | P1 |
| 2 | `testSubflows` | Subflow elements within units | P2 |
| 3 | `testDedupeCodeFinderCodes` | Code finder deduplication of inline codes | P2 |
| 4 | `testSimpleMeta` | Metadata (mda:metadata) on units | P2 |
| 5 | `testInline` | Inline elements: pc, ph, sc/ec, mrk | P1 |
| 6 | `testInlineCopyOf` | copyOf attribute on inline elements | P2 |
| 7 | `testFromFile` | Extraction from test01.xlf file | P1 |
| 8 | `testFromFile2` | Extraction from test02.xlf file | P1 |
| 9 | `testFromEscapedFile` | Escaped content in XLIFF 2 file | P1 |
| 10 | `testGroupHandling` | Group elements with nested units | P1 |
| 11 | `testWriteXLIFF2AsXliff12` | Convert XLIFF 2 to XLIFF 1.2 output | P2 |
| 12 | `testIgnoreable` | Ignorable elements handling | P2 |
| 13 | `roundTripTests` | Data-driven roundtrip tests from roundtrips/ directory | P1 |
| 14 | `updateTarget` | Target update/creation in writer | P1 |
| 15 | `handleInvalidCodeTypes` | Invalid code type attribute values | P2 |
| 16 | `testDiscardInvalidTargets` | Invalid targets discarded | P2 |
| 17 | `testDoubleExtraction` | Double extraction roundtrip | P1 |
| 18 | `testStateChangeTranslated` | Segment state change to translated | P1 |
| 19 | `testStateChangeInitial` | Segment state change to initial | P1 |
| 20 | `testWriteOriginalDataOption` | Original data preservation in writer | P2 |
| 21 | `testSubFilterWithDefaultIcu` | ICU message format subfilter (default) | P2 |
| 22 | `testSubFilterWithAllOptionsIcu` | ICU subfilter with all options | P2 |
| 23 | `testSubFilterWithAllOptionsIcuRoundtrip` | ICU subfilter roundtrip test | P2 |
| 24 | `testMetadataXLIFF2intoXliff12` | Metadata conversion to XLIFF 1.2 | P2 |
| 25 | `testSegmentStateAndSubstateXLIFF2intoXliff12` | Segment state conversion | P2 |

#### Xliff2FilterWriterTest.java (52 @Test methods)

Tests the XLIFF 2 writer: roundtrip of simple/complex files, placeholder handling, original data, state changes, version output, empty placeholders, subfilter roundtrips.

#### XLIFF2CodeFinderRoundTripTest.java (test count varies)

Tests code finder integration with XLIFF 2 roundtrip.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripXliff2IT` | `integration-tests/okapi/src/test/java/.../RoundTripXliff2IT.java` | 2 |

**Test files used**: 56 files in `integration-tests/okapi/src/test/resources/xliff2/`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `Xliff2XliffCompareIT` | `integration-tests/okapi/src/test/java/.../Xliff2XliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/xliff2/src/test/resources/`

38 files including `.xlf` files, gold standard outputs, roundtrip input/output pairs.

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/xliff2/`

56 files including subfilter_html subdirectory.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/xliff2/src/test/resources/*.xlf okapi-testdata/okf_xliff2/
cp okapi/filters/xliff2/src/test/resources/*.html okapi-testdata/okf_xliff2/
cp -r okapi/filters/xliff2/src/test/resources/gold okapi-testdata/okf_xliff2/gold/
cp -r okapi/filters/xliff2/src/test/resources/roundtrips okapi-testdata/okf_xliff2/roundtrips/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/xliff2/* okapi-testdata/okf_xliff2/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_xliff2`

Build tag: `//go:build integration`

#### xliff2_test.go - Extraction Tests

```go
func TestExtract_BasicUnit(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        // ... from XLIFF2FilterTest
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_xliff2/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_xliff2/
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
- Filter-specific quirks:
  - XLIFF 2.0 uses different inline model: pc (paired code), ph (placeholder), sc/ec (start/end code), mrk (marker)
  - Segment state model: initial, translated, reviewed, final (with substates)
  - Original data stored in `<originalData>` section, referenced by inline codes via `dataRef`
  - Groups contain units; files contain groups/units
  - Ignorable elements are extracted but marked as non-translatable
  - Supports ICU message format subfilter for parameterized content
  - Can convert XLIFF 2.0 to XLIFF 1.2 output format
  - copyOf attribute allows code reuse within segments

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/xliff2/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XLIFF2FilterTest.java` | `okapi/filters/xliff2/src/test/java/.../` | 25 |
| `Xliff2FilterWriterTest.java` | `okapi/filters/xliff2/src/test/java/.../` | 52 |
| `XLIFF2CodeFinderRoundTripTest.java` | `okapi/filters/xliff2/src/test/java/.../` | varies |
