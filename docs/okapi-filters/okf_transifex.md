# okf_transifex - Transifex Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_transifex` |
| Java Class | `net.sf.okapi.filters.transifex.TransifexFilter` |
| MIME Types | `application/x-transifex` |
| Extensions | `.txp` |
| Okapi Module | `transifex` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/transifex/src/test/java/`

**NO Java tests exist** for this filter. The `transifex` module has no test directory.

### Integration Tests

No integration tests exist for the Transifex filter.

## Test Data Files

### Unit test resources

No unit test resources exist.

### Integration test resources

No integration test resources exist.

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.txp` | Minimal valid Transifex project file for smoke test | Create a simple .txp file structure |
| `roundtrip.txp` | Basic roundtrip test | Simple Transifex project with translatable content |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Synthetic test data only (no existing Okapi test files)
# Create minimal.txp and roundtrip.txp manually
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/transifex`

Build tag: `//go:build integration`

#### transifex_test.go - Extraction Tests (synthetic)

```go
func TestExtract_BasicTransifex(t *testing.T) {
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
            input: "testdata/minimal.txp",
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
        "roundtrip.txp",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/transifex/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/transifex/
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
- Only parameter: `openProject` (boolean, default true) -- controls whether the project file is opened before processing
- Configurations: `okf_transifex` (default, with prompt), `okf_transifex-noPrompt` (without prompt)
- The Transifex filter processes `.txp` project files used by the Transifex localization platform
- This filter may have limited utility as Transifex has moved to API-based workflows
- Low priority filter with minimal parameter surface area

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/transifex/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| (none) | - | 0 |
