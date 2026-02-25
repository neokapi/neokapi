# okf_sdlpackage - SDL Package Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_sdlpackage` |
| Java Class | `net.sf.okapi.filters.sdlpackage.SdlPackageFilter` |
| MIME Types | `application/x-sdlpackage` |
| Extensions | `.sdlppx, .sdlrpx` |
| Okapi Module | `sdlpackage` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/sdlpackage/src/test/java/`

#### SdlPackageFilterTests.java (5 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testInformation` | Verifies filter name, MIME type, display name, and configuration metadata | P2 |
| 2 | `testSimpleRead` | Opens ts2017-test01.sdlppx, verifies 2 subdocuments, 4 segments, finds expected segment text and subdocument path | P1 |
| 3 | `testSdlppxWithSubFolders` | Opens test-packages.sdlppx with subfolder structure, verifies 4 subdocuments, 3 segments, finds "Text in test-in-subdir." | P1 |
| 4 | `testSdlrpxWithSubFolders` | Opens test-packages.sdlrpx return package, reads target segments, verifies 4 subdocuments, 3 target segments, finds "FR Text in test-in-subdir." | P1 |
| 5 | `testSimpleReadWrite` | Reads ts2017-test01.sdlppx, writes with uppercased target translations, reads back and verifies all targets are uppercased | P1 |

### Integration Tests

No dedicated integration tests found for okf_sdlpackage.

## Test Data Files

### Unit test resources

Source: `okapi/filters/sdlpackage/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `ts2017-test01.sdlppx` | `testSimpleRead`, `testSimpleReadWrite` | SDL Trados Studio 2017 project package with en-US source, 4 segments |
| `test-packages.sdlppx` | `testSdlppxWithSubFolders` | SDLPPX with subdirectory structure, 3 segments across 4 subdocuments |
| `test-packages.sdlrpx` | `testSdlrpxWithSubFolders` | SDL return package with target translations in subdirectories |

### Integration test resources

None.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.sdlppx` | Minimal valid SDL project package for smoke test | If existing test files are sufficient, skip this |
| `multilingual.sdlppx` | Multi-language SDL package | For testing multiple target language extraction |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx okapi-testdata/okf_sdlpackage/
cp okapi/filters/sdlpackage/src/test/resources/test-packages.sdlppx okapi-testdata/okf_sdlpackage/
cp okapi/filters/sdlpackage/src/test/resources/test-packages.sdlrpx okapi-testdata/okf_sdlpackage/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_sdlpackage`

Build tag: `//go:build integration`

#### sdlpackage_test.go - Extraction Tests

```go
func TestExtract_sdlppx(t *testing.T) {
    // Table-driven: maps 1:1 to Java SdlPackageFilterTests
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:       "simple sdlppx read",
            input:      "testdata/ts2017-test01.sdlppx",
            wantBlocks: 2,
            wantTexts:  []string{"It has several paragraphs and several sentences."},
            javaRef:    "SdlPackageFilterTests#testSimpleRead",
        },
        {
            name:       "sdlppx with subfolders",
            input:      "testdata/test-packages.sdlppx",
            wantTexts:  []string{"Text in test-in-subdir."},
            javaRef:    "SdlPackageFilterTests#testSdlppxWithSubFolders",
        },
        {
            name:       "sdlrpx with subfolders",
            input:      "testdata/test-packages.sdlrpx",
            wantTexts:  []string{"FR Text in test-in-subdir."},
            javaRef:    "SdlPackageFilterTests#testSdlrpxWithSubFolders",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_fileNamesAndConfigIds(t *testing.T) {
    // Maps to schema properties: fileNames, configIds, mimeType
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "default file patterns and config IDs",
            params: nil,
            input:  "testdata/ts2017-test01.sdlppx",
            javaRef: "synthetic - schema fileNames/configIds properties",
        },
        {
            name:   "custom file patterns",
            params: map[string]any{
                "fileNames": "*.xlf",
                "configIds": "okf_xliff",
            },
            input:  "testdata/ts2017-test01.sdlppx",
            javaRef: "synthetic",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java SdlPackageFilterTests#testSimpleReadWrite
    testFiles := []string{
        "ts2017-test01.sdlppx",
        "test-packages.sdlppx",
        "test-packages.sdlrpx",
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_sdlpackage/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_sdlpackage/
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
  - SDLPPX = SDL project package, SDLRPX = SDL return package (both ZIP-based)
  - Package contains XLIFF files and TMX files inside; the filter delegates to okf_xliff/okf_tmx sub-filters
  - Configurable properties: `fileNames` (wildcards), `configIds` (matching filter configs), `mimeType`
  - Also has `moveLeadingAndTrailingCodesToSkeleton`, `mergeAdjacentCodes`, `simplifierRules` properties
  - SDLPPX has empty `<target>` elements requiring forced copy with COPY_ALL
  - Subdocument events are important: each XLIFF file inside the package produces a START_SUBDOCUMENT event
  - The testSimpleReadWrite test verifies full write-back capability with modified targets

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/sdlpackage/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `SdlPackageFilterTests.java` | `okapi/filters/sdlpackage/src/test/java/.../` | 5 |
