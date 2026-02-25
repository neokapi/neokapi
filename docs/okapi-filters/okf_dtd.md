# okf_dtd - DTD Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_dtd` |
| Java Class | `net.sf.okapi.filters.dtd.DTDFilter` |
| MIME Types | `application/xml+dtd` |
| Extensions | `.dtd, .ent` |
| Okapi Module | `dtd` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/dtd/src/test/java/`

#### DTDFilterTest.java (6 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata (name, mime type) | P3 |
| 2 | `testStartDocument` | Start document event properties | P3 |
| 3 | `testSimpleEntry` | Basic entity extraction from DTD | P1 |
| 4 | `testLineBreaks` | Line break handling in entity content | P1 |
| 5 | `testEntryWithEnitties` | Entities containing entity references | P1 |
| 6 | `testEntryWithNCRs` | Entities with numeric character references | P1 |
| 7 | `testDoubleExtraction` | Double extraction consistency check | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripDtdIT` | `integration-tests/okapi/src/test/java/.../RoundTripDtdIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/dtd/`): No files found (empty dir or auto-generated).

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `DtdXliffCompareIT` | `integration-tests/okapi/src/test/java/.../DtdXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/dtd/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.dtd` | `DTDFilterTest#testDoubleExtraction` | Basic DTD entities |
| `Test02.dtd` | `DTDFilterTest#testDoubleExtraction` | Additional DTD test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/dtd/`

No integration test resource files found.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.dtd` | Minimal valid DTD for smoke test | `<!ENTITY greeting "Hello world">` |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/dtd/src/test/resources/Test01.dtd okapi-testdata/okf_dtd/
cp okapi/filters/dtd/src/test/resources/Test02.dtd okapi-testdata/okf_dtd/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_dtd`

Build tag: `//go:build integration`

#### dtd_test.go - Extraction Tests

```go
func TestExtract_simpleEntry(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {
            name:    "simple entity extraction",
            javaRef: "DTDFilterTest#testSimpleEntry",
        },
        {
            name:    "entities with entity references",
            javaRef: "DTDFilterTest#testEntryWithEnitties",
        },
        {
            name:    "entities with NCRs",
            javaRef: "DTDFilterTest#testEntryWithNCRs",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_dtd/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_dtd/
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
  - Extracts ENTITY values from DTD files, not element/attribute definitions
  - Supports code finder for inline patterns
  - Parameters: useCodeFinder, moveLeadingAndTrailingCodesToSkeleton, mergeAdjacentCodes, simplifierRules
  - Serialization format is stringParameters (not YAML/JSON)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/dtd/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `DTDFilterTest.java` | `okapi/filters/dtd/src/test/java/net/sf/okapi/filters/dtd/` | 6 |
