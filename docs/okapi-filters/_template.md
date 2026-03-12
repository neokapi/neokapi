# okf_{id} - {Display Name} Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_{id}` |
| Java Class | `{full.java.class}` |
| MIME Types | `{mime/type}` |
| Extensions | `{.ext1, .ext2}` |
| Okapi Module | `{module}` |
| Has Native Go Reader | {Yes/No} |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/{module}/src/test/java/`

#### {TestClassName}.java ({N} @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `methodName` | Description of what this test verifies | P1/P2/P3 |

<!-- Repeat table for each test class -->

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTrip{Format}IT` | `integration-tests/okapi/src/test/java/.../RoundTrip{Format}IT.java` | N |

**Test files used** (from `integration-tests/okapi/src/test/resources/{format}/`):
- `file1.ext`
- `file2.ext`

**Known failing files**: None / list

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `{Format}XliffCompareIT` | `integration-tests/okapi/src/test/java/.../...XliffCompareIT.java` | N |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplify{Format}IT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplify{Format}IT.java` | N |

#### Memory Leak IT

| Class | File | Test Count |
|-------|------|------------|
| `{Format}MemoryLeakTestIT` | `integration-tests/okapi/src/test/java/.../...MemoryLeakTestIT.java` | N |

## Test Data Files

### Unit test resources

Source: `okapi/filters/{module}/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `file.ext` | `TestClass#method` | Description |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/{format}/`

| File | Type | Purpose |
|------|------|---------|
| `file.ext` | roundtrip / xliff-compare | Description |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.ext` | Minimal valid document for smoke test | If no suitable test file exists |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/{module}/src/test/resources/{files} okapi-testdata/okf_{id}/

# Integration test resources
cp integration-tests/okapi/src/test/resources/{format}/{files} okapi-testdata/okf_{id}/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/{id}`

Build tag: `//go:build integration`

#### {id}_test.go - Extraction Tests

```go
func TestExtract_{scenario}(t *testing.T) {
    // Table-driven: maps 1:1 to Java {TestClass}#{method}
    tests := []struct {
        name     string
        input    string // inline or testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string // e.g. "HtmlSnippetsTest#testSimple"
    }{
        // ... test cases ...
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_{scenario}(t *testing.T) {
    // Maps to Java {ConfigTestClass}
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string // expected extracted texts
        javaRef string
    }{
        // ... test cases ...
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTrip{Format}IT
    testFiles := []string{
        // files from testdata/roundtrip/
    }
    knownFailing := map[string]string{
        // "file.ext": "reason",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

#### xliff_compare_test.go - XLIFF Compare Tests

```go
func TestXliffCompare(t *testing.T) {
    // Maps to Java {Format}XliffCompareIT
    // Verifies Part structure matches expected XLIFF output
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/{id}/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/{id}/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] XLIFF compare structure matches
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks: {list any known issues}

## Java Source References

For change tracking against Okapi baseline commit `{SHA}`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/{module}/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `{File}.java` | `okapi/filters/{module}/src/test/java/.../` | N |
