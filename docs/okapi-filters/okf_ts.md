# okf_ts - Qt TS Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_ts` |
| Java Class | `net.sf.okapi.filters.ts.TsFilter` |
| MIME Types | `application/x-ts` |
| Extensions | `.ts` |
| Okapi Module | `ts` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/ts/src/test/java/`

#### TsFilterTest.java (39 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `StartDocument` | Start document event properties from snippet | P3 |
| 2 | `DocumentPartTsPart` | Document part from TS element | P2 |
| 3 | `StartGroupContextPart` | Context (group start) extraction | P1 |
| 4 | `TextUnitMessageUnfinished` | Unfinished message extraction with source/target | P1 |
| 5 | `TestDecodeByteFalse` | Byte value decoding disabled | P2 |
| 6 | `TestDecodeByteTrueDec` | Decimal byte value decoding | P2 |
| 7 | `TestDecodeByteTrueHex` | Hex byte value decoding | P2 |
| 8 | `testTranslationStatus` | Translation status (unfinished/finished/obsolete) | P1 |
| 9 | `testInlineCodes` | Inline codes (`<byte>`, `<numerusform>`) extraction | P1 |
| 10 | `testInlineCodesOutput` | Inline codes in roundtrip output | P1 |
| 11 | `TestDecodeByteTrueHex2` | Extended hex byte decoding | P2 |
| 12 | `TestEncodeIncludedChars` | Character encoding for included chars | P2 |
| 13 | `TestEncodeExcludedChars` | Character encoding for excluded chars | P2 |
| 14 | `AllEvents` | Full event stream from TS file | P1 |
| 15 | `StartDocument_FromFile` | Start document from file | P1 |
| 16 | `StartGroupContextPart_FromFile` | Context group from file | P1 |
| 17 | `TextUnitMessageUnfinished_FromFile` | Unfinished message from file | P1 |
| 18 | `TextUnitMessageApproved_FromFile` | Approved message from file | P1 |
| 19 | `TextUnitMessageObsolete_FromFile` | Obsolete message from file | P1 |
| 20 | `TextUnitMessageMissingTranslation_FromFile` | Missing translation from file | P1 |
| 21 | `TextUnitMessageMissingSourceAndTranslation_FromFile` | Missing source and translation | P1 |
| 22 | `TextUnitMessageMissingSourceNotTranslation_FromFile` | Missing source but has translation | P2 |
| 23 | `TextUnitMessageEmptySource_FromFile` | Empty source from file | P2 |
| 24 | `TextUnitMessageEmptyTranslation_FromFile` | Empty translation from file | P2 |
| 25 | `StartGroupNumerusPart_FromFile` | Numerus (plural) group from file | P1 |
| 26 | `TextUnitNumerus_FromFile` | Numerus form extraction from file | P1 |
| 27 | `testDoubleExtraction` | Double extraction consistency | P1 |
| 28 | `testGetName` | Filter name accessor | P3 |
| 29 | `testGetMimeType` | MIME type accessor | P3 |
| 30 | `testSourceLangNotSpecified` | Exception when source lang missing | P2 |
| 31 | `testTargetLangNotSpecified` | Exception when target lang missing | P2 |
| 32 | `testTargetLangNotSpecified2` | Variant of missing target lang | P2 |
| 33 | `testSourceLangEmpty` | Exception when source lang empty | P2 |
| 34 | `testTargetLangEmpty` | Exception when target lang empty | P2 |
| 35 | `testInputStream` | Opening from InputStream | P1 |
| 36 | `testConsolidatedStream` | Consolidated bilingual stream | P1 |
| 37 | `testTu` | Basic translation unit extraction | P1 |
| 38 | `testStartDocument` | Start document from file | P3 |
| 39 | `runTest` | Parameterized test runner | P2 |
| 40 | `testExtraComment` | Extra comment handling | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTsIT` | `integration-tests/okapi/src/test/java/.../RoundTripTsIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/ts/`):
- `alarm_ro.ts`, `autoSample.ts`, `Complete_valid_utf8_bom_crlf.ts`
- `issue531.ts`, `Test_nautilus.af.ts`, `TestInQT_Saved.ts`
- `TestInQT.ts`, `tstest.ts`, `TSTest01.ts`

**Known failing files**: `issue531.ts`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TsXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TsXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/ts/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `alarm_ro.ts` | `TsFilterTest` | Romanian alarm TS file |
| `autoSample.ts` | `TsFilterTest` | Auto-generated sample |
| `Complete_valid_utf8_bom_crlf.ts` | `TsFilterTest` | UTF-8 BOM + CRLF |
| `Test_nautilus.af.ts` | `TsFilterTest` | Nautilus Afrikaans TS |
| `TestInQT_Saved.ts` | `TsFilterTest` | QT Creator saved TS |
| `TestInQT.ts` | `TsFilterTest` | QT Creator TS |
| `TSTest01.ts` | `TsFilterTest#testDoubleExtraction` | Basic TS test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/ts/`

| File | Type | Purpose |
|------|------|---------|
| `tstest.ts` | roundtrip | Standard TS roundtrip |
| `issue531.ts` | roundtrip | Issue 531 test (known failing) |
| All unit test files | roundtrip | Shared with unit tests |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/ts/src/test/resources/*.ts okapi-testdata/okf_ts/

# Integration test resources
cp integration-tests/okapi/src/test/resources/ts/*.ts okapi-testdata/okf_ts/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_ts`

Build tag: `//go:build integration`

#### ts_test.go - Extraction Tests

```go
func TestExtract_messages(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "unfinished message", javaRef: "TsFilterTest#TextUnitMessageUnfinished"},
        {name: "translation status", javaRef: "TsFilterTest#testTranslationStatus"},
        {name: "inline codes", javaRef: "TsFilterTest#testInlineCodes"},
        {name: "context group", javaRef: "TsFilterTest#StartGroupContextPart"},
        {name: "numerus forms", javaRef: "TsFilterTest#TextUnitNumerus_FromFile"},
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    knownFailing := map[string]string{
        "issue531.ts": "Known issue with TS format edge case",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_ts/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_ts/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - Bilingual filter: Qt TS format has source + translation in same file
  - Translation states: unfinished, finished (type=""), obsolete (type="obsolete")
  - Context element provides grouping (like namespace)
  - Numerus forms for plural handling
  - `<byte>` elements for special character encoding (hex/decimal)
  - Requires both source and target locale

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/ts/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TsFilterTest.java` | `okapi/filters/ts/src/test/java/net/sf/okapi/filters/ts/` | 39 |
