# okf_rtf - RTF Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_rtf` |
| Java Class | `net.sf.okapi.filters.rtf.RTFFilter` |
| MIME Types | `application/rtf` |
| Extensions | `.rtf` |
| Okapi Module | `rtf` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/rtf/src/test/java/`

#### RTFFilterTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testBasicProcessing` | Opens Test01.rtf and processes all events without error | P1 |
| 2 | `testSimpleTU` | First TU: source "Text (to) translate." / target "Texte a traduire."; Second TU: source with bold formatting, target with bold formatting | P1 |

#### RtfEventTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testStartDoc` | StartDocument event from RTF snippet has non-null FilterWriter | P2 |

#### RtfFullFileTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testAllExternalFiles` | Processes all external test RTF files without error | P1 |

#### RtfSnippetsTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testBold` | Empty test (no-op placeholder) | P3 |

#### RtfTestUtils.java (0 @Test methods)

Helper class providing test file list.

#### integration/ExtractionComparisionTest.java (1 @Test method)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | (integration extraction comparison) | Extraction comparison test | P2 |

### Integration Tests

None found specific to RTF roundtrip/xliff/simplifier/memory leak.

## Test Data Files

### Unit test resources

Source: `okapi/filters/rtf/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.rtf` | `RTFFilterTest#testBasicProcessing`, `RTFFilterTest#testSimpleTU` | Bilingual RTF with source/target translations |

Additional external RTF files referenced by RtfTestUtils.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.rtf` | Minimal valid RTF document for smoke test | Simple paragraph with basic text |
| `formatting.rtf` | RTF with bold, italic, underline formatting | Test inline code extraction |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/rtf/src/test/resources/net/sf/okapi/filters/rtf/*.rtf okapi-testdata/okf_rtf/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_rtf`

Build tag: `//go:build integration`

#### rtf_test.go - Extraction Tests

```go
func TestExtract_BasicText(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "simple_bilingual",
            input: "Test01.rtf",
            wantTexts: []string{"Text (to) translate."},
            javaRef: "RTFFilterTest#testSimpleTU",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "Test01.rtf",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_rtf/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_rtf/
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
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - RTF is a bilingual format: source and target are both present in the file
  - Requires source and target locale IDs for proper extraction
  - Uses windows-1252 encoding by default
  - Very limited test coverage (6 actual @Test methods, one is a no-op)
  - RtfSnippetsTest.java is entirely commented out
  - Need synthetic test data for comprehensive coverage
  - Bold, italic, and other formatting create inline codes

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/rtf/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `RTFFilterTest.java` | `okapi/filters/rtf/src/test/java/.../` | 2 |
| `RtfEventTest.java` | `okapi/filters/rtf/src/test/java/.../` | 1 |
| `RtfFullFileTest.java` | `okapi/filters/rtf/src/test/java/.../` | 1 |
| `RtfSnippetsTest.java` | `okapi/filters/rtf/src/test/java/.../` | 1 (no-op) |
| `ExtractionComparisionTest.java` | `okapi/filters/rtf/src/test/java/.../integration/` | 1 |
