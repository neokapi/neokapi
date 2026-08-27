---
id: e-02-format-system
sidebar_position: 2
title: "E-02: The format system"
description: "Formats are pluggable reader/writer pairs that convert between on-disk files and the Part stream, with three skeleton strategies for byte-exact write-back and a three-way reader output policy that surfaces non-translatable context as content."
keywords: [neokapi, architecture decision, format system, DataFormatReader, DataFormatWriter, skeleton, streaming, detection, content fidelity, non-translatable content]
---

# E-02: The format system

## Summary

Formats are pluggable readers and writers that convert between on-disk
representations and the Part stream. The framework ships a broad set of built-in
formats under `core/formats/`, each implementing `DataFormatReader` and
`DataFormatWriter` on top of shared `BaseFormatReader` / `BaseFormatWriter`
embeds. A single `FormatRegistry` exposes a factory-based lookup that serves
native Go formats, plugin formats, and bridge formats uniformly. Detection
cascades through extension, priority, and content sniffing. Byte-exact
write-back is supported by three interchangeable skeleton strategies, and a
reader classifies its input **three** ways — translatable content, pure
structure, and non-translatable-but-meaningful context that is *surfaced* rather
than buried.

## Context

The framework must read a large variety of file formats and write them back with
byte-exact fidelity — every newline, every entity reference, every attribute
quote style. Formats vary widely in structure: linear text (plain text,
Markdown), tree-structured markup (HTML, XML, OpenXML), line-oriented key-value
(Java properties, Apple strings), grid-based (CSV, spreadsheets), and
exchange-specific (XLIFF, TMX, Gettext).

Formats also frequently contain embedded content in another format (HTML inside
JSON, Markdown inside CSV), and the reader/writer contract must accommodate that
recursion without special cases.

A second consumer complicates the picture. The translation path wants only the
prose a human would translate; model ingestion wants *all* textual context —
code listings, captions, alt-text, formulas, do-not-translate strings,
config-excluded values, and comments. A two-way classification, where a fragment
becomes either a translatable Block or opaque skeleton bytes, makes everything
the first consumer skips invisible to the second.

## Decision

### Reader and writer interfaces

These interfaces implement the `file` *source* and *sink* binding in
[E-04](e-04-flows-and-io-binding.md). Other bindings — the project store, a
`.kpz` workspace, interchange import/export — feed and drain the same `Part`
stream without a reader or writer, so a flow is agnostic to where its content
enters and leaves.

Each interface is a composition of narrow facets, so a consumer that only needs
to identify a format, or only to apply configuration, can depend on that facet
alone:

```go
type DataFormatReader interface {
    FormatDescriptor // Name, DisplayName, Signature
    Configurable     // Config, SetConfig
    PartReader       // Open, Read, Close
}

type DataFormatWriter interface {
    Name() string
    OutputSink // SetOutput(path), SetOutputWriter(io.Writer)
    SetLocale(locale model.LocaleID)
    SetEncoding(encoding string)
    PartWriter // Write, Close
}
```

The reader lifecycle is `Open → Read → Close`. `Open` attaches the reader to a
`model.RawDocument` (raw bytes or a streaming reader, plus metadata such as
source locale and file path). `Read` returns a channel of
`model.PartResult{Part, Error}` — the reader produces Parts until the document is
exhausted or an error occurs, then closes the channel. `Close` releases held
resources.

The writer lifecycle is `SetOutput → Write → Close`. `Write` consumes a channel
of `*model.Part` until the channel closes, producing output on the writer's
destination.

### BaseFormatReader and BaseFormatWriter

`BaseFormatReader` and `BaseFormatWriter` provide shared behaviour that concrete
formats embed:

- Document-level Layer bracketing (`PartLayerStart`/`PartLayerEnd` for the root
  document layer)
- Locale metadata propagation and source/target locale accessors
- Consistent error handling and channel lifecycle

A concrete format implements the format-specific parsing and serialization and
delegates lifecycle to the base embed.

`BaseFormatWriter` also owns the shared **byte-level output options**
(`format.OutputOptions`): `output.bom` (`add|remove|keep`), `output.newline`
(`lf|crlf|keep`), and `output.encoding` (any charset in `core/encoding`; default
UTF-8 passthrough). Readers already normalize BOM, charset, and newlines at parse
time, so these exist only to control *output* style — writer configuration, not a
pipeline stage. They are set under the reserved `output` key of the ordinary
per-format config (`defaults.formats[<id>].config` in a `kapi.yaml` recipe);
`format.SplitOutputConfig` strips that key before per-format reader/writer config
is applied, and the base writer wraps its output stream with the post-encode chain
(newline conversion → BOM policy → charset encoding), so every writer that embeds
the base inherits the behaviour with no per-format code.

### Built-in formats

The built-in formats under `core/formats/` span several families:

- **Markup** — HTML, XML, Markdown / MDX, AsciiDoc, and structured-document
  formats.
- **Translation exchange** — XLIFF 1.2 / 2.x, TMX, Gettext PO/MO.
- **Structured data** — JSON, YAML, CSV/TSV, and design-token and app
  message-catalog variants (`xcstrings`, `arb`, `i18next`, `resx`, Android
  strings, Apple strings, …).
- **Office and publishing** — OpenXML (`.docx`, `.xlsx`, `.pptx`), ODF, EPUB, and
  related packaged formats.
- **Subtitle and media** — SRT, VTT, plus audio, video, image, and PDF readers.

The full, authoritative list of registered formats — with extensions, MIME types,
and per-format options — is the generated [Format Reference](/formats). It is
derived from the live registry, so it never drifts from the code.

Each format package under `core/formats/<name>/` contains `reader.go`,
`writer.go`, and `config.go`. Formats register both factories in
`core/formats/register.go` via `init()`.

### FormatRegistry

A single `*registry.FormatRegistry` exposes factory lookup. Names are the
`FormatID` string type; registration takes a factory plus static metadata, so no
reader instance is built at startup:

```go
func (r *FormatRegistry) RegisterReader(name FormatID, factory FormatReaderFactory, sig format.FormatSignature, displayName string)
func (r *FormatRegistry) RegisterWriter(name FormatID, factory FormatWriterFactory)
func (r *FormatRegistry) NewReader(name FormatID) (format.DataFormatReader, error)
func (r *FormatRegistry) NewWriter(name FormatID) (format.DataFormatWriter, error)
func (r *FormatRegistry) FormatInfos() []FormatInfo
func (r *FormatRegistry) Detect(path string, opts DetectOptions) (FormatID, error)
```

Tiered registration makes native, plugin, and bridge formats indistinguishable to
callers:

1. **Native built-ins** — registered at program start via `init()` hooks in
   `core/formats/register.go`.
2. **Plugin formats** — registered from the `formats` capability declared in each
   plugin's `manifest.json`, read from disk during plugin discovery without
   launching a subprocess.
3. **Bridge formats** — served by a Mode-C daemon plugin over a Unix-socket gRPC
   connection; the host registers proxy factories that dial the daemon on demand
   (see [E-05](e-05-plugin-system.md)).

A format reference in user-facing configuration uses the syntax
`name[@version][:preset]`, e.g. `okf_html@1.46.0:wellFormed`. The registry
resolves the reference to the appropriate factory.

### Format detection

`Detect(path, DetectOptions)` resolves the format for a path. By default it
detects by extension **and**, when that extension is claimed by more than one
format, by reading the file head to decide between them (`.xliff` can be XLIFF
1.x or 2.x; `.xml` is claimed by several formats). `DetectOptions` carries
`ExtensionOnly` for the deterministic extension/priority pick, plus source
restriction and per-call priority overrides. Only the head of the file is read;
any read error falls back to the extension pick.

Each format registers a `FormatSignature` declaring the MIME types, extensions,
magic bytes, and optional sniff function it claims, so the cascade is data-driven
rather than hardcoded. `FormatSignature.Binary` marks a format whose reader
consumes binary content, so detection does not decline it for binary input the
way it declines a text format matched only by extension.

kapi's own on-disk conventions use **compound suffixes** — `.kbf.json`,
`.memory.json`, `.terms.json`, `.overlays.json`, `.overlays.jsonl` — so the
marker survives while the file still reads as the JSON it is. `path/filepath.Ext`
reports `.json` for all of them, so extension-driven code goes through
`format.Ext` / `TrimExt` / `Stem` instead, which return the most specific
registered suffix.

### Skeleton strategies

Three interchangeable strategies preserve non-translatable content for
write-back. A format picks the one that fits its structure:

- **SkeletonStore streaming** (HTML, XML). A temp-file-backed binary store. The
  reader writes non-translatable bytes and block references during extraction;
  the writer reads entries sequentially to reconstruct the document with
  byte-exact fidelity. Peak memory is a small constant per document regardless of
  document size. Preferred for new formats. See
  [Skeleton Store](/contribute/implementation/engine/skeleton-store) for the binary
  format and wiring.

- **Re-parse** (JSON, YAML, PO, plain text). The writer re-opens the source
  document and replaces translatable content in place. Simple, but holds the
  document in memory twice while writing.

- **Fragment-based** (XLIFF, some XML dialects). An interleaved skeleton of
  non-translatable markup plus references to translatable blocks, carried inline
  on the `Data`/`Block` resources. Suits formats whose translatable content is
  sparse.

All three present the same `DataFormatWriter` interface to the pipeline.

### Streaming readers and bounded-memory I/O

The read → process → write path streams end-to-end so peak memory tracks a
bounded window, not the document size. Three edges cooperate:

1. **Source edge.** The file-run path (`core/flow.FileRunner`) hands the reader a
   *streaming*, byte-budgeted `io.ReadCloser` over the file (`core/safeio`
   enforced uniformly) instead of reading the whole file into a buffer up front.
   A line or record reader pulls bytes on demand and never holds the whole input;
   a whole-document reader still reads it once, in full.
2. **Reader → executor.** When the reader declares the **`format.StreamingReader`**
   capability, its `Read` channel is fed straight into the executor rather than
   being collected into a `[]*Part` slice between reader and tools, so the reader
   runs concurrently with the writer. This is gated on the capability because it
   overlaps the read and the write: only in-process, pure-Go readers may opt in,
   never a daemon-backed plugin, which keeps the read-fully-then-write order its
   one-stream-at-a-time contract requires.
3. **Skeleton.** A byte-exact round-trip needs the skeleton, but the buffered
   skeleton writer collects every block into a map and replays the skeleton only
   after it is fully written — O(blocks) memory. A reader and writer that both
   declare streaming (**`StreamingReader`** + **`format.StreamingWriter`**)
   instead share a **concurrent, channel-backed skeleton store**: the reader
   appends entries while the writer pops them, consuming each `SkeletonRef`'s
   block from the Part stream *on demand* (`format.StreamSkeletonWrite`). Because
   a streaming reader emits refs and their blocks in the same order, the
   pending-block window stays small. **Output is byte-identical to the buffered
   skeleton path** — the same entries in the same order, just consumed
   interleaved instead of after a flush. Both capabilities are bare markers
   (`StreamingReader()` / `StreamingWriter()`), probed via `IsStreamingReader` /
   `IsStreamingWriter`; a writer signals it took the streaming path by checking
   `SkeletonStore.IsStreaming()` in `Write`.

The adopters are the record- and entry-oriented formats whose readers emit each
record as it is parsed, holding only the in-progress unit: the app
message-catalog family (`androidxml`, `arb`, `i18next`, `messageformat`, `resx`,
`ts`, `xcstrings`), the structured-data pair (`json`, `xml`), `properties`,
`srt`, and `tmx`. Each declares the marker on both its reader and its writer, and
its writer's `SkeletonRef` rendering is factored into a shared helper so the
buffered and streaming skeleton paths are byte-identical.

A format that must materialise its whole input to parse it — a packaged
zip-backed format, a DOM-building markup reader — does not declare the
capability, and a uniform fallback keeps its output byte-identical. The container
path drives one archive entry through `FileRunner.RunStream` (bytes in, bytes
out, no temp file); a streaming-capable inner format is not even buffered whole
([E-04](e-04-flows-and-io-binding.md)).

### Writer output modes: generative vs skeleton-bound

A skeleton is **format-specific** — it is the non-translatable scaffolding of
*one* file, captured by that format's reader. So a writer's ability to produce
output depends on whether it can build a whole document from the content model
alone, or only by injecting translated text back into a skeleton it was given.
Two capabilities, deliberately **orthogonal**, capture this:

- **Generative** — the writer can serialize a complete, valid document from the
  content model (roles, runs, structure) with no skeleton.
- **Skeleton-consuming** — the writer uses a skeleton *when given one*, for
  byte-exact fidelity, via the `SkeletonStoreConsumer` interface. This is about
  *using* a skeleton, not *requiring* one.

These compose into three writer classes:

- **Generative document and data writers** (`generative`, not interchange). HTML
  is the archetype: with the source file's skeleton it round-trips losslessly,
  and **without** one it still writes a clean document — so it is also a target
  for content that arrived from a different format. Markdown, DocLang, AsciiDoc,
  plain text, and the data and catalog formats behave the same. **These are the
  `convert` targets.**
- **Bilingual interchange writers** (`generative` **and** `interchange`). XLIFF,
  PO, TMX, MO, and KBF are generative *files*, but they belong to the
  extract → translate → merge loop, not to document conversion: `kapi extract`
  captures the source skeleton so `kapi merge` can round-trip translations back
  into the *original* format. A `convert`-produced interchange file carries no
  skeleton and cannot be merged back — a dead end — so interchange formats are
  **excluded as `convert` targets** and reached via `extract`/`merge`
  ([M-01](../multilingual/m-01-bilingual-interop.md)).
- **Skeleton-bound writers** (not generative). Packaged formats — OpenXML, ODF,
  EPUB, image — wrap content in a fixed package that cannot be regenerated from
  the model; they only ever write back into their *own* skeleton. Same-format and
  merge writers, never a cross-format target.

**Cross-format conversion** ([S-04](../surfaces/s-04-toolbox.md)) reconstructs the
target from the content model and never carries a foreign skeleton into the
writer. A writer is a valid conversion *target* iff it is **generative and not
interchange**. Both are **declared writer capabilities** — the writer states what
it can write via `GenerativeWriter.Generative()` (the inverse of
`BaseFormatWriter.RequiresSkeleton`) and `InterchangeWriter.IsInterchange()`. The
registry records them on `FormatInfo.Generative` / `FormatInfo.Interchange`:
probed once from the built-in writer at registration, and for plugin formats taken
from the cached manifest's `generative` / `interchange` capabilities — so
conversion, the [Conversion Lab](/lab/convert), and `kapi formats` read one
authoritative source **without loading any plugin**. Neither is derived from
`SkeletonStoreConsumer` (nearly every writer consumes a skeleton if offered, so
that bit does not distinguish a target) nor probed empirically.

### Marks a writer can draw

A block's runs carry what the source document structurally contained. What a
governance pass concluded about that content rides stand-off on a
`model.Anchor`: a located term, a voice finding, a style violation
([F-02](../foundations/f-02-content-model.md)). Some formats have somewhere to
put such a conclusion, and most do not: XLIFF has `<mrk>`, HTML has `<span>`,
plain text has nothing at all.

So drawing them is a **declared writer capability**, the fourth of the kind
`Generative` and `Interchange` already are. `InlineAnnotationWriter` names the
annotation types the writer knows how to draw, and the registry records them on
`FormatInfo.InlineAnnotations`, probed once from the built-in writer at
registration and for plugin formats taken from the cached manifest, so `kapi
formats` and the writer selection read one authoritative source without loading
any plugin. A writer that implements nothing carries no marks.

The declaration is a **ceiling**, and `defaults.annotations.write` in the recipe
narrows it:

```yaml
defaults:
  annotations:
    write: [term]      # voice findings stay stand-off
```

Narrowing only, never widening, and two things follow from that direction. A
format that gains the capability starts projecting without anyone editing a
recipe, which is what keeps the recipe from being a list a project has to
maintain against the format catalog. And naming a type a format cannot carry
asks for nothing rather than failing, because one recipe describes many outputs
and a project that wants terms marked wherever they can be should not have to
enumerate which of its formats happen to support it.

The default is that a document leaves as a document. An annotation travels
beside the content in an [overlay
sidecar](/reference/serialization/overlays) unless a writer both can draw it and
was not told otherwise.

XLIFF 2 is the first writer to declare the capability, and it draws a term as an
`<sm>`/`<em>` pair rather than an `<mrk>`. Both are spec shapes and `<mrk>` reads
better when a span nests cleanly inside one element, but only the pair can carry
a span that does not — and a term running from before a `<pc>` to after it is
exactly what an `Anchor` is built to express. Because the two markers are
independent nodes, each is placed wherever its own boundary lands, including
inside an element whose partner sits outside. One shape for every span keeps the
awkward case from being the least-tested one.

A span a segmentation cannot carry — one straddling two `<segment>` elements —
is recorded rather than half-drawn, and the writer reports it.

**Skeletons are typed per format.** A `SkeletonStore` carries an `OriginFormat`
stamp, and `format.WireSkeleton(store, reader, writer)` connects a reader's
skeleton emission to a writer **only when they are the same format** — so the
rule that a skeleton from format A is foreign to format B's writer is enforced
centrally, not left to each call site. A cross-format conversion therefore never
feeds a foreign skeleton into the target writer; that writer takes the generative
content-model route every writer shares.

### Reader output policy: three destinations, not two

The skeleton is not the only home for non-translatable content. A reader
classifies each fragment three ways:

| Fragment | Destination |
| --- | --- |
| Translatable prose | `Block{Translatable: true}` — the pipeline processes it |
| Pure structure (delimiters, quoting, whitespace) | skeleton bytes |
| Non-translatable but meaningful context | **surfaced** — see the two channels below |

The third category — code, verbatim and literal text, captions, alt-text,
formulas, do-not-translate strings, config-excluded values, comments — is
surfaced rather than collapsed into the second. It becomes content an ingestion
consumer can read, while staying outside the translation payload.

This introduces no new content-model type. The `Translatable` flag, the
`SemanticRole` taxonomy, `Data`, and notes are all the content model's
([F-02](../foundations/f-02-content-model.md)); the skeleton and sub-skeleton
mechanisms are this AD's.

#### Two surfacing channels

What a fragment *is* determines which channel carries it. Renderable content —
text that has a place in the rendered document — becomes a content block;
out-of-band annotation, text *about* the document, becomes data or a note.

| Channel | Carrier | Used for | Round-trip |
| --- | --- | --- | --- |
| Renderable contextual content | `Block{Translatable:false}` + `SemanticRole` + skeleton ref | code blocks, literal text, captions, alt-text, formulas, do-not-translate strings, config-excluded values | verbatim bytes stay in the skeleton; the surfaced body rides a skeleton ref, so the writer replays the original exactly |
| Comment / metadata context | `Data` part or a note annotation | developer and translator comments, review annotations, editorial notes | the comment bytes round-trip verbatim through the skeleton; the surfaced copy is informational only |

A block from the first channel carries the role that names its kind — alt-text as
`RoleCaption`, a code listing as `RoleCode`, an equation as `RoleFormula`, a
non-translatable cell as `RoleTableCell` — and is flagged so translation skips it
([E-07](e-07-model-providers.md)). The second channel keeps comment context as
*data* deliberately: a comment is not part of the rendered text, so promoting it
to a content block would misrepresent the document's structure; it stays a `Data`
part or a note that ingestion can read and the editor can show.

#### Default on, via an inverted opt-out

Surfacing is the **default**, controlled per format by a single boolean,
`extractNonTranslatableContent`, exposed as a schema property in the generated
format reference and accepted in `ApplyMap` under that key. The implementation is
deliberately an **inverted private field**:

```go
// zero value false ⇒ surfacing ON (the opt-out default)
disableNonTranslatableContent bool

func (c *Config) ExtractNonTranslatableContent() bool     { return !c.disableNonTranslatableContent }
func (c *Config) SetExtractNonTranslatableContent(v bool) { c.disableNonTranslatableContent = !v }
```

The inversion is the point: a freshly zero-valued config — a new format that has
not yet learned about the flag, a caller that constructs a config without calling
`Reset` — surfaces content automatically, because the *disable* bit must be set
explicitly to turn it off. The safe-for-ingestion behaviour is what you get for
free; opting out is the deliberate act. The off switch exists for two callers:
the parity harness, which pins the bridge-matching configuration, and
validation-only or pure-passthrough flows that want nothing but skeleton.

A format may also **scope** what counts as meaningful context. The design-tokens
reader composes the generic JSON reader but calls
`SetExtractNonTranslatableContent(false)` on that inner config: a token's
`$value`, `$type`, and `$extensions` are structured machine data — colours,
dimensions, font names — not contextual prose, so design tokens surface only
`$description` as translatable prose and let everything else pass through as
non-translatable structure. The convention is uniform; each reader decides which
of its fragments are genuinely *context* rather than inert data.

Which formats expose the flag, and exactly what each surfaces, is generated into
the [Format Reference](/formats) rather than enumerated here. The per-format
finding, carrier, and skeleton-strategy ledger lives in
[content-fidelity](/contribute/implementation/engine/content-fidelity).

#### Round-trip, translation-skip, and parity all still hold

Surfacing is additive over the existing guarantees, not a relaxation of them.

- **Byte-exact round-trip.** The verbatim source bytes never leave the skeleton.
  A surfaced renderable block stands in for the rendered body via a skeleton ref,
  or a **sub-skeleton** — verbatim segments interleaved with refs to translatable
  spans inside an otherwise-opaque payload. A surfaced comment's bytes are copied
  verbatim. An untranslated round-trip is byte-identical whether the flag is on or
  off. Translation of a surfaced *translatable* span splices in place; the
  surrounding structure is untouched.
- **Translation-skip.** A surfaced block carries `Translatable: false`, so the
  translation tools skip it by the same rule they always have
  ([E-07](e-07-model-providers.md)); the payload sent to a model is unchanged.
- **Parity.** The bridge has no notion of surfaced context, so a head-to-head with
  surfacing on would diverge by construction: the native stream would carry extra
  `Block`/`Data` parts the bridge never emits, and the canonical projection
  compares the `PartType` sequence and per-block `Translatable` flag. The parity
  contract is "same semantic config → same results", not "same defaults" — the
  parity runner duck-types `interface{ SetExtractNonTranslatableContent(bool) }`
  on the reader's config and forces it **false** before reading, so the native
  stream matches the bridge. The roles and properties a surfaced block carries are
  additionally **parity-safe carriers** — the canonical projection excludes
  `SemanticRole`, `Properties`, `Annotations`, and the placeholder `Equiv`/`Disp`
  — but it is the flag, not the projection, that keeps the surfaced *parts
  themselves* out of the head-to-head
  ([A-02](../assurance/a-02-parity.md)).

The `SkeletonStore` also supports the **sub-skeleton** in its own right: verbatim
segments of an otherwise-opaque payload interleaved with refs to translatable
spans inside it. This is how translatable prose embedded in an opaque structure —
the natural-language text inside a Word equation — is translated while the
surrounding math is replayed byte-for-byte
([M-04](../multilingual/m-04-math-and-equations.md); see
[Skeleton Store](/contribute/implementation/engine/skeleton-store)).

### Subfilters and nested layers

Format readers can emit child Layers when they encounter embedded content in a
different format (HTML inside JSON, Markdown inside CSV). The child reader is
resolved via a `SubfilterResolver` injected by the `FormatRegistry`. The
mechanism is defined in [F-02](../foundations/f-02-content-model.md) — format
readers just implement `SubfilterAware` and declare patterns in their config.

### Implementing a new format

1. Create `core/formats/<name>/` with `reader.go`, `writer.go`, and `config.go`.
2. Implement `DataFormatReader` by embedding `BaseFormatReader` and providing the
   format-specific parse logic.
3. Implement `DataFormatWriter` by embedding `BaseFormatWriter` and providing the
   format-specific serialize logic.
4. Populate every field on each inline-code run for any inline markup — `ID`,
   `Type`/`SubType`, `Data`, `Disp`, `Equiv`, `Constraints`
   ([F-02](../foundations/f-02-content-model.md)).
5. Pick a skeleton strategy appropriate to the format's structure, and declare
   the streaming markers if the reader and writer can both honour them.
6. Register the reader and writer factories in `core/formats/register.go` via an
   `init()` call.
7. If the format can host embedded content, implement `SubfilterAware` and accept
   `Subfilters []SubfilterMapping` in the config.

See [Implementing Formats](/contribute/implementation/engine/implementing-formats) for a
walkthrough, and [Skeleton Store](/contribute/implementation/engine/skeleton-store) for
the preferred strategy's details.

## Consequences

- Format readers emit the same streaming Part protocol regardless of source
  format, so tools never need format-specific code.
- Format writers replay `Run.Data` verbatim
  ([F-02](../foundations/f-02-content-model.md)), so write-back fidelity is
  inherited from the content model.
- Native, plugin, and bridge formats coexist in one registry; the pipeline treats
  them identically.
- The extension/priority/content cascade resolves most files without user
  configuration; ambiguous cases fall back to an explicit format flag.
- Three skeleton strategies cover the full span of file formats from streaming
  text to zip-packaged markup, and the streaming markers give record-oriented
  formats bounded memory without changing anyone's output bytes.
- New formats plug in by adding a directory and registering in `init()`; no
  changes to the engine are needed.
- The three-way reader output policy serves ingestion and translation from one
  parse, and the inverted opt-out means a format that has not thought about the
  question still surfaces context rather than hiding it.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md) — the Parts readers produce and writers consume; the Run model that drives write-back fidelity; the `SemanticRole` taxonomy
- [E-01: The processing engine](e-01-processing-engine.md) — how readers and writers plug into the pipeline
- [E-03: The tool system](e-03-tool-system.md) — the tools that sit between reader and writer
- [E-04: Flows and I/O binding](e-04-flows-and-io-binding.md) — readers and writers as the `file` binding; other bindings feed the same stream
- [E-05: The plugin system](e-05-plugin-system.md) — how plugin and bridge formats register
- [M-04: Math and equations](../multilingual/m-04-math-and-equations.md) — the sub-skeleton extension to the skeleton strategies
- [A-02: Parity](../assurance/a-02-parity.md) — the "same semantic config → same results" contract and the parity-safe carriers
- [Implementing Formats](/contribute/implementation/engine/implementing-formats) — implementation walkthrough
- [Skeleton Store](/contribute/implementation/engine/skeleton-store) — binary skeleton format and wiring
- [content-fidelity](/contribute/implementation/engine/content-fidelity) — the per-format surfacing ledger
