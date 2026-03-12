# Okapi Bridge Test Migration Plan

Migrate all Java Okapi filter tests into Go bridge integration tests so every filter is thoroughly exercised through the bridge.

## Summary

| Metric | Count |
|--------|-------|
| Filters | 57 |
| Java unit test files | 181 |
| Java unit @Test methods | 2,546 |
| Java integration test files | 132 |
| Java integration @Test methods | 261 |
| **Total Java test files** | **313** |
| **Total @Test methods** | **2,807** |

## Status Legend

- `[ ]` Not started
- `[~]` In progress
- `[x]` Complete
- `[-]` Skipped (no applicable tests)

## Test Data Strategy

Test data files and Surefire XML reports are stored in GitHub releases to avoid bloating the repository.

### Test Resources

- **Release**: `gh release` in gokapi/gokapi tagged `okapi-testdata-1.48.0`
- **Fetch script**: `scripts/fetch-okapi-testdata.sh` downloads and extracts to `./okapi-testdata/`
- **Publish script**: `scripts/publish-okapi-testdata.sh` builds from Okapi GitLab repo
- **Gitignored**: `okapi-testdata/` is in `.gitignore`
- **CI**: The fetch script runs before integration tests in CI
- **Structure**: `okapi-testdata/okf_{id}/` per filter, with `roundtrip/` subdirectory for integration test resources

### Surefire XML Reports

- **Release**: `gh release` in gokapi/gokapi tagged `okapi-surefire-1.48.0`
- **Fetch script**: `scripts/fetch-okapi-surefire.sh` downloads and extracts to `./okapi-surefire/`
- **Publish script**: `scripts/publish-okapi-surefire.sh` runs Maven tests on local Okapi checkout
- **Gitignored**: `okapi-surefire/` is in `.gitignore`
- **Structure**: `okapi-surefire/{filter}/TEST-*.xml` (flat per-filter layout)
- **Used by**: `make generate-test-comparison` to build the test comparison dashboard

---

## Phase 1 - Infrastructure + High-Value Text Formats (10 filters)

| Filter | Doc | Extraction | Config | Roundtrip | XLIFF Compare | Simplifier | Memory | Java Unit Tests | Java IT Tests |
|--------|-----|------------|--------|-----------|---------------|------------|--------|-----------------|---------------|
| [okf_html](okapi-filters/okf_html.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 10 files (177 methods) | RT, XC, Simp, Mem |
| [okf_json](okapi-filters/okf_json.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 2 files (51 methods) | RT, XC, Simp |
| [okf_xmlstream](okapi-filters/okf_xmlstream.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 12 files (136 methods) | RT, XC, Simp, Mem(xml) |
| [okf_xliff](okapi-filters/okf_xliff.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 10 files (246 methods) | RT, XC |
| [okf_xliff2](okapi-filters/okf_xliff2.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 3 files (31 methods) | RT, XC |
| [okf_properties](okapi-filters/okf_properties.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 1 file (30 methods) | RT, XC, Simp |
| [okf_po](okapi-filters/okf_po.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 2 files (51 methods) | RT, XC, Simp |
| [okf_yaml](okapi-filters/okf_yaml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 4 files (39 methods) | RT, XC, Simp, MsgFmt |
| [okf_plaintext](okapi-filters/okf_plaintext.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [ ] | 5 files (39 methods) | RT, XC, Mem |
| [okf_markdown](okapi-filters/okf_markdown.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 4 files (209 methods) | RT, XC |

## Phase 2 - Remaining Text Formats (19 filters)

| Filter | Doc | Extraction | Config | Roundtrip | XLIFF Compare | Simplifier | Memory | Java Unit Tests | Java IT Tests |
|--------|-----|------------|--------|-----------|---------------|------------|--------|-----------------|---------------|
| [okf_html5](okapi-filters/okf_html5.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 2 files (38 methods) | RT(its), XC(its), Simp(its) |
| abstractmarkup | [ ] | [ ] | [-] | [-] | [-] | [-] | [-] | (57 methods) | - |
| its | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | (188 methods) | RT, XC, Simp |
| subtitles | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | (43 methods) | RT, XC |
| [okf_xml](okapi-filters/okf_xml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 4 files (98 methods) | RT, XC, Simp, Mem |
| [okf_dtd](okapi-filters/okf_dtd.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 1 file (8 methods) | RT, XC |
| [okf_tmx](okapi-filters/okf_tmx.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (60 methods) | RT, XC |
| [okf_ts](okapi-filters/okf_ts.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 1 file (41 methods) | RT, XC |
| [okf_regex](okapi-filters/okf_regex.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 1 file (23 methods) | RT, XC, Simp, Mem |
| [okf_doxygen](okapi-filters/okf_doxygen.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (31 methods) | RT, XC |
| [okf_tex](okapi-filters/okf_tex.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (30 methods) | RT, XC |
| [okf_wiki](okapi-filters/okf_wiki.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (14 methods) | RT, XC |
| [okf_mosestext](okapi-filters/okf_mosestext.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 2 files (21 methods) | RT |
| [okf_vtt](okapi-filters/okf_vtt.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (28 methods) | RT, XC |
| [okf_ttml](okapi-filters/okf_ttml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 2 files (15 methods) | RT, XC |
| [okf_phpcontent](okapi-filters/okf_phpcontent.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 1 file (64 methods) | - |
| [okf_messageformat](okapi-filters/okf_messageformat.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 6 files (97 methods) | RT(json), RT(yaml) |
| [okf_transtable](okapi-filters/okf_transtable.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 1 file (8 methods) | RT, XC |

## Phase 3 - Binary/Container Formats (12 filters)

| Filter | Doc | Extraction | Config | Roundtrip | XLIFF Compare | Simplifier | Memory | Java Unit Tests | Java IT Tests |
|--------|-----|------------|--------|-----------|---------------|------------|--------|-----------------|---------------|
| [okf_openxml](okapi-filters/okf_openxml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 45 files (412 methods) | RT, XC, Simp, Mem |
| [okf_idml](okapi-filters/okf_idml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 4 files (86 methods) | RT, XC, Simp |
| [okf_icml](okapi-filters/okf_icml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 1 file (13 methods) | RT, XC, Simp |
| [okf_openoffice](okapi-filters/okf_openoffice.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | 3 files (12 methods) | RT, XC, Simp |
| [okf_odf](okapi-filters/okf_odf.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | (shared with openoffice) | - |
| [okf_mif](okapi-filters/okf_mif.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 5 files (49 methods) | RT |
| [okf_rtf](okapi-filters/okf_rtf.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 6 files (6 methods) | - |
| [okf_epub](okapi-filters/okf_epub.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (2 methods) | - |
| [okf_archive](okapi-filters/okf_archive.md) | [ ] | [ ] | [-] | [ ] | [ ] | [-] | [-] | 1 file (11 methods) | RT, XC |
| [okf_pdf](okapi-filters/okf_pdf.md) | [ ] | [ ] | [-] | [-] | [-] | [-] | [-] | 1 file (4 methods) | - |
| [okf_ttx](okapi-filters/okf_ttx.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 1 file (55 methods) | RT, XC |
| [okf_txml](okapi-filters/okf_txml.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 1 file (13 methods) | RT, XC |

## Phase 4 - Specialized/Table/Plaintext Variants (13 filters)

| Filter | Doc | Extraction | Config | Roundtrip | XLIFF Compare | Simplifier | Memory | Java Unit Tests | Java IT Tests |
|--------|-----|------------|--------|-----------|---------------|------------|--------|-----------------|---------------|
| [okf_table](okapi-filters/okf_table.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | [ ] | 4 files (91 methods) | RT, XC, Simp, Mem |
| [okf_commaseparatedvalues](okapi-filters/okf_commaseparatedvalues.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | (shared with table) | - |
| [okf_tabseparatedvalues](okapi-filters/okf_tabseparatedvalues.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | (shared with table) | - |
| [okf_fixedwidthcolumns](okapi-filters/okf_fixedwidthcolumns.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | (shared with table) | - |
| [okf_basetable](okapi-filters/okf_basetable.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | (shared with table) | - |
| [okf_baseplaintext](okapi-filters/okf_baseplaintext.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | (shared with plaintext) | - |
| [okf_paraplaintext](okapi-filters/okf_paraplaintext.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (16 methods) | - |
| [okf_regexplaintext](okapi-filters/okf_regexplaintext.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (6 methods) | - |
| [okf_splicedlines](okapi-filters/okf_splicedlines.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (5 methods) | - |
| [okf_versifiedtext](okapi-filters/okf_versifiedtext.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 0 files | - |
| [okf_vignette](okapi-filters/okf_vignette.md) | [ ] | [ ] | [ ] | [ ] | [-] | [-] | [-] | 2 files (14 methods) | - |
| [okf_transifex](okapi-filters/okf_transifex.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 0 files | - |
| [okf_xini](okapi-filters/okf_xini.md) | [ ] | [ ] | [ ] | [ ] | [ ] | [-] | [-] | 10 files (58 methods) | RT, XC |

## Phase 5 - Package/Bundle Filters + Cross-Cutting (7 filters + shared tests)

| Filter | Doc | Extraction | Config | Roundtrip | XLIFF Compare | Simplifier | Memory | Java Unit Tests | Java IT Tests |
|--------|-----|------------|--------|-----------|---------------|------------|--------|-----------------|---------------|
| [okf_xinirainbowkit](okapi-filters/okf_xinirainbowkit.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | (shared with xini) | - |
| [okf_rainbowkit](okapi-filters/okf_rainbowkit.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (4 methods) | - |
| [okf_sdlpackage](okapi-filters/okf_sdlpackage.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (5 methods) | - |
| [okf_wsxzpackage](okapi-filters/okf_wsxzpackage.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (3 methods) | - |
| [okf_pensieve](okapi-filters/okf_pensieve.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 0 files | Pensieve ITs |
| [okf_autoxliff](okapi-filters/okf_autoxliff.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (3 methods) | - |
| [okf_multiparsers](okapi-filters/okf_multiparsers.md) | [ ] | [ ] | [-] | [ ] | [-] | [-] | [-] | 1 file (6 methods) | - |
| cascadingfilter | [ ] | [ ] | [-] | [-] | [-] | [-] | [-] | (5 methods) | - |

### Shared Cross-Cutting Tests

| Test Suite | Status | Java Source | Go Target |
|------------|--------|-------------|-----------|
| Memory leak suite | [ ] | `BaseMemoryLeakTestIT` + 7 filter-specific | `filters/memory_test.go` |
| Simplifier suite | [ ] | `PostSegmentationSimplifierIT` + 15 filter-specific | `filters/simplifier_test.go` |
| Abstract simplifier rules | [ ] | `SimplifierRulesTest` (57 methods) | `filters/simplifier_rules_test.go` |

---

## Infrastructure Checklist

- [x] Create `bridgetest` helper package (`core/plugin/bridge/filters/bridgetest/`)
  - [x] `helpers.go` - `SharedBridge()`, `ReadString()`, `ReadBytes()`, `ReadFile()`
  - [x] `roundtrip.go` - `RoundTrip()`, `AssertRoundTrip()`, `RoundTripTestFiles()`
  - [x] `golden.go` - `CompareGolden()`
- [x] Create `scripts/fetch-okapi-testdata.sh`
- [x] Create `okapi-testdata` GitHub release with test resources
- [x] Add `okapi-testdata/` to `.gitignore`
- [ ] CI: Add testdata fetch step before integration tests
- [ ] CI: Add bridge filter tests to integration test job
- [x] Create `scripts/publish-okapi-surefire.sh` for Surefire XML reports
- [x] Create `scripts/fetch-okapi-surefire.sh` for fetching Surefire XML
- [x] Add `okapi-surefire/` to `.gitignore`
- [x] Test comparison dashboard (`make generate-test-comparison`)

---

## Native Format Tests

In addition to bridge tests (which verify Java→Go fidelity through the bridge JAR), native format implementations in `core/formats/` have their own test suites. Native tests verify the pure Go implementations directly.

Native format tests are included in `make generate-test-comparison` via `make test-native-json` and appear alongside bridge tests in the test comparison dashboard. They use `// okapi:` annotations to map Go tests to their Java counterparts.

| Native Format | Go Package | Status |
|---------------|-----------|--------|
| HTML | `core/formats/html` | Active |
| JSON | `core/formats/json` | Active |
| XLIFF | `core/formats/xliff` | Active |
| XLIFF 2 | `core/formats/xliff2` | Active |
| Properties | `core/formats/properties` | Active |
| PO | `core/formats/po` | Active |
| YAML | `core/formats/yaml` | Active |
| Plaintext | `core/formats/plaintext` | Active |
| Markdown | `core/formats/markdown` | Active |
| XML Stream | `core/formats/xmlstream` | Active |

---

## Go Test Directory Structure

```
core/plugin/bridge/filters/
  bridgetest/              # Shared test helpers
    helpers.go             # SharedBridge(), ReadString(), ReadBytes(), ReadFile()
    roundtrip.go           # RoundTrip(), AssertRoundTrip(), RoundTripTestFiles()
    golden.go              # CompareGolden()
  okf_html/
    html_test.go           # Extraction + full-file tests
    config_test.go         # Configuration/parameter tests
    roundtrip_test.go      # Roundtrip tests
    xliff_compare_test.go  # XLIFF compare tests
  okf_json/
    ...
  ... (one package per filter)
  memory_test.go           # Cross-filter memory stress tests
  simplifier_test.go       # Cross-filter simplifier tests
```

All files use `//go:build integration` build tag and require `GOKAPI_BRIDGE_JAR`.

## Java → Go Test Category Mapping

| Java Category | Go File | Go Pattern |
|---|---|---|
| `*SnippetsTest` (inline strings) | `{id}_test.go` | Table-driven `TestExtract_*` with inline input strings |
| `*FilterTest` (full files) | `{id}_test.go` | Table-driven `TestFullFile` reading from testdata/ |
| `*ConfigurationTest` | `config_test.go` | Table-driven `TestConfig_*` with varying filterParams |
| `RoundTrip*IT` | `roundtrip_test.go` | Table-driven `TestRoundTrip` over testdata/roundtrip/ |
| `*XliffCompareIT` | `xliff_compare_test.go` | `TestXliffCompare` verifying Part structure |
| `RoundTripSimplify*IT` | shared `simplifier_test.go` | Per-filter entries in shared simplifier suite |
| `*MemoryLeakTestIT` | shared `memory_test.go` | Loop N iterations, check allocs via runtime.ReadMemStats |

---

## Java Source Change Tracking

| Field | Value |
|-------|-------|
| Okapi repo | `/Users/asgeirf/src/okapi/okapi-java` |
| Baseline commit | `3da02f86ec17c8168d6d49f80aaf55c1c04a7d47` |
| Sync date | 2026-02-24 |

### Detecting upstream changes

```bash
cd /Users/asgeirf/src/okapi/okapi-java
git log --since="2026-02-24" --name-only -- "okapi/filters/*/src/test/" "integration-tests/"
```

---

## Per-Filter Migration Docs

All 57 per-filter docs are in [`docs/okapi-filters/`](okapi-filters/):

| Filter ID | Doc | Phase |
|-----------|-----|-------|
| okf_archive | [okf_archive.md](okapi-filters/okf_archive.md) | 3 |
| okf_autoxliff | [okf_autoxliff.md](okapi-filters/okf_autoxliff.md) | 5 |
| okf_baseplaintext | [okf_baseplaintext.md](okapi-filters/okf_baseplaintext.md) | 4 |
| okf_basetable | [okf_basetable.md](okapi-filters/okf_basetable.md) | 4 |
| okf_commaseparatedvalues | [okf_commaseparatedvalues.md](okapi-filters/okf_commaseparatedvalues.md) | 4 |
| okf_doxygen | [okf_doxygen.md](okapi-filters/okf_doxygen.md) | 2 |
| okf_dtd | [okf_dtd.md](okapi-filters/okf_dtd.md) | 2 |
| okf_epub | [okf_epub.md](okapi-filters/okf_epub.md) | 3 |
| okf_fixedwidthcolumns | [okf_fixedwidthcolumns.md](okapi-filters/okf_fixedwidthcolumns.md) | 4 |
| okf_html | [okf_html.md](okapi-filters/okf_html.md) | 1 |
| okf_html5 | [okf_html5.md](okapi-filters/okf_html5.md) | 2 |
| okf_icml | [okf_icml.md](okapi-filters/okf_icml.md) | 3 |
| okf_idml | [okf_idml.md](okapi-filters/okf_idml.md) | 3 |
| okf_json | [okf_json.md](okapi-filters/okf_json.md) | 1 |
| okf_markdown | [okf_markdown.md](okapi-filters/okf_markdown.md) | 1 |
| okf_messageformat | [okf_messageformat.md](okapi-filters/okf_messageformat.md) | 2 |
| okf_mif | [okf_mif.md](okapi-filters/okf_mif.md) | 3 |
| okf_mosestext | [okf_mosestext.md](okapi-filters/okf_mosestext.md) | 2 |
| okf_multiparsers | [okf_multiparsers.md](okapi-filters/okf_multiparsers.md) | 5 |
| okf_odf | [okf_odf.md](okapi-filters/okf_odf.md) | 3 |
| okf_openoffice | [okf_openoffice.md](okapi-filters/okf_openoffice.md) | 3 |
| okf_openxml | [okf_openxml.md](okapi-filters/okf_openxml.md) | 3 |
| okf_paraplaintext | [okf_paraplaintext.md](okapi-filters/okf_paraplaintext.md) | 4 |
| okf_pdf | [okf_pdf.md](okapi-filters/okf_pdf.md) | 3 |
| okf_pensieve | [okf_pensieve.md](okapi-filters/okf_pensieve.md) | 5 |
| okf_phpcontent | [okf_phpcontent.md](okapi-filters/okf_phpcontent.md) | 2 |
| okf_plaintext | [okf_plaintext.md](okapi-filters/okf_plaintext.md) | 1 |
| okf_po | [okf_po.md](okapi-filters/okf_po.md) | 1 |
| okf_properties | [okf_properties.md](okapi-filters/okf_properties.md) | 1 |
| okf_rainbowkit | [okf_rainbowkit.md](okapi-filters/okf_rainbowkit.md) | 5 |
| okf_regex | [okf_regex.md](okapi-filters/okf_regex.md) | 2 |
| okf_regexplaintext | [okf_regexplaintext.md](okapi-filters/okf_regexplaintext.md) | 4 |
| okf_rtf | [okf_rtf.md](okapi-filters/okf_rtf.md) | 3 |
| okf_sdlpackage | [okf_sdlpackage.md](okapi-filters/okf_sdlpackage.md) | 5 |
| okf_splicedlines | [okf_splicedlines.md](okapi-filters/okf_splicedlines.md) | 4 |
| okf_table | [okf_table.md](okapi-filters/okf_table.md) | 4 |
| okf_tabseparatedvalues | [okf_tabseparatedvalues.md](okapi-filters/okf_tabseparatedvalues.md) | 4 |
| okf_tex | [okf_tex.md](okapi-filters/okf_tex.md) | 2 |
| okf_tmx | [okf_tmx.md](okapi-filters/okf_tmx.md) | 2 |
| okf_transifex | [okf_transifex.md](okapi-filters/okf_transifex.md) | 4 |
| okf_transtable | [okf_transtable.md](okapi-filters/okf_transtable.md) | 2 |
| okf_ts | [okf_ts.md](okapi-filters/okf_ts.md) | 2 |
| okf_ttml | [okf_ttml.md](okapi-filters/okf_ttml.md) | 2 |
| okf_ttx | [okf_ttx.md](okapi-filters/okf_ttx.md) | 3 |
| okf_txml | [okf_txml.md](okapi-filters/okf_txml.md) | 3 |
| okf_versifiedtext | [okf_versifiedtext.md](okapi-filters/okf_versifiedtext.md) | 4 |
| okf_vignette | [okf_vignette.md](okapi-filters/okf_vignette.md) | 4 |
| okf_vtt | [okf_vtt.md](okapi-filters/okf_vtt.md) | 2 |
| okf_wiki | [okf_wiki.md](okapi-filters/okf_wiki.md) | 2 |
| okf_wsxzpackage | [okf_wsxzpackage.md](okapi-filters/okf_wsxzpackage.md) | 5 |
| okf_xini | [okf_xini.md](okapi-filters/okf_xini.md) | 4 |
| okf_xinirainbowkit | [okf_xinirainbowkit.md](okapi-filters/okf_xinirainbowkit.md) | 5 |
| okf_xliff | [okf_xliff.md](okapi-filters/okf_xliff.md) | 1 |
| okf_xliff2 | [okf_xliff2.md](okapi-filters/okf_xliff2.md) | 1 |
| okf_xml | [okf_xml.md](okapi-filters/okf_xml.md) | 2 |
| okf_xmlstream | [okf_xmlstream.md](okapi-filters/okf_xmlstream.md) | 1 |
| okf_yaml | [okf_yaml.md](okapi-filters/okf_yaml.md) | 1 |
