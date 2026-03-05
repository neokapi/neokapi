# okf_phpcontent - PHP Content Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_phpcontent` |
| Java Class | `net.sf.okapi.filters.php.PHPContentFilter` |
| MIME Types | `application/x-php` |
| Extensions | `.php` |
| Okapi Module | `php` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/php/src/test/java/`

#### PHPContentFilterTest.java (49 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testEntityReferences` | HTML entity references in PHP strings | P1 |
| 3 | `testReferencesLooklike` | Look-alike references that aren't entities | P2 |
| 4 | `testConcatSQStrings` | Concatenated single-quoted strings | P1 |
| 5 | `testConcatDQStringsWithCodesAndVariable` | Double-quoted strings with codes and $var | P1 |
| 6 | `testCommaCaseWithConcat` | Comma case with string concatenation | P2 |
| 7 | `testConcatWithVariable` | Concatenation with PHP variables | P1 |
| 8 | `testConcatMultipleStrings` | Multiple string concatenation | P1 |
| 9 | `testConcatWithEndings` | Concatenation with line endings | P2 |
| 10 | `testConcatSGAndDQStrings` | Mixed single/double-quoted concatenation | P1 |
| 11 | `testEntryWithCodes` | PHP strings with inline codes | P1 |
| 12 | `testSimpleHTMLCodes` | HTML tags as inline codes | P1 |
| 13 | `testParitalStartingHTMLCodes` | Partial starting HTML tags | P2 |
| 14 | `testParitalClosingHTMLCodes` | Partial closing HTML tags | P2 |
| 15 | `testSpecialHTMLCodes` | Special HTML elements | P2 |
| 16 | `testEscapeCodes` | PHP escape sequences (\n, \t, etc.) | P1 |
| 17 | `testLinefeedCodes` | Linefeed as inline code | P1 |
| 18 | `testOutputLinefeedCodes` | Linefeed codes in output | P1 |
| 19 | `testVariableCodes` | PHP variables as inline codes | P1 |
| 20 | `testCommentsSingleLine` | Single-line PHP comments | P1 |
| 21 | `testCommentsMultiline` | Multi-line PHP comments | P1 |
| 22 | `testEmptyComment` | Empty comment handling | P2 |
| 23 | `testCommentsWithApos` | Comments containing apostrophes | P2 |
| 24 | `testSkipDirective` | Skip directive for non-translatable strings | P1 |
| 25 | `testSkipDirectiveOnConcat` | Skip directive on concatenation | P2 |
| 26 | `testTextInBSkipDirective` | Text in begin-skip directive | P2 |
| 27 | `testESkipDirective` | End-skip directive | P2 |
| 28 | `testDirectiveInMultilineComment` | Directive inside multi-line comment | P2 |
| 29 | `testBTextDirective` | Begin-text directive | P2 |
| 30 | `testETextDirective` | End-text directive | P2 |
| 31 | `testSkipOutsideDirective` | Skip directive scope | P2 |
| 32 | `testDisabledDirectives` | Disabled directive handling | P2 |
| 33 | `testDirectiveScope` | Directive scoping rules | P2 |
| 34 | `testSingleQuotedString` | Single-quoted string extraction | P1 |
| 35 | `testDoubleQuotedString` | Double-quoted string extraction | P1 |
| 36 | `testHeredocString` | Heredoc string extraction (<<<LABEL) | P1 |
| 37 | `testQuotedHeredocString` | Quoted heredoc string | P1 |
| 38 | `testQuotedNowdocString` | Nowdoc string extraction (<<<'LABEL') | P1 |
| 39 | `testSemiColumnHeredocString` | Heredoc with semicolon ending | P2 |
| 40 | `testMultipleLinesHeredocString` | Multi-line heredoc | P1 |
| 41 | `testEmptyHeredocStringAndOutput` | Empty heredoc and output | P2 |
| 42 | `testWhiteHeredocStringAndOutput` | Whitespace-only heredoc | P2 |
| 43 | `testOutputSimple` | Simple output roundtrip | P1 |
| 44 | `testLineBreakType` | Line break type detection | P1 |
| 45 | `testOutputWithNoStrings` | Output with no translatable strings | P2 |
| 46 | `testOutputHeredoc` | Heredoc output roundtrip | P1 |
| 47 | `testOutputMix` | Mixed string types output | P1 |
| 48 | `testSQIndex` | Single-quoted array index | P1 |
| 49 | `testnoStringIndex` | Non-string array index | P2 |
| 50 | `testDQIndex` | Double-quoted array index | P1 |
| 51 | `testHeredocIndex` | Heredoc array index | P1 |
| 52 | `testQuotedHeredocIndex` | Quoted heredoc array index | P1 |
| 53 | `testNowdocIndex` | Nowdoc array index | P1 |
| 54 | `testOutputArrayKeys` | Array key output | P1 |
| 55 | `testFilteringOfHtmlLikeTags` | HTML-like tag filtering | P2 |
| 56 | `testDoubleExtraction` | Double extraction consistency | P1 |

### Integration Tests

No dedicated integration tests found for okf_phpcontent.

## Test Data Files

### Unit test resources

Source: `okapi/filters/php/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `test01.phpcnt` | `PHPContentFilterTest#testDoubleExtraction` | PHP content test file |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.php` | Minimal PHP for smoke test | `<?php $text = "Hello world";` |

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/php/src/test/resources/test01.phpcnt okapi-testdata/okf_phpcontent/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/phpcontent`

Build tag: `//go:build integration`

#### phpcontent_test.go - Extraction Tests

```go
func TestExtract_phpStrings(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "single-quoted string", javaRef: "PHPContentFilterTest#testSingleQuotedString"},
        {name: "double-quoted string", javaRef: "PHPContentFilterTest#testDoubleQuotedString"},
        {name: "heredoc string", javaRef: "PHPContentFilterTest#testHeredocString"},
        {name: "nowdoc string", javaRef: "PHPContentFilterTest#testQuotedNowdocString"},
        {name: "concatenated strings", javaRef: "PHPContentFilterTest#testConcatSQStrings"},
        {name: "PHP variable codes", javaRef: "PHPContentFilterTest#testVariableCodes"},
        {name: "HTML codes in strings", javaRef: "PHPContentFilterTest#testSimpleHTMLCodes"},
        {name: "skip directive", javaRef: "PHPContentFilterTest#testSkipDirective"},
    }
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/phpcontent/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/phpcontent/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - Extracts translatable strings from PHP source files
  - Supports single-quoted, double-quoted, heredoc (<<<), and nowdoc (<<<') strings
  - String concatenation (.) merges adjacent strings into one TU
  - PHP variables ($var) become inline codes in double-quoted strings
  - HTML tags become inline codes
  - Escape sequences (\n, \t, etc.) become inline codes
  - Skip/text directives for controlling extraction scope
  - Array keys extracted with proper indexing
  - No dedicated integration roundtrip tests

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/php/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `PHPContentFilterTest.java` | `okapi/filters/php/src/test/java/net/sf/okapi/filters/php/` | 49 |
