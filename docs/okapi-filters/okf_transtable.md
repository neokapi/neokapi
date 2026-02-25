# okf_transtable - TransTable Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_transtable` |
| Java Class | `net.sf.okapi.filters.transtable.TransTableFilter` |
| MIME Types | `text/x-transtable` |
| Extensions | (none - used for pipeline output) |
| Okapi Module | `transtable` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/transtable/src/test/java/`

#### TransTableFilterTest.java (7 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testStartDocument` | Start document event | P3 |
| 2 | `testMinimalInput` | Minimal TransTable input parsing | P1 |
| 3 | `testMinimalSourceTarget` | Source and target column extraction | P1 |
| 4 | `testQuotesInput` | Quoted field handling | P1 |
| 5 | `testUnSegmented` | Unsegmented text extraction | P1 |
| 6 | `testSegmented` | Segmented text extraction | P1 |
| 7 | `testSegmentedWithTarget` | Segmented with target translations | P1 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTranstableIT` | `integration-tests/okapi/src/test/java/.../RoundTripTranstableIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/transtable/`):
- `test01.xml.txt`

**Known failing files**: `test01.xml.txt`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TranstableXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TranstableXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/transtable/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test01.xml.txt` | `TransTableFilterTest` | TransTable format test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/transtable/`

| File | Type | Purpose |
|------|------|---------|
| `test01.xml.txt` | roundtrip | TransTable roundtrip (known failing) |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/transtable/src/test/resources/test01.xml.txt okapi-testdata/okf_transtable/

# Integration test resources
cp integration-tests/okapi/src/test/resources/transtable/test01.xml.txt okapi-testdata/okf_transtable/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_transtable`

Build tag: `//go:build integration`

#### transtable_test.go - Extraction Tests

```go
func TestExtract_transTable(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "minimal input", javaRef: "TransTableFilterTest#testMinimalInput"},
        {name: "source and target", javaRef: "TransTableFilterTest#testMinimalSourceTarget"},
        {name: "quoted fields", javaRef: "TransTableFilterTest#testQuotesInput"},
        {name: "segmented text", javaRef: "TransTableFilterTest#testSegmented"},
        {name: "segmented with target", javaRef: "TransTableFilterTest#testSegmentedWithTarget"},
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    knownFailing := map[string]string{
        "test01.xml.txt": "Known failing in Java integration tests",
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_transtable/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_transtable/
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
  - Tab-separated translation table format (source\ttarget per line)
  - Designed for Okapi pipeline output, not typically a source format
  - Quoted fields for content containing tabs/newlines
  - Supports segmented and unsegmented modes
  - No standard file extension
  - Only roundtrip test file is known failing

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/transtable/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TransTableFilterTest.java` | `okapi/filters/transtable/src/test/java/net/sf/okapi/filters/transtable/` | 7 |
