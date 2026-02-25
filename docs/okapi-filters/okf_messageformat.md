# okf_messageformat - MessageFormat Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_messageformat` |
| Java Class | `net.sf.okapi.filters.messageformat.MessageFormatFilter` |
| MIME Types | `text/x-messageformat` |
| Extensions | (none - used as sub-filter) |
| Okapi Module | `messageformat` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/messageformat/src/test/java/`

#### MessageFormatFilterTest.java (33 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Default filter metadata | P3 |
| 2 | `testStartDocument` | Start document event | P3 |
| 3 | `testLineBreaks_CR` | CR line break handling | P1 |
| 4 | `testineBreaks_CRLF` | CRLF line break handling | P1 |
| 5 | `testLineBreaks_LF` | LF line break handling | P1 |
| 6 | `testEntry` | Basic message format entry | P1 |
| 7 | `testCode1` | Placeholder code pattern {0} | P1 |
| 8 | `testCode2` | Named placeholder {name} | P1 |
| 9 | `testCode3` | Nested placeholder patterns | P1 |
| 10 | `testCode4` | Complex placeholder patterns | P1 |
| 11 | `testCode5` | Additional code patterns | P1 |
| 12 | `testCode6` | Extended code pattern handling | P1 |
| 13 | `testMultipleEmbedded` | Multiple embedded select/plural | P1 |
| 14 | `testMany1` | Many text units extraction | P1 |
| 15 | `testGenderNames` | Gender-based select names | P1 |
| 16 | `testPluralNames` | Plural form names | P1 |
| 17 | `testPluralNames2` | Plural form names variant 2 | P1 |
| 18 | `testPluralNames3` | Plural form names variant 3 | P1 |
| 19 | `testEmbeddedPluralNames` | Embedded plural within select | P1 |
| 20 | `testInvalid` | Invalid message format (expects exception) | P2 |
| 21 | `testLiterals` | Literal text extraction | P1 |
| 22 | `testOneQuote` | Single quote handling | P1 |
| 23 | `testQuotedQuote` | Quoted quote ('') handling | P1 |
| 24 | `testDeepQuotes` | Deeply nested quote handling | P2 |
| 25 | `testMessageFormatSubfilterJson` | MessageFormat as sub-filter for JSON | P1 |
| 26 | `testMessageFormatSubfilterYaml` | MessageFormat as sub-filter for YAML | P1 |
| 27 | `testDeepEmbeddedSubfilterYaml` | Deep embedded sub-filter in YAML | P2 |
| 28 | `testDeepEmbeddedSubfilterJson` | Deep embedded sub-filter in JSON | P2 |
| 29 | `testOffset` | Plural offset handling | P2 |
| 30 | `testSelectOrdinal` | selectordinal form handling | P1 |
| 31 | `testInlineCodes` | Inline codes within message format | P1 |

#### MessageFormatNormalizerTest.java, MessageFormatParserTest.java, MessageFormatPluralTest.java, MessageFormatToFormattedTest.java, PluralRulesDiffTest.java

Internal parser and normalizer tests.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripJSONMessageFormatIT` | `integration-tests/okapi/src/test/java/.../RoundTripJSONMessageFormatIT.java` | 3 |

Note: Tests MessageFormat as sub-filter within JSON filter using `okf_json@messageformat_expand` config.

**Test files used** (from `integration-tests/okapi/src/test/resources/messageformat/`):
- `JSON/` directory with expand/collapse test files
- `YAML/` directory with YAML variants

**Known failing files**: None

## Test Data Files

### Unit test resources

Source: `okapi/filters/messageformat/src/test/resources/`

No dedicated test resource files (tests use inline snippets).

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/messageformat/`

| File | Type | Purpose |
|------|------|---------|
| `JSON/` | roundtrip | JSON with MessageFormat strings |
| `YAML/` | roundtrip | YAML with MessageFormat strings |

## Test Data Collection

```bash
# Integration test resources
cp -r integration-tests/okapi/src/test/resources/messageformat/ okapi-testdata/okf_messageformat/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_messageformat`

Build tag: `//go:build integration`

#### messageformat_test.go - Extraction Tests

```go
func TestExtract_messageFormat(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "basic entry", javaRef: "MessageFormatFilterTest#testEntry"},
        {name: "placeholder codes", javaRef: "MessageFormatFilterTest#testCode1"},
        {name: "plural forms", javaRef: "MessageFormatFilterTest#testPluralNames"},
        {name: "gender select", javaRef: "MessageFormatFilterTest#testGenderNames"},
        {name: "embedded plural", javaRef: "MessageFormatFilterTest#testEmbeddedPluralNames"},
        {name: "selectordinal", javaRef: "MessageFormatFilterTest#testSelectOrdinal"},
        {name: "quoted text", javaRef: "MessageFormatFilterTest#testQuotedQuote"},
    }
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_messageformat/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_messageformat/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All sub-filter integration tests pass
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Filter-specific quirks:
  - Primarily used as a sub-filter for JSON/YAML filters, not standalone
  - ICU MessageFormat syntax: {name, type, style}
  - Plural forms: {count, plural, one{...} other{...}}
  - Select forms: {gender, select, male{...} female{...} other{...}}
  - selectordinal for ordinal numbers
  - Single quotes are escape characters in MessageFormat
  - Offset handling for plural forms
  - Each plural/select branch becomes a separate text unit
  - No file extensions - activated via sub-filter configuration

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/messageformat/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `MessageFormatFilterTest.java` | `okapi/filters/messageformat/src/test/java/.../` | 33 |
| `MessageFormatNormalizerTest.java` | `okapi/filters/messageformat/src/test/java/.../` | - |
| `MessageFormatParserTest.java` | `okapi/filters/messageformat/src/test/java/.../` | - |
| `MessageFormatPluralTest.java` | `okapi/filters/messageformat/src/test/java/.../` | - |
| `MessageFormatToFormattedTest.java` | `okapi/filters/messageformat/src/test/java/.../` | - |
| `PluralRulesDiffTest.java` | `okapi/filters/messageformat/src/test/java/.../` | - |
