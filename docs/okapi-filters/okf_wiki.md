# okf_wiki - Wiki Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_wiki` |
| Java Class | `net.sf.okapi.filters.wiki.WikiFilter` |
| MIME Types | `text/x-wiki-txt` |
| Extensions | `.txt` |
| Okapi Module | `wiki` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/wiki/src/test/java/`

#### WikiFilterTest.java (11 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testStartDocument` | Start document event | P3 |
| 3 | `testSimpleLine` | Simple line extraction | P1 |
| 4 | `testMultipleLines` | Multiple lines extraction | P1 |
| 5 | `testHeader` | Wiki header extraction (== Header ==) | P1 |
| 6 | `testTable` | Wiki table extraction | P1 |
| 7 | `testImageCaption` | Image caption extraction | P1 |
| 8 | `testSimilarHtmlTags` | HTML-like tags in wiki markup | P2 |
| 9 | `testComplexSeparatingWhitespace` | Complex whitespace between elements | P2 |
| 10 | `testDoubleExtraction` | Double extraction consistency | P1 |
| 11 | `testOpenTwiceWithString` | Filter reopen from string | P1 |

#### WikiWriterTest.java

Writer tests for wiki output.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripWikiIT` | `integration-tests/okapi/src/test/java/.../RoundTripWikiIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/wikitext/`):
- `api_simple.wiki`, `dokuwiki.wiki`, `mediawiki.wiki`, `simple.wiki`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `WikXliffCompareIT` | `integration-tests/okapi/src/test/java/.../WikXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/wiki/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `dokuwiki.txt` | `WikiFilterTest#testDoubleExtraction` | DokuWiki format test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/wikitext/`

| File | Type | Purpose |
|------|------|---------|
| `api_simple.wiki` | roundtrip | Simple API wiki page |
| `dokuwiki.wiki` | roundtrip | DokuWiki format |
| `mediawiki.wiki` | roundtrip | MediaWiki format |
| `simple.wiki` | roundtrip | Simple wiki page |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/wiki/src/test/resources/dokuwiki.txt okapi-testdata/okf_wiki/

# Integration test resources
cp integration-tests/okapi/src/test/resources/wikitext/*.wiki okapi-testdata/okf_wiki/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/wiki`

Build tag: `//go:build integration`

#### wiki_test.go - Extraction Tests

```go
func TestExtract_wikiContent(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple line", javaRef: "WikiFilterTest#testSimpleLine"},
        {name: "headers", javaRef: "WikiFilterTest#testHeader"},
        {name: "tables", javaRef: "WikiFilterTest#testTable"},
        {name: "image captions", javaRef: "WikiFilterTest#testImageCaption"},
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/wiki/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/wiki/
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
  - Supports MediaWiki and DokuWiki markup variants
  - Headers (== ... ==), tables, image captions extracted
  - Wiki markup inline formatting preserved as codes
  - HTML-like tags within wiki content handled
  - Uses LocaleId.EMPTY for source locale

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/wiki/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `WikiFilterTest.java` | `okapi/filters/wiki/src/test/java/net/sf/okapi/filters/wiki/` | 11 |
| `WikiWriterTest.java` | `okapi/filters/wiki/src/test/java/net/sf/okapi/filters/wiki/` | - |
