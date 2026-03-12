# okf_autoxliff - Auto XLIFF Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_autoxliff` |
| Java Class | `net.sf.okapi.filters.autoxliff.AutoXLIFFFilter` |
| MIME Types | `application/x-xliff+xml` |
| Extensions | - |
| Okapi Module | `autoxliff` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/autoxliff/src/test/java/`

#### TestAutoXLIFFFilter.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDelegateXLIFF20` | Opens xliff2.xlf, verifies delegation to XLIFF 2.0 filter, extracts 1 TU "Sample segment.", roundtrips against gold/xliff2.xlf | P1 |
| 2 | `testDelegateXLIFF12` | Opens xliff12.xlf, verifies delegation to XLIFF 1.2 filter, extracts 1 TU "Segment one.", roundtrips against gold/xliff12.xlf | P1 |
| 3 | `testDelegateSDLXLIFF` | Configures xliff12Config as "okf_xliff-sdl", opens sdlxliff.xlf, extracts 1 TU with 3 sentences, verifies SDLXLIFF-specific metadata (segment confirmation "Translated") and French target "Premiere phrase" | P1 |

### Integration Tests

No dedicated integration tests found for okf_autoxliff.

## Test Data Files

### Unit test resources

Source: `okapi/filters/autoxliff/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `xliff2.xlf` | `testDelegateXLIFF20` | Minimal XLIFF 2.0 file with 1 segment |
| `xliff12.xlf` | `testDelegateXLIFF12` | Minimal XLIFF 1.2 file with 1 segment |
| `sdlxliff.xlf` | `testDelegateSDLXLIFF` | SDLXLIFF file with segment confirmation metadata |
| `gold/xliff2.xlf` | `testDelegateXLIFF20` | Expected roundtrip output for XLIFF 2.0 |
| `gold/xliff12.xlf` | `testDelegateXLIFF12` | Expected roundtrip output for XLIFF 1.2 |

### Integration test resources

None.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `xliff12-multitu.xlf` | XLIFF 1.2 with multiple TUs for richer extraction test | Test extraction of multiple segments |
| `xliff2-multitu.xlf` | XLIFF 2.0 with multiple TUs | Test XLIFF 2.0 delegation with more complex content |
| `ambiguous.xlf` | XLIFF file that tests version auto-detection edge cases | Verify correct filter delegation |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/autoxliff/src/test/resources/xliff2.xlf okapi-testdata/okf_autoxliff/
cp okapi/filters/autoxliff/src/test/resources/xliff12.xlf okapi-testdata/okf_autoxliff/
cp okapi/filters/autoxliff/src/test/resources/sdlxliff.xlf okapi-testdata/okf_autoxliff/
cp okapi/filters/autoxliff/src/test/resources/gold/xliff2.xlf okapi-testdata/okf_autoxliff/gold/
cp okapi/filters/autoxliff/src/test/resources/gold/xliff12.xlf okapi-testdata/okf_autoxliff/gold/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/autoxliff`

Build tag: `//go:build integration`

#### autoxliff_test.go - Extraction Tests

```go
func TestExtract_autoDetection(t *testing.T) {
    // Table-driven: maps 1:1 to Java TestAutoXLIFFFilter
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:       "delegate to XLIFF 2.0",
            input:      "testdata/xliff2.xlf",
            wantBlocks: 1,
            wantTexts:  []string{"Sample segment."},
            javaRef:    "TestAutoXLIFFFilter#testDelegateXLIFF20",
        },
        {
            name:       "delegate to XLIFF 1.2",
            input:      "testdata/xliff12.xlf",
            wantBlocks: 1,
            wantTexts:  []string{"Segment one."},
            javaRef:    "TestAutoXLIFFFilter#testDelegateXLIFF12",
        },
        {
            name:       "delegate to SDLXLIFF",
            input:      "testdata/sdlxliff.xlf",
            wantBlocks: 1,
            wantTexts:  []string{"First sentence. Second longer sentence. Followed by a third one."},
            params:     map[string]any{"xliff_config": "okf_xliff-sdl"},
            javaRef:    "TestAutoXLIFFFilter#testDelegateSDLXLIFF",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_delegateConfigs(t *testing.T) {
    // Maps to schema properties: xliff_config, xliff2_config
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "default xliff config (okf_xliff)",
            params: nil,
            input:  "testdata/xliff12.xlf",
            want:   []string{"Segment one."},
            javaRef: "TestAutoXLIFFFilter#testDelegateXLIFF12",
        },
        {
            name:   "sdlxliff config override",
            params: map[string]any{"xliff_config": "okf_xliff-sdl"},
            input:  "testdata/sdlxliff.xlf",
            want:   []string{"First sentence. Second longer sentence. Followed by a third one."},
            javaRef: "TestAutoXLIFFFilter#testDelegateSDLXLIFF",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java TestAutoXLIFFFilter roundtrip helper
    testFiles := []string{
        "xliff2.xlf",
        "xliff12.xlf",
    }
    knownFailing := map[string]string{
        // none known
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/autoxliff/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/autoxliff/
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
  - This is a meta-filter that auto-detects the XLIFF version and delegates to the appropriate filter
  - `xliff_config` (default: "okf_xliff") controls which filter handles XLIFF 1.2 files
  - `xliff2_config` (default: "okf_xliff2") controls which filter handles XLIFF 2.0 files
  - Can be configured to use "okf_xliff-sdl" for SDLXLIFF handling
  - The filter saves the correct delegate filter's parameters in StartDocument
  - Java tests use xmlunit's CompareMatcher for XML-identical roundtrip verification
  - SDLXLIFF test verifies extraction of SDL-specific metadata (segment confirmation status)
  - No file extensions registered by default; the filter relies on MIME type detection

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/autoxliff/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TestAutoXLIFFFilter.java` | `okapi/filters/autoxliff/src/test/java/.../` | 3 |
