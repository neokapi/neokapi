# okf_vtt - WebVTT Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_vtt` |
| Java Class | `net.sf.okapi.filters.vtt.VTTFilter` |
| MIME Types | `text/vtt` |
| Extensions | `.vtt, .srt` |
| Okapi Module | `subtitles` |
| Has Native Go Reader | No |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/subtitles/src/test/java/`

#### VTTFilterTest.java (7 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testSimple` | Simple WebVTT cue extraction with timestamps | P1 |
| 2 | `testMergeCaptions` | Merge adjacent captions into single text unit | P1 |
| 3 | `testMergeCaptionsWithChapters` | Caption merging with chapter headers | P1 |
| 4 | `testMergeCaptionsWithCueSettings` | Caption merging with position/alignment settings | P1 |
| 5 | `testQuotePunctuation` | Punctuation and quote handling in captions | P1 |
| 6 | `testVoiceSpans` | `<v Speaker>` voice spans in captions | P1 |
| 7 | `testEmptyCaption` | Empty caption handling | P2 |

#### VTTSkeletonWriterTest.java

Writer tests for VTT output.

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripVttIT` | `integration-tests/okapi/src/test/java/.../RoundTripVttIT.java` | 2 |

**Test files used** (from `integration-tests/okapi/src/test/resources/vtt/`):
- `example1.vtt`, `example2.vtt`

**Known failing files**: None

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `VttXliffCompareIT` | `integration-tests/okapi/src/test/java/.../VttXliffCompareIT.java` | 1 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/subtitles/src/test/resources/`

No dedicated VTT test files in unit test resources directory (tests use inline snippets).

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/vtt/`

| File | Type | Purpose |
|------|------|---------|
| `example1.vtt` | roundtrip | Standard WebVTT file |
| `example2.vtt` | roundtrip | WebVTT variant |

### Synthetic test data to create

| File | Purpose | Notes |
|------|---------|-------|
| `minimal.vtt` | Minimal valid WebVTT | `WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHello world` |

## Test Data Collection

```bash
# Integration test resources
cp integration-tests/okapi/src/test/resources/vtt/*.vtt okapi-testdata/okf_vtt/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/vtt`

Build tag: `//go:build integration`

#### vtt_test.go - Extraction Tests

```go
func TestExtract_captions(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        wantTexts  []string
        javaRef    string
    }{
        {name: "simple cue extraction", javaRef: "VTTFilterTest#testSimple"},
        {name: "merge captions", javaRef: "VTTFilterTest#testMergeCaptions"},
        {name: "voice spans", javaRef: "VTTFilterTest#testVoiceSpans"},
        {name: "quote punctuation", javaRef: "VTTFilterTest#testQuotePunctuation"},
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/vtt/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/vtt/
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
  - WebVTT subtitle format: timestamps + cue text
  - Caption merging feature: combines multi-line captions into single TUs
  - Voice spans (`<v Name>text</v>`) preserved as inline codes
  - Also handles SRT format (same module)
  - Cue settings (position, alignment) preserved in skeleton
  - Chapter headers handled during merge

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/subtitles/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `VTTFilterTest.java` | `okapi/filters/subtitles/src/test/java/net/sf/okapi/filters/vtt/` | 7 |
| `VTTSkeletonWriterTest.java` | `okapi/filters/subtitles/src/test/java/net/sf/okapi/filters/vtt/` | - |
