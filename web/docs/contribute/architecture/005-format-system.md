---
id: 005-format-system
sidebar_position: 5
title: "AD-005: Format System"
description: "Architecture decision: formats are pluggable DataFormatReader/Writer pairs that convert between on-disk files and the Part stream. Built-in formats span localization, document, data, subtitle, and office families."
keywords: [format system, DataFormatReader, DataFormatWriter, pluggable formats, architecture decision, neokapi]
---

# AD-005: Format System

## Summary

Formats are pluggable readers and writers that convert between on-disk
representations and the Part stream. The framework ships a broad set of
built-in formats under `core/formats/`, each implementing `DataFormatReader`
and `DataFormatWriter` on top of shared `BaseFormatReader` /
`BaseFormatWriter` embeds. A single `FormatRegistry` exposes a factory-based
lookup that
serves native Go formats, plugin formats, and Okapi-bridge formats
uniformly. Format detection cascades through MIME type, extension, magic
bytes, and content sniffing. Roundtrip fidelity is supported by three
interchangeable skeleton strategies.

## Context

A localization framework must read a large variety of file formats and
write them back with byte-exact fidelity — every newline, every entity
reference, every attribute quote style. Formats vary widely in structure:
linear text (plain text, Markdown), tree-structured markup (HTML, XML,
DOCX), line-oriented key-value (Java properties, iOS strings), grid-based
(CSV, XLSX), and translation-specific (XLIFF, TMX, TBX, Gettext).

At the same time, formats frequently contain embedded content in other
formats (HTML inside JSON, Markdown inside CSV), and the reader/writer
contract must accommodate this recursion without special cases.

## Decision

### Reader and writer interfaces

These interfaces implement the `file` *source* and *sink* binding in
[AD-026: Flow I/O Binding](026-flow-io-binding.md). Other bindings — the project
store, a `.klz` workspace, interchange import/export — feed and drain the same
`Part` stream without a reader or writer, so a flow is agnostic to where its
content enters and leaves.

```go
type DataFormatReader interface {
    Open(ctx context.Context, doc *RawDocument) error
    Read(ctx context.Context) <-chan PartResult
    Close() error
}

type DataFormatWriter interface {
    SetOutput(path string) error
    Write(ctx context.Context, in <-chan *Part) error
    Close() error
}
```

The reader lifecycle is `Open → Read → Close`. `Open` attaches the reader
to a `RawDocument` (raw bytes plus metadata such as source locale and file
path). `Read` returns a channel of `PartResult{Part, Error}` — the reader
produces Parts until the document is exhausted or an error occurs, then
closes the channel. `Close` releases any held resources.

The writer lifecycle is `SetOutput → Write → Close`. `SetOutput` sets the
destination path. `Write` consumes a channel of `*Part` until the channel
closes, producing output on the writer's destination.

### BaseFormatReader and BaseFormatWriter

`BaseFormatReader` and `BaseFormatWriter` provide shared behavior that
concrete formats embed:

- Document-level Layer bracketing (`PartLayerStart`/`PartLayerEnd` for the
  root document layer)
- Locale metadata propagation
- Source/target locale accessors
- Consistent error handling and channel lifecycle

A concrete format implements the format-specific parsing/serialization and
delegates lifecycle to the base embed.

### Built-in formats

The built-in formats under `core/formats/` span several families:

- **Markup** — HTML, XML, Markdown / MDX, and structured-document formats.
- **Translation exchange** — XLIFF 1.2 / 2.0, TMX, Gettext PO/MO.
- **Structured data** — JSON, YAML, CSV, and design-token / app-localization
  variants (`xcstrings`, `arb`, `i18next`, `resx`, Android strings, iOS
  strings, …).
- **Office and publishing** — OpenXML (`.docx`, `.xlsx`, `.pptx`), ODF, IDML,
  and related packaged formats.
- **Subtitle / media** — SRT, VTT, TTML, and similar.

The full, authoritative list of registered formats — with extensions, MIME
types, and per-format options — is the generated
[Format Reference](/reference/formats/html). It is derived from the live
registry, so it never drifts from the code.

Each format package under `core/formats/<name>/` contains `reader.go`,
`writer.go`, and `config.go`. Formats register both the reader factory
and writer factory in `core/formats/register.go` via `init()`.

### FormatRegistry

A single `*FormatRegistry` (a concrete struct in `core/registry`) exposes
factory lookup. Names are the `FormatID` string type; registration takes a
factory plus static metadata, so no reader instance is built at startup:

```go
func (r *FormatRegistry) RegisterReader(name FormatID, factory FormatReaderFactory, sig format.FormatSignature, displayName string)
func (r *FormatRegistry) RegisterWriter(name FormatID, factory FormatWriterFactory)
func (r *FormatRegistry) NewReader(name FormatID) (format.DataFormatReader, error)
func (r *FormatRegistry) NewWriter(name FormatID) (format.DataFormatWriter, error)
func (r *FormatRegistry) FormatInfos() []FormatInfo
```

Detection is delegated to a `*format.Detector`, reachable via `r.Detector()`.
The registry's `DetectByExtension(ext)` (and the source-scoped
`DetectByExtensionForSources`) wrap it, falling back to the lazy plugin-load
`onMiss` hook on a first miss.

Tiered registration makes native, plugin, and bridge formats
indistinguishable to callers:

1. **Native built-ins** — registered at program start via `init()` hooks
   in `core/formats/register.go`.
2. **Plugin formats** — registered from the `formats` capability declared
   in each plugin's `manifest.json`, read from disk during plugin discovery
   (`cli/pluginhost`) without launching a subprocess.
3. **Bridge formats** — served by a Mode-C daemon plugin (the Okapi bridge)
   over a Unix-socket gRPC connection; the host registers proxy factories
   that dial the daemon on demand (see
   [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md)).

A format reference in user-facing configuration uses the syntax
`name[@version][:preset]`, e.g. `okf_html@1.46.0:wellFormed`. The registry
resolves the reference to the appropriate factory.

### Format detection

`Detector.Detect(path, reader, mimeType)` returns the best-matching format
name using a cascade:

1. **MIME type** — explicit declaration wins if present.
2. **File extension** — `.html`, `.xliff`, `.json`, etc. resolve
   deterministically.
3. **Magic bytes** — binary signatures (BOM, XML declaration, ZIP
   signature for OpenXML).
4. **Content sniffing** — heuristic analysis for formats that share
   extensions (e.g., distinguishing XLIFF 1.2 from XLIFF 2.0).

Each format registers a `FormatMeta` record that declares the MIME types
and extensions it claims, so the cascade is data-driven rather than
hardcoded.

### Skeleton strategies

Three interchangeable strategies preserve non-translatable content for
roundtrip writing. A format picks the one that fits its structure:

- **SkeletonStore streaming** (HTML, XML). A temp-file-backed binary
  store. The reader writes non-translatable bytes and block references
  during extraction; the writer reads entries sequentially to reconstruct
  the document with byte-exact fidelity. Peak memory is ~100 KB per
  document regardless of document size. Preferred for new formats. See
  [Skeleton Store](/contribute/notes-internal/skeleton-store) for the binary format and
  wiring.

- **Re-parse** (JSON, YAML, PO, Plaintext). The writer re-opens the
  source document and replaces translatable content in place. Simple but
  holds the document in memory twice during writing.

- **Fragment-based** (XLIFF, some XML dialects). Interleaved skeleton of
  non-translatable markup plus references to translatable blocks,
  carried inline on the `Data`/`Block` resources. Suits formats whose
  translatable content is sparse.

All three strategies present the same `DataFormatWriter` interface to the
pipeline.

### Writer output modes — generative vs skeleton-bound

A skeleton is **format-specific** — it is the non-translatable scaffolding of
*one* file, captured by that format's reader. So a writer's ability to produce
output depends on whether it can build a whole document from the content model
alone, or only by injecting translated text back into a skeleton it was given.
Two capabilities, deliberately **orthogonal**, capture this:

- **Generative** — the writer can serialize a complete, valid document from the
  content model (roles, runs, structure) with no skeleton. Markdown, HTML,
  DocLang, AsciiDoc, plain text, XLIFF / PO / TMX, and the data/catalog formats
  are generative.
- **Skeleton-consuming** — the writer uses a skeleton *when given one* (for
  byte-exact fidelity), via the `SkeletonStoreConsumer` interface. This is about
  *using* a skeleton, not *requiring* one.

These compose into the two writer classes that matter for conversion:

- **Generative writers** (`generative`, skeleton-consuming optional). HTML is the
  archetype: with the source file's skeleton it round-trips losslessly to HTML,
  and **without** one it still writes a clean document — so it can also be a
  target for content that arrived from a different format. Generative-but-not-
  skeleton formats (e.g. DocLang) behave the same, minus the same-format fidelity
  path.
- **Skeleton-bound writers** (not generative). OpenXML (`.docx`), ODF, IDML, and
  EPUB wrap content in a fixed package that cannot be regenerated from the model;
  they only ever write back into their *own* skeleton. They are same-format /
  merge writers, never a cross-format conversion target.

**Cross-format conversion** ([AD-023: Toolbox — `kconv`](023-toolbox-utilities.md))
reconstructs the target from the content model and never carries a foreign
skeleton into the writer. A writer is therefore a valid conversion *target* iff
it is **generative**. This eligibility is a **declared writer capability** — the
writer states "what I can write" via `GenerativeWriter.Generative()` (the inverse
of `BaseFormatWriter.RequiresSkeleton`). The registry records it on
`FormatInfo.Generative`: probed once from the built-in writer at registration,
and for plugin formats taken from the cached manifest's `generative` capability —
so `kconv`, the [Conversion Lab](/lab/convert), and `kapi formats` read one
authoritative source **without loading any plugin**. It is **not** derived from
`SkeletonStoreConsumer` (nearly every writer consumes a skeleton if offered, so
that bit does not distinguish a target) nor probed empirically.

**Skeletons are typed per format.** A `SkeletonStore` carries an `OriginFormat`
stamp, and `format.WireSkeleton(store, reader, writer)` connects a reader's
skeleton emission to a writer **only when they are the same format** — so the
"a skeleton from format A is foreign to format B's writer" rule is enforced
centrally, not left to each call site. A cross-format conversion therefore never
feeds a foreign skeleton into the target writer; that writer takes the generative
content-model route every writer shares.

### Subfilters and nested layers

Format readers can emit child Layers when they encounter embedded content
in a different format (HTML inside JSON, Markdown inside CSV). The child
reader is resolved via a `SubfilterResolver` injected by the
`FormatRegistry`. This mechanism is defined in
[AD-002: Content Model](002-content-model.md) — format readers just
implement `SubfilterAware` and declare patterns in their config.

### Implementing a new format

To add a new format:

1. Create `core/formats/<name>/` with `reader.go`, `writer.go`, and
   `config.go`.
2. Implement `DataFormatReader` by embedding `BaseFormatReader` and
   providing the format-specific parse logic.
3. Implement `DataFormatWriter` by embedding `BaseFormatWriter` and
   providing the format-specific serialize logic.
4. Populate every field on each inline-code run for any inline markup —
   `ID`, `Type`/`SubType`, `Data`, `Disp`, `Equiv`, `Constraints`
   ([AD-002: Content Model](002-content-model.md)).
5. Pick a skeleton strategy appropriate to the format's structure.
6. Register the reader and writer factories in
   `core/formats/register.go` via an `init()` call.
7. If the format can host embedded content, implement `SubfilterAware`
   and accept `Subfilters []SubfilterMapping` in the config.

See [Implementing Formats](/contribute/notes-internal/implementing-formats) for a
walkthrough, and [Skeleton Store](/contribute/notes-internal/skeleton-store) for the
preferred skeleton strategy details.

## Consequences

- Format readers emit the same streaming Part protocol regardless of
  source format, so tools never need format-specific code.
- Format writers replay `Run.Data` verbatim via `RenderRunsWithData`
  ([AD-002: Content Model](002-content-model.md)), so roundtrip fidelity
  is inherited from the content model.
- Native, plugin, and bridge formats coexist in one registry; the
  pipeline treats them identically.
- MIME/extension/magic/content cascade resolves most files without user
  configuration; ambiguous cases fall back to explicit format flags.
- Three skeleton strategies cover the full span of file formats from
  streaming text to zip-packaged markup.
- New formats plug in by adding a directory and registering in `init()`;
  no core changes needed.
- SkeletonStore gives bounded memory for large markup documents, at the
  cost of a temp file and a binary protocol between reader and writer.

## Related

- [AD-002: Content Model](002-content-model.md) — Parts that readers produce and writers consume; the Run model that drives roundtrip fidelity
- [AD-004: Processing Engine](004-processing-engine.md) — how readers and writers plug into the pipeline
- [AD-006: Tool System](006-tool-system.md) — the tools that sit between reader and writer
- [AD-026: Flow I/O Binding](026-flow-io-binding.md) — readers/writers as the `file` binding; other bindings (store, `.klz`, interchange) feed the same stream
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — how plugin and bridge formats register
- [Implementing Formats](/contribute/notes-internal/implementing-formats) — implementation walkthrough
- [Skeleton Store](/contribute/notes-internal/skeleton-store) — binary skeleton format and wiring
