# okf_xinirainbowkit - XINI RainbowKit Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_xinirainbowkit` |
| Java Class | `net.sf.okapi.filters.xini.rainbowkit.XINIRainbowkitFilter` |
| MIME Types | `text/x-xini` |
| Extensions | - |
| Okapi Module | `xini` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/xini/src/test/java/`

Note: Tests for okf_xinirainbowkit are in the `xini` module under the `rainbowkit` subpackage.

#### XINIRainbowKitReaderTest.java (3 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `textSplitTagCodeNumbering` | Reads default XINI content file and verifies code numbering across split text fragments with opening/closing/placeholder tags | P1 |
| 2 | `textSplitTagCodeNumberingDescending` | Reads XINI with descending placeholder IDs and verifies code numbering with nested opening/closing tags | P2 |
| 3 | `textSplitTagCodeNumberingAscending` | Reads XINI with ascending placeholder IDs and verifies code numbering with nested opening/closing tags | P2 |

#### XINIRainbowkitWriterTest.java (2 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `writerUnderTestSavesGroupProperties` | Verifies START_GROUP events push group to stack via handleEvent | P2 |
| 2 | `writerUnderTestDeletesGroupValueWhenHandlingEndGroupEvent` | Verifies END_GROUP events pop group from stack | P2 |

#### FilterEventsToXiniTransformerTest.java (8 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `exportsPreTranslations` | Transforms a TextUnit with source + two targets (de, en) and verifies all 3 segments/translations appear in XINI output | P1 |
| 2 | `exportsNonBreakingSpaceAsEmptyTranslation` | NBSP-only source content is flagged as empty translation in XINI output | P2 |
| 3 | `xiniFieldStoresFieldLabelFromTuProperty` | Field label set via TU property is preserved in XINI field element | P2 |
| 4 | `xiniFieldIsNullIfTuHasNoProperty` | Field label is null when TU has no FIELD_LABEL_PROPERTY | P2 |
| 5 | `xiniFieldStoresFieldLabelFromStartGroupProperty` | Field label from StartGroup property propagates to XINI field | P2 |
| 6 | `labelFromOuterStartGroupIsOveriddenByInnerStartGroup` | Nested StartGroup labels: inner overrides outer | P2 |
| 7 | `labelFromOuterStartGroupIsUsedAfterEndingInnerGroup` | After popping inner group, outer label is restored | P2 |
| 8 | `labelFromStartGroupGetsResetByEndGroup` | After popping the only group, label resets to null | P2 |

### Integration Tests

#### RoundTrip IT

Note: The RoundTrip IT is for the base `okf_xini` filter, not `okf_xinirainbowkit` specifically.

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripXiniIT` | `integration-tests/okapi/src/test/java/.../RoundTripXiniIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/xini/`):
- `contents.xini`
- `ascendingPhs.xini`
- `descendingPhs.xini`
- `defaultSegmentation.srx`

**Known failing files**: None

## Test Data Files

### Unit test resources

Source: `okapi/filters/xini/src/test/resources/`

| File | Used By | Purpose |
|------|---------|---------|
| `contents.xini` | `XINIRainbowKitReaderTest#textSplitTagCodeNumbering` | Default XINI content with text units and placeholders |
| `ascendingPhs.xini` | `XINIRainbowKitReaderTest#textSplitTagCodeNumberingAscending` | XINI with ascending placeholder IDs |
| `descendingPhs.xini` | `XINIRainbowKitReaderTest#textSplitTagCodeNumberingDescending` | XINI with descending placeholder IDs |

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/xini/`

| File | Type | Purpose |
|------|------|---------|
| `contents.xini` | roundtrip | Default XINI content roundtrip |
| `ascendingPhs.xini` | roundtrip | Ascending placeholders roundtrip |
| `descendingPhs.xini` | roundtrip | Descending placeholders roundtrip |
| `defaultSegmentation.srx` | config | Segmentation rules |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.xini` | Minimal valid XINI document for smoke test | Single page, single element, single field with one segment |

## Test Data Collection

Files to include in the `okapi-testdata` GitHub release:

```bash
# Unit test resources
cp okapi/filters/xini/src/test/resources/contents.xini okapi-testdata/okf_xinirainbowkit/
cp okapi/filters/xini/src/test/resources/ascendingPhs.xini okapi-testdata/okf_xinirainbowkit/
cp okapi/filters/xini/src/test/resources/descendingPhs.xini okapi-testdata/okf_xinirainbowkit/

# Integration test resources
cp integration-tests/okapi/src/test/resources/xini/contents.xini okapi-testdata/okf_xinirainbowkit/roundtrip/
cp integration-tests/okapi/src/test/resources/xini/ascendingPhs.xini okapi-testdata/okf_xinirainbowkit/roundtrip/
cp integration-tests/okapi/src/test/resources/xini/descendingPhs.xini okapi-testdata/okf_xinirainbowkit/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/xinirainbowkit`

Build tag: `//go:build integration`

#### xinirainbowkit_test.go - Extraction Tests

```go
func TestExtract_basicContent(t *testing.T) {
    // Table-driven: maps 1:1 to Java XINIRainbowKitReaderTest
    tests := []struct {
        name     string
        input    string // testdata path
        wantBlocks int
        wantTexts  []string
        params   map[string]any
        javaRef  string
    }{
        {
            name:    "default content with code numbering",
            input:   "testdata/contents.xini",
            wantTexts: []string{"Test!"},
            javaRef: "XINIRainbowKitReaderTest#textSplitTagCodeNumbering",
        },
        {
            name:    "ascending placeholder IDs",
            input:   "testdata/ascendingPhs.xini",
            wantTexts: []string{"Test!"},
            javaRef: "XINIRainbowKitReaderTest#textSplitTagCodeNumberingAscending",
        },
        {
            name:    "descending placeholder IDs",
            input:   "testdata/descendingPhs.xini",
            wantTexts: []string{"Test!"},
            javaRef: "XINIRainbowKitReaderTest#textSplitTagCodeNumberingDescending",
        },
    }
}
```

#### roundtrip_test.go - Roundtrip Tests

```go
func TestRoundTrip(t *testing.T) {
    // Maps to Java RoundTripXiniIT (covers xini format generally)
    testFiles := []string{
        "contents.xini",
        "ascendingPhs.xini",
        "descendingPhs.xini",
    }
    knownFailing := map[string]string{
        // none known
    }
    bridgetest.RoundTripTestFiles(t, b, filterClass, mime, "testdata/roundtrip", exts, knownFailing, nil)
}
```

## Verification

```bash
# Run this filter's tests
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/xinirainbowkit/ -v

# Run with race detector
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/xinirainbowkit/
```

### Success criteria

- [ ] All extraction tests pass (inline strings)
- [ ] All full-file extraction tests pass
- [ ] All configuration/parameter tests pass
- [ ] Roundtrip tests pass for all non-failing files
- [ ] No race conditions detected

## Notes for Agent

- Build tag: `//go:build integration`
- Required env: `GOKAPI_BRIDGE_JAR`
- Helper package: `bridgetest`
- Test data fetched via `scripts/fetch-okapi-testdata.sh` to `./okapi-testdata/`
- Filter-specific quirks:
  - This is the RainbowKit variant of the XINI filter, used for ONTRAM T-Kit round-tripping
  - Schema has no configurable parameters (empty properties object)
  - Writer tests use PowerMockito/Mockito (mock-based, not directly portable)
  - The FilterEventsToXiniTransformer tests are internal transformation tests; focus on reader/roundtrip tests for bridge migration

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/xini/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `XINIRainbowKitReaderTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 3 |
| `XINIRainbowkitWriterTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 2 |
| `FilterEventsToXiniTransformerTest.java` | `okapi/filters/xini/src/test/java/.../rainbowkit/` | 8 |
