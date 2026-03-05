# okf_archive - Archive Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_archive` |
| Java Class | `net.sf.okapi.filters.archive.ArchiveFilter` |
| MIME Types | `application/x-archive` |
| Extensions | `.zip, .archive` |
| Okapi Module | `archive` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/archive/src/test/java/`

#### ArchiveFilterTest.java (10 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSubFilterOpen` | Opens ZIP entry from test1_es.archive with XLIFF subfilter | P2 |
| 2 | `testFilterOpen` | Opens ZIP entry from test1_es.archive via filter pipeline | P2 |
| 3 | `testNoTUs` | Archive with unknown file types produces only document parts, no TUs | P1 |
| 4 | `testMimeType` | Default MIME type is "application/x-archive"; custom MIME via params | P2 |
| 5 | `testExtractXLIFFOnly` | fileNames="*.xlf", configIds="okf_xliff": extracts only XLIFF TU "About..." | P1 |
| 6 | `testExtractTMXOnly` | fileNames="*.tmx", configIds="okf_tmx": extracts only TMX TU "test en" | P1 |
| 7 | `testExtractXLIFFandTMX` | Combined fileNames/configIds extracts both XLIFF and TMX TUs | P1 |
| 8 | `testNoExtraction` | Empty fileNames/configIds extracts nothing | P2 |
| 9 | `testMissingFilter` | Missing filter config throws OkapiIOException | P2 |
| 10 | `testWithStream` | Stream-based read/write pipeline with archive filter, verifies TMX and XLIFF output | P1 |

(Also includes `testDoubelextraction` - double extraction roundtrip for 3 archive files)

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripArchiveIT` | `integration-tests/okapi/src/test/java/.../RoundTripArchiveIT.java` | 2 |

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `ArchiveXliffCompareIT` | `integration-tests/okapi/src/test/java/.../ArchiveXliffCompareIT.java` | N/A |

#### Simplifier IT

None found.

#### Memory Leak IT

None found.

## Test Data Files

### Unit test resources

Source: `okapi/filters/archive/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test1_es.archive` | `testSubFilterOpen`, `testFilterOpen`, `testDoubelextraction` | Archive with XLIFF content |
| `test2_unknownfiles.archive` | `testNoTUs`, `testDoubelextraction` | Archive with unknown file types |
| `test3_es.archive` | `testExtractXLIFFOnly`, `testExtractTMXOnly`, `testExtractXLIFFandTMX`, `testNoExtraction`, `testWithStream`, `testDoubelextraction` | Archive with both XLIFF (.xlf) and TMX (.tmx) files |

### Synthetic test data to create

None needed - test data covers key scenarios.

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/archive/src/test/resources/net/sf/okapi/filters/archive/*.archive okapi-testdata/okf_archive/

# Integration test resources
cp integration-tests/okapi/src/test/resources/archive/*.archive okapi-testdata/okf_archive/roundtrip/
cp integration-tests/okapi/src/test/resources/archive/*.zip okapi-testdata/okf_archive/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/archive`

Build tag: `//go:build integration`

#### archive_test.go - Extraction Tests

```go
func TestExtract_SubFilterContent(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:  "xliff_only",
            input: "test3_es.archive",
            params: map[string]any{"fileNames": "*.xlf", "configIds": "okf_xliff"},
            wantTexts: []string{"About..."},
            javaRef: "ArchiveFilterTest#testExtractXLIFFOnly",
        },
        {
            name:  "no_extractable_content",
            input: "test2_unknownfiles.archive",
            wantBlocks: 0,
            javaRef: "ArchiveFilterTest#testNoTUs",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    testFiles := []string{
        "test1_es.archive",
        "test2_unknownfiles.archive",
        "test3_es.archive",
    }
    knownFailing := map[string]string{}
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/archive/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/archive/
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
- Filter-specific quirks:
  - Archive filter is a container filter that delegates to sub-filters based on file patterns
  - Parameters: fileNames (glob patterns like "*.xlf, *.tmx"), configIds (filter config IDs like "okf_xliff, okf_tmx"), mimeType
  - Requires FilterConfigurationMapper to be set with sub-filter configurations
  - Archives with no matching patterns produce only document parts (no TUs)
  - Missing sub-filter configurations throw OkapiIOException
  - Works with standard ZIP format archives

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/archive/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `ArchiveFilterTest.java` | `okapi/filters/archive/src/test/java/.../` | 10 |
