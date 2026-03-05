# okf_yaml - YAML Filter

## Filter Metadata

| Field | Value |
|-------|-------|
| Filter ID | `okf_yaml` |
| Java Class | `net.sf.okapi.filters.yaml.YamlFilter` |
| MIME Types | `text/x-yaml` |
| Extensions | `.yaml, .yml` |
| Okapi Module | `yaml` |
| Has Native Go Reader | Yes |

## Java Test Inventory

### Unit Tests

**Module**: `okapi/filters/yaml/src/test/java/`

#### YamlFilterTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1 | `testInlineCodeFinderNewLineCharacterDoubleQuotedString` | Code finder detects \\n in double-quoted YAML strings | P2 |
| 2 | `testInlineCodeFinderNewLineCharacterSingleQuotedString` | Code finder with \\n in single-quoted YAML strings | P2 |
| 3 | `testInlineCodeFinderNewLineCharacterStringWithoutQuotes` | Code finder with \\n in unquoted YAML strings | P2 |
| 4 | `testInlineCodeFinderWithQuoteInCode` | Code finder handling quotes within inline codes | P2 |

#### YmlFilterTest.java (31 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-31 | YAML filter tests | Key-value extraction, nested structures, arrays/sequences, multi-line strings (literal block `|`, folded block `>`), anchors/aliases, flow style, compact notation, recursive structures, HTML subfilter, line continuation, supplemental Unicode, double extraction roundtrip, encoding handling | P1-P2 |

#### parser/YamlParserTest.java (4 @Test methods)

| # | Test Method | What it Tests | Priority |
|---|-------------|---------------|----------|
| 1-4 | Parser tests | YAML parser tokenization, event generation, error handling | P3 |

### Integration Tests

#### RoundTrip IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripYamlIT` | `integration-tests/okapi/src/test/java/.../RoundTripYamlIT.java` | 2 |

**Test files used**: 158 files in `integration-tests/okapi/src/test/resources/yaml/`

**Known failing files**: None known in roundtrip

#### XLIFF Compare IT

| Class | File | Test Count |
|-------|------|------------|
| `YamlXliffCompareIT` | `integration-tests/okapi/src/test/java/.../YamlXliffCompareIT.java` | 1 |

#### Simplifier IT

| Class | File | Test Count |
|-------|------|------------|
| `RoundTripSimplifyYamlIT` | `integration-tests/okapi/src/test/java/.../RoundTripSimplifyYamlIT.java` | 2 |

## Test Data Files

### Unit test resources

Source: `okapi/filters/yaml/src/test/resources/yaml/`

176 files including:
- Basic YAML files (.yaml, .yml)
- Compact notation examples (12 examples, 9 error cases)
- Template/Velocity integration test files
- Recursive structure test files
- Flow style, line continuation, supplemental Unicode files
- HTML subfilter config (.fprm)

### Integration test resources

Source: `integration-tests/okapi/src/test/resources/yaml/`

158 files including compact notation examples, recursive structures, various YAML formats.

## Test Data Collection

```bash
# Unit test resources
cp -r okapi/filters/yaml/src/test/resources/yaml/* okapi-testdata/okf_yaml/

# Integration test resources
cp -r integration-tests/okapi/src/test/resources/yaml/* okapi-testdata/okf_yaml/roundtrip/
```

## Go Test Migration Plan

### Package: `core/plugin/bridge/filters/yaml`

Build tag: `//go:build integration`

#### yaml_test.go - Extraction Tests

```go
func TestExtract_BasicKeyValue(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        wantTexts  []string
        wantNames  []string
        params   map[string]any
        javaRef  string
    }{
        // ... from YmlFilterTest
    }
}

func TestExtract_MultiLineStrings(t *testing.T) {
    // Maps to YmlFilterTest: literal block |, folded block >
}

func TestExtract_NestedStructures(t *testing.T) {
    // Maps to YmlFilterTest: nested mappings, sequences
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
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration ./core/plugin/bridge/filters/yaml/ -v
GOKAPI_BRIDGE_JAR=/path/to/jar go test -tags=integration -race ./core/plugin/bridge/filters/yaml/
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
  - YAML 1.2 compliant parsing
  - Supports literal block (`|`), folded block (`>`), and flow styles
  - Key paths used as TU names (e.g., parent.child.key)
  - Anchors (&) and aliases (*) resolved during parsing
  - Compact notation for Rails i18n style files
  - HTML subfilter can be applied to values containing HTML
  - Inline code finder detects patterns like \\n within strings
  - Quoting style (single, double, unquoted) preserved on output
  - Recursive/circular reference handling
  - Boolean resolution: yes/no treated as strings (not booleans) via custom SnakeYAML resolver

## Current Go Coverage

### Bridge Tests (`core/plugin/bridge/filters/yaml/`)

| Java Method | Go Test | Status |
|-------------|---------|--------|
| `RoundTripYamlIT` | `TestRoundTrip` | Mapped |

**Coverage**: ~1 of 39 Surefire methods have bridge `// okapi:` annotations (~3%). YAML bridge tests exist but most lack `// okapi:` annotations.

## Java Source References

For change tracking against Okapi baseline commit `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47`:

```bash
git log --since="2026-02-24" --name-only -- "okapi/filters/yaml/src/test/"
```

| Java File | Path | @Test Count |
|-----------|------|-------------|
| `YamlFilterTest.java` | `okapi/filters/yaml/src/test/java/.../` | 4 |
| `YmlFilterTest.java` | `okapi/filters/yaml/src/test/java/.../` | 31 |
| `YamlParserTest.java` | `okapi/filters/yaml/src/test/java/.../parser/` | 4 |
