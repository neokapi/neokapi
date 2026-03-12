# okf_pdf - PDF Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_pdf` |
| Java Class | `net.sf.okapi.filters.pdf.PdfFilter` |
| MIME Types | `application/pdf` |
| Extensions | `.pdf` |
| Okapi Module | `pdf` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/pdf/src/test/java/`

#### PdfFilterTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testDefaultInfo` | Non-null parameters, name, non-empty configurations | P2 |
| 2 | `testStartDocument` | StartDocument event from OmegaT_documentation_en.PDF | P2 |
| 3 | `firstTextUnit` | First text unit extraction from 3 PDFs: PALC_2011_LT.pdf ("Translation Quality Checking in LanguageTool"), OmegaT_documentation_en.PDF ("OmegaT 3.1 - User's Guide Vito Smolej"), TAUS-QualityDashboard-September.pdf ("TAUS Quality Dashboard") | P1 |
| 4 | `firstParagraphTextUnit` | Paragraph-level extraction with line/paragraph separators, verifies abstract text starts correctly in multiple PDFs | P1 |

### Integration Tests

None found (PDF is read-only, no roundtrip support).

## Test Data Files

### Unit test resources

Source: `okapi/filters/pdf/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `OmegaT_documentation_en.PDF` | `testStartDocument`, `firstTextUnit`, `firstParagraphTextUnit` | Multi-page PDF documentation |
| `PALC_2011_LT.pdf` | `firstTextUnit`, `firstParagraphTextUnit` | Academic paper PDF |
| `TAUS-QualityDashboard-September.pdf` | `firstTextUnit`, `firstParagraphTextUnit` | Report PDF |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.pdf` | Minimal single-page PDF for smoke test | Simple text content |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/pdf/src/test/resources/net/sf/okapi/filters/pdf/*.pdf okapi-testdata/okf_pdf/
cp okapi/filters/pdf/src/test/resources/net/sf/okapi/filters/pdf/*.PDF okapi-testdata/okf_pdf/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/pdf`

Build tag: `//go:build integration`

Note: PDF is **read-only** in Okapi. There is no roundtrip support. Only extraction tests apply.

#### pdf_test.go - Extraction Tests

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
            name:  "palc_first_text",
            input: "PALC_2011_LT.pdf",
            params: map[string]any{"lineSeparator": "", "paragraphSeparator": "\n\n"},
            wantTexts: []string{"Translation Quality Checking in LanguageTool"},
            javaRef: "PdfFilterTest#firstTextUnit",
        },
        {
            name:  "omegat_first_text",
            input: "OmegaT_documentation_en.PDF",
            wantTexts: []string{"OmegaT 3.1 - User's Guide Vito Smolej"},
            javaRef: "PdfFilterTest#firstTextUnit",
        },
    }
}
```

#### config_test.go - Configuration Tests

```go
func TestConfig_Separators(t *testing.T) {
    tests := []struct {
        name   string
        params map[string]any
        input  string
        want   []string
        javaRef string
    }{
        {
            name:   "paragraph_separator",
            params: map[string]any{"lineSeparator": "\n", "paragraphSeparator": "\n"},
            input:  "PALC_2011_LT.pdf",
            javaRef: "PdfFilterTest#firstParagraphTextUnit",
        },
    }
}
```

## Verification

```bash
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/pdf/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/pdf/
```

### Success criteria

- [ ] All extraction tests pass
- [ ] Configuration parameter tests pass (lineSeparator, paragraphSeparator)
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - **PDF is READ-ONLY** - no roundtrip support, no filter writer
  - Only extraction (read) tests apply - no write/roundtrip tests
  - Parameters: lineSeparator (default: empty), paragraphSeparator (default: empty)
  - Different separator settings change how text units are split
  - Only 4 @Test methods - limited coverage
  - PDF text extraction is inherently imprecise (no semantic structure)
  - Test PDFs are real documents (OmegaT docs, academic paper, TAUS report)

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/pdf/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `PdfFilterTest.java` | `okapi/filters/pdf/src/test/java/.../` | 4 |
