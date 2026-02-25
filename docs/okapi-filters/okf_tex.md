# okf_tex - TeX/LaTeX Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_tex` |
| Java Class | `net.sf.okapi.filters.tex.TEXFilter` |
| MIME Types | `text/x-tex-text` |
| Extensions | `.tex` |
| Okapi Module | `tex` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/tex/src/test/java/`

#### TEXFilterTest.java (28 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testJava8Split` | Java 8 string split compatibility | P3 |
| 3 | `testStartDocument` | Start document event | P3 |
| 4 | `testMathMode` | Math mode content excluded from extraction | P1 |
| 5 | `testComments` | TeX comment handling (% character) | P1 |
| 6 | `testRussian` | Russian/Cyrillic text extraction | P1 |
| 7 | `testSplitTUonNewlines` | Text unit splitting on newlines | P1 |
| 8 | `testSplitTUonNewlines2` | Text unit splitting on double newlines | P1 |
| 9 | `testSimpleText` | Simple paragraph text extraction | P1 |
| 10 | `testRunawayCurly` | Runaway curly brace handling | P2 |
| 11 | `testOneArgNoTextCommands` | One-arg commands that are not text (\label, \ref, etc.) | P1 |
| 12 | `testOneArgInlineTextCommands` | One-arg inline text commands (\textbf, \emph, etc.) | P1 |
| 13 | `testoneArgParaTextCommands` | One-arg paragraph-level commands (\title, \section, etc.) | P1 |
| 14 | `testHeaderCommands` | Header commands (\documentclass, \usepackage) | P1 |
| 15 | `testHeaderText` | Text in header area | P1 |
| 16 | `testLatvianSymbols` | Latvian special symbols handling | P1 |
| 17 | `testLatvianSymbolsEscaping` | Latvian symbol escaping in output | P1 |
| 18 | `testTable` | Table environment extraction | P1 |
| 19 | `testTable2` | Complex table extraction | P1 |
| 20 | `testEquation` | Equation environment excluded | P1 |
| 21 | `testHierarchy` | Nested command hierarchy | P2 |
| 22 | `testDemoFile` | Full demo.tex file extraction | P1 |
| 23 | `testDemoFileWin` | Demo file with Windows line endings | P1 |
| 24 | `testDemoFile2` | Second demo file | P1 |
| 25 | `testNested` | Nested commands | P2 |
| 26 | `testScript` | Script-like content | P2 |
| 27 | `testLineBreaks` | Line break handling | P1 |

#### TEXWriterTest.java

Writer tests for TeX output.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripTexIT` | `integration-tests/okapi/src/test/java/.../RoundTripTexIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/tex/`):
- `sample.tex`, `sample1.tex`, `Test01.tex`, `Test02.tex`, `Test03.tex`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `TexXliffCompareIT` | `integration-tests/okapi/src/test/java/.../TexXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/tex/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.tex` | `TEXFilterTest#testDemoFile` | Main demo TeX file |
| `Test02.tex` | `TEXFilterTest#testDemoFile2` | Second TeX test |
| `Test03.tex` | `TEXFilterTest` | Third TeX test |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/tex/`

| File | Type | Purpose |
|------|------|---------|
| `sample.tex` | roundtrip | Sample TeX roundtrip |
| `sample1.tex` | roundtrip | Sample variant |
| `Test01.tex` through `Test03.tex` | roundtrip | TeX roundtrip tests |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/tex/src/test/resources/*.tex okapi-testdata/okf_tex/

# Integration test resources
cp integration-tests/okapi/src/test/resources/tex/*.tex okapi-testdata/okf_tex/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_tex`

Build tag: `//go:build integration`

#### tex_test.go - Extraction Tests

```go
func TestExtract_texContent(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple text", javaRef: "TEXFilterTest#testSimpleText"},
        {name: "math mode excluded", javaRef: "TEXFilterTest#testMathMode"},
        {name: "comments excluded", javaRef: "TEXFilterTest#testComments"},
        {name: "inline text commands", javaRef: "TEXFilterTest#testOneArgInlineTextCommands"},
        {name: "paragraph commands", javaRef: "TEXFilterTest#testoneArgParaTextCommands"},
        {name: "table environment", javaRef: "TEXFilterTest#testTable"},
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_tex/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_tex/
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
  - Math mode ($...$, \[...\], equation environment) excluded from extraction
  - Comments (% to end of line) excluded
  - Commands classified as: no-text (\label, \ref), inline-text (\textbf, \emph), paragraph-text (\title, \section)
  - Header area (\documentclass to \begin{document}) handled specially
  - Double newlines split text units
  - Special character handling for non-ASCII (Latvian, Cyrillic)
  - Table and equation environments handled as blocks

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/tex/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `TEXFilterTest.java` | `okapi/filters/tex/src/test/java/net/sf/okapi/filters/tex/` | 28 |
| `TEXWriterTest.java` | `okapi/filters/tex/src/test/java/net/sf/okapi/filters/tex/` | - |
