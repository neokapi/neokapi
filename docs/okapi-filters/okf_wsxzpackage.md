# okf_wsxzpackage - WSXZ Package Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_wsxzpackage` |
| Java Class | `net.sf.okapi.filters.wsxzpackage.WsxzPackageFilter` |
| MIME Types | `application/x-wsxzpackage` |
| Extensions | `.wsxz` |
| Okapi Module | `wsxzpackage` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/wsxzpackage/src/test/java/`

#### WsxzPackageFilterTests.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testInformation` | Verifies filter name ("okf_wsxzpackage"), MIME type, display name ("WSXZ Filter"), and single configuration | P2 |
| 2 | `testSimpleRead` | Opens test1.wsxz, verifies 2 subdocuments, 15 segments, finds "Applications" text and subdocument path | P1 |
| 3 | `testSimpleReadWrite` | Reads test1.wsxz, writes with uppercased targets, reads back and verifies all targets are uppercase | P1 |

### Integration Tests

No dedicated integration tests found for okf_wsxzpackage.

## Test Data Files

### Unit test resources

Source: `okapi/filters/wsxzpackage/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test1.wsxz` | `testSimpleRead`, `testSimpleReadWrite` | WorldServer WSXZ package with en-US/es-ES, 15 segments across 2 subdocuments |

### Integration test resources

None.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `multilingual.wsxz` | Test with different language pairs | Synthetic WSXZ with multiple XLIFF files |
| `empty.wsxz` | Edge case: WSXZ with no translatable content | Verify graceful handling of empty packages |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/wsxzpackage/src/test/resources/test1.wsxz okapi-testdata/okf_wsxzpackage/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_wsxzpackage`

Build tag: `//go:build integration`

#### wsxzpackage_test.go - Extraction Tests

```go
func TestExtract_wsxz(t *testing.T) {
    // Table-driven: maps 1:1 to Java WsxzPackageFilterTests
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:       "simple read",
            input:      "testdata/test1.wsxz",
            wantBlocks: 2,
            wantTexts:  []string{"Applications"},
            javaRef:    "WsxzPackageFilterTests#testSimpleRead",
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
            name:   "default file patterns",
            params: nil,
            input:  "testdata/test1.wsxz",
            javaRef: "synthetic - schema fileNames/configIds properties",
        },
        {
            name:   "custom file names pattern",
            params: map[string]any{
                "fileNames": "*.xlf",
                "configIds": "okf_xliff",
            },
            input:  "testdata/test1.wsxz",
            javaRef: "synthetic",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java WsxzPackageFilterTests#testSimpleReadWrite
    testFiles := []string{
        "test1.wsxz",
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_wsxzpackage/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_wsxzpackage/
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
  - WSXZ = WorldServer XLIFF Zip package format
  - Structurally very similar to okf_sdlpackage (ZIP-based, delegates to sub-filters)
  - Same schema properties as okf_sdlpackage: `fileNames`, `configIds`, `mimeType`, `moveLeadingAndTrailingCodesToSkeleton`, `mergeAdjacentCodes`, `simplifierRules`
  - Default patterns: `*.tmx,*.xlf,*.xlff` with config IDs `okf_tmx,okf_xliff,okf_xliff`
  - Like SDLPPX, WSXZ has empty `<target>` elements requiring forced copy with COPY_ALL
  - Only 1 test resource file exists; synthetic files recommended for broader coverage

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/wsxzpackage/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `WsxzPackageFilterTests.java` | `okapi/filters/wsxzpackage/src/test/java/.../` | 3 |
