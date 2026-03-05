# okf_versifiedtext - Versified Text Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_versifiedtext` |
| Java Class | `net.sf.okapi.filters.versifiedtxt.VersifiedTextFilter` |
| MIME Types | `text/x-versified-txt` |
| Extensions | - |
| Okapi Module | `versifiedtxt` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/versifiedtxt/src/test/java/`

**NO Java tests exist** for this filter. The `versifiedtxt` module has no test directory.

### Integration Tests

No integration tests exist for the Versified Text filter.

## Test Data Files

### Unit test resources

No unit test resources exist.

### Integration test resources

No integration test resources exist.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.txt` | Minimal valid versified text document for smoke test | Create a simple versified text with verse markers |
| `roundtrip.txt` | Basic roundtrip test | Simple document with multiple verses |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Synthetic test data only (no existing Okapi test files)
# Create minimal.txt and roundtrip.txt manually
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/versifiedtext`

Build tag: `//go:build integration`

#### versifiedtext_test.go - Extraction Tests (synthetic)

```go
func TestExtract_BasicVersifiedText(t *testing.T) {
    // Synthetic tests - no Java test equivalents
    tests := []struct {
        name       string
        input      string
        params     map[string]any
        wantBlocks int
        wantTexts  []string
    }{
        {
            name:  "minimal_extraction",
            input: "testdata/minimal.txt",
            // verify basic extraction works
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests (synthetic)

```go
func TestRoundTrip(t *testing.T) {
    // Synthetic roundtrip test
    testFiles := []string{
        "roundtrip.txt",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/versifiedtext/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/versifiedtext/
```

### Success criteria

- [ ] Minimal extraction test passes
- [ ] Roundtrip test passes for synthetic test file
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- **No Java tests exist** -- all Go tests will be synthetic
- Only parameter: `forceTargetOutput` (boolean, default true)
- Only one configuration: `okf_versifiedtxt` (note: config ID uses `versifiedtxt`, filter ID uses `versifiedtext`)
- Versified text format uses verse markers to structure text content
- Low priority filter with minimal parameter surface area

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/versifiedtxt/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| (none) | - | 0 |
