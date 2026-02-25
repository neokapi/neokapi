# Okapi Filter & Tool Frameworks: Java Dependencies and Go Equivalents

This document provides a comprehensive analysis of the Java libraries and frameworks used by each
Okapi Framework filter and tool, along with an evaluation of Go equivalents for reimplementation
in gokapi. Special attention is given to whitespace preservation, format fidelity, and roundtrip
capability — the defining requirements of a localization framework.

**Legend — Implementation Status:**
- **Implemented** — Already exists in gokapi with reader + writer
- **Planned** — Not yet implemented, analysis below guides the approach
- **Bridge** — Recommended for Java bridge (Okapi plugin) rather than native Go

---

## Table of Contents

1. [Core Java Libraries](#1-core-java-libraries)
2. [Filter Matrix: Java Dependencies → Go Equivalents](#2-filter-matrix)
3. [XML & Markup Filters](#3-xml--markup-filters)
4. [Data & Configuration Filters](#4-data--configuration-filters)
5. [Office & Publishing Filters](#5-office--publishing-filters)
6. [Table & Spreadsheet Filters](#6-table--spreadsheet-filters)
7. [Translation Exchange Filters](#7-translation-exchange-filters)
8. [Subtitle & Media Filters](#8-subtitle--media-filters)
9. [Package & Container Filters](#9-package--container-filters)
10. [Specialized / Legacy Filters](#10-specialized--legacy-filters)
11. [Pipeline Tools & Steps](#11-pipeline-tools--steps)
12. [Cross-Cutting Infrastructure](#12-cross-cutting-infrastructure)
13. [Implementation Priority Matrix](#13-implementation-priority-matrix)
14. [Recommendations](#14-recommendations)

---

## 1. Core Java Libraries

These are the foundational libraries that Okapi's filters share. Understanding them is key to
planning the Go reimplementation, since replacing one core library affects many filters.

### 1.1 Woodstox (StAX XML Streaming)

**Java:** `com.fasterxml.woodstox:woodstox-core:7.0.0` + `org.codehaus.woodstox:stax2-api:4.2.2`

**Used by:** XLIFF, XLIFF2, AutoXLIFF, IDML, OpenXML, OpenOffice/ODF, TMX, TTX, TXML, TS,
XmlStream (~12 filters)

**What it provides:**
- Pull-based streaming XML parser (memory-efficient for large documents)
- Full namespace support with prefix preservation
- Whitespace preservation in mixed content
- CDATA section preservation (distinct from CharData)
- Processing instruction and comment preservation
- DTD handling and custom entity resolution
- Self-closing element preservation (`<br/>` vs `<br></br>`)
- Attribute quoting style preservation

**Go equivalent: `encoding/xml` (stdlib) — with significant gaps**

| Capability | `encoding/xml` | Gap |
|---|---|---|
| Streaming/pull parsing | `Decoder.Token()` / `RawToken()` | No gap |
| Namespace support | `Token()` resolves URIs; `RawToken()` preserves prefixes | No gap |
| Comments / ProcInst | `xml.Comment`, `xml.ProcInst` tokens | No gap |
| CDATA preservation | CDATA merged with CharData | **Critical gap** ([Go #12611](https://github.com/golang/go/issues/12611)) |
| Roundtrip fidelity | Not guaranteed | **Critical gap** ([Go #43168](https://github.com/golang/go/issues/43168)) |
| Self-closing elements | `<br/>` indistinguishable from `<br></br>` | **Gap** ([Go #69273](https://github.com/golang/go/issues/69273)) |
| Entity resolution | `Decoder.Entity` map (manual only) | **Moderate gap** (no DTD-driven resolution) |
| Attribute quote style | Normalized to `"` on output | **Minor gap** |

**Mitigation strategy:** Build a thin **roundtrip-safe XML wrapper** around `encoding/xml` that:
1. Uses `RawToken()` + `Decoder.InputOffset()` (Go 1.20+) to track byte positions
2. Correlates raw bytes to detect CDATA vs CharData
3. Preserves self-closing element form via raw byte inspection
4. Maintains a skeleton of raw bytes for reconstruction on write
5. Pre-populates `Decoder.Entity` from DTD scanning

**Complement:** `github.com/beevik/etree` (1,652 stars, BSD-2, active) for DOM-style manipulation
when formats like OpenXML or IDML need element-level modifications. Not a streaming parser.

**Effort estimate:** ~2000-3000 lines for the roundtrip XML layer.

### 1.2 Jericho HTML Parser

**Java:** `net.htmlparser.jericho:jericho-html:3.4`

**Used by:** HTML, AbstractMarkup (base class), Doxygen, Properties, TTML (~5 filters)

**What it provides:**
- Streaming HTML parsing that does NOT correct malformed markup
- Preserves original source byte-for-byte (including broken tags, irregular whitespace)
- Non-well-formed HTML handled gracefully
- `StreamedSource` for large file processing

**Go equivalent: `golang.org/x/net/html` Tokenizer API**

The `x/net/html` package has two distinct APIs:

| API | Behavior | Suitability |
|---|---|---|
| **Tokenizer** (`html.NewTokenizer()`) | Streaming, non-correcting, `Raw()` returns unmodified bytes | **Good Jericho replacement** |
| **Tree Parser** (`html.Parse()`) | Builds DOM, corrects HTML (injects `<html>`, `<head>`, `<body>`) | **Not suitable** (normalizes source) |

The **Tokenizer API** is the correct replacement:
- `Raw()` gives byte-accurate original source per token
- Does not inject missing tags or reorder elements
- Handles malformed HTML without crashing
- Structured accessors (`TagName()`, `TagAttr()`) available alongside raw bytes

**Important:** gokapi's current HTML reader uses `html.Parse()` (tree parser), which normalizes
the HTML. A migration to the Tokenizer API would improve format fidelity.

### 1.3 Flexmark (Markdown Parser)

**Java:** `com.vladsch.flexmark:flexmark:0.64.8` + 5 extension modules

**Used by:** Markdown filter

**Go equivalent: `github.com/yuin/goldmark` — already in use**

| Capability | goldmark | Notes |
|---|---|---|
| CommonMark compliance | Full | Passes spec suite |
| GFM extensions | Built-in (`extension.GFM`) | Tables, strikethrough, task lists |
| YAML front matter | Via `go.abhg.dev/goldmark/frontmatter` | Mature plugin |
| Admonitions | Via `goldmark-admonitions` | Community plugin |
| AST access | `text.Segment` byte offsets | Fully walkable tree |
| Roundtrip fidelity | Via skeleton approach | AST preserves source byte positions |

**No change needed.** goldmark is the right choice and is already integrated.

### 1.4 SnakeYAML Engine

**Java:** `org.snakeyaml:snakeyaml-engine:2.8`

**Used by:** YAML filter, AbstractMarkup configuration

**Go equivalent: `gopkg.in/yaml.v3` — already in use**

| Capability | yaml.v3 | Notes |
|---|---|---|
| YAML spec compliance | YAML 1.1 (partial 1.2) | SnakeYAML Engine is full 1.2 |
| Scalar style preservation | `yaml.Node.Style` field | LiteralStyle, FoldedStyle, etc. |
| Comment preservation | `HeadComment`, `LineComment`, `FootComment` | Re-serialization may shift indentation |
| Key ordering | Preserved via `yaml.Node.Content` | MappingNode stores in order |
| Anchor/alias | `yaml.Node.Anchor` + `AliasNode` kind | Supported |
| Multi-document | Sequential `Decode()` calls | Supported |

**Alternative:** `github.com/goccy/go-yaml` (~1.2k stars) offers better YAML 1.2 compliance and
token-level access via its `lexer` package, which is the closest Go equivalent to SnakeYAML
Engine's event API. Worth evaluating if YAML 1.2 edge cases become an issue.

### 1.5 JavaCC (Parser Generator)

**Java:** JavaCC Maven plugin generates custom lexer/parser code

**Used by:** JSON filter (custom JSON parser), YAML filter (custom YAML parser)

Okapi uses JavaCC to generate custom, format-preserving parsers for JSON and YAML rather than
using standard libraries. This ensures whitespace, comments, and formatting quirks are preserved.

**Go equivalent:** Not directly applicable. Go has parser generator tools (`goyacc`, `participle`,
`pigeon` PEG parser) but the better approach is to use format-preserving libraries:

- **JSON:** `github.com/tailscale/hujson` — byte-for-byte roundtrip, supports comments and
  trailing commas. See [Section 4.1](#41-json-filter).
- **YAML:** `gopkg.in/yaml.v3` with `yaml.Node` API preserves structure. See above.

### 1.6 Apache PDFBox

**Java:** `org.apache.pdfbox:pdfbox:3.0.3` + `pdfbox-io:3.0.3`

**Used by:** PDF filter

**Go equivalent: Tiered approach**

| Library | Type | Text Extraction | Layout-Aware | License |
|---|---|---|---|---|
| `github.com/pdfcpu/pdfcpu` | Pure Go | Basic | Limited | Apache 2.0 |
| `github.com/unidoc/unipdf` | Pure Go | Best-in-class | Yes | **Commercial** |
| `poppler/pdftotext` via exec | External | Excellent | Yes (`-layout`) | GPL |

**Recommendation:** `pdfcpu` for pure-Go deployment; `poppler` via exec as fallback for
production-grade layout-aware extraction. PDF is inherently a presentation format — for
localization, text extraction is typically for reference/review, not roundtrip reconstruction.

### 1.7 ICU4J

**Java:** `com.ibm.icu:icu4j:75.1`

**Used by:** PDF (charset detection), segmentation, Unicode processing across all filters

**Go equivalent: Multiple `golang.org/x/text` packages**

| ICU4J Feature | Go Package | Status |
|---|---|---|
| NFC/NFD/NFKC/NFKD | `golang.org/x/text/unicode/norm` | Full parity |
| Collation | `golang.org/x/text/collate` | Good parity |
| Bidi text | `golang.org/x/text/unicode/bidi` | Functional (minor bug [#69819](https://github.com/golang/go/issues/69819)) |
| Case mapping | `golang.org/x/text/cases` | Full parity |
| CLDR data | `golang.org/x/text/unicode/cldr` | Lower-level than ICU |
| Language tags | `golang.org/x/text/language` | BCP 47 parity |
| Break iterators | `github.com/rivo/uniseg` or `github.com/clipperhouse/uax29` | UAX #29 parity |
| Charset detection | `github.com/saintfish/chardet` | ICU algorithm port |

### 1.8 Apache Commons CSV

**Java:** `org.apache.commons:commons-csv:1.12.0`

**Used by:** Table, CSV, TSV, MultiParsers filters

**Go equivalent: `encoding/csv` (stdlib) — already in use**

Fully adequate. Preserves field whitespace ("white space is considered part of a field"),
configurable delimiters via `Reader.Comma`, lazy quoting via `LazyQuotes`. No third-party
library needed.

### 1.9 JAXB (XML Data Binding)

**Java:** `jakarta.xml.bind-api`, `com.sun.xml.bind:jaxb-core:4.0.5`

**Used by:** OpenXML, XINI, Xini RainbowKit

**Go equivalent: `encoding/xml` struct tags — already in use**

Go's `encoding/xml` struct tag-based marshaling/unmarshaling is directly analogous to JAXB
annotations. Already used extensively in gokapi's XLIFF, XLIFF2, TMX readers/writers.

---

## 2. Filter Matrix

### Java Dependencies → Go Equivalents

| Filter | Java Libraries | Go Equivalent | gokapi Status |
|---|---|---|---|
| **XML** | Woodstox/StAX | `encoding/xml` + roundtrip wrapper | **Implemented** |
| **XML Stream** | Woodstox/StAX | `encoding/xml` + roundtrip wrapper | Planned |
| **HTML** | Jericho 3.4 | `x/net/html` Tokenizer | **Implemented** |
| **HTML5/ITS** | nu.validator 1.4 | `x/net/html` Tree Parser + custom ITS | Planned |
| **XLIFF 1.2** | Woodstox/StAX | `encoding/xml` | **Implemented** |
| **XLIFF 2.0** | Woodstox/StAX2 | `encoding/xml` | **Implemented** |
| **Auto XLIFF** | Woodstox | `encoding/xml` | Planned |
| **JSON** | Custom JavaCC parser | `tailscale/hujson` (upgrade from `encoding/json`) | **Implemented** |
| **YAML** | Custom JavaCC + SnakeYAML | `gopkg.in/yaml.v3` | **Implemented** |
| **Markdown** | Flexmark 0.64.8 (6 modules) | `yuin/goldmark` + extensions | **Implemented** |
| **PO** | Custom (regex) | Custom | **Implemented** |
| **Properties** | Jericho (via AbstractMarkup) | Custom (line-based + regex) | **Implemented** |
| **Plain Text** | BufferedReader | `bufio.Scanner` | **Implemented** |
| **CSV** | Commons CSV 1.12 | `encoding/csv` | **Implemented** |
| **TSV** | Commons CSV | `encoding/csv` (tab delimiter) | Planned (trivial) |
| **TMX** | Woodstox/StAX | `encoding/xml` | **Implemented** |
| **VTT** | Custom line-based | Custom line-based | **Implemented** |
| **SRT** | Custom line-based | Custom line-based | **Implemented** |
| **OpenXML** | Woodstox + JAXB + ZIP | `archive/zip` + `encoding/xml` | Planned |
| **OpenOffice/ODF** | Woodstox + ZIP | `archive/zip` + `encoding/xml` | Planned |
| **IDML** | Woodstox + StAX2 + ZIP | `archive/zip` + `encoding/xml` | Planned |
| **ICML** | DOM (DocumentBuilder) | `encoding/xml` or `beevik/etree` | Planned |
| **RTF** | Custom parser | Custom parser | **Bridge** |
| **MIF** | Custom parser | Custom parser | **Bridge** |
| **PDF** | PDFBox 3.0.3 + ICU4J | `pdfcpu` / poppler exec | Planned |
| **DTD** | dtdparser 1.21 | Custom (~500 lines) | Planned |
| **TS (Qt)** | Woodstox/StAX | `encoding/xml` | Planned |
| **TTML** | Jericho (via AbstractMarkup) | `encoding/xml` | Planned |
| **EPUB** | Archive + HTML filters | `archive/zip` + existing HTML reader | Planned |
| **Doxygen** | Jericho (via AbstractMarkup) | Custom (comment extraction) | Planned |
| **Wiki** | Custom (regex + streaming) | Custom (regex + streaming) | Planned |
| **TeX** | Custom parser | Custom parser | Planned |
| **PHP Content** | Custom (regex) | Custom (regex) | Planned |
| **Regex** | `java.util.regex` | `regexp` + `regexp2` | Planned |
| **Fixed Width** | Custom line-based | Custom line-based | Planned |
| **TTX** | Woodstox/StAX | `encoding/xml` | Planned |
| **TXML** | Woodstox/StAX | `encoding/xml` | Planned |
| **MessageFormat** | Custom (regex) | Custom (regex) | Planned |
| **Archive** | `java.util.zip` | `archive/zip` | Planned |
| **Rainbow Kit** | Multi-format | Multi-format | Planned |
| **XINI** | JAXB | `encoding/xml` struct tags | Planned |
| **Transifex** | Custom | Custom | **Bridge** (legacy) |
| **Vignette** | SAX + regex | `encoding/xml` + regex | **Bridge** (CMS-specific) |
| **SDL Package** | ZIP-based | `archive/zip` | **Bridge** (TMS-specific) |
| **WSXZ Package** | ZIP-based | `archive/zip` | **Bridge** (TMS-specific) |
| **Pensieve** | Custom binary | Custom | **Bridge** (Okapi internal) |
| **Moses Text** | Line-based | `bufio.Scanner` | Planned |
| **Versified Text** | Line-based | `bufio.Scanner` | Planned |
| **Multi Parsers** | Commons CSV + cascading | `encoding/csv` + cascading | Planned |
| **Spliced Lines** | Line-based + continuation | `bufio.Scanner` + continuation | Planned |

---

## 3. XML & Markup Filters

### 3.1 XML Filter (okf_xml / okf_xmlstream)

**Java implementation:**
- `AbstractMarkupFilter` base class using Jericho `StreamedSource`
- `XmlStreamFilter` using Woodstox StAX with YAML-based tag configuration
- DITA native support via preconfigured rules
- CDATA handling (direct pass-through or subfiltered)
- Attribute-level subfiltering

**Java libraries:** Woodstox 7.0.0, StAX2 API 4.2.2, Jericho 3.4

**Go approach:**
- gokapi already has `core/formats/xml/` using `encoding/xml`
- XmlStream variant needs YAML-based configuration system for tagged rules
- DITA support requires preconfigured element/attribute extraction rules
- CDATA handling requires the roundtrip XML wrapper (see Section 1.1)

**Complexity for XmlStream:** Medium (~1500 lines). The filter itself is a configurable rule
engine on top of the XML streaming parser — most of the work is in the configuration system.

### 3.2 HTML Filter (okf_html)

**Java implementation:**
- Extends `AbstractMarkupFilter` → Jericho streaming parser
- Two modes: well-formed (XHTML) and non-well-formed (HTML)
- Whitespace collapse configurable per element
- `translate` attribute support
- META tag encoding detection/injection
- `dir` attribute update for locale

**Java libraries:** Jericho 3.4

**Go approach:**
- gokapi has `core/formats/html/` using `x/net/html` tree parser
- **Recommended migration:** Switch from `html.Parse()` to `html.NewTokenizer()` for Jericho-like
  behavior (non-correcting, streaming, raw byte preservation)
- Skeleton-based writer using `Raw()` bytes for roundtrip fidelity

**Complexity for migration:** Medium (~800-1000 lines of changes to existing reader/writer)

### 3.3 HTML5/ITS Filter (okf_html5)

**Java implementation:**
- nu.validator HTML5 parser building DOM tree
- ITS 2.0 (Internationalization Tag Set) data category processing
- LQI (Localization Quality Issue), provenance, terminology metadata
- Locale filtering, storage size constraints

**Java libraries:** nu.validator:htmlparser:1.4

**Go approach:**
- Use `x/net/html` tree parser (`html.Parse()`) — it implements the WHATWG HTML5 algorithm,
  same as nu.validator
- ITS processing must be implemented as a custom layer walking the DOM tree
- ITS data categories: `translate`, `its-loc-note`, `its-term`, `its-loc-quality-issue`, etc.

**Complexity:** High (~2500-3000 lines). ITS 2.0 is a W3C spec with 20+ data categories, each
with global rules (applied via selectors) and local attributes. No Go ITS implementation exists.

### 3.4 DTD Filter (okf_dtd)

**Java implementation:**
- `com.wutka:dtdparser:1.21` — parses DTD entity declarations
- Extracts `<!ENTITY name "value">` for translation
- Parameter entity support
- Comment preservation

**Go approach:**
- **No Go DTD parser exists.** `encoding/xml` does not parse DTD internal subsets
  ([Go #68388](https://github.com/golang/go/issues/68388))
- Write a focused entity parser: state machine parsing `<!ENTITY ...>` declarations
- Constrained grammar — well-suited to a simple recursive descent parser

**Complexity:** Low-Medium (~500-800 lines). DTD entity declarations have a well-defined syntax.

---

## 4. Data & Configuration Filters

### 4.1 JSON Filter (okf_json)

**Java implementation:**
- Custom JavaCC-generated lexer/parser (NOT a standard JSON library)
- Event-driven JSON handler (`IJsonHandler`)
- Preserves object/array structure, whitespace, and formatting
- Relaxed JSON (supports `//`, `#`, `/* */`, `<!-- -->` comments)
- `extractAllPairs` with exceptions, `useFullKeyPath`
- HTML subfiltering within string values
- Extensive metadata rules (noteRules, idRules, extractionRules)

**Java libraries:** Custom JavaCC parser, json-simple 1.1.1

**Go current:** `encoding/json` — does NOT preserve formatting, key order, or comments.

**Go recommended: `github.com/tailscale/hujson`** (~600 stars, Tailscale, MIT)

| Capability | `encoding/json` | `tailscale/hujson` |
|---|---|---|
| Whitespace preservation | No | **Byte-for-byte roundtrip** |
| Comment support | No | `//` line and `/* */` block comments |
| Trailing commas | No | Yes |
| Key ordering | Lost in `map[string]any` | Preserved in syntax tree |
| Streaming | `Decoder.Token()` | No (full parse to AST) |
| Standard JSON | Yes | Yes (JWCC superset) |

hujson's AST (`Value` → `Object`/`Array`/`Literal` with `Extra` whitespace nodes) allows
targeted modification of string values while preserving all surrounding formatting.

**Alternative for the future:** `encoding/json/v2` (experimental, Go 1.25+) adds `jsontext`
package with token-level access and `PreserveRawStrings`. Not yet stable.

**Complexity for migration:** Medium (~500-800 lines to switch from `encoding/json` to hujson
in the existing JSON reader/writer while preserving the Part emission logic).

### 4.2 YAML Filter (okf_yaml)

**Java implementation:**
- Custom JavaCC-generated YAML parser (NOT SnakeYAML directly for parsing)
- Streaming event handler (`IYamlHandler`)
- YAML 1.2 compliance
- Preserves scalar styles (literal, folded, quoted)
- Anchor/alias resolution
- HTML subfilter for string values
- Inline code detection
- Rails i18n compact notation

**Java libraries:** Custom JavaCC parser, SnakeYAML Engine 2.8

**Go current:** `gopkg.in/yaml.v3` with `yaml.Node` API — already implemented.

The `yaml.Node` API preserves structure, scalar styles, comments, and key ordering. The main
gap is YAML 1.2 compliance (yaml.v3 is YAML 1.1 with partial 1.2). For most localization files,
this difference is negligible.

**Potential upgrade:** `github.com/goccy/go-yaml` (~1.2k stars) for better YAML 1.2 compliance
and token-level access via its `lexer` package.

### 4.3 Properties Filter (okf_properties)

**Java implementation:**
- Extends `AbstractMarkupFilter` (Jericho-based)
- Key=value extraction with regex pattern matching
- Comment preservation (`#` and `!` prefixes)
- Escape sequence handling (`\n`, `\t`, `\uXXXX`)
- Continuation lines (trailing `\`)
- ISO 8859-1 default encoding with Unicode escapes

**Java libraries:** Jericho 3.4 (via AbstractMarkup), `java.util.regex`

**Go current:** Already implemented in gokapi at `core/formats/properties/`.

### 4.4 PO Filter (okf_po)

**Java implementation:**
- Streaming line-based parser with regex
- `msgid`, `msgstr`, `msgctxt` (context), plural forms (`msgid_plural`, `msgstr[N]`)
- Fuzzy flag and other metadata comments
- Printf-style format code detection (`%s`, `%d`, etc.)
- Monolingual mode option
- `protectApproved` for skipping approved translations

**Java libraries:** `java.util.regex`

**Go current:** Already implemented in gokapi at `core/formats/po/`.

### 4.5 TS Filter (okf_ts) — Qt Linguist

**Java implementation:**
- Woodstox StAX streaming XML parser
- Bilingual format (source + translation in same file)
- Translation states: unfinished, finished, obsolete
- Context elements for grouping
- Numerus (plural) forms
- Byte elements for special character encoding

**Java libraries:** Woodstox 7.0.0

**Go approach:** `encoding/xml` streaming. Qt .ts files are simple, well-structured XML.
The format has a small, stable schema.

**Complexity:** Low-Medium (~800-1200 lines for reader + writer).

### 4.6 PHP Content Filter (okf_phpcontent)

**Java implementation:**
- Streaming parser with regex
- String extraction from PHP source (single-quoted, double-quoted, heredoc, nowdoc)
- String concatenation handling
- PHP variables as inline codes
- `/*skip*/` and `/*text*/` directives

**Java libraries:** `java.util.regex`

**Go approach:** Custom streaming parser. PHP string parsing is regex-friendly.

**Complexity:** Medium (~1000-1500 lines). The variety of PHP string syntaxes adds complexity.

### 4.7 MessageFormat Filter (okf_messageformat)

**Java implementation:**
- ICU MessageFormat pattern parsing
- Plural and gender rule extraction
- Argument placeholder handling

**Java libraries:** Core Java only

**Go approach:** Custom parser for ICU MessageFormat syntax. The format has a well-defined
grammar: `{variable, type, style}` with nested plural/select blocks.

**Complexity:** Medium (~800-1200 lines). Recursive parsing of nested `{...}` blocks.

---

## 5. Office & Publishing Filters

### 5.1 OpenXML Filter (okf_openxml) — DOCX / XLSX / PPTX

**This is the largest and most complex filter in Okapi** (412+ test methods, 77 core tests +
111 roundtrip tests + 31 PPTX-specific + 34 XLSX-specific).

**Java implementation:**
- ZIP archive extraction → StAX XML parsing of internal parts
- Does NOT use Apache POI — raw ZIP + XML throughout
- **Word:** `<w:r>/<w:t>` run/text extraction, styles, hidden text, complex fields, content
  controls (SDT), tracked revisions, bookmarks, comments, headers/footers
- **Excel:** Shared strings table (`<si>/<t>`), sheet names, source/target column mapping,
  merged cells, formulas, conditional formatting, hidden content
- **PowerPoint:** `<a:r>/<a:t>` run/text extraction, slide ordering, notes, charts, diagrams,
  SmartArt, hidden slides, speaker notes
- OOXML strict mode support
- Style-based inclusion/exclusion (by name, color, highlight)
- Aggressive cleanup and style optimization

**Java libraries:** Woodstox 7.0.0, JAXB (jakarta.xml.bind), TwelveMonkeys common-io 3.12.0

**Go approach:** Follow Okapi's same architecture — raw `archive/zip` + `encoding/xml`:
1. Open OOXML as ZIP
2. Parse `[Content_Types].xml` and `*.rels` for document part discovery
3. StAX-equivalent streaming of each XML part
4. Extract translatable runs while preserving all other XML as skeleton
5. On write, splice translated text back into XML stream within ZIP

**Go libraries involved:**
- `archive/zip` (stdlib) — ZIP read/write with `CreateHeader()` for metadata preservation
- `encoding/xml` — streaming XML token processing
- The roundtrip XML wrapper (Section 1.1) is important here for preserving formatting

**No high-level Office library should be used.** Libraries like `excelize` or `unioffice` create
DOM representations that lose formatting fidelity. The localization use case requires byte-level
XML preservation — only the translatable text content changes.

**Complexity:** Very High (~2000-3000 lines per format, ~7000-9000 total for all three).
DOCX is the most complex (tracked changes, complex fields, nested structures). XLSX is simpler
(shared strings model). PPTX is structurally similar to DOCX.

### 5.2 OpenOffice/ODF Filter (okf_openoffice / okf_odf)

**Java implementation:**
- ZIP archive + Woodstox StAX for `content.xml`, `styles.xml`
- ODF namespaces: `text:`, `table:`, `style:`, `draw:`, `office:`
- Metadata extraction (title, description, keywords)
- Formula results
- Bookmark references

**Java libraries:** Woodstox 7.0.0

**Go approach:** Same as OpenXML — `archive/zip` + `encoding/xml`. ODF's XML schema is actually
cleaner than OOXML, making the implementation somewhat simpler.

**Go libraries for ODF:** None suitable for localization. Existing libraries (`kpmy/odf`,
`knieriem/odf`, `go-openoffice`) are minimal, focused on spreadsheets, and do not preserve
formatting.

**Complexity:** Medium-High (~2000-2500 lines per document type). Implement ODT first.

### 5.3 IDML Filter (okf_idml) — Adobe InDesign

**Java implementation:**
- ZIP package → StAX XML parsing of Story XML files
- Character style ignorance thresholds (kerning, tracking, leading, baseline shift)
- Font mappings with chaining
- Hidden pasteboard items
- Custom text variables, index topics, hyperlinks, endnotes
- Table cell extraction
- Cross-reference handling

**Java libraries:** Woodstox 7.0.0, StAX2 API 4.2.2

**Go approach:** `archive/zip` + `encoding/xml`. IDML is structurally similar to OOXML (ZIP of
XML files). The XML schema is InDesign-specific but well-documented.

**Existing Go library:** `github.com/dimelords/idmllib` — minimal, basic story access only.
Not robust enough for localization.

**Complexity:** High (~3000-4000 lines). InDesign's content model (stories, paragraphs,
character styles, tables, nested frames) is complex. ICML (InCopy) is a simpler subset
(~1500 lines).

### 5.4 ICML Filter (okf_icml) — Adobe InCopy

**Java implementation:**
- DOM-based (javax.xml.parsers.DocumentBuilder) — NOT streaming
- InCopy Markup Language (XML, not ZIP)
- Master spreads, notes, breaks
- Simplification options, skip thresholds

**Java libraries:** javax.xml.parsers (DOM)

**Go approach:** Since ICML files are single XML files (not ZIP packages), either:
- `encoding/xml` streaming (preferred for large files)
- `beevik/etree` DOM (convenient for the tree-walking pattern Okapi uses)

**Complexity:** Medium (~1500-2000 lines).

### 5.5 PDF Filter (okf_pdf)

**Java implementation:**
- PDFBox `PDFTextStripper` for text extraction
- `PDDocument` for PDF manipulation
- ICU4J for character encoding detection
- Extraction-only (no roundtrip modification)
- Delegates extracted text to PlainText/ParaPlainText filter

**Java libraries:** PDFBox 3.0.3, pdfbox-io 3.0.3, ICU4J 75.1

**Go approach (tiered):**

| Tier | Library | Quality | License | Dependencies |
|---|---|---|---|---|
| 1 | `github.com/pdfcpu/pdfcpu` | Basic extraction | Apache 2.0 | Pure Go |
| 2 | `poppler/pdftotext` via exec | Excellent | GPL (external) | Requires poppler-utils |
| 3 | `github.com/unidoc/unipdf` | Best-in-class | **Commercial** | Pure Go |

**Recommendation:** Start with `pdfcpu`. For production, offer `poppler` as a configurable
external extractor. Feed extracted text through the existing PlainText format reader.

**Complexity:** Low-Medium (~500-800 lines for the extraction wrapper + PlainText delegation).

### 5.6 MIF Filter (okf_mif) — Adobe FrameMaker

**Java implementation:**
- Custom streaming MIF parser
- S-expression-like syntax with angle-bracket tags
- Document hierarchy, cross-references
- Paragraph/character catalog awareness

**Java libraries:** Core Java only (custom parser)

**Go approach:** No Go MIF parser exists. FrameMaker usage is declining (legacy
aerospace/defense/technical publishing).

**Complexity:** High (~3000-4000 lines) for a full roundtrip parser.

**Recommendation:** **Bridge only.** Defer to Java bridge (Okapi plugin). Not worth native
Go implementation unless specific demand arises.

### 5.7 RTF Filter (okf_rtf)

**Java implementation:**
- Custom streaming RTF parser
- Control word/symbol parsing with group nesting
- Font table tracking for codepage switching
- Unicode escape sequences (`\u12345?`)
- Bilingual format support (source + target in same file)
- Formatting preservation through control codes

**Java libraries:** Core Java only (custom parser)

**Go approach:** No suitable Go RTF parser exists. Existing options:
- `j45k4/rtf` — text stripping only, no roundtrip
- `aiq/go-rtf` — too new/immature

RTF spec is ~200 pages. Key challenges: recursive group nesting, Unicode escapes with fallback
characters, font table tracking for codepage switching, style sheet inheritance.

**Complexity:** High (~3000-4000 lines).

**Recommendation:** **Bridge only.** RTF is a declining format.

---

## 6. Table & Spreadsheet Filters

### 6.1 CSV Filter (okf_commaseparatedvalues)

**Java implementation:**
- Apache Commons CSV with configurable delimiters and qualifiers
- CatKeys format support
- Column mapping (source/target/notes columns)
- Text qualifier escaping (double/backslash modes)
- Blank cell/row/column handling

**Java libraries:** Commons CSV 1.12.0

**Go current:** Already implemented using `encoding/csv` (stdlib).

### 6.2 TSV Filter (okf_tabseparatedvalues)

**Java implementation:** Same as CSV with implicit tab delimiter.

**Go approach:** Trivial — `encoding/csv` with `Comma = '\t'`.

**Complexity:** Trivial (reuse CSV reader with different config).

### 6.3 Fixed-Width Columns Filter (okf_fixedwidthcolumns)

**Java implementation:**
- Position-based column extraction
- `columnStartPositions` and `columnEndPositions` (comma-separated)
- Line-based processing

**Java libraries:** Core Java only

**Go approach:** Custom line-based parser with column position slicing.

**Complexity:** Low (~300-500 lines).

### 6.4 Table Filter / Base Table (okf_table / okf_basetable)

**Java implementation:**
- Meta-filter dispatching to CSV/TSV/Fixed-Width sub-filters
- Shared column mapping infrastructure
- Locale-defined columns

**Go approach:** Dispatcher pattern delegating to existing CSV reader and planned TSV/FWC readers.

**Complexity:** Low (~300-500 lines for the dispatcher + configuration).

### 6.5 Multi Parsers Filter (okf_multiparsers)

**Java implementation:**
- Composite filter using multiple sub-parsers
- Format auto-detection between table variants

**Java libraries:** Commons CSV 1.12.0

**Go approach:** Dispatcher with format detection heuristics.

**Complexity:** Low (~200-400 lines).

---

## 7. Translation Exchange Filters

### 7.1 XLIFF 1.2 Filter (okf_xliff)

**Java implementation:**
- Woodstox StAX streaming parser
- 185+ unit tests
- Inline codes: `<bx/>`, `<ex/>`, `<bpt>`, `<ept>`, `<ph>`, `<it>`, `<x/>`, `<g>`, `<mrk>`
- Alt-trans (alternative translations)
- SDL/MemoQ dialect support
- State handling, code balancing
- Length constraints

**Java libraries:** Woodstox 7.0.0

**Go current:** Already implemented at `core/formats/xliff/` using `encoding/xml`.

### 7.2 XLIFF 2.0 Filter (okf_xliff2)

**Java implementation:**
- StAX streaming with custom XLIFF2 library
- New inline model: `<pc>`, `<ph>`, `<sc>`, `<ec>`, `<mrk>`
- Segment state/substate
- Original data section
- ICU message format subfilter
- XLIFF 1.2 output conversion

**Java libraries:** Woodstox 7.0.0, okapi-lib-xliff2

**Go current:** Already implemented at `core/formats/xliff2/` using `encoding/xml`.

### 7.3 Auto XLIFF Filter (okf_autoxliff)

**Java implementation:**
- Meta-filter that detects XLIFF version (1.2 vs 2.0) and delegates
- Configurable delegates (standard, SDL, MemoQ variants)

**Go approach:** Version detection by peeking at XML namespace/root element, then delegation
to existing XLIFF or XLIFF2 readers.

**Complexity:** Low (~200-300 lines).

### 7.4 TMX Filter (okf_tmx)

**Java implementation:**
- Woodstox StAX streaming
- TMX 1.1 and 1.4a support
- Translation unit (tu) extraction with segment types
- Inline codes: `<bpt>`, `<ept>`, `<it>`, `<ph>`, `<ut>`, `<hi>`, `<sub>`
- DTD support
- Multiple targets per source

**Java libraries:** Woodstox 7.0.0

**Go current:** Already implemented at `core/formats/tmx/` using `encoding/xml`.

### 7.5 TTX Filter (okf_ttx)

**Java implementation:** StAX XML parsing of Trados TagEditor format.

**Complexity:** Low-Medium (~800-1000 lines). Well-structured XML.

### 7.6 TXML Filter (okf_txml)

**Java implementation:** StAX XML parsing of translatable XML with TXML extension.

**Complexity:** Low-Medium (~800-1000 lines).

### 7.7 Trans Table Filter (okf_transtable)

**Java implementation:** Table-based translation memory format.

**Complexity:** Low (~400-600 lines). Table parsing + TM mapping.

---

## 8. Subtitle & Media Filters

### 8.1 VTT Filter (okf_vtt) — WebVTT

**Java implementation:** Line-based parsing of WebVTT cues with timing and settings.

**Go current:** Already implemented at `core/formats/vtt/`.

### 8.2 TTML Filter (okf_ttml) — Timed Text Markup Language

**Java implementation:**
- XML-based (Jericho via AbstractMarkup inheritance)
- Timing and style metadata
- Nested structure handling

**Java libraries:** Jericho 3.4

**Go approach:** `encoding/xml` streaming. TTML is straightforward XML with timing attributes.

**Complexity:** Medium (~1000-1500 lines). Timing metadata preservation is the main challenge.

### 8.3 Versified Text Filter (okf_versifiedtext)

**Java implementation:** Line-based with verse numbering.

**Complexity:** Low (~300-500 lines).

### 8.4 Moses Text Filter (okf_mosestext)

**Java implementation:** Line-based source/target sentence pairs for Moses SMT.

**Complexity:** Low (~200-400 lines).

---

## 9. Package & Container Filters

### 9.1 Archive Filter (okf_archive)

**Java implementation:**
- ZIP extraction with glob-based file pattern matching
- Delegates to sub-filters based on `configIds` mapping
- Requires `FilterConfigurationMapper` for sub-filter resolution

**Java libraries:** `java.util.zip`

**Go approach:** `archive/zip` + format registry delegation. The container walks ZIP entries,
matches patterns, and dispatches each entry to the appropriate format reader.

**Complexity:** Medium (~500-800 lines). The infrastructure for sub-filter delegation is the
main work.

### 9.2 EPUB Filter (okf_epub)

**Java implementation:**
- ZIP + OPF package file parsing
- Delegates XHTML content documents to HTML filter
- Navigation document handling
- Metadata extraction

**Java libraries:** Archive filter + HTML filter

**Go approach:** `archive/zip` for the EPUB container, parse `META-INF/container.xml` to find
the OPF, parse OPF for content document manifest, delegate each XHTML to gokapi's existing
HTML format reader.

**Go EPUB libraries:** Several exist (`go-shiori/go-epub`, `taylorskalyo/goreader`) but none
handle the localization use case. The wrapper is simple enough to write directly.

**Complexity:** Low-Medium (~500-800 lines for container logic).

### 9.3 Rainbow Kit Filter (okf_rainbowkit / okf_xinirainbowkit)

**Java implementation:** Okapi's own extensible package format for roundtripping.

**Complexity:** Medium (~800-1200 lines). Mostly a packaging/manifest format.

### 9.4 SDL Package / WSXZ Package (okf_sdlpackage / okf_wsxzpackage)

**Java implementation:** ZIP-based TMS-specific package formats.

**Recommendation:** **Bridge only.** These are vendor-specific formats (SDL Trados, WorldServer)
with limited adoption outside their respective ecosystems.

---

## 10. Specialized / Legacy Filters

### 10.1 Doxygen Filter (okf_doxygen)

**Java implementation:**
- Extends AbstractMarkupFilter (Jericho)
- Extracts translatable text from code documentation comments
- Supports `///`, `//!`, `/** */`, `/*! */` comment styles
- Inline commands (`@e`, `@a`, `@b`)
- Code/verbatim exclusion
- Python docstrings

**Complexity:** Medium (~1000-1500 lines). Comment syntax detection + content extraction.

### 10.2 Wiki Filter (okf_wiki)

**Java implementation:** Streaming + regex for MediaWiki/DokuWiki markup.

**Complexity:** Medium (~1200-1800 lines). Wiki markup is ambiguous and context-dependent.

### 10.3 TeX Filter (okf_tex)

**Java implementation:**
- Custom TeX parser
- Command and macro parsing
- Math mode and verbatim preservation
- Package/document structure awareness

**Complexity:** Medium-High (~1500-2000 lines). TeX's macro system makes parsing challenging.

### 10.4 Regex Filter (okf_regex)

**Java implementation:**
- Configurable regex patterns for extracting translatable content from arbitrary text formats
- Capture group-based extraction
- `java.util.regex` with lookbehind/lookahead

**Go approach:** `regexp` (stdlib, RE2) for simple patterns + `github.com/dlclark/regexp2`
(~1.1k stars, .NET-compatible) for patterns requiring lookbehind/lookahead.

**Important:** Go's stdlib `regexp` does NOT support lookaround assertions. Many Okapi regex
configurations and SRX segmentation rules rely on lookaround. `regexp2` fills this gap with
full PCRE-compatible features (3-10x slower than RE2 for simple patterns but comparable for
complex ones).

**Complexity:** Medium (~800-1200 lines for the configurable regex engine).

### 10.5 Transifex, Vignette, Pensieve

**Recommendation:** **Bridge only.** These are legacy/vendor-specific formats:
- **Transifex:** Legacy format; Transifex moved to API-based workflows. No Java tests exist.
- **Vignette:** CMS-specific export format.
- **Pensieve:** Okapi's internal TM format (superseded by standard TM formats).

---

## 11. Pipeline Tools & Steps

Beyond filters, Okapi provides pipeline steps (tools) that process content. Here is the analysis
of key steps and their Go equivalents.

### 11.1 SRX Segmentation

**Java:** Custom SRX implementation (`net.sf.okapi.lib.segmentation`)
- SRX (Segmentation Rules eXchange) XML rule file parsing
- Language-specific segmentation rules with regex `<beforebreak>` and `<afterbreak>` patterns
- Rule cascade/priority ordering
- ICU break iterators as fallback

**Go status:** gokapi has `core/tools/segmentation.go` with regex-based segmentation rules.

**Go SRX libraries:** **None exist.** No Go implementation of the SRX standard.

**Recommended approach:** Write a native SRX engine:
1. Parse SRX XML files using `encoding/xml`
2. Compile regex rules using `regexp2` (needed for lookaround in `<beforebreak>`/`<afterbreak>`)
3. Apply rules in priority order at candidate break positions
4. Use `rivo/uniseg` or `clipperhouse/uax29` as fallback segmenters

**Complexity:** Medium (~1500-2500 lines). SRX is well-documented. The engine is a loop over
candidate positions applying regex rules. Tricky parts: rule cascade ordering, `<formatHandle>`
elements, and ensuring regex compatibility with Java patterns.

### 11.2 Translation Memory Leveraging

**Java:** Apache Lucene-based TM indexing with token-based fuzzy matching.

**Go status:** gokapi already has `core/sievepen/` with 3-tier matching (generalized, structural,
plain) and custom Levenshtein distance/ratio functions. This is actually more sophisticated than
Okapi's raw Lucene approach.

**Scaling option:** `github.com/blevesearch/bleve` (~10.4k stars, Couchbase-backed) for
disk-backed indexing of large TMs (100k+ entries). Would serve as a pre-filter for candidate
retrieval before the existing match scorer.

### 11.3 Quality Assurance Checks

**Java:** Various QA steps checking:
- Missing/extra tags
- Number consistency
- Terminology compliance
- Length restrictions
- Pattern matching (regex-based)
- Whitespace issues

**Go status:** gokapi has `core/tools/` with qa-check, term-check, and other tools.

### 11.4 Pseudotranslation

**Java:** Text rewriting with configurable accented characters, expansion factors, brackets.

**Go status:** Already implemented as `pseudo-translate` tool.

### 11.5 XSLT Transformation

**Java:** `javax.xml.transform` for XSLT 1.0/2.0 processing.

**Go status:** gokapi has `xslt-transform` tool. Go XSLT options:
- `github.com/nicktrav/go-xslt` — CGo wrapper around libxslt (XSLT 1.0)
- No pure-Go XSLT 2.0 implementation exists

### 11.6 Text Rewriting / Search-Replace

**Java:** Pattern-based text transformation steps.

**Go status:** Already implemented as `search-replace` tool.

### 11.7 Encoding Detection/Conversion

**Java:** ICU4J `CharsetDetector` + `java.nio.charset`

**Go status:** gokapi has `core/encoding/encoding.go` with `golang.org/x/text/encoding`.
BOM-based detection. Could be enhanced with `saintfish/chardet` for statistical detection.

---

## 12. Cross-Cutting Infrastructure

### 12.1 Character Encoding

| Capability | Java (ICU4J + NIO) | Go Equivalent |
|---|---|---|
| Encoding conversion | `java.nio.charset` (~200+ charsets) | `golang.org/x/text/encoding` (~40 charsets) |
| Statistical detection | `ICU4J CharsetDetector` | `github.com/saintfish/chardet` |
| BOM detection | Built-in | Custom (gokapi has this) |
| HTML charset | HTML META/HTTP-Equiv | `golang.org/x/net/html/charset` |

The Go encoding coverage handles 99%+ of real-world localization documents. The gap is mainly
legacy/obscure encodings (IBM EBCDIC variants, Thai TIS-620, Vietnamese VISCII).

### 12.2 Unicode Processing

| Capability | Java (ICU4J) | Go Equivalent |
|---|---|---|
| Normalization | `com.ibm.icu.text.Normalizer2` | `golang.org/x/text/unicode/norm` |
| Collation | `com.ibm.icu.text.RuleBasedCollator` | `golang.org/x/text/collate` |
| Bidi algorithm | `com.ibm.icu.text.Bidi` | `golang.org/x/text/unicode/bidi` |
| Case mapping | `com.ibm.icu.lang.UCharacter` | `golang.org/x/text/cases` |
| Break iteration | `com.ibm.icu.text.BreakIterator` | `rivo/uniseg` or `clipperhouse/uax29` |
| Properties | `com.ibm.icu.lang.UProperty` | `unicode` stdlib package |
| CLDR data | ICU resource bundles | `golang.org/x/text/unicode/cldr` |

### 12.3 Regular Expressions

| Feature | Java (`java.util.regex`) | Go `regexp` | Go `regexp2` |
|---|---|---|---|
| Named groups | `(?<name>...)` | `(?P<name>...)` | Both syntaxes |
| Lookahead | `(?=...)`, `(?!...)` | **Not supported** | Supported |
| Lookbehind | `(?<=...)`, `(?<!...)` | **Not supported** | Supported |
| Backreferences | `\1`, `\k<name>` | **Not supported** | Supported |
| Unicode categories | `\p{L}`, `\p{N}` | Supported | Supported |
| Performance | Backtracking (exponential worst-case) | RE2 (linear guarantee) | Backtracking |

**Strategy:** Use `regexp` by default for performance. Use `regexp2` for SRX rules and any
pattern requiring lookaround. Create a thin wrapper that auto-selects or use `regexp2`
universally in the SRX engine.

### 12.4 ZIP Archive Handling

**Go:** `archive/zip` (stdlib) — fully adequate for all Office document formats (OOXML, ODF,
IDML, EPUB). Preserves entry order and metadata when using `CreateHeader()` with original
`FileHeader`. No enhanced library needed.

---

## 13. Implementation Priority Matrix

### Tier 1: Already Implemented (14 formats, 12+ tools)

| Format | gokapi Package | Status |
|---|---|---|
| Plain Text | `core/formats/plaintext/` | Complete |
| HTML | `core/formats/html/` | Complete (migration to Tokenizer recommended) |
| XML | `core/formats/xml/` | Complete |
| XLIFF 1.2 | `core/formats/xliff/` | Complete |
| XLIFF 2.0 | `core/formats/xliff2/` | Complete |
| JSON | `core/formats/json/` | Complete (hujson upgrade recommended) |
| YAML | `core/formats/yaml/` | Complete |
| Markdown | `core/formats/markdown/` | Complete |
| PO | `core/formats/po/` | Complete |
| Properties | `core/formats/properties/` | Complete |
| CSV | `core/formats/csv/` | Complete |
| SRT | `core/formats/srt/` | Complete |
| VTT | `core/formats/vtt/` | Complete |
| TMX | `core/formats/tmx/` | Complete |

### Tier 2: High Priority — Native Go Implementation

| Format | Effort | Approach | Dependencies |
|---|---|---|---|
| **OpenXML (DOCX)** | Very High | `archive/zip` + `encoding/xml` | Roundtrip XML wrapper |
| **OpenXML (XLSX)** | High | `archive/zip` + `encoding/xml` | Roundtrip XML wrapper |
| **OpenXML (PPTX)** | High | `archive/zip` + `encoding/xml` | Roundtrip XML wrapper |
| **IDML** | High | `archive/zip` + `encoding/xml` | Roundtrip XML wrapper |
| **EPUB** | Low-Medium | `archive/zip` + existing HTML reader | None new |
| **TS (Qt)** | Low-Medium | `encoding/xml` | None new |
| **Auto XLIFF** | Low | Format detection + delegation | None new |

### Tier 3: Medium Priority — Native Go Implementation

| Format | Effort | Approach | Dependencies |
|---|---|---|---|
| **ODF (ODT/ODS/ODP)** | Medium-High | `archive/zip` + `encoding/xml` | Roundtrip XML wrapper |
| **ICML** | Medium | `encoding/xml` or `beevik/etree` | None new |
| **XML Stream** | Medium | `encoding/xml` + YAML config | None new |
| **DTD** | Low-Medium | Custom entity parser | None new |
| **TTML** | Medium | `encoding/xml` | None new |
| **Regex** | Medium | `regexp` + `regexp2` | `regexp2` |
| **TSV** | Trivial | CSV reader with `Comma = '\t'` | None |
| **Fixed Width** | Low | Custom line parser | None |
| **Table/Base Table** | Low | Dispatcher over CSV/TSV/FWC | None |
| **TTX** | Low-Medium | `encoding/xml` | None new |
| **TXML** | Low-Medium | `encoding/xml` | None new |
| **PDF** | Low-Medium | `pdfcpu` / poppler exec | `pdfcpu` |
| **Archive** | Medium | `archive/zip` + registry | None new |

### Tier 4: Low Priority — Native Go or Bridge

| Format | Effort | Recommendation |
|---|---|---|
| HTML5/ITS | High | Native Go (ITS is a localization standard) |
| TeX | Medium-High | Native Go (academic/technical publishing) |
| Doxygen | Medium | Native Go (code documentation) |
| Wiki | Medium | Native Go (MediaWiki content) |
| PHP Content | Medium | Native Go |
| MessageFormat | Medium | Native Go (ICU standard) |
| Moses Text | Low | Native Go (trivial) |
| Versified Text | Low | Native Go (trivial) |
| Spliced Lines | Low | Native Go (trivial) |
| Multi Parsers | Low | Native Go |

### Tier 5: Bridge Only — Java Plugin

| Format | Reason |
|---|---|
| RTF | Declining format, complex spec (~200 pages), High effort |
| MIF (FrameMaker) | Niche legacy format, no Go parser exists, High effort |
| Vignette | CMS-specific, no external demand |
| Transifex | Legacy format, no Java tests, abandoned by vendor |
| SDL Package | Vendor-specific (SDL Trados) |
| WSXZ Package | Vendor-specific (WorldServer) |
| Pensieve | Okapi internal format |
| XINI / Xini Rainbow Kit | ONTRAM-specific format |
| Rainbow Kit | Okapi-specific packaging |

### Infrastructure Priority

| Component | Effort | Impact | Status |
|---|---|---|---|
| **Roundtrip XML wrapper** | High | Enables OOXML, ODF, IDML, all XML filters | Required for Tier 2 |
| **HTML Tokenizer migration** | Medium | Improves HTML format fidelity | Recommended |
| **JSON hujson migration** | Medium | Enables format-preserving JSON roundtrip | Recommended |
| **SRX segmentation engine** | Medium | Standard segmentation for all pipelines | Planned |
| **charset detection (chardet)** | Low | Statistical encoding detection fallback | Enhancement |

---

## 14. Recommendations

### 14.1 Invest in the Roundtrip XML Wrapper First

The single most impactful infrastructure investment is a roundtrip-safe XML processing layer.
It unblocks OOXML (largest filter), ODF, IDML, and improves all existing XML-based filters.
This layer wraps `encoding/xml` and provides:
- CDATA preservation via raw byte correlation
- Self-closing element form tracking
- Attribute whitespace/quote style preservation
- Skeleton-based write-back from original bytes

### 14.2 Use the Java Bridge for Legacy/Niche Formats

The Okapi Java bridge (gokapi's `core/plugin/bridge/`) is the right tool for formats that are:
- Declining in usage (RTF, MIF)
- Vendor-specific (SDL, WSXZ, Vignette)
- Okapi-internal (Pensieve, Rainbow Kit)

This lets gokapi focus native Go effort on formats with the highest localization demand.

### 14.3 Format-Preserving Library Upgrades

Two library upgrades would significantly improve roundtrip fidelity:

1. **JSON:** Migrate from `encoding/json` to `tailscale/hujson` for byte-for-byte roundtrip,
   comment preservation, and key ordering preservation.

2. **HTML:** Migrate from `html.Parse()` (tree parser) to `html.NewTokenizer()` (tokenizer)
   for Jericho-like non-correcting, streaming, raw-byte-preserving HTML processing.

### 14.4 Write-from-Scratch Strategy for Office Formats

Do NOT use high-level Office libraries (excelize, unioffice, etc.) for the OpenXML, ODF, or
IDML filters. Follow Okapi's proven approach: raw ZIP + XML streaming. This ensures:
- Byte-level formatting preservation
- No unwanted normalization or correction
- Full control over the content extraction and reinsertion process

### 14.5 Go Library Selection Criteria for Localization

When evaluating Go libraries for localization use cases, apply these criteria in order:

1. **Format fidelity** — Does the library preserve the original document byte-for-byte for
   non-translatable content? Can you roundtrip without changes?
2. **Streaming capability** — Can it process large documents without loading everything into
   memory? Localization workflows handle documents of all sizes.
3. **Structural access** — Can you identify and extract translatable content at the right
   granularity (elements, attributes, key paths)?
4. **Whitespace handling** — Does it normalize, collapse, or trim whitespace? For localization,
   the answer should be "no" by default.
5. **Active maintenance** — Is the library maintained and used in production?

A library that excels at data extraction but normalizes whitespace is worse than no library at
all for a localization framework.
