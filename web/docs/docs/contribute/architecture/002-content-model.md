---
id: 002-content-model
sidebar_position: 2
title: "AD-002: Content Model"
description: "Architecture decision: documents are represented as a stream of Part values (Layer, Block, Fragment, Data, Media) so that tools and translations work independently of source file format."
keywords: [content model, Part, Block, Fragment, Layer, architecture decision, neokapi]
---

# AD-002: Content Model

## Summary

Documents in neokapi are represented as a stream of `Part` values, each
carrying a `PartType` discriminator and a `Resource`. Translatable content is
a `Block`; text with inline markup is a `Fragment` containing ordered `Span`
values referenced positionally by Unicode private-use-area markers. Spans
carry six layers of metadata (native markup, abstract identity, semantic
type, display text, text equivalent, editing constraints) so that
format-aware tools can process content semantically while writers roundtrip
native markup exactly.

## Context

A localization content model must represent translatable documents in a way
that is format-independent, type-safe, extensible, and able to represent
recursive embedded content naturally. Go's composition and interface system
(no class inheritance) shapes the design toward discriminated unions and
explicit resource types rather than deep type hierarchies.

Beyond structural representation, real-world localization workflows demand:

- **Stable content identity** across extraction cycles for incremental
  processing.
- **Dynamic properties** for extensible metadata.
- **Display hints** that guide UI rendering without coupling the model to any
  particular frontend.
- **A format-independent inline code model** that supports TM matching, AI
  translation, and editor rendering across all source formats.

### The inline-code challenge

Documents contain inline formatting (bold, italic, links, images, variables,
placeholders) embedded within translatable text, and every source format
represents these constructs differently:

| Concept     | HTML           | Markdown             | DOCX (OpenXML)  | XLIFF 2.0                           |
| ----------- | -------------- | -------------------- | --------------- | ----------------------------------- |
| Bold        | `<b>`          | `**`                 | `<w:b/>`        | `<pc type="fmt" subType="xlf:b">`   |
| Link        | `<a href="…">` | `[text](url)`        | `<w:hyperlink>` | `<pc type="link">`                  |
| Line break  | `<br/>`        | two spaces + newline | `<w:br/>`       | `<ph type="fmt" subType="xlf:lb"/>` |
| Placeholder | —              | —                    | —               | `<ph>`                              |

A framework must make these constructs processable in a format-agnostic way —
TM matching, AI translation, QA checks, and terminology lookup must not need
to know whether the bold text came from HTML or Markdown. At the same time,
perfect roundtrip fidelity to the original format is required: a `<b
class="emphasis">` must roundtrip as exactly that, not as a generic bold tag.

### The embedded-content challenge

Documents frequently contain embedded content in a different format: HTML
strings inside JSON values, HTML in CDATA sections of XML, Markdown in CSV
columns. A JSON reader that only sees `"<p>Hello <b>world</b></p>"` as a flat
string misses the inline formatting and produces inferior translation
results.

## Decision

### Part and Resource

A single `Part` struct carries a `PartType` enum and a `Resource`:

```go
type Part struct {
    Type     PartType
    Resource Resource  // Block, Layer, Data, or Media
}

type Resource interface {
    ResourceID() string
}
```

`PartType` values are `PartLayerStart`, `PartLayerEnd`, `PartBlock`,
`PartData`, `PartMedia`, and `PartCustom`.

Resource types:

- **Layer** — structural grouping (document, section, embedded content).
- **Block** — translatable content with source segments and target segments
  per locale.
- **Data** — non-translatable structure (skeleton, metadata).
- **Media** — binary content (images, embedded files).

`PartResult{Part, Error}` carries both content and errors on the same
channel, letting tools decide how to handle errors (skip, retry, fail)
without maintaining separate error channels.

### Block

```go
type Block struct {
    ID          BlockIdentity
    Source      []Segment
    Targets     map[LocaleID][]Segment
    Annotations map[string]Annotation
    Properties  map[string]any
}
```

#### Content-addressable identity

```go
type BlockIdentity struct {
    ContentHash  string  // SHA-256 of normalized source text
    ContextHash  string  // Hash of surrounding context (prev/next blocks)
    SourcePath   string  // XPath, JSON path, or line number (display hint)
    SequenceNum  int     // Order in document
}
```

The `ContentHash` is computed from normalized source text
(whitespace-normalized, inline codes stripped). Combined with `ContextHash`,
this produces stable identity across extraction cycles: the same content
always produces the same identity, so only blocks whose identity has changed
need reprocessing. Identical blocks across documents share the same
`ContentHash`, letting translation memory and AI tools avoid redundant work.

Block identity also carries a separate project-unique internal ID tracked by
the store layer — see [AD-003: Identity](003-identity.md) for the dual-ID
scheme.

#### Dynamic properties

The `Properties` map carries arbitrary key-value metadata that tools and
connectors attach as blocks flow through the pipeline. Examples:

- `"translation-origin": "tm"` — how the translation was produced
- `"segment-count": 3` — number of segments in the block
- `"word-count": 42` — count from the wordcount tool
- `"cms-path": "/en/blog/post-1"` — source location in a CMS

Properties are serialized and carried through the pipeline. Tools add
metadata without content-model changes. This replaces the pattern of adding
dedicated fields for every new piece of metadata.

#### Annotations

Blocks carry an `Annotations` map for metadata produced by pipeline tools.
Each annotation implements the `Annotation` interface:

```go
type Annotation interface {
    AnnotationType() string
}
```

Built-in annotation types include:

| Annotation                | Type Key          | Producer              | Purpose                                |
| ------------------------- | ----------------- | --------------------- | -------------------------------------- |
| `AltTranslation`          | `alt-translation` | TM leverage, AI tools | Alternative translations with scores   |
| `TMMatchAnnotation`       | `tm-match`        | tm-leverage           | Best TM match with fuzzy score         |
| `TermAnnotation`          | `term`            | term-lookup           | Matched terminology with target terms  |
| `TermCandidateAnnotation` | `term-candidate`  | ai-terminology        | Term extraction candidates from LLM    |
| `EntityAnnotation`        | `entity`          | entity-annotate       | Named entities (people, places, dates) |
| `QAFindingAnnotation`     | `qa-finding`      | qa-check              | Quality check findings with severity   |
| `WordCountAnnotation`     | `word-count`      | word-count            | Token and character counts             |

Annotations are keyed by type and instance (`"term:0"`, `"term:1"`) to
support multiple annotations of the same type per Block. They carry
character-level positions via `TextRange` (start/end offsets), enabling
precise inline highlighting without re-detecting boundaries at render time.

Tools communicate by reading annotations produced upstream and writing their
own downstream, keeping tools loosely coupled through the shared data model
rather than direct dependencies.

### Fragment and coded text

Text with inline formatting is a `Fragment` using coded text. Unicode
Private-Use-Area markers replace inline markup within the text string. Each
marker maps positionally to a `Span` carrying the code's metadata:

```go
const (
    MarkerOpening     rune = '\uE001'
    MarkerClosing     rune = '\uE002'
    MarkerPlaceholder rune = '\uE003'
)

type Fragment struct {
    CodedText string  // Text with PUA markers at span positions
    Spans     []*Span // Metadata per marker, indexed positionally
}
```

The Nth marker character in `CodedText` corresponds to `Spans[N]`. Three
marker runes distinguish opening, closing, and placeholder (standalone)
codes. Tools process `CodedText` as a simple string (skipping marker runes)
while preserving complete inline-code information.

### Span: six layers

Each `Span` carries all six layers of the inline-code model:

```go
type Span struct {
    // Layer 2: Abstract identity
    SpanType    SpanType  // Opening, Closing, or Placeholder
    ID          string    // Sequential per-fragment: "1", "2", "3"

    // Layer 3: Semantic type
    Type        string    // From semantic type vocabulary
    SubType     string    // Format-specific refinement (e.g., "xlf:b")

    // Layer 1: Native markup
    Data        string    // Native format markup (e.g., "<b>", "**", "<w:b/>")
    OuterData   string    // Outer context when Data alone is insufficient

    // Layer 4: Display text
    DisplayText string    // Translator-facing label: "[B]", "[/B]", "[IMG]"

    // Layer 5: Text equivalent
    EquivText   string    // Plain text equivalent ("\n" for <br>, "" for bold)

    // Layer 6: Editing constraints
    Deletable   bool
    Cloneable   bool
    CanReorder  bool

    // Internal tracking
    OriginalID  string    // ID before merge/split operations
    Flags       int       // SpanFlagHasRef, SpanFlagAdded, SpanFlagMerged, etc.
    Annotations map[string]Annotation
}
```

### Semantic type vocabulary

The `Type` field uses a defined vocabulary of format-independent semantic
types, grouped into categories by namespace prefix:

**Formatting** (`fmt:`):

| Type                | Meaning             | HTML              | Markdown | DOCX                                 |
| ------------------- | ------------------- | ----------------- | -------- | ------------------------------------ |
| `fmt:bold`          | Bold text           | `<b>`, `<strong>` | `**`     | `<w:b/>`                             |
| `fmt:italic`        | Italic text         | `<i>`, `<em>`     | `*`, `_` | `<w:i/>`                             |
| `fmt:underline`     | Underlined text     | `<u>`             | —        | `<w:u/>`                             |
| `fmt:strikethrough` | Struck-through text | `<s>`, `<del>`    | `~~`     | `<w:strike/>`                        |
| `fmt:subscript`     | Subscript text      | `<sub>`           | —        | `<w:vertAlign w:val="subscript"/>`   |
| `fmt:superscript`   | Superscript text    | `<sup>`           | —        | `<w:vertAlign w:val="superscript"/>` |
| `fmt:code`          | Inline code         | `<code>`          | `` ` ``  | —                                    |
| `fmt:highlight`     | Highlighted text    | `<mark>`          | —        | `<w:highlight/>`                     |

**Linking** (`link:`):

| Type             | Meaning         | HTML                  | Markdown          |
| ---------------- | --------------- | --------------------- | ----------------- |
| `link:hyperlink` | Hyperlink       | `<a href="…">`        | `[text](url)`     |
| `link:crossref`  | Cross-reference | `<a href="#id">`      | `[text](#anchor)` |
| `link:email`     | Email link      | `<a href="mailto:…">` | —                 |

**Media** (`media:`):

| Type          | Meaning      | HTML      | Markdown      |
| ------------- | ------------ | --------- | ------------- |
| `media:image` | Inline image | `<img>`   | `![alt](url)` |
| `media:video` | Inline video | `<video>` | —             |
| `media:audio` | Inline audio | `<audio>` | —             |

**Structure** (`struct:`):

| Type               | Meaning            | HTML     | Markdown |
| ------------------ | ------------------ | -------- | -------- |
| `struct:break`     | Line break         | `<br>`   | `  \n`   |
| `struct:pagebreak` | Page break         | —        | —        |
| `struct:footnote`  | Footnote reference | —        | `[^id]`  |
| `struct:ruby`      | Ruby annotation    | `<ruby>` | —        |

**Code** (`code:`) — non-translatable inline tokens:

| Type               | Meaning                  | Examples                       |
| ------------------ | ------------------------ | ------------------------------ |
| `code:variable`    | Named variable           | `{name}`, `$name`, `%s`        |
| `code:placeholder` | Positional placeholder   | `{0}`, `%1$s`                  |
| `code:function`    | ICU function             | `{count, plural, …}`           |
| `code:markup`      | Generic preserved markup | arbitrary format-specific tags |

**Entity** (`entity:`) — also used by entity annotations:

| Type                  | Meaning           |
| --------------------- | ----------------- |
| `entity:person`       | Person name       |
| `entity:organization` | Organization name |
| `entity:product`      | Product name      |
| `entity:location`     | Place name        |
| `entity:date`         | Date value        |
| `entity:time`         | Time value        |
| `entity:currency`     | Currency amount   |
| `entity:measurement`  | Measurement value |

### Format-specific refinement via SubType

The `SubType` field provides format-specific refinement using a prefix
convention. Reserved prefixes:

- `xlf:` — XLIFF 2.0 subtypes (`xlf:b`, `xlf:i`, `xlf:u`, `xlf:lb`, `xlf:pb`, `xlf:var`)
- `html:` — HTML element names (`html:span`, `html:div`, `html:em`)
- `md:` — Markdown constructs (`md:emphasis`, `md:strong`)
- `docx:` — OpenXML run properties (`docx:w:b`, `docx:w:i`)

Custom subtypes use a reverse-domain prefix: `com.acme:custom-tag`.

### Span ID assignment

Format readers assign sequential numeric IDs to spans within each Fragment.
Opening and closing spans that form a pair share the same ID; placeholder
spans get their own ID. IDs start at `"1"`:

```
Input:  Click <b>here</b> for <a href="x">info</a>.
Spans:  [Opening id="1"] [Closing id="1"] [Opening id="2"] [Closing id="2"]
```

This produces stable structural keys for TM matching: HTML `<b>Click</b>`
and Markdown `**Click**` both yield `{1}Click{/1}` — TM entries created from
HTML match Markdown sources at the structural tier.

### Fragment text projections

Fragment provides multiple views of its content:

```go
// Plain text — markers stripped, spans ignored.
// Use: word count, search, QA text comparison.
func (f *Fragment) Text() string

// Structural text — markers replaced by numbered placeholders.
// Use: TM exact matching (structural tier).
func (f *Fragment) StructuralText() string

// Generalized text — entity spans become typed placeholders ({PERSON}),
// structural spans become numbered.
// Use: TM generalized matching (entity-agnostic tier).
func (f *Fragment) GeneralizedText() string

// Semantic HTML — renders spans as semantic HTML elements.
// Use: MT APIs (DeepL tag_handling=html, Google mime_type=text/html),
// translation editor preview, AI translation with context.
func (f *Fragment) SemanticHTML() string

// Placeholder text — numbered XML placeholders for LLM prompts.
// <x id="1"/>text<x id="/1"/> format.
// Use: LLM translation prompts where tag preservation is critical.
func (f *Fragment) PlaceholderText() string
```

Example, a Fragment `"Click \uE001here\uE002 for info"` with
`Span[0]={SpanOpening, Type:"fmt:bold", ID:"1", Data:"<b class='x'>"}` and
`Span[1]={SpanClosing, Type:"fmt:bold", ID:"1", Data:"</b>"}`:

```
Text():            "Click here for info"
StructuralText():  "Click {1}here{/1} for info"
SemanticHTML():    "Click <b>here</b> for info"
PlaceholderText(): "Click <x id=\"1\"/>here<x id=\"/1\"/> for info"
```

The semantic-type-to-HTML mapping is defined by a small `SemanticHTMLMap`.
Unknown types fall back to `<span data-type="…">`.

### Run sequences as canonical inline content

`Block.Source` and `Block.Targets` are sequences of `Segment`, and each
`Segment.Runs` is a flat `[]Run` — a discriminated union that is the
**canonical representation of inline content** in neokapi:

```go
type Run struct {
    Text   *TextRun        // plain text chunk
    Ph     *PlaceholderRun // self-closing: variable, icon, <br>, redaction
    PcOpen *PcOpenRun      // opening half of a paired code (<a>, <b>, …)
    PcClose *PcCloseRun    // closing half of a paired code (</a>, </b>, …)
    Sub    *SubRun         // reference to a nested Block (subfilter output)
    Plural *PluralRun      // ICU plural with per-form Runs
    Select *SelectRun      // ICU select with per-case Runs
}
```

Every `PcOpenRun` carries an `id` and is paired with a matching
`PcCloseRun` of the same `id` later in the same Run sequence. Pairs nest
LIFO; runs may appear inside plural / select forms with their own scope.

`Run[]` is isomorphic to `Fragment{CodedText, Spans}` — `RunsToFragment` and
the inverse `MarshalRuns` / `UnmarshalRuns` (`core/model/coded_text.go`)
bridge the two whenever code paths still consume the `CodedText` form for
persistence or XLIFF round-trip. Going forward `Run[]` is the canonical
form; `Fragment` is the persistence/bridge view.

### Boundaries: structural canonical, projections at consumers

The neokapi inline-code model is **structural-canonical**. `Run[]` is the
single source of truth for inline content inside a Segment. Every other
representation that crosses a boundary — to a translator, an LLM, an MT
provider, a CAT tool, a runtime, a TM index — is a **projection** computed
from `Run[]` on demand.

This separation is deliberate:

- **Structural inside.** Every internal pipeline component (filters, tools,
  store, editor, runtime resolvers) reads and writes `Run[]`. Type-rich,
  format-agnostic, lossless.
- **Textual at boundaries.** Each external consumer gets a textual form
  purpose-built for it. Several projections coexist; each is tuned to the
  consumer's expectations and quality characteristics.

The framework provides:

| Projection                     | Surface                          | Consumer                                                                  |
| ------------------------------ | -------------------------------- | ------------------------------------------------------------------------- |
| `Run[]` (no projection)        | `Segment.Runs`, KLF JSON wire    | Pipeline tools, store, format readers/writers                             |
| `RenderRunsWithData(runs)`     | native source markup             | Format writers (HTML, Markdown, XLIFF fallback) — replays `Data` verbatim |
| `RunsStructuralText(runs)`     | `Click {1}here{/1} for info`     | TM matching (structural tier) — cross-format leverage                     |
| `RunsGeneralizedText(runs)`    | structural + entity placeholders | TM matching (generalized tier)                                            |
| `RunsPlaceholderText(runs)`    | `<x id="1"/>here<x id="/1"/>`    | LLM prompts where tag preservation is critical                            |
| `RunsSemanticHTML(runs, reg)`  | `<a href="…">here</a>`           | Commercial MT (DeepL, Google) and HTML-style LLM prompts                  |
| `flattenRuns(runs)` (TS)       | `Click {=m0}here{/=m0}`          | ICU runtime, kapi-react `__tx` re-attach                                  |
| `runsToCoded(runs)` (TS)       | PUA-marker text + `SpanInfo[]`   | Visual editor (chips, formatting, semantic spans rendered as styled text) |
| `Fragment.CodedText` + `Spans` | PUA-marker text + `Span[]`       | Persistence bridge, XLIFF round-trip via `Span.Data`                      |

Two consequences fall out of the convention:

1. **No single "translator format."** A user editing in the framework's
   visual editor sees nested chips with semantic formatting (`<b>` rendered
   bold); the same Block in an external CAT tool comes through as XLIFF
   `<pc>`; the same Block sent to an LLM goes as `RunsPlaceholderText` or
   `RunsSemanticHTML`. The structural Block is identical; each consumer
   renders it differently.
2. **Format extensions follow the same rule.** A new format reader, a new
   extractor (e.g., kapi-react), a new translator surface — each emits
   `Run[]` and lets the framework's existing projections handle every
   consumer. New textual conventions are only introduced when an existing
   projection is genuinely insufficient.

### Reader and writer contracts

**Readers** populate all six span layers when producing Fragments:

1. **ID** — sequential numeric, paired opening/closing share the same ID.
2. **Type** — from the semantic-type vocabulary.
3. **Data** — verbatim native markup for roundtrip fidelity.
4. **DisplayText** — short human-readable label (`"[B]"`, `"[IMG]"`).
5. **EquivText** — plain text equivalent where applicable.
6. **Editing constraints** — `Deletable`, `Cloneable`, `CanReorder` based on
   format semantics.

**Writers** reconstruct output using `Span.Data` (the native markup), not the
semantic type. This ensures perfect roundtrip fidelity — the writer replays
exactly what the reader captured:

```go
func renderFragment(frag *Fragment) string {
    var buf strings.Builder
    spanIdx := 0
    for _, r := range frag.CodedText {
        if isMarker(r) && spanIdx < len(frag.Spans) {
            buf.WriteString(frag.Spans[spanIdx].Data)
            spanIdx++
        } else {
            buf.WriteRune(r)
        }
    }
    return buf.String()
}
```

### Layers and embedded content

Embedded content is modeled as nested Layers. A Layer carries its own
DataFormat identifier and a `ParentID` linking it to the enclosing layer.
When a format reader encounters embedded content (e.g., an HTML string
inside a JSON value), it emits a child Layer with `Format: "html"`
containing the parsed HTML Blocks, nested between the parent Layer's Parts:

```
PartLayerStart (format="json", id="doc1")
  PartBlock (key: "title", text: "Hello")
  PartLayerStart (format="html", id="sf1", parentID="doc1")
    PartBlock ("Welcome to <b>our site</b>")
  PartLayerEnd (id="sf1")
  PartData (structural JSON)
PartLayerEnd (id="doc1")
```

Each Layer is independently processable by format-aware tools. Layers nest
recursively: HTML in JSON in YAML is three levels deep with no special
cases.

### SubfilterResolver

Format-to-format embedding is coordinated by a small interface:

```go
type SubfilterResolver interface {
    ResolveReader(formatName string) (DataFormatReader, error)
    ResolveWriter(formatName string) (DataFormatWriter, error)
}
```

`FormatRegistry` implements this through its `NewReader` / `NewWriter`
methods. The interface decouples format readers from the registry, prevents
circular imports, and enables test mocks.

Readers and writers that support subfiltering implement a marker interface:

```go
type SubfilterAware interface {
    SetSubfilterResolver(r SubfilterResolver)
}
```

The resolver is injected before `Open` / `Write` is called. Any registered
format (native, plugin, or bridge) can serve as a subfilter.

Format configs declare subfilter mappings that bind content locations to a
format reader:

```yaml
subfilters:
  - pattern: "*.body"
    format: html
  - pattern: "*.description"
    format: markdown
```

Patterns use `filepath.Match` semantics with `.` as the path separator. JSON
readers use key-path globs; XML readers use element-path patterns.

When a reader encounters content matching a subfilter pattern, it emits
`PartLayerStart`, delegates extraction to the sub-reader resolved via the
`SubfilterResolver`, and emits `PartLayerEnd` when the sub-reader finishes.
Writers buffer all parts between matching Layer boundaries, delegate to the
sub-writer, and insert the rendered string into the parent format.

### Integration with AI, MT, and TM

AI tools and MT providers pick the appropriate Fragment projection based on
the backend's tag-handling capability:

- **Commercial MT APIs** (DeepL, Google, Amazon) — use `SemanticHTML()`. The
  API preserves the semantic HTML tags; the framework restores the native
  markup from the original Span data.
- **LLM translation** — use `PlaceholderText()` or `SemanticHTML()` depending
  on prompt strategy. `ParsePlaceholderText` reconstructs a Fragment from
  the LLM response by matching placeholder tags back to source Spans.
- **TM matching** — three-tier matching uses `Text()`, `StructuralText()`,
  and `GeneralizedText()` in order. Because structural keys use semantic
  types and not native markup, TM entries created from HTML match Markdown
  at the structural tier.

## Consequences

- Type dispatch via `switch part.Type` replaces instanceof; linters provide
  compile-time exhaustiveness.
- Adding new resource types requires only a new PartType constant and a
  struct implementing `Resource`.
- Tools that only handle Blocks ignore all other Part types via the BaseTool
  pass-through behavior ([AD-006: Tool System](006-tool-system.md)).
- The Part stream remains a single ordered channel; no fan-out complexity in
  the base pipeline.
- Content-addressable identity enables incremental extraction and
  deduplication across documents.
- Dynamic properties and annotations let tools and connectors carry metadata
  without content-model changes.
- The semantic-type abstraction lets TM match across formats and lets AI
  prompts receive consistent inline-code representations.
- Writers replay `Span.Data` verbatim, so roundtrip fidelity is a property of
  the model, not of each format's implementation.
- Layers nest recursively with no special cases — embedded content is a
  first-class pipeline citizen.
- The six-layer Span design aligns with XLIFF 2.0, making XLIFF
  serialization a natural mapping rather than a lossy conversion.

## Related

- [AD-001: Vision and Module Architecture](001-vision-and-modules.md)
- [AD-003: Identity](003-identity.md) — project-unique internal IDs
- [AD-004: Processing Engine](004-processing-engine.md) — how Parts stream
- [AD-005: Format System](005-format-system.md) — readers/writers that produce Parts
- [AD-006: Tool System](006-tool-system.md) — tools that consume Parts
