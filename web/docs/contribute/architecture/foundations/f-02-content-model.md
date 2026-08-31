---
id: f-02-content-model
sidebar_position: 2
title: "F-02: The content model"
description: "Documents are a stream of Part values; a Block's inline content is a flat sequence of Run values; and every interpretation of that content (segmentation, terms, entities, findings) is a stand-off overlay layered over the runs rather than a rewrite of them."
keywords: [content model, Part, Block, Run, overlay, stand-off annotation, anchor, geometry, timing, provenance, Origin, variant, target, architecture decision, neokapi]
---

import { TypeDiagram } from "@neokapi/docs-shared";

# F-02: The content model

## Summary

Documents are a stream of `Part` values, each carrying a `PartType`
discriminator and a `Resource`. Modifiable content is a `Block`, and a Block's
content is a flat `[]Run` per side: a discriminated union of `Text`, `Ph`,
`PcOpen`, `PcClose`, `Sub`, `Plural`, and `Select`. Each inline-code run carries
its own metadata (native markup, abstract identity, semantic type, display text,
text equivalent, canonical attributes, editing constraints), so tools process
content semantically while writers replay native markup exactly.

Interpretations *of* that content (sentence segmentation, terms, entities,
findings, alignment) are **stand-off overlays**: typed, run-anchored span sets
layered over the runs, never rewriting them. There is no `Segment` type; a
segment is a span in a segmentation overlay. Where a Block is **anchored** in its
source follows the medium: a run range in text, a box on a rendered page, a time
span in timed media. Content that was *recognized* rather than parsed records the
extracting engine and a confidence in the same `Origin` a translation carries.
Targets are first-class records keyed by a **variant** (locale plus optional
tone or channel).

## Context

The content model has to represent documents in a way that is
format-independent, type-safe, extensible, and able to express recursive embedded
content naturally. Go's composition and interface system shapes the design toward
discriminated unions and explicit resource types rather than deep type
hierarchies. Both the Part stream and the inline-content model are discriminated
unions: one keyed by `PartType`, the other by which `Run` field is set.

Beyond structural representation, real workflows demand stable content identity
across extraction cycles, extensible metadata, display hints that guide rendering
without coupling the model to a frontend, and a format-independent inline-code
model that serves memory matching, model-backed translation, and editor rendering
alike.

### The inline-code challenge

Documents contain inline formatting (bold, links, images, variables,
placeholders) embedded in the text, and every source format spells these
differently:

| Concept | HTML | Markdown | DOCX (OpenXML) | XLIFF 2.0 |
| --- | --- | --- | --- | --- |
| Bold | `<b>` | `**` | `<w:b/>` | `<pc type="fmt" subType="xlf:b">` |
| Link | `<a href="…">` | `[text](url)` | `<w:hyperlink>` | `<pc type="link">` |
| Line break | `<br/>` | two spaces + newline | `<w:br/>` | `<ph type="fmt" subType="xlf:lb"/>` |
| Placeholder | none | none | none | `<ph>` |

Memory matching, translation, verification, and term lookup must not need to know
whether the bold came from HTML or Markdown. At the same time round-trip fidelity
is non-negotiable: `<b class="emphasis">` must round-trip as exactly that, not as
a generic bold tag.

### The embedded-content challenge

Documents frequently contain content in a different format: HTML inside JSON
values, HTML in XML CDATA, Markdown in spreadsheet columns. A reader that sees
only `"<p>Hello <b>world</b></p>"` as a flat string misses the inline formatting
and produces worse results downstream.

## Decision

### Part and Resource

One `Part` struct carries a `PartType` enum and a `Resource`:

```go
type Part struct {
    Type     PartType
    Resource Resource // Block, Layer, Group, Data, or Media
}

type Resource interface {
    ResourceID() string
}
```

`PartType` values are `PartUnknown` (the zero value), `PartLayerStart`,
`PartLayerEnd`, `PartGroupStart`, `PartGroupEnd`, `PartBlock`, `PartData`,
`PartMedia`, `PartRawDocument`, and `PartCustom`. Constants carry explicit
integer values for wire compatibility: numbers are never renumbered, and retired
slots stay reserved.

<TypeDiagram
  caption="A document is a stream of Parts. Layers nest; a Block holds a flat Run sequence per side plus stand-off overlays."
  boxes={[
    {
      name: "Layer",
      col: 0,
      role: "io",
      self: "nests, embedded content",
      fields: [
        { name: "Format", type: "string" },
        { name: "ParentID", type: "string" },
      ],
    },
    {
      name: "Block",
      col: 1,
      role: "translate",
      fields: [
        { name: "Source", type: "[]Run" },
        { name: "Targets", type: "map[VariantKey]*Target" },
        { name: "Overlays", type: "[]Overlay" },
        { name: "Annotations", type: "map[string]Payload" },
      ],
    },
    { name: "Run", col: 2, sub: "Text | Ph | PcOpen | …" },
    { name: "Overlay", col: 2, role: "qa", sub: "typed span set", fields: [{ name: "Spans", type: "[]Span" }] },
  ]}
  edges={[
    { from: 0, to: 1, label: "contains" },
    { from: 1, to: 2, label: "flat sequence" },
    { from: 1, to: 3, label: "anchored to runs" },
  ]}
/>

Resource types:

- **Layer**: structural grouping (document, section, embedded content),
  delimited by `PartLayerStart` / `PartLayerEnd`.
- **Group**: a nested structural group within a layer, such as a table,
  delimited by `PartGroupStart` / `PartGroupEnd`.
- **Block**: modifiable content: a source run sequence, per-variant target run
  sequences, and optional stand-off overlays.
- **Data**: non-content structure (skeleton, metadata).
- **Media**: binary content. A `Media` is a binary **reference**, never inlined
  bytes: resolution precedence is `BlobKey` (a content-addressed key in a blob
  store) `>` `URI` (an external reference) `>` `Data` (inline, for small
  pipeline-internal assets only). A large raster or media track therefore never
  streams through the Part channel or the plugin boundary; a single helper at the
  consuming boundary materializes the bytes. This is the one binary idiom shared
  by the content model, path-based plugin IO, and model-provider parts
  ([E-07](../engine/e-07-model-providers.md)).

`PartResult{Part, Error}` carries content and errors on the same channel, letting
a tool decide how to handle an error (skip, retry, fail) without a second
channel.

### Block

```go
type Block struct {
    ID           string
    Name         string
    Unit         string                 // durable identity, resolved by reconciliation (F-03)
    Type         string
    MimeType     string
    Translatable bool                   // eligible for modification or extraction
    SourceLocale LocaleID
    SourceStatus SourceStatus           // authoring lifecycle: authored → checked → approved
    Source       []Run                  // whole source content
    Targets      map[VariantKey]*Target // first-class targets, keyed by variant
    Overlays     []Overlay              // positional, run-anchored stand-off layers
    Annotations  map[string]Payload     // block-scoped typed metadata, keyed by type
    Identity     *BlockIdentity         // content-addressable hashes
    Properties   map[string]string      // opaque pass-through metadata only
    // …skeleton link, content ref, display hint, whitespace flag
}
```

A Block holds one source run sequence and one target run sequence per variant:
the whole content, unsegmented. There is no `Segment` container: most blocks
*are* a single string and its translations, and the model says exactly that. When
a workflow needs sentence boundaries (a review surface, exact-match memory keys,
XLIFF or TMX export), a tool computes them and attaches a segmentation overlay.
The overlay layers over the runs; the runs are never repartitioned, so
segmentation is reversible by construction.

`Translatable` is a parse-time classification, not a policy: a reader marks which
Parts are authored content and which are surrounding structure. Blocks left
unmarked stay in the skeleton, untouched by tools that edit, check, or translate.

`SourceStatus` is the source-side counterpart of a target's status. New (`""`)
reads as the authored baseline; a source edit resets it, a clean source check
stamps `checked`, and an explicit approval stamps `approved`.

#### Targets and variants

A target is a **first-class record** carrying status and provenance, keyed by a
**variant** rather than by locale alone:

```go
// Locale is the only required dimension; tone and channel are optional and
// zero-valued by default, so the common case carries no extra ceremony.
type VariantKey struct {
    Locale  LocaleID
    Tone    string
    Channel string
}

// Target is the committed translation for one variant: content plus its
// lifecycle and provenance. Candidate translations (memory hits, machine
// output, model proposals) stay as alt-translation annotations.
type Target struct {
    Runs   []Run
    Status TargetStatus // "" (new) | draft | translated | reviewed | signed-off
    Origin Origin       // how the content was produced, and what governed it
    Score  float64
}
```

Ergonomic accessors keep the locale-only path a one-liner: `block.Target("fr")`
resolves `VariantKey{Locale: "fr"}`, while `block.TargetVariant(key)` reaches the
general case. Code that only knows about locales never has to mention tone or
channel: richer variants are strictly opt-in. A `Target`'s runs carry their own
overlays, scoped to that variant.

This separates two things a bare `map[LocaleID][]Run` conflates: the **committed
translation per variant**, with status and provenance, from **candidate
proposals**, of which there may be many per variant, each scored. Accumulated
history across runs and review trails are a persistence concern, outside the
content model.

#### Content-addressable identity

```go
type BlockIdentity struct {
    ContentHash string // SHA-256 of the whitespace-trimmed source text
    ContextHash string // SHA-256 of name, type, and identifying properties
}
```

The same content always produces the same identity, so only blocks whose identity
has changed need reprocessing, and identical blocks across documents share a
content hash. `Unit` is the durable key that grading the pair against the
previous read produces: a decision, a translation and a history entry are filed
under it, and a venue stores it as the block's source id. A reader leaves it
empty; reconciliation fills it, and `Name` stands in until it does. How the pair
is graded, and how the store key is derived, is [F-03: Identity](f-03-identity.md).

#### Properties

`Properties` carries arbitrary key-value metadata that readers, tools, and
connectors attach as blocks flow through the pipeline: a source path in an
upstream system, a format hint, a structural coordinate. It is **opaque
pass-through metadata only**. Analytic and interpretive results a tool produces
(match scores, word counts, findings) are overlays or annotations, not
properties, and the tool IO contract ([E-03](../engine/e-03-tool-system.md))
declares which a tool consumes and produces.

A property whose key begins with `@` is **advisory**: carried on the block but
skipped when hashing. Readers use it for derived locators (the line a block
starts on, a byte offset): anything that says *where to find this* rather than
*what this is*. A locator moves whenever anything above it moves, so folding one
into the context hash would make an untouched block report as changed every time
a blank line was added earlier in the file.

#### Overlays and annotations: two stand-off carriers

Typed stand-off interpretations of a Block come in two kinds, kept in two fields
because they differ in shape and lifecycle:

- **Overlays** are *positional*: run-anchored span sets that point into the
  content: segmentation, terms, entities, term candidates, findings,
  source-to-target alignment. Each `Span` carries a run `Range` (its position),
  optional string `Props`, and an optional typed payload `Value`. Because ranges
  anchor to runs, a source rewrite shifts them. A transforming tool's edit plan is
  a structured span-to-replacement map, so the framework applier rebases the
  survivors onto the new runs with `model.RemapOverlays`: spans overlapping an
  edit are dropped, the rest shift to follow it. An opaque whole-block rewrite has
  no mapping and drops them.
- **Annotations** are *block-scoped*: a keyed map of typed payloads describing the
  block as a whole: alt-translations, notes, analysis results, format round-trip
  state, and the block's anchor and role facets. A source rewrite does not
  invalidate them.

```go
type Overlay struct {
    Type    OverlayType // segmentation | term | entity | qa | alignment | term-candidate | editor-anchor
    Variant *VariantKey // nil = source side
    Layer   string      // segmentation granularity; LayerPrimary ("") = primary
    Spans   []Span      // each with a run Range, string Props, and a typed Value
}
```

Whether an interpretation is positional is *structural* (it is an `Overlay`)
rather than a runtime flag. Annotations are reached through the
`Anno`/`SetAnno`/`DelAnno`/`AnnoMap` helpers, keyed by type; overlays through
`OverlayOf`/`AddOverlaySpan`/`OverlaySpan`/`RemoveOverlay`.

| Interpretation | Carrier | Purpose |
| --- | --- | --- |
| Segmentation | overlay `segmentation` | Sentence or chunk boundaries over runs |
| Terms | overlay `term` | Matched terms with their target forms |
| Term candidates | overlay `term-candidate` | Extraction candidates awaiting a decision |
| Entities | overlay `entity` | Named entities (people, places, dates) |
| Findings | overlay `qa` | Verification findings with severity |
| Alignment | overlay `alignment` | Source-span to target-span links |
| Alt-translations | annotation `alt-translation` | Candidate translations with scores |
| Editor anchors | overlay `editor-anchor` | An integration binding into the native editor that owns the format (a Word content control, a Figma node), set by a connector rather than a tool |

Both overlay span values and annotation values are typed payloads registered with
one payload registry (`RegisterPayload` / `NewPayload`), so the wire and store
layers rehydrate the typed value from its type name.

Tools communicate by reading the overlays and annotations produced upstream and
writing their own downstream, staying loosely coupled through the shared data
model rather than through direct dependencies.

Two annotations, `structure` and `geometry`, are held in private Block fields
rather than in the map, while every accessor presents them exactly as if they sat
in `Annotations`, so the wire, the stores, and every consumer see no difference.
They are the two set on nearly every structured block rather than occasionally: a
role on any block a format gives structure to, a position on any block that comes
from a grid or a page. A map entry costs around 285 bytes, so a spreadsheet of a
million cells was paying a quarter of a gigabyte to hold one enum and four small
integers per cell. Nothing else in the vocabulary is common enough to be worth
taking out of the map, and the map stays for all of it.

#### Anchoring across media, and source provenance {#anchoring-and-provenance}

Two things about a Block depend on where its content came from: **where it sits in
its source**, and **how its source text was produced**. Both are block-scoped
typed annotations, riding the same carrier as the analytic results above.

**Anchor.** A Block's position in its source is a coordinate, and the coordinate
system follows the medium:

- **Text** anchors by **run position**: the `Anchor` every overlay, finding
  and stand-off annotation already uses. Run anchoring is the text facet of
  one general idea rather than a separate concept.
- **Rendered media** (an image, a page) anchors **spatially**: a `geometry`
  annotation holding a page and a bounding box.
- **Timed media** (audio, video) anchors **temporally**: a `timing` annotation
  recording the time span in milliseconds.

A Block carries whichever facets its medium defines, and they compose: text
burned into a video frame carries both a `geometry` box and a `timing` span.
Alongside anchor, a `structure` annotation records the block's logical **role**
and plane independent of medium. The role vocabulary (`core/model/structure.go`)
is broad and aligned with document-structure standards: paragraph, title,
heading, caption, table cell, code, formula, picture, list, footnote, section
roles, and the form-field family. Where a block's name embeds the text of its
ancestors, the `structure` annotation also carries `Address`, the same
structural path with each such segment replaced by that ancestor's own identity,
so the block reads the same in every language
([F-03](f-03-identity.md#identity-across-languages)). A tool reads only the
facet it needs; a Block with no media facet is plain anchored-by-runs text.

The role also disambiguates **non-modifiable content surfaced for context**. A
block is not always modifiable: a reader may emit `Block{Translatable: false}`
carrying a role such as code or formula so a listing or an equation is visible to
model and retrieval ingestion while translation skips it. That surfacing is a
default-on, per-format opt-out described in
[E-02: The format system](../engine/e-02-format-system.md).

**Source provenance.** A `Target` records its `Origin`: how the translation was
produced (`human`, `memory`, `mt`, `ai`), the engine, a reference, a timestamp. A
Block's **source** carries the same `Origin` when it was *recognized* rather than
parsed: an OCR or speech engine, or a model, produced the text, so the `Origin`
names that engine (`ocr`, `asr`, or `llm-refined` for a recognition a multimodal
model re-read) and carries a **confidence** in `[0,1]`. A block read losslessly
from a text format has no source `Origin`; one produced by recognition does, and a
confidence-gated refinement step reads it to decide what to re-examine
([M-03](../multilingual/m-03-multimodal-content.md)). Source and target
provenance are one record on two sides of the Block.

**What governed it.** `Origin` records not only *how* a target was produced but
*what governed it*: the `Profile` and `ProfileVersion` in force, and a
`ContextFingerprint`, a hash of the rendered voice guidance and the terms as they
actually reached the producer. That second half cannot be reconstructed after the
fact, because a profile is edited in place and terms carry no version of their
own, so it is stamped at production time or lost. The fingerprint is a change
detector, not a snapshot: it answers "have the governing inputs moved since this
was produced?" and nothing more. It is narrower than an engine's
config fingerprint, which also moves on a model or prompt change and would make a
model swap read as a governance change.

Every non-pseudo producer (model, machine translation, and recycled content
memory) stamps it, computed one way (`profile.GovernanceContext`), so targets
written under one context share a fingerprint whichever engine produced them and
fall stale together when the context moves. A recycled memory hit resolves its
*own* fingerprint from the context governing the collection at fill time rather
than inheriting the matched entry's approving one: a fill asserts the entry as
valid under the governance in force now, so it must fall stale when that
governance moves. Pseudo-translation takes no governing context and stamps none.
See [C-05: Freshness and the composite ref](../context/c-05-freshness.md).

### The Run sequence

A Block's content is a flat sequence of `Run` values held directly on the
Block, with no embedded marker characters:

```go
type Run struct {
    Text    *TextRun        // plain text chunk
    Ph      *PlaceholderRun // self-closing: variable, icon, <br>, redaction
    PcOpen  *PcOpenRun      // opening half of a paired code
    PcClose *PcCloseRun     // closing half of a paired code
    Sub     *SubRun         // reference to a nested Block (subfilter output)
    Plural  *PluralRun      // ICU plural with per-form Runs
    Select  *SelectRun      // ICU select with per-case Runs
}
```

Exactly one pointer field is set, which is the run's *kind*; `Run.Kind()` returns
the discriminator and `Run.RunID()` the id for the kinds that carry one.
Intention-revealing constructors (`TextR`, `PhR`, and siblings) build each kind
without hand-writing the union. Text and inline codes interleave positionally;
there is no parallel side-table and no marker characters.

A `TextRun` marked `NoTranslate` is text the reader classified as code rather
than prose: the contents of a code span, a `<kbd>`, a `<samp>`. It stays a run,
so positions and round-trip are unaffected, and every translation path leaves
it as it is ([E-02](../engine/e-02-format-system.md)).

### Stand-off overlays are the one interpretation mechanism {#stand-off-overlays}

The runs are the content. Everything that *interprets* the content (where the
sentence boundaries fall, which spans are terms or entities, what a check flagged,
how a source span aligns to a target span) is a stand-off overlay layered over
the run sequence without rewriting it.

The split is by **origin**. A run records what the source document
structurally contained; an overlay records what something concluded about it.
So a conclusion never becomes a run, however a format happens to spell it: an
XLIFF `<mrk type="term">` arrives as a term overlay span rather than as a run
kind, and a term this engine locates leaves as a `<mrk>` the writer draws around
runs it does not alter. Because marking is a projection at write time and a
reading at parse time, the same content round-trips through a format that
carries marks and one that carries none. What a writer will draw is a declared
capability ([E-02](../engine/e-02-format-system.md)).

```go
type Span struct {
    ID    string            // overlay-local id (e.g. a segment id "s1")
    Range Anchor            // run-anchored, never a flattened-string offset
    Props map[string]string // type-specific, e.g. the "ignorable" marker
    Value Payload           // typed payload from the payload registry
}
```

`Anchor` is the one type that says where inside a block something is, and every
producer records positions with it: overlays, check findings, and the stand-off
annotations an [overlay sidecar](/reference/serialization/overlays) carries.
It addresses one of four things, discriminated by `Kind`: the whole block, one
run by id, a half-open span of characters, or one branch of a plural or select
run.

```go
type Anchor struct {
    Kind  AnchorKind // block | run | range | form
    Path  RunPath    // the run sequence addressed; empty is the block's own runs
    RunID string     // for kind run
    Start RunPos     // for kind range, half-open [Start, End)
    End   RunPos
    Key   string     // for kind form: a plural form or a select case
}

// RunPos is a character boundary: an index into a run sequence and a rune
// offset into that run's text.
type RunPos struct{ Run, Offset int }
```

Two properties make it hold up. Positions are **run-relative**, so a boundary
stays where it was put when a neighbouring run is rewritten and can sit either
side of a placeholder, which a flat character offset can do neither of. And they
are **pathed**: a block's content is a tree, so `Path` walks to the sequence
being addressed and a term inside the `other` form of a plural is addressable
rather than approximated by a position in the flattening.

Four properties follow from anchoring interpretations to runs rather than baking
them into structure:

- **Segmentation is opt-in and dynamic.** Nothing segments by default; a tool
  computes boundaries and writes a `segmentation` overlay. Whole-block content is
  the norm, which is also what document-level translation wants for coherence. The
  engine is pluggable and selected per run; see
  [M-02: Segmentation](../multilingual/m-02-segmentation.md).
- **It is reversible by construction.** Desegmentation is "drop the overlay."
  There is no inverse operation to get wrong and no inter-segment material to
  lose: the gaps between segment spans are runs that no span covers.
- **It is uniform.** Terms, entities, and findings are the same kind of overlay,
  anchored the same run-aware way, rather than each re-detecting boundaries at
  render time.
- **Several layers coexist.** A primary sentence segmentation (`LayerPrimary`, the
  zero value, the one bilingual formats project to and from) can sit alongside a
  token-budgeted chunking layer, each its own overlay distinguished by `Layer`.

A span marked `ignorable` in its `Props` is non-content structural material: an
XLIFF `<ignorable>`, inter-segment whitespace, an ICU plural selector. Tools
preserve such a span's target verbatim instead of translating it, and the span
still occupies its run range so neighbouring segment positions stay aligned. It is
the format-agnostic marker shared by native readers and by plugin-backed ones.

### Inline-code runs carry their own metadata

`PlaceholderRun` and `PcOpenRun` carry the full set of inline-code concerns
directly on their fields:

```go
type PcOpenRun struct {
    ID          string            // abstract identity; shared with the matching PcClose
    Type        string            // semantic type from the vocabulary ("fmt:bold")
    SubType     string            // format-specific refinement ("html:b", "xlf:b")
    Data        string            // native markup, replayed verbatim by writers
    Equiv       string            // plain-text equivalent ("" for bold, "\n" for <br>)
    Disp        string            // translator-facing label ("[B]")
    Attrs       map[string]string // canonical, format-neutral attributes
    Constraints *RunConstraints   // Deletable, Cloneable, Reorderable
}
```

`PlaceholderRun` has the same shape; it is self-closing, so it has no pairing
partner. `PcCloseRun` is leaner: it shares `ID` with its opener and replays its
own `Data`, and it carries no `Constraints`, because the closing half inherits its
opener's behavior. `SubRun` references a subblock produced by a subfilter;
`PluralRun` and `SelectRun` are structured ICU constructs whose branches are
themselves `[]Run`.

Three fields double as **portable cross-format carriers**, so a writer for a
format that knows nothing about the source markup can still re-synthesize the
construct natively:

- **`Attrs`** holds canonical, format-neutral attributes: `href`, `src`, `alt`,
  `title`. A reader populates them from the source markup; a foreign writer emits
  a Markdown `[text](href "title")` or an HTML `<a href title>` from the same
  values without parsing the source format's literal `Data`. The set is open;
  those four are the canonical keys.
- **`Equiv` and `Disp`** carry a plain-text equivalent and a translator label,
  and beyond that a format-independent rendering of an opaque payload. Equations
  use this: the OpenXML reader keeps the OMML on `Data`, replayed verbatim for a
  byte-exact round-trip, and places portable LaTeX on `Equiv` and `Disp`, so the
  Markdown writer renders the formula from the carrier
  ([M-04](../multilingual/m-04-math-and-equations.md)).

`Equiv` and `Disp` are excluded from the parity canonical projection, so carrying
them never disturbs parity with the reference implementation
([A-02](../assurance/a-02-parity.md)).

### Semantic types and format-specific refinement

The `Type` field draws on a **vocabulary**: a registered, data-defined type system
that maps format-specific markup to format-independent semantic types grouped by
namespace prefix: `fmt:` for formatting, `link:` for linking, `media:` for
media, `struct:` for structural constructs, `code:` for non-modifiable inline
tokens, `entity:` for named entities. A vocabulary entry says what a type means,
how it renders and is labeled, and what a translator may do with it. The
authoritative catalog of types, and the file format for defining new ones, are the
[Vocabularies](/framework/vocabularies) concept page and the
[Authoring vocabularies](/contribute/vocabularies) guide.

`SubType` refines a type for one format using a prefix convention: `xlf:` for
XLIFF 2.0 subtypes, `html:` for element names, `md:` for Markdown constructs,
`docx:` for OpenXML run properties. Custom subtypes use a reverse-domain prefix
(`com.acme:custom-tag`).

### Run ID assignment and pairing

Readers assign sequential numeric IDs to inline-code runs within each run
sequence. A `PcOpenRun` and the `PcCloseRun` that closes it share the same `ID`; a
`PlaceholderRun` gets its own. IDs start at `"1"`, pairs nest last-in-first-out,
and runs inside a `Plural` or `Select` branch form their own scope. From
`Click <b>here</b> for <a href="x">info</a>.` a reader produces the text run
`Click `, a `PcOpen{ID:"1", Type:"fmt:bold"}` around `here`, its matching
`PcClose{ID:"1"}`, then ` for `, a `PcOpen{ID:"2", Type:"link:hyperlink"}` around
`info`, its `PcClose{ID:"2"}`, and a final `.`.

This produces stable structural keys for memory matching: HTML `<b>Click</b>` and
Markdown `**Click**` both yield `{1}Click{/1}`, so an entry created from HTML
matches a Markdown source at the structural tier.

### Run text projections

A Run sequence is the single source of truth; every textual form that crosses a
boundary is a **projection** computed from `[]Run` on demand. The framework
provides, in `core/model`:

```go
// Plain flattening: TextRun content verbatim, placeholders contribute {equiv},
// paired codes contribute their inner content, plural/select take the 'other'
// branch. Use: word count, search, text comparison.
func FlattenRuns(runs []Run) string

// Structural text: inline-code runs become numbered placeholders ({1}, {/1},
// {2/}). Use: memory exact matching at the structural tier.
func RunsStructuralText(runs []Run) string

// Generalized text: entity placeholder runs become typed placeholders
// ({PERSON}), other inline codes numbered. Use: generalized memory matching.
func RunsGeneralizedText(runs []Run) string

// Markup-preserving render: re-emits each run's captured Data verbatim.
// Use: writers splicing opaque markup back into a string.
func RenderRunsWithData(runs []Run) string

// Placeholder and semantic-HTML forms for prompt and provider boundaries.
func RunsPlaceholderText(runs []Run) string
func RunsSemanticHTML(runs []Run, reg *VocabularyRegistry) string
```

For the run sequence `Click `, `PcOpen{ID:"1", Type:"fmt:bold",
Data:"<b class='x'>"}`, `here`, `PcClose{ID:"1", Data:"</b>"}`, ` for info`:

```
FlattenRuns():        Click here for info
RunsStructuralText(): Click {1}here{/1} for info
RenderRunsWithData(): Click <b class='x'>here</b> for info
```

### Boundaries: structural inside, textual at the edges

The inline-code model is **structural-canonical**. `[]Run` is the single source of
truth for a Block's content. Every representation that crosses a boundary (to a
translator, a model, a translation provider, an external editing tool, a runtime,
a memory index) is a projection computed on demand.

| Projection | Surface | Consumer |
| --- | --- | --- |
| `[]Run` (no projection) | `Block.Source` / `Targets`, KBF wire | Pipeline tools, stores, readers and writers |
| `RenderRunsWithData` | native source markup | Format writers; replays `Data` verbatim |
| `RunsStructuralText` | `Click {1}here{/1} for info` | Memory matching, structural tier |
| `RunsGeneralizedText` | structural + entity placeholders | Memory matching, generalized tier |
| `RunsPlaceholderText` | `<x id="1"/>here<x id="/1"/>` | Prompts where tag preservation is critical |
| `RunsSemanticHTML` | `<a href="…">here</a>` | Providers and prompts that expect HTML |
| `flattenRuns` (TypeScript) | `Click {=m1}here{/=m1}` | ICU runtime and the React i18n re-attach |
| `runsToCoded` (TypeScript) | private-use-area marker text + span info | The visual editor's styled chips |

Two consequences fall out of the convention:

1. **There is no single "translator format."** A user editing in the visual editor
   sees nested chips with semantic formatting; the same Block in an external
   editing tool arrives as XLIFF `<pc>`; the same Block sent to a model goes as a
   placeholder or semantic-HTML projection. The structural Block is identical.
2. **Extensions follow the same rule.** A new reader, a new extractor, a new
   translator surface: each emits `[]Run` and lets the existing projections serve
   every consumer. A new textual convention is introduced only when an existing
   projection is genuinely insufficient.

### Reader and writer contracts

**Readers** populate every field on each inline-code run they emit: a sequential
`ID` shared by a pair; `Type` and `SubType` from the vocabulary plus a
format-specific refinement; `Data` as verbatim native markup; `Disp` as a short
human-readable label; `Equiv` as a plain-text equivalent where one applies;
`Attrs` for the canonical attributes the construct carries; and `Constraints`
derived from format semantics.

**Writers** reconstruct output from `Run.Data`, the native markup, not from the
semantic type. That is what makes round-trip fidelity a property of the model
rather than of each format's implementation: the writer replays exactly what the
reader captured. A writer for a *different* format than the source instead
synthesizes from `Type` plus `Attrs`.

### Layers and embedded content

Embedded content is modeled as nested Layers. A Layer carries its own format
identifier and a `ParentID` linking it to the enclosing layer. When a reader
encounters embedded content (an HTML string inside a JSON value), it emits a
child Layer with that format containing the parsed Blocks, nested between the
parent Layer's Parts:

```
PartLayerStart (format="json", id="doc1")
  PartBlock (key: "title", text: "Hello")
  PartLayerStart (format="html", id="sf1", parentID="doc1")
    PartBlock ("Welcome to <b>our site</b>")
  PartLayerEnd (id="sf1")
  PartData (structural JSON)
PartLayerEnd (id="doc1")
```

Each Layer is independently processable. Layers nest recursively: HTML in JSON in
YAML is three levels deep with no special cases.

### Subfilter resolution

Format-to-format embedding is coordinated by a small interface:

```go
type SubfilterResolver interface {
    ResolveReader(formatName string) (DataFormatReader, error)
    ResolveWriter(formatName string) (DataFormatWriter, error)
}
```

`FormatRegistry` implements it through its `NewReader` and `NewWriter` methods.
The interface decouples readers from the registry, prevents circular imports, and
allows test doubles. Readers and writers that support subfiltering implement a
marker interface, `SubfilterAware`, and the resolver is injected before `Open` or
`Write` is called. Any registered format (native, plugin, or bridged) can serve
as a subfilter.

Format configs declare subfilter mappings that bind content locations to a format:

```yaml
subfilters:
  - pattern: "*.body"
    format: html
  - pattern: "*.description"
    format: markdown
```

Patterns use `filepath.Match` semantics with `.` as the separator; JSON readers
use key-path globs, XML readers use element-path patterns. When a reader matches a
pattern it emits `PartLayerStart`, delegates to the resolved sub-reader, and emits
`PartLayerEnd` when the sub-reader finishes. Writers buffer the parts between
matching Layer boundaries, delegate to the sub-writer, and splice the rendered
string into the parent format.

## Consequences

- Type dispatch via `switch part.Type` replaces run-time type inspection, and
  linters provide exhaustiveness checking.
- Adding a resource type needs only a new `PartType` constant and a struct
  implementing `Resource`.
- Tools that only handle Blocks ignore every other Part type through the base tool
  pass-through behavior ([E-03](../engine/e-03-tool-system.md)).
- The Part stream stays a single ordered channel; no fan-out complexity in the
  base pipeline.
- Content-addressable identity enables incremental extraction and deduplication
  across documents.
- Properties, overlays, and annotations let tools carry metadata without
  content-model changes.
- The semantic-type abstraction lets memory match across formats and lets prompts
  receive a consistent inline-code representation.
- Writers replay `Run.Data` verbatim, so round-trip fidelity is a property of the
  model rather than of each format.
- Layers nest recursively with no special cases; embedded content is a first-class
  pipeline citizen.
- The Run union (paired `PcOpen`/`PcClose`, self-closing `Ph`, structured
  `Plural`/`Select`) maps naturally onto XLIFF 2.0's `<pc>`/`<ph>` model, making
  XLIFF serialization a mapping rather than a lossy conversion.
- Segmentation, terms, entities, and findings share one run-anchored stand-off
  mechanism; segmentation never mutates content, so it is opt-in, multi-layered,
  and losslessly reversible.
- Anchoring is one idea with per-medium facets (run range, geometry, timing), so
  image, audio, and video content sits in the same Block model as text, and a
  recognized source carries the same `Origin` a translation does. Multimodal work
  adds no new positional primitive.
- Bilingual interchange formats that carry sentence segments project to and from
  segmentation plus alignment overlays at the reader and writer boundary, without
  forcing segment structure into the content model.
- Targets are first-class records keyed by a variant, so the variant axis extends
  beyond locale without adding ceremony at locale-only call sites.

## See also

- [F-01: The framework and its modules](f-01-framework-and-modules.md)
- [F-03: Identity](f-03-identity.md): hashes, reconciliation, and the store key
- [F-04: The content-model wire schema](f-04-wire-schema.md): the canonical serialization of these types
- [E-01: The processing engine](../engine/e-01-processing-engine.md): how Parts stream
- [E-02: The format system](../engine/e-02-format-system.md): readers and writers that produce Parts
- [E-03: The tool system](../engine/e-03-tool-system.md): tools that consume Parts
- [E-07: Model and translation providers](../engine/e-07-model-providers.md): multimodal messages carrying a Block's media anchor
- [E-08: Document structure tiers](../engine/e-08-document-structure-tiers.md): where roles and geometry come from
- [M-01: Bilingual format interop](../multilingual/m-01-bilingual-interop.md): segment and alignment projection
- [M-02: Segmentation](../multilingual/m-02-segmentation.md): the segmentation overlay's producers
- [M-03: Multimodal content](../multilingual/m-03-multimodal-content.md): the temporal facet and recognition confidence
- [M-04: Math and equations](../multilingual/m-04-math-and-equations.md): `Equiv` and `Disp` as portable math carriers
- [Vocabularies](/framework/vocabularies): the semantic type system
