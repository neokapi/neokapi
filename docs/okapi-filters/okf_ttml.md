# okf_ttml - TTML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_ttml` |
| Java Class | `net.sf.okapi.filters.ttml.TTMLFilter` |
| MIME Types | `application/ttml+xml` |
| Extensions | `.ttml` |
| Okapi Module | `subtitles` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/subtitles/src/test/java/`

#### TTMLFilterTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testProcessTextUnit` | Basic TTML `<p>` element text unit extraction | P1 |
| 2 | `testMergeCaptions` | Merge adjacent captions into single text unit | P1 |
| 3 | `testDontMergeCaptions` | Captions not merged when disabled | P1 |
| 4 | `testQuoteCaptions` | Quote and punctuation handling in captions | P1 |
| 5 | `testEmptyCaptions` | Empty caption handling | P2 |
| 6 | `testReadMaxCharMaxLine` | Max characters and max lines per caption | P2 |
| 7 | `testCodeFinder` | Inline code patterns in TTML content | P1 |
| 8 | `testProcessTextUnitNonEscapeBrMode` | Non-escaped `<br>` mode handling | P1 |

#### TTMLSkeletonWriterTest.java

Writer tests for TTML output.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTtmlIT` | `integration-tests/okapi/src/test/java/.../RoundTripTtmlIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/ttml/`):
- `example1.ttml`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TtmlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TtmlXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/subtitles/src/test/resources/`

No dedicated TTML test files in unit test resources (tests use inline snippets).

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/ttml/`

| File | Type | Purpose |
|------|------|---------|
| `example1.ttml` | roundtrip | Standard TTML document |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.ttml` | Minimal valid TTML | Basic `<tt>` with single `<p>` element |

## Test Data Collection

```bash
# Integration test resources
cp integration-tests/okapi/src/test/resources/ttml/*.ttml okapi-testdata/okf_ttml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_ttml`

Build tag: `//go:build integration`

#### ttml_test.go - Extraction Tests

```go
func TestExtract_captions(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        params     map[string]any
        javaRef    string
    }{
        {name: "basic text unit", javaRef: "TTMLFilterTest#testProcessTextUnit"},
        {name: "merge captions", javaRef: "TTMLFilterTest#testMergeCaptions"},
        {name: "dont merge captions", javaRef: "TTMLFilterTest#testDontMergeCaptions"},
        {name: "code finder patterns", javaRef: "TTMLFilterTest#testCodeFinder"},
        {name: "non-escape br mode", javaRef: "TTMLFilterTest#testProcessTextUnitNonEscapeBrMode"},
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_ttml/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_ttml/
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
  - Timed Text Markup Language (W3C standard for subtitles)
  - XML-based format with `<tt>`, `<body>`, `<div>`, `<p>` elements
  - Caption merging feature for multi-line subtitles
  - `<br>` handling modes (escaped vs non-escaped)
  - Max character/line limits for subtitle display
  - Code finder for inline patterns
  - Shares module with VTT filter

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/subtitles/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TTMLFilterTest.java` | `okapi/filters/subtitles/src/test/java/net/sf/okapi/filters/ttml/` | 8 |
| `TTMLSkeletonWriterTest.java` | `okapi/filters/subtitles/src/test/java/net/sf/okapi/filters/ttml/` | - |
