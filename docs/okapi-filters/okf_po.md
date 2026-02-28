# okf_po - PO/Gettext Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_po` |
| Java Class | `net.sf.okapi.filters.po.POFilter` |
| MIME Types | `application/x-gettext` |
| Extensions | `.po` |
| Okapi Module | `po` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/po/src/test/java/`

#### POFilterTest.java (43 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testPOTHeader` | POT file header parsing (Content-Type, PO-Revision-Date, Plural-Forms) | P1 |
| 2 | `testPOHeader` | PO file header parsing | P1 |
| 3 | `testHeaderNoNPlurals` | Header without nplurals directive | P2 |
| 4 | `testHeaderWithEmptyEntryAfter` | Header followed by empty entry | P2 |
| 5 | `testDefaultInfo` | Filter has name, display name, configurations | P3 |
| 6 | `testPluralFormAccess` | Plural form accessor methods | P1 |
| 7 | `testPluralFormDefaults` | Default plural form values | P2 |
| 8 | `testSourceOnly` | Source-only extraction (ignored, Issue #398) | P3 |
| 9 | `testStartDocument` | StartDocument event correct | P1 |
| 10 | `testOuputOptionLine_JustFormatWithMacLB` | Output with Mac line breaks | P2 |
| 11 | `testOuputOptionLine_FormatFuzzy` | Fuzzy flag in output | P1 |
| 12 | `testInlines` | Inline code detection (printf format: %s, %d, etc.) | P1 |
| 13 | `testEscapes` | PO escape sequences (\\n, \\t, \\", \\\\) | P1 |
| 14 | `testUnescapedRead` | Unescaped character reading | P1 |
| 15 | `testUnescapedRewrite` | Unescaped character roundtrip | P1 |
| 16 | `testIDWithContext` | msgid with msgctxt as compound ID | P1 |
| 17 | `testProtectApproved` | Protect approved (non-fuzzy) translations | P2 |
| 18 | `testOutputProtectApproved` | Protected approved output | P2 |
| 19 | `testHtmlSubfilterBilingualMode` | HTML subfilter in bilingual PO mode | P2 |
| 20 | `testWithNoCodesLookingLikeCodes` | Text that looks like codes but isn't | P2 |
| 21 | `testWithLetterCodes` | Letter-based format codes (%s, %d) | P1 |
| 22 | `testOuputOptionLine_FuzyFormat` | Fuzzy format line output | P2 |
| 23 | `testOuputWithAllowedEmpty` | Output with allowed empty translations | P2 |
| 24 | `testOuputOptionLine_StuffFuzyFormat` | Complex fuzzy format output | P2 |
| 25 | `testOuputSimpleEntry` | Simple msgid/msgstr output | P1 |
| 26 | `testOuputEntryWithCTXT` | Entry with msgctxt output | P1 |
| 27 | `testOuputAddTranslation` | Adding translation to empty msgstr | P1 |
| 28 | `testTUEmptyIDEntry` | Empty msgid entry handling | P2 |
| 29 | `testTUContextParsing` | msgctxt parsing into TU properties | P1 |
| 30 | `testNoQuoteOnSameLine` | Multi-line msgid without quote on same line | P2 |
| 31 | `testOuputNoQuoteOnSameLine` | Multi-line output format | P2 |
| 32 | `testTUCompleteEntry` | Complete entry: comments, msgctxt, msgid, msgstr with attributes | P1 |
| 33 | `testTUPluralEntry_DefaultGroup` | Plural entry as group | P1 |
| 34 | `testTUPluralEntry_DefaultSingular` | Singular form of plural entry | P1 |
| 35 | `testTUPluralEntry_DefaultPlural` | Plural form extraction | P1 |
| 36 | `testOuputPluralEntry` | Plural entry output format | P1 |
| 37 | `testTrailingSkeleton` | Trailing whitespace/comments after last entry | P2 |
| 38 | `testPluralEntryFuzzy` | Fuzzy plural entry handling | P2 |
| 39 | `testOuputPluralEntryFuzzy` | Fuzzy plural entry output | P2 |
| 40-43 | Additional PO tests | Various edge cases | P2 |

#### POWriterTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-8 | Writer tests | PO writer output format, encoding, escaping, bilingual mode | P2 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripPoIT` | `integration-tests/okapi/src/test/java/.../RoundTripPoIT.java` | 2 |

**Test files used**: 24 files in `integration-tests/okapi/src/test/resources/po/`

**Known failing files**: None known in roundtrip

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `PoXliffCompareIT` | `integration-tests/okapi/src/test/java/.../PoXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyPoIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyPoIT.java` | 2 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/po/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `Test01.po` | `testStartDocument`, `testPOHeader` | Basic PO file |
| `Test02.po` - `Test05.po` | various | Additional PO test files |
| `POT-Test01.pot` | `testPOTHeader` | POT template file |
| `AllCasesTest.po` | various | Comprehensive PO test cases |
| `escaping.po` | `testEscapes` | Escape sequence test |
| `fail.po` | error handling | Malformed PO file |
| `msgctxt_notes.po` | `testTUContextParsing` | Context and notes |
| `plurals.po` / `plurals-2.po` | plural tests | Plural form tests |
| `potest.po` | various | General PO test |
| `Test_DrupalRussianCP1251.po` | encoding tests | Cyrillic encoding |
| `Test_nautilus.af.po` | various | Afrikaans translation |
| `TestMonoLingual_EN.po` / `TestMonoLingual_FR.po` | monolingual tests | Monolingual mode |
| `okf_po@Monolingual.fprm` | config | Monolingual config |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/po/`

24 files including simple with/without plurals, with/without translations, fuzzy entries.

## Test Data Collection

```bash
# Unit test resources
cp okapi/filters/po/src/test/resources/*.po okapi-testdata/okf_po/
cp okapi/filters/po/src/test/resources/*.pot okapi-testdata/okf_po/
cp okapi/filters/po/src/test/resources/*.fprm okapi-testdata/okf_po/

# Integration test resources
cp integration-tests/okapi/src/test/resources/po/*.po okapi-testdata/okf_po/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/okf_po`

Build tag: `//go:build integration`

#### po_test.go - Extraction Tests

```go
func TestExtract_BasicEntry(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        wantNames  []string
        params   map[string]any
        javaRef  string
    }{
        // ... from POFilterTest
    }
}

func TestExtract_Plurals(t *testing.T) {
    // Maps to POFilterTest plural tests
}

func TestExtract_Escapes(t *testing.T) {
    // Maps to POFilterTest#testEscapes
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/okf_po/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/okf_po/
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
- Filter-specific quirks:
  - PO format: msgid (source), msgstr (target), msgctxt (context)
  - Bilingual format: both source and target in same file
  - Plural forms: msgid_plural + msgstr[0], msgstr[1], ... msgstr[N]
  - nplurals and Plural-Forms header control plural handling
  - Fuzzy flag (#, fuzzy) marks uncertain translations
  - Printf-style format codes (%s, %d, %f, etc.) detected as inline codes
  - #. translator-comments, #: references, #, flags, #| previous strings
  - Monolingual mode extracts only source (msgid)
  - protectApproved option protects non-fuzzy entries from modification
  - PO escape sequences: \\n (newline), \\t (tab), \\" (quote), \\\\ (backslash)

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/okf_po/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `POFilterTest#testOuputSimpleEntry` | `TestExtract_SimpleEntry` | Mapped |
| `POFilterTest#testIDWithContext` | `TestExtract_Context` | Mapped |
| `POFilterTest#testTUCompleteEntry` | `TestExtract_CompleteEntry` | Mapped |
| `POFilterTest#testNoQuoteOnSameLine` | `TestExtract_MultiLine` | Mapped |
| `POFilterTest#testTUPluralEntry_DefaultGroup` | `TestExtract_Plurals` | Mapped |
| `POFilterTest#testTUPluralEntry_DefaultPlural` | `TestExtract_Plurals` | Mapped |
| `POFilterTest#testEscapes` | `TestExtract_Escapes` | Mapped |
| `RoundTripPoIT` | `TestRoundTrip` | Mapped |

### Native Tests (`core/formats/po/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `POFilterTest#testOuputSimpleEntry` | `TestReadSimpleEntry` | Mapped |
| `POFilterTest#testPOHeader` | `TestReadHeader` | Mapped |
| `POFilterTest#testNoQuoteOnSameLine` | `TestReadMultiLine` | Mapped |
| `POFilterTest#testIDWithContext` | `TestReadContext` | Mapped |
| `POFilterTest#testTUPluralEntry_DefaultGroup` | `TestReadPlurals` | Mapped |
| `POFilterTest#testEscapes` | `TestReadEscapes` | Mapped |
| `POFilterTest#testOuputEntryWithCTXT` | `TestWriteContext` | Mapped |
| `POFilterTest#testOuputPluralEntry` | `TestWritePlurals` | Mapped |
| `POFilterTest#testOuputOptionLine_FormatFuzzy` | `TestWriteFuzzy` | Mapped |
| `POFilterTest#testUnescapedRead` | `TestReadUnescaped` | Mapped |
| `POFilterTest#testUnescapedRewrite` | `TestWriteUnescaped` | Mapped |

**Coverage**: ~7 of 51 Surefire methods have bridge `// okapi:` annotations (~14%).

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/po/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `POFilterTest.java` | `okapi/filters/po/src/test/java/.../` | 43 |
| `POWriterTest.java` | `okapi/filters/po/src/test/java/.../` | 8 |
