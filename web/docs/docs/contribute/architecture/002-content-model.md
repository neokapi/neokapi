---
id: 002-content-model
sidebar_position: 2
title: "AD-002: Content Model"
description: "Architecture decision: documents are represented as a stream of Part values (Layer, Block, Data, Media) and a Block's inline content is a flat sequence of Run values, so that tools and translations work independently of source file format."
keywords: [content model, Part, Block, Run, Segment, overlay, stand-off annotation, segmentation, Layer, architecture decision, neokapi]
---

# AD-002: Content Model

## Summary

Documents in neokapi are represented as a stream of `Part` values, each
carrying a `PartType` discriminator and a `Resource`. Translatable content is
a `Block`; a Block's content is a flat `[]Run` per locale — a discriminated
union (`Text`, `Ph`, `PcOpen`, `PcClose`, `Sub`, `Plural`, `Select`) defined
by RFC 0001. Inline-code runs carry the metadata that the older model spread
across six span "layers" (native markup, abstract identity, semantic type,
display text, text equivalent, editing constraints) directly on their fields,
so format-aware tools can process content semantically while writers roundtrip
native markup exactly. Interpretations *of* that content — sentence
segmentation, terminology, entities, QA findings — are **stand-off overlays**:
typed, run-anchored span sets layered over the runs on demand, never rewriting
them. `Segment` is not a structural type; a segment is a span in a
segmentation overlay. Targets are first-class records keyed by a *variant* —
locale plus optional tone or channel — not bare locale-keyed strings.

## Context

A localization content model must represent translatable documents in a way
that is format-independent, type-safe, extensible, and able to represent
recursive embedded content naturally. Go's composition and interface system
(no class inheritance) shapes the design toward discriminated unions and
explicit resource types rather than deep type hierarchies. Both the Part
stream and the inline-content model are discriminated unions — one keyed by
`PartType`, the other by which `Run` field is set.

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
- **Block** — translatable content: a source run sequence and per-locale
  target run sequences, plus optional stand-off overlays.
- **Data** — non-translatable structure (skeleton, metadata).
- **Media** — binary content (images, embedded files).

`PartResult{Part, Error}` carries both content and errors on the same
channel, letting tools decide how to handle errors (skip, retry, fail)
without maintaining separate error channels.

### Block

```go
type Block struct {
    ID           string
    SourceLocale LocaleID
    Source       []Run                  // whole source content
    Targets      map[VariantKey]*Target // first-class targets, keyed by variant
    Overlays     []Overlay              // opt-in stand-off interpretations (usually none)
    Identity     *BlockIdentity        // content-addressable hash for dedup
    Annotations  map[string]Annotation // block-level (non-positional) metadata
    Properties   map[string]string
    // …skeleton link, display hint, whitespace flag, etc.
}
```

A Block holds one source run sequence and one target run sequence per locale —
the whole content, unsegmented. There is no `Segment` container: most blocks
*are* a single string and its translations, and the model says exactly that.
When a workflow needs sentence boundaries (review UI, exact-match TM keys,
XLIFF/TMX export), a flow tool computes them and attaches a **segmentation
overlay** (see [Stand-off overlays](#stand-off-overlays-segmentation-terminology-entities)).
The overlay is layered over the runs; the runs are never repartitioned, so
segmentation is reversible by construction — dropping the overlay restores the
unsegmented content with no loss.

#### Targets and variants

A target is a **first-class record**, not a bare run sequence, and it is keyed
by a **variant** rather than a bare locale:

```go
// VariantKey identifies a target variant. Locale is the only required
// dimension; tone, channel, and any future axis are optional and zero-valued
// by default, so the common case carries no extra ceremony.
type VariantKey struct {
    Locale  LocaleID // required
    Tone    string   // optional ("" = unspecified)
    Channel string   // optional
}

// Target is the committed translation for one variant: content plus its
// lifecycle and provenance. Candidate/alternative translations (TM, MT, AI
// proposals) remain stand-off overlays; Target is the chosen one.
type Target struct {
    Runs   []Run
    Status TargetStatus // draft | translated | reviewed | signed-off
    Origin Origin       // human | tm | mt | ai, plus engine, tool, timestamp, author
    Score  float64
}
```

Ergonomic accessors keep the locale-only path a one-liner: `block.Target("fr-FR")`
resolves `VariantKey{Locale: "fr-FR"}`, while `block.TargetVariant(key)` reaches
the general case. Code that only knows about locales never has to mention tone or
channel — richer variants are strictly opt-in. A `Target`'s `Runs` carry their own
overlays (target-side segmentation, target terms), scoped to that variant.

This separates two things the older `map[LocaleID][]Run` conflated: the
**committed translation per variant** (a `Target`, with status and provenance)
from **candidate proposals** (stand-off `alt-translation` overlays, possibly many
per variant, each scored). The in-flight `Target` holds the current committed
translation; accumulated history across runs and review trails are a
persistence-layer concern, outside the content model.

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
- `"word-count": 42` — count from the wordcount tool
- `"cms-path": "/en/blog/post-1"` — source location in a CMS

Properties are serialized and carried through the pipeline. Tools add
metadata without content-model changes. This replaces the pattern of adding
dedicated fields for every new piece of metadata.

#### Annotations

Block-level, non-positional metadata produced by pipeline tools lives in the
`Annotations` map. Each annotation implements the `Annotation` interface:

```go
type Annotation interface {
    AnnotationType() string
}
```

Interpretations fall into two carriers. **Positional** ones that point into the
content — segmentation, terminology, entities, QA findings, source↔target
alignment — are **stand-off overlays** with run-anchored spans (see
[Stand-off overlays](#stand-off-overlays-segmentation-terminology-entities)).
**Non-positional** ones that describe the block as a whole stay in the
`Annotations` map:

| Interpretation   | Type Key          | Carrier          | Producer              | Purpose                                |
| ---------------- | ----------------- | ---------------- | --------------------- | -------------------------------------- |
| Segmentation     | `segmentation`    | overlay (spans)  | segment annotator     | Sentence / chunk boundaries over runs  |
| Terminology      | `term`            | overlay (spans)  | term-lookup           | Matched terminology with target terms  |
| Term candidates  | `term-candidate`  | overlay (spans)  | ai-terminology        | Term extraction candidates from an LLM |
| Entities         | `entity`          | overlay (spans)  | entity-annotate       | Named entities (people, places, dates) |
| QA findings      | `qa-finding`      | overlay (spans)  | qa-check              | Quality findings with severity         |
| Alignment        | `alignment`       | overlay (spans)  | aligner, readers      | Source-span ↔ target-span links        |
| Alt-translations | `alt-translation` | block annotation | TM leverage, AI tools | Candidate translations with scores     |
| Best TM match    | `tm-match`        | block annotation | tm-leverage           | Best TM match with fuzzy score         |
| Word count       | `word-count`      | block annotation | word-count            | Token and character counts             |

Block annotations are keyed by type and instance (`"alt-translation:0"`,
`"alt-translation:1"`) to support several of one type per Block.

Tools communicate by reading annotations and overlays produced upstream and
writing their own downstream, keeping tools loosely coupled through the shared
data model rather than direct dependencies.

### The Run sequence

A Block's content is not a string with embedded markers — it is a flat
sequence of `Run` values, held directly on the Block (`Source`, and each
`Targets[variant].Runs`):

`Run` is a discriminated union: exactly one of its pointer fields is set,
which is the run's *kind*. `Run.Kind()` returns the discriminator and
`Run.RunID()` returns the id for the kinds that carry one.

```go
type Run struct {
    Text    *TextRun        // plain text chunk
    Ph      *PlaceholderRun // self-closing: variable, icon, <br>, redaction
    PcOpen  *PcOpenRun      // opening half of a paired code (<a>, <b>, …)
    PcClose *PcCloseRun     // closing half of a paired code (</a>, </b>, …)
    Sub     *SubRun         // reference to a nested Block (subfilter output)
    Plural  *PluralRun      // ICU plural with per-form Runs
    Select  *SelectRun      // ICU select with per-case Runs
}
```

Text and inline codes interleave positionally in the slice; there is no
parallel side-table and no marker characters. A reader builds the slice by
appending a `TextRun` for each text chunk and an inline-code run for each
construct it encounters (see `core/formats/*/run_builder.go`).

### Stand-off overlays (segmentation, terminology, entities) {#stand-off-overlays-segmentation-terminology-entities}

The runs are the content. Everything that *interprets* the content — where the
sentence boundaries fall, which spans are terms or named entities, what a QA
check flagged, how a source span aligns to a target span — is a **stand-off
overlay**: a typed set of spans anchored to a run sequence, layered over it
without rewriting it. This is the one mechanism for every positional
interpretation; segmentation is simply the overlay of type `segmentation`.

```go
// Overlay is a typed stand-off layer over one side of a Block.
type Overlay struct {
    Type    OverlayType // "segmentation" | "term" | "entity" | "qa" | "alignment" | …
    Variant *VariantKey // which run sequence it annotates; nil = source
    Spans   []Span
}

type Span struct {
    Range   RunRange // run-anchored, never a flattened-string offset
    Payload any      // type-specific: segment id, term, entity kind, score, alignment ref
}

// RunRange anchors a span on the run sequence — a start and end position plus
// an intra-text-run character offset — so boundaries stay stable across inline
// codes and survive run-preserving edits. Inline-code runs already carry ids;
// text runs carry stable ids for the same purpose.
type RunRange struct {
    StartRun, StartOffset int
    EndRun,   EndOffset   int
}
```

Four properties follow from anchoring interpretations to runs rather than
baking them into structure:

- **Segmentation is opt-in and dynamic.** A `segment` flow tool — backed by an
  SRX ruleset, a UAX-29 baseline, or an ML/LLM segmenter — computes boundaries
  and writes a segmentation overlay. Nothing runs it by default; whole-block
  content is the norm, which is also what document-level AI translation wants
  for coherence. Several segmentations can coexist (e.g. `sentence` and a
  token-budgeted `llm-chunk`), each its own overlay.
- **It is reversible by construction.** Desegmentation is "drop the overlay."
  There is no inverse operation to get wrong and no inter-segment "ignorable"
  material to lose — the gaps between segment spans are simply runs no span
  covers.
- **It is uniform.** Terminology, entities, and QA findings are the same kind
  of overlay, anchored the same run-aware way, rather than each re-detecting
  boundaries at render time.
- **Leverage is hybrid.** TM matching works at the whole-block level (including
  embedding/semantic similarity) and, when a segmentation overlay is present,
  also computes exact and structural keys per segment span via the Run
  projections below.

### Inline-code runs carry the former span "layers"

The earlier content model spread inline-code metadata across six "span
layers." Those layers now live directly on the inline-code run structs.
`PlaceholderRun` and `PcOpenRun` carry the full set:

```go
type PcOpenRun struct {
    ID          string          // abstract identity; shared with the matching PcClose
    Type        string          // semantic type from the vocabulary ("fmt:bold")
    SubType     string          // format-specific refinement ("html:b", "xlf:b")
    Data        string          // native markup, replayed verbatim by writers ("<b class='x'>")
    Equiv       string          // plain-text equivalent ("" for bold, "\n" for <br>)
    Disp        string          // translator-facing label ("[B]")
    Constraints *RunConstraints // editing constraints
}

type RunConstraints struct {
    Deletable   bool
    Cloneable   bool
    Reorderable bool
}
```

`PlaceholderRun` has the same shape (it is self-closing, so it has no pairing
partner). `PcCloseRun` is the closing half of a paired code and is leaner — it
shares `ID` with its `PcOpen` and replays its own `Data`, but it has no
`Constraints` field because the closing half inherits its opener's behavior.
The six concerns map onto these fields as follows:

| Former span "layer" | Run field                  |
| ------------------- | -------------------------- |
| Abstract identity   | `ID` (+ `Kind()`)          |
| Semantic type       | `Type`, `SubType`          |
| Native markup       | `Data`                     |
| Display text        | `Disp`                     |
| Text equivalent     | `Equiv`                    |
| Editing constraints | `Constraints`              |

`SubRun` references a subblock produced by a subfilter (`ID`, `Ref`, `Equiv`);
`PluralRun` / `SelectRun` are structured ICU constructs whose branches are
themselves `[]Run`.

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

### Run ID assignment and pairing

Format readers assign sequential numeric IDs to inline-code runs within each
run sequence. A `PcOpenRun` and the `PcCloseRun` that closes it share the same
`ID`; a `PlaceholderRun` gets its own. IDs start at `"1"`. Pairs nest LIFO,
and runs inside a `Plural`/`Select` branch form their own scope:

```
Input: Click <b>here</b> for <a href="x">info</a>.
Runs:  TextRun "Click "
       PcOpen{ID:"1", Type:"fmt:bold"}        TextRun "here"  PcClose{ID:"1"}
       TextRun " for "
       PcOpen{ID:"2", Type:"link:hyperlink"}  TextRun "info"  PcClose{ID:"2"}
       TextRun "."
```

This produces stable structural keys for TM matching: HTML `<b>Click</b>`
and Markdown `**Click**` both yield `{1}Click{/1}` — TM entries created from
HTML match Markdown sources at the structural tier.

### Run text projections

A Run sequence is the single source of truth; every textual form that crosses
a boundary is a **projection** computed from `[]Run` on demand. The framework
provides (in `core/model/`):

```go
// Plain flattening — TextRun content verbatim, placeholders contribute
// {equiv}, paired codes contribute their inner content, plural/select take
// the 'other' branch. Use: word count, search, QA text comparison. A
// text-only variant (inline codes contribute nothing) backs plain-text views.
func FlattenRuns(runs []Run) string

// Structural text — inline-code runs become numbered placeholders ({1},
// {/1}, {2/}). Use: TM exact matching (structural tier).
func RunsStructuralText(runs []Run) string

// Generalized text — entity Ph runs become typed placeholders ({PERSON}),
// other inline codes become numbered. Use: TM generalized matching.
func RunsGeneralizedText(runs []Run) string

// Markup-preserving render — re-emits each run's captured Data verbatim.
// Use: HTML/XML/Markdown writers splicing opaque markup back into a string.
func RenderRunsWithData(runs []Run) string
```

Example, the Run sequence `TextRun "Click "`,
`PcOpen{ID:"1", Type:"fmt:bold", Data:"<b class='x'>"}`, `TextRun "here"`,
`PcClose{ID:"1", Data:"</b>"}`, `TextRun " for info"`:

```
FlattenRuns():        "Click here for info"
RunsStructuralText(): "Click {1}here{/1} for info"
RenderRunsWithData(): "Click <b class='x'>here</b> for info"
```

Higher-level consumers layer further projections on top of the same `[]Run`:
the TypeScript side renders a PUA-coded form for the visual editor's styled
chips, an `<x id="1"/>…` placeholder form for LLM prompts, and semantic HTML
for commercial MT. The Block is identical; each consumer renders it
differently.

> **Historical note.** An earlier model represented inline content as a
> `Fragment{CodedText, Spans}` pair — text with Unicode private-use-area
> markers plus a positional `[]*Span` side-table — bridged to runs by
> `RunsToFragment` / `MarshalRuns` / `UnmarshalRuns` in a
> `core/model/coded_text.go`. That bridge has been removed (RFC 0001):
> `[]Run` is now the sole inline-content representation, with no `Fragment`,
> `Span`, or coded-text marker types.

> **Historical note.** An earlier model made segmentation structural —
> `Block.Source []*Segment`, each `Segment` owning its own `[]Run` — and the
> segmentation tool rewrote that structure in place. Segmentation is now a
> stand-off overlay over a single per-locale run sequence; `Segment` is a span,
> not a type. Bilingual formats that carry sentence segments (XLIFF, TMX)
> project to and from segmentation + alignment overlays at the reader/writer
> boundary.

### Boundaries: structural canonical, projections at consumers

The neokapi inline-code model is **structural-canonical**. `Run[]` is the
single source of truth for a Block's content. Every other
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
| `Run[]` (no projection)        | `Block.Source` / `Targets`, KLF wire | Pipeline tools, store, format readers/writers                         |
| `RenderRunsWithData(runs)`     | native source markup             | Format writers (HTML, Markdown, XLIFF fallback) — replays `Data` verbatim |
| `RunsStructuralText(runs)`     | `Click {1}here{/1} for info`     | TM matching (structural tier) — cross-format leverage                     |
| `RunsGeneralizedText(runs)`    | structural + entity placeholders | TM matching (generalized tier)                                            |
| `RunsPlaceholderText(runs)`    | `<x id="1"/>here<x id="/1"/>`    | LLM prompts where tag preservation is critical                            |
| `RunsSemanticHTML(runs, reg)`  | `<a href="…">here</a>`           | Commercial MT (DeepL, Google) and HTML-style LLM prompts                  |
| `flattenRuns(runs)` (TS)       | `Click {=m0}here{/=m0}`          | ICU runtime, kapi-react `__tx` re-attach                                  |
| `runsToCoded(runs)` (TS)       | PUA-marker text + `SpanInfo[]`   | Visual editor (chips, formatting, semantic spans rendered as styled text) |

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

**Readers** populate every field on each inline-code run they emit:

1. **ID** — sequential numeric; a paired `PcOpen`/`PcClose` share the same ID.
2. **Type** / **SubType** — from the semantic-type vocabulary plus a
   format-specific refinement.
3. **Data** — verbatim native markup for roundtrip fidelity.
4. **Disp** — short human-readable label (`"[B]"`, `"[IMG]"`).
5. **Equiv** — plain-text equivalent where applicable.
6. **Constraints** — `Deletable`, `Cloneable`, `Reorderable` based on format
   semantics.

**Writers** reconstruct output using `Run.Data` (the native markup), not the
semantic type. This ensures perfect roundtrip fidelity — the writer replays
exactly what the reader captured, which is what `RenderRunsWithData` does:

```go
func RenderRunsWithData(runs []Run) string {
    var buf strings.Builder
    for _, r := range runs {
        switch {
        case r.Text != nil:
            buf.WriteString(r.Text.Text)
        case r.Ph != nil:
            buf.WriteString(r.Ph.Data)
        case r.PcOpen != nil:
            buf.WriteString(r.PcOpen.Data)
        case r.PcClose != nil:
            buf.WriteString(r.PcClose.Data)
        // …Sub replays Ref; Plural/Select recurse into the 'other' branch.
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

AI tools and MT providers pick the appropriate Run projection based on the
backend's tag-handling capability:

- **Commercial MT APIs** (DeepL, Google, Amazon) — use `RunsSemanticHTML`.
  The API preserves the semantic HTML tags; the framework restores the native
  markup from each run's original `Data`.
- **LLM translation** — use `RunsPlaceholderText` or `RunsSemanticHTML`
  depending on prompt strategy. The response is parsed back into a `[]Run` by
  matching placeholder tags to the source runs.
- **TM matching** — three-tier matching uses `FlattenRuns`,
  `RunsStructuralText`, and `RunsGeneralizedText` in order. Because structural
  keys use run IDs and not native markup, TM entries created from HTML match
  Markdown at the structural tier.

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
- Writers replay `Run.Data` verbatim, so roundtrip fidelity is a property of
  the model, not of each format's implementation.
- Layers nest recursively with no special cases — embedded content is a
  first-class pipeline citizen.
- The Run union (paired `PcOpen`/`PcClose`, self-closing `Ph`, structured
  `Plural`/`Select`) aligns with XLIFF 2.0's `<pc>`/`<ph>` model, making XLIFF
  serialization a natural mapping rather than a lossy conversion.
- Segmentation, terminology, entities, and QA share one run-anchored stand-off
  overlay mechanism; segmentation never mutates content, so it is opt-in,
  multi-layered, and losslessly reversible.
- Bilingual interchange formats that carry sentence segments (XLIFF 2.0
  `<segment>`/`<ignorable>`, TMX) project to and from segmentation + alignment
  overlays at the reader/writer boundary — opt-in byte-faithful round-trip —
  without forcing segment structure into the content model.
- Targets are first-class records keyed by a variant (locale plus optional
  tone/channel); committed translations carry status and provenance, candidate
  proposals stay as `alt-translation` overlays, and the variant axis extends
  beyond locale without ceremony at locale-only call sites.

## Related

- [AD-001: Vision and Module Architecture](001-vision-and-modules.md)
- [AD-003: Identity](003-identity.md) — project-unique internal IDs
- [AD-004: Processing Engine](004-processing-engine.md) — how Parts stream
- [AD-005: Format System](005-format-system.md) — readers/writers that produce Parts
- [AD-006: Tool System](006-tool-system.md) — tools that consume Parts
- [AD-017: Bilingual Format Interop](017-bilingual-format-interop.md) — XLIFF/TMX segment + alignment projection
