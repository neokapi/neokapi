# okf_epub - EPUB Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_epub` |
| Java Class | `net.sf.okapi.filters.epub.EpubFilter` |
| MIME Types | `application/epub+zip` |
| Extensions | `.epub` |
| Okapi Module | `epub` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/epub/src/test/java/`

#### EpubFilterTests.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testInformation` | MIME type, name, display name, configuration class and ID match expected values | P2 |
| 2 | `testSimpleReadWrite` | Full read-write cycle: reads test1.epub, writes with uppercased target text, reads output and verifies all text is uppercase | P1 |

### Integration Tests

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/epub/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test1.epub` | `testSimpleReadWrite` | EPUB document for read/write roundtrip |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.epub` | Minimal valid EPUB for smoke test | Single chapter with basic content |
| `with_images.epub` | EPUB with embedded images | Verify non-text content preserved |
| `multi_chapter.epub` | Multi-chapter EPUB | Test multiple internal HTML files |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/epub/src/test/resources/net/sf/okapi/filters/epub/*.epub okapi-testdata/okf_epub/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_epub`

Build tag: `//go:build integration`

#### epub_test.go - Extraction Tests

```go
func TestExtract_BasicDocument(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "simple_epub",
            input: "test1.epub",
            javaRef: "EpubFilterTests#testSimpleReadWrite",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "test1.epub",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_epub/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_epub/
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
  - EPUB is a ZIP package containing XHTML documents, CSS, images, metadata
  - The filter wraps the HTML filter for extracting text from internal XHTML files
  - Only 2 @Test methods - will need extensive synthetic tests
  - setOptions(srcLang, trgLang, encoding, generateTarget) must be called before open
  - Read/write cycle tested: creates target by uppercasing source content
  - No integration roundtrip/xliff compare tests exist

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/epub/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `EpubFilterTests.java` | `okapi/filters/epub/src/test/java/.../` | 2 |
