# okf_rainbowkit - RainbowKit Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_rainbowkit` |
| Java Class | `net.sf.okapi.filters.rainbowkit.RainbowKitFilter` |
| MIME Types | `application/x-rainbowkit` |
| Extensions | `.rkm, .rkp` |
| Okapi Module | `rainbowkit` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/rainbowkit/src/test/java/`

#### MergingInfoTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimpleWrite` | Serializes MergingInfo to XML string and verifies docId, extractionType, paths, filterId, encoding, and filter parameters in output | P1 |
| 2 | `testSimpleWriteBase64` | Serializes MergingInfo to XML with base64-encoded filter parameters, verifies encoded content matches expected | P2 |
| 3 | `testSimpleWriteAndRead` | Round-trips MergingInfo through XML write then DOM parse/read, verifies all fields survive serialization | P1 |
| 4 | `testSimpleWriteAndReadBase64` | Round-trips MergingInfo through base64-encoded XML, verifies all fields survive serialization | P2 |

### Integration Tests

No dedicated integration tests found for okf_rainbowkit.

## Test Data Files

### Unit test resources

Source: `okapi/filters/rainbowkit/src/test/resources/`

No test resource files found. The MergingInfoTest constructs all test data programmatically using in-memory MergingInfo objects.

### Integration test resources

None.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.rkm` | Minimal valid Rainbow Translation Kit manifest for smoke test | Must contain manifest.rkm with at least one document reference |
| `sample.rkp` | Minimal RainbowKit package for roundtrip test | ZIP-based package with manifest and at least one translatable file |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# No existing test resources to copy - synthetic files must be created
# Synthetic test data
# (create minimal.rkm and sample.rkp manually)
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_rainbowkit`

Build tag: `//go:build integration`

#### rainbowkit_test.go - Extraction Tests

```go
func TestExtract_minimalManifest(t *testing.T) {
    // Synthetic test - no direct Java equivalent
    // Verifies basic extraction from a minimal .rkm file
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:    "minimal rainbowkit manifest",
            input:   "testdata/minimal.rkm",
            javaRef: "synthetic",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_openManifest(t *testing.T) {
    // Maps to schema property: openManifest (boolean, default true)
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "default openManifest=true",
            params: nil,
            input:  "testdata/sample.rkp",
            javaRef: "synthetic - schema openManifest property",
        },
        {
            name:   "openManifest=false",
            params: map[string]any{"openManifest": false},
            input:  "testdata/sample.rkp",
            javaRef: "synthetic - matches okf_rainbowkit-noprompt config",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Synthetic - no Java roundtrip IT exists
    testFiles := []string{
        "sample.rkp",
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_rainbowkit/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_rainbowkit/
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
  - RainbowKit is Okapi's own translation kit format (ZIP-based packages)
  - Has three configurations: default, package (.rkp), and no-prompt
  - The `openManifest` parameter controls whether the manifest file is opened before processing
  - Java tests are entirely about MergingInfo XML serialization (internal data structure), not filter I/O
  - Synthetic test files must be created for meaningful bridge testing
  - No test resource files exist in the Java source

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/rainbowkit/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `MergingInfoTest.java` | `okapi/filters/rainbowkit/src/test/java/.../` | 4 |
