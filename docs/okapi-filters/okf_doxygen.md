# okf_doxygen - Doxygen Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_doxygen` |
| Java Class | `net.sf.okapi.filters.doxygen.DoxygenFilter` |
| MIME Types | `text/x-doxygen-txt` |
| Extensions | `.c, .cpp, .h, .java, .m, .py` |
| Okapi Module | `doxygen` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/doxygen/src/test/java/`

#### DoxygenFilterTest.java (28 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testStartDocument` | Start document event | P3 |
| 3 | `testSimpleLine` | Single line Doxygen comment extraction | P1 |
| 4 | `testMultipleLines` | Multi-line Doxygen comment extraction | P1 |
| 5 | `testOneLiner` | One-liner comment extraction | P1 |
| 6 | `testBlankOneLiner` | Blank one-liner comment | P2 |
| 7 | `testJavadocLine` | Javadoc-style comment line | P1 |
| 8 | `testJavadocMultiline` | Javadoc-style multi-line comment | P1 |
| 9 | `testDoxygenClassCommand1` | Doxygen @class command variant 1 | P1 |
| 10 | `testDoxygenClassCommand2` | Doxygen @class command variant 2 | P1 |
| 11 | `testDoxygenCodeCommand` | Doxygen @code/@endcode blocks excluded | P1 |
| 12 | `testDoxygenItalicCommand` | Doxygen @e/@a italic commands as inline codes | P2 |
| 13 | `testDoxygenImageCommand` | Doxygen @image command handling | P2 |
| 14 | `testHtmlBoldCommand` | HTML `<b>` tag in Doxygen comments | P1 |
| 15 | `testOutputSimpleLine` | Simple line roundtrip output | P1 |
| 16 | `testOutputOneLiner` | One-liner roundtrip output | P1 |
| 17 | `testOutputMultipleLines` | Multi-line roundtrip output | P1 |
| 18 | `testOutputMultipleLineList` | (Ignored) Multi-line list output | P2 |
| 19 | `testOutputJavadocMultipleLines` | Javadoc multi-line output | P1 |
| 20 | `testOrphanedEndCommand` | Orphaned @end command handling | P2 |
| 21 | `testPositiveFloatListFalsePositive` | Float in list not misidentified | P2 |
| 22 | `testDoubleExtractionSample` | Double extraction: sample.h | P1 |
| 23 | `testDoubleExtractionQtStyle` | Double extraction: Qt-style | P1 |
| 24 | `testDoubleExtractionJavadocStyle` | Double extraction: Javadoc-style | P1 |
| 25 | `testDoubleExtractionSpecialCommands` | Double extraction: special commands | P1 |
| 26 | `testDoubleExtractionLists` | Double extraction: lists | P1 |
| 27 | `testOpenTwiceWithString` | Filter reopen from string | P1 |
| 28 | `testDelimiterTokenizer` | Delimiter-based tokenization | P2 |
| 29 | `testPrefixSuffixTokenizer` | Prefix/suffix-based tokenization | P2 |

#### DoxygenWriterTest.java

Writer tests for Doxygen output (included in DoxygenFilterTest).

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripDoxygenIT` | `integration-tests/okapi/src/test/java/.../RoundTripDoxygenIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/doxygen/`):
- `javadoc-style.h`, `lists.h`, `python.py`, `qt-style.h`, `sample.h`, `special_commands.h`

**Known failing files**: On Windows (CRLF): `javadoc-style.h`, `sample.h`, `python.h`, `python.py`, `special_commands.h`

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `DoxygenXliffCompareIT` | `integration-tests/okapi/src/test/java/.../DoxygenXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/doxygen/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `javadoc-style.h` | `DoxygenFilterTest#testDoubleExtractionJavadocStyle` | Javadoc-style comments |
| `lists.h` | `DoxygenFilterTest#testDoubleExtractionLists` | Doxygen lists |
| `python.py` | `DoxygenFilterTest` | Python docstrings |
| `qt-style.h` | `DoxygenFilterTest#testDoubleExtractionQtStyle` | Qt-style comments |
| `sample.h` | `DoxygenFilterTest#testDoubleExtractionSample` | Mixed sample |
| `special_commands.h` | `DoxygenFilterTest#testDoubleExtractionSpecialCommands` | Special commands |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/doxygen/`

Same files as unit test resources.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/doxygen/src/test/resources/*.h okapi-testdata/okf_doxygen/
cp okapi/filters/doxygen/src/test/resources/*.py okapi-testdata/okf_doxygen/

# Integration test resources
cp integration-tests/okapi/src/test/resources/doxygen/* okapi-testdata/okf_doxygen/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/doxygen`

Build tag: `//go:build integration`

#### doxygen_test.go - Extraction Tests

```go
func TestExtract_comments(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple line comment", javaRef: "DoxygenFilterTest#testSimpleLine"},
        {name: "multiple line comment", javaRef: "DoxygenFilterTest#testMultipleLines"},
        {name: "javadoc-style comment", javaRef: "DoxygenFilterTest#testJavadocLine"},
        {name: "class command", javaRef: "DoxygenFilterTest#testDoxygenClassCommand1"},
        {name: "code command excluded", javaRef: "DoxygenFilterTest#testDoxygenCodeCommand"},
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/doxygen/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/doxygen/
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
  - Extracts translatable text from Doxygen/Javadoc comments in source code
  - Supports `///`, `//!`, `/** */`, `/*! */` comment styles
  - Commands like @code/@endcode, @verbatim/@endverbatim are excluded
  - Inline commands (@e, @a, @b) become inline codes
  - Image commands handled specially
  - CRLF line ending issues on Windows for some files
  - Python docstrings also supported

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/doxygen/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `DoxygenFilterTest.java` | `okapi/filters/doxygen/src/test/java/net/sf/okapi/filters/doxygen/` | 28 |
| `DoxygenWriterTest.java` | `okapi/filters/doxygen/src/test/java/net/sf/okapi/filters/doxygen/` | - |
