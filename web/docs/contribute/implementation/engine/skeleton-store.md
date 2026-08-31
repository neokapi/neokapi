---
sidebar_position: 2
title: Skeleton store and streaming
description: Implementation note for E-02. The SkeletonStore entry format and API, the wiring seam every runner shares, the sub-skeleton, the streaming reader and writer protocol, which formats stream and which stay buffered, and the tokenizer-based HTML reader and writer.
keywords: [SkeletonStore, streaming, skeleton, bounded memory, tokenizer, HTML reader, implementation note, neokapi]
---

import { StreamDiagram } from "@neokapi/docs-shared";

# Skeleton store and streaming

Implementation details for the `SkeletonStore` framework type, the seam that
wires it into a run, and the streaming protocol that lets a reader and a writer
share it concurrently. Parent AD:
[E-02](/contribute/architecture/engine/e-02-format-system) (the skeleton store;
streaming readers and bounded-memory I/O).

## SkeletonStore (`core/format/skeleton.go`)

An append-only store of typed entries. The reader writes entries during
extraction; the writer reads them during reconstruction. The pipeline never sees
the skeleton; it carries only blocks.

### Entry format

Each entry is:

```
[type:1 byte] [length:4 bytes big-endian] [data:N bytes]
```

| Type | Constant | Data |
| --- | --- | --- |
| `0` | `SkeletonText` | Non-translatable raw bytes |
| `1` | `SkeletonRef` | Block ID as a UTF-8 string |
| `2` | `SkeletonLang` | Source-locale `lang` / `xml:lang` attribute value (the raw bytes between the quotes), spliced for language retargeting |
| `3` | `SkeletonOriginal` | `EncodeSkeletonPair(rendered, original)`: the text the next ref renders to while unedited, and the source bytes to replay instead |
| `4` | `SkeletonTrimmed` | `EncodeSkeletonPair(rendered, trimmed)`: the text the previous ref rendered to, and the bytes the reader trimmed after it |

`Lang` lets a writer retarget the document language: when the stored value is
the same language as the source locale it emits the target locale, otherwise it
emits the stored value verbatim. The HTML and OpenXML readers emit it. A writer
that does not understand an entry type must treat it as inert; emitting nothing
would drop the attribute value.

`Original` and `Trimmed` make a no-op round trip byte-identical where extraction
normalized whitespace. An `Original` entry applies to the ref that immediately
follows it: if that block is unedited (its rendering equals the recorded one),
the writer emits the original bytes instead of the normalized join and skips
the encoding pass, because the bytes are already the document's own. A
`Trimmed` entry applies to the ref just before it and is emitted only when that
ref rendered as expected. Any other entry between an `Original` and its ref
cancels the pairing. The HTML token reader and the ODF reader emit them;
`core/formats/html/writer.go` is the reference replay.

The store is append-only during writing and sequential during reading. After
`Flush()` the writer reads entries with `Next()` until `io.EOF`.

### API

```go
// Backings
func NewSkeletonStore() (*SkeletonStore, error)              // temp file in os.TempDir()
func NewMemorySkeletonStore() *SkeletonStore                  // in-memory buffer
func NewStreamingSkeletonStore() *SkeletonStore               // channel-backed, for a concurrent pair
func NewSkeletonStoreAt(path string) (*SkeletonStore, error)  // persisted; left in place on Close
func OpenSkeletonStore(path string) (*SkeletonStore, error)   // read an existing persisted store
func NewSkeletonStoreFromBytes(data []byte) *SkeletonStore    // read mode over bytes

// Writing. Write errors are retained and surfaced by Flush and Bytes.
func (s *SkeletonStore) WriteText(data []byte)                // skips empty data
func (s *SkeletonStore) WriteRef(blockID string)
func (s *SkeletonStore) WriteLang(value string)
func (s *SkeletonStore) WriteOriginal(rendered, original []byte)
func (s *SkeletonStore) WriteTrimmed(rendered, trimmed []byte)
func (s *SkeletonStore) Flush() error
func (s *SkeletonStore) CloseWrite()                          // streaming: no more entries
func (s *SkeletonStore) Bytes() ([]byte, error)

// Reading
func (s *SkeletonStore) Next() (SkeletonEntry, error)         // io.EOF at end; blocks on a streaming store
func (s *SkeletonStore) IsStreaming() bool
func (s *SkeletonStore) OriginFormat() string                 // the format that wrote it
func (s *SkeletonStore) Close() error                         // removes a temp file
```

### Interfaces

```go
// Implemented by readers that write skeleton data during extraction.
type SkeletonStoreEmitter interface {
    SetSkeletonStore(store *SkeletonStore)
}

// Implemented by writers that read skeleton data during reconstruction.
type SkeletonStoreConsumer interface {
    SetSkeletonStore(store *SkeletonStore)
}
```

### The wiring seam

The store must be wired before `reader.Read()` is called, because the reader
writes entries as it reads, which means the writer is created before reading.
Every runner wires it through one seam:

- `format.SkeletonPairEligible(reader, writer)` reports whether a pair has a
  skeleton path at all: the reader emits, the writer consumes, and they are the
  same format. A skeleton is meaningless to a foreign writer, and
  `format.WireSkeleton` refuses to connect one.
- `format.NewWiredSkeleton(reader, writer)` creates the temp-file store and
  wires both ends. It returns `(nil, nil)` when the pair has no skeleton path,
  in which case the writer reconstructs from the content model, the documented
  lower-fidelity path. It returns `(nil, err)` when a store is needed and cannot
  be created. That error must never be swallowed: discarding it turns a
  byte-exact same-format pass into a re-serialization from the content model
  while the command still reports success.
- `format.NewWiredStreamingSkeleton(reader, writer)` is the channel-backed
  counterpart for a caller that drives the concurrent protocol below. It cannot
  fail, so it returns nil only for a pair with no skeleton path.

The file runner (`core/flow/filerunner.go`) decides per run with two
predicates. `streamingFeed(reader, preReadContent)` is true when the reader
declares `StreamingReader` and nothing pre-read the source;
`wireSkeleton(reader, writer, streaming)` then picks
`NewWiredStreamingSkeleton` when the writer also declares `StreamingWriter`,
and `NewWiredSkeleton` otherwise. The other call sites (`host/toolrun.go`, the
toolbox edit, convert and archive paths, `host/venue/source`) call
`NewWiredSkeleton` directly and fail the file on error. The runner closes the
store explicitly on each error path and at completion rather than through a
single `defer`, because the writer outlives the function through the
temp-then-rename output.

## Sub-skeleton: translatable spans inside an opaque payload

Some extractable content is embedded *inside* a payload the reader otherwise
captures opaquely and replays verbatim: the natural-language prose (`<m:nor/>`
runs such as "where", "otherwise", units) inside a Word equation's OMML, which
is captured as one opaque sentinel run for a byte-exact DOCX round trip
([M-04](/contribute/architecture/multilingual/m-04-math-and-equations)). A flat
`Text` / `Ref` skeleton cannot reach into that blob.

The **sub-skeleton** is the same `Text` / `Ref` mechanism applied recursively
over the opaque bytes: instead of emitting the whole payload as one `Text`
entry, the reader emits the verbatim byte segments *between* the translatable
spans as `Text` and a `Ref` to a translatable block for each span. No new entry
type is needed; these are ordinary `WriteText` / `WriteRef` calls whose `Text`
happens to be slices of an opaque blob rather than top-level structure.

For OMML (`core/formats/openxml/omml_math.go`, `writeOMathSubSkeleton`):

1. `xmath.NorSpans(raw)` (`core/math/nor.go`) returns each prose span's text
   plus its **byte offset range** into the raw OMML (captured via
   `xml.Decoder.InputOffset`).
2. The writer validates that the offsets are monotonic and in range. If they
   are not, or there are no spans, it writes the payload verbatim, so a
   pure-math equation is unaffected.
3. Otherwise it walks the spans, emitting `skelText(raw[cursor:span.Start])`
   (verbatim OMML) then `skelRef(blockID)` for a translatable `omml-nor` block,
   advancing `cursor` past the span.

On write, the openxml `renderBlock` renders an `omml-nor` block as **bare
element-content text** (`xmlEscape`, matching `captureRawElement`'s CharData
escaping) so it lands directly inside `<m:t>…</m:t>`. An untranslated span
therefore reproduces the original bytes exactly; a translated one splices the
translation in place while the surrounding math is replayed byte for byte. The
cross-format writers (markdown, doclang) **skip** `omml-nor` blocks, because
the prose already rides inside the formula's LaTeX carrier, so the spans are
translated for the DOCX round trip without being duplicated on export.

## Streaming

### The protocol

A reader that declares `format.StreamingReader` is fed straight into the
executor rather than collected into a slice, so it runs concurrently with the
writer. A writer that declares `format.StreamingWriter` consumes the
channel-backed store interleaved with the Part stream: it pops entries as the
reader appends them and takes each `SkeletonRef`'s block from the stream on
demand (`format.StreamSkeletonWrite`), instead of holding the whole block map
until a flush. Because a streaming reader emits refs and their blocks in the
same order, the pending-block window stays small.

The reader's feeder calls `CloseWrite` when the stream ends. A writer that calls
`Next()` on a streaming store before that blocks, which is why the streaming
store is wired only for a pair that both stream and only when the feed is
concurrent (`streamingFeed`); pairing it with a buffered feed would deadlock.
Output is byte-identical to the buffered path: the same entries in the same
order, consumed interleaved rather than after a flush. A writer signals it took
the streaming path by checking `SkeletonStore.IsStreaming()` in `Write`.

The markers are bare (`StreamingReader()` / `StreamingWriter()`), probed with
`IsStreamingReader` / `IsStreamingWriter`. Only an in-process reader may
declare them; a daemon-backed plugin reader never does, because the plugin
protocol reads fully before it writes.

### Which formats stream

A format streams when both its reader and its writer declare the markers. The
generated [Format Reference](/formats) is the authoritative list; the substrates
are:

| Substrate | Formats | How the reader bounds memory |
| --- | --- | --- |
| Line and record readers | `properties`, `srt`, `messageformat` | one record at a time from a `bufio` window; `messageformat` parses one message per line and surfaces a syntax error on the Read channel rather than from `Open`, which the file runner and the subfilter driver both propagate |
| JSON token stream | `json`, and `i18next` over it | `core/formats/json/streamscanner.go` tokenizes from an `io.Reader` behind the `tokenStream` seam (`tokenstream.go`), so the forward, ancestor-only walk (`walkTokenValue`) is unchanged and never materializes the token slice. `readContentStreaming` is taken for a same-format skeleton round trip with validation off; validation mode needs the buffer for its snippet windows and the cross-format path stores `json.original` |
| JSON-substrate catalogs | `arb`, `designtokens`, `xcstrings` | a bounded `streamScanner` / `json.Decoder` twin emits the identical token sequence. `arb` and `designtokens` run a two-pass over a re-openable file (pass 1 a bounded metadata pre-scan, pass 2 the stream); `xcstrings` buffers one entry subtree at a time. A non-file input falls back to the buffered walk |
| `encoding/xml` | `xml`, `tmx`, `ts` | a `CaptureReader` (`core/format/capturereader.go`) resolves `InputOffset()` ranges to raw bytes over a sliding window, and skeleton is flushed at record boundaries. `xml` runs a two-pass read: global `<its:rules>` are extracted from a re-opened copy first (`its.ExtractRulesReader`), then the extraction streams, since the ITS resolver is ancestor-only. `tmx` additionally requires UTF-8 input, because its UTF-16 transcode is whole-document |
| Custom tokenizers | `resx`, `androidxml` | a streaming variant of the format's own tokenizer buffers one entry subtree at a time and retains the `.original` layer property only when no skeleton store is wired |

The writers in the `encoding/xml` and custom-tokenizer groups stream too, with
the shared `StreamSkeletonWrite` helper or a small rolling window for composite
refs. Each streaming format is its own byte-exact conversion, validated by its
skeleton suite through the streaming path and by a bounded-memory test over an
`io.Pipe`-fed document.

### What stays buffered, and why

- **HTML.** Leaf-or-container classification (`forwardScanForBlockChildren`)
  needs forward lookahead to the parent's end tag, whose distance is unbounded,
  while the streaming tokenizer's buffered window is a few kilobytes. Streaming
  would misclassify large leaf blocks as containers and break byte-exactness,
  so the buffered reader stays.
- **YAML and Markdown / MDX.** `yaml.v3` and `goldmark` materialize a whole AST,
  and byte mapping needs the full parse.
- **applestrings.** The reader transcodes UTF-16 to UTF-8 and detects the BOM
  over the whole document before parsing.
- **Packaged formats** (OpenXML, ODF, EPUB, IDML, ICML) stream at the
  archive-entry level ([E-04](/contribute/architecture/engine/e-04-flows-and-io-binding)),
  not within an entry.
- **Validation mode and malformed-input byte offsets** need the buffer on every
  format and are gated to the buffered path.

The guard for the buffered formats is the `core/safeio` input cap and bounded
concurrency.

## HTML tokenizer reader (`core/formats/html/tokenreader.go`)

Single-pass reader using Go's `html.Tokenizer` (from `golang.org/x/net/html`).
No `html.Parse()`, no DOM tree, no pre-scan pass. Writes skeleton entries as it
processes tokens.

### Element classification

When the tokenizer enters a block-level element, it needs to know whether the
element is a **container** (has block-level children) or a **leaf block**
(contains only inline content). Instead of a pre-scan pass over the entire
document, the reader **forward-scans** from the current position through the
element's buffered content:

- If any direct child is a block-level start tag, the element is a
  **container**: mixed content mode, where the element's start and end tags go
  to the skeleton and the children are processed recursively.
- If no block children are found by the end tag, the element is a **leaf
  block**: its content is extracted as a translatable block with inline spans.

The forward scan skips inline element subtrees (tracking depth) and only checks
direct children. After classification, the scanner's buffered tokens are
replayed for processing.

### Token processing

| Token type | Action |
| --- | --- |
| Non-translatable element start (`<script>`, `<style>`) | Write raw bytes to skeleton, consume until close tag |
| Block-level element start (container) | Write start tag to skeleton, process children recursively |
| Block-level element start (leaf) | Extract translatable attributes as skeleton refs, buffer content, build a `[]Run` for the block |
| Inline element start/end | Part of leaf block content; becomes a paired-code run (`PcOpen` / `PcClose`) |
| Text token | Part of leaf block content; appended as a `TextRun` |
| Comment | Written to skeleton (non-translatable) |
| Doctype | Written to skeleton |

### Translatable attributes

For elements with translatable attributes (`title`, `alt`, `content` on meta
tags), the reader splits the raw tag bytes at attribute value boundaries to
create interleaved skeleton text and ref entries:

<StreamDiagram
  title={'<p title="Tooltip">'}
  items={[
    { kind: "skeleton.WriteText", detail: `'<p title="'`, role: "meta" },
    { kind: "skeleton.WriteRef", detail: "tu1", role: "block", note: 'block for "Tooltip"' },
    { kind: "skeleton.WriteText", detail: `'">'`, role: "meta" },
  ]}
/>

The `findAttrValueRange` function locates the byte range of an attribute value
within the raw tag bytes by scanning for `attrKey=` followed by a quote
character.

`lang` / `xml:lang` attribute values are handled the same way, but spliced as a
typed `SkeletonLang` entry rather than verbatim text (`extractLangFromToken`),
so the writer can retarget the document language on output instead of emitting
the source-locale value.

### Run sequence building

For leaf block elements, tokens between start and end tag are collected and
built into a `[]model.Run` through the HTML `runBuilder`
(`core/formats/html/run_builder.go`):

- Text tokens append a `TextRun` (`AddText`, which coalesces adjacent text).
- Inline element open and close become a paired `PcOpenRun` / `PcCloseRun`
  (sharing an `ID`) with `Data = string(raw)`, which preserves the original
  quote style, attribute order and whitespace.
- A self-closing inline element becomes a `PlaceholderRun`.
- A comment within inline content becomes a `PlaceholderRun`.

Where the reader normalized a leaf block's whitespace, it writes the source
bytes beside the ref as a `SkeletonOriginal` entry, so an unedited block
replays the document's own bytes.

## HTML writer skeleton mode (`core/formats/html/writer.go`)

When a skeleton store is available, the writer reads entries sequentially and
fills in block content, with no tokenizer, no DOM and no state machine beyond
the pairing rules for `Original` and `Trimmed`. `writeFromSkeleton` handles
each entry type:

- `SkeletonText`: written through verbatim.
- `SkeletonOriginal`: decoded and held for the ref that immediately follows.
- `SkeletonRef`: the block's text is rendered. If a held `Original` matches the
  rendering, the original bytes are written instead and the encoding pass is
  skipped; otherwise the rendering is HTML-encoded and written. Attribute refs
  still substitute inside a replayed span.
- `SkeletonTrimmed`: written only when the previous ref rendered as the entry
  expects.
- `SkeletonLang`: when retargeting and the stored value is the same language as
  the source locale, the writer's target locale is emitted; otherwise the stored
  value verbatim.

### Writer fallback chain

The writer tries three modes in order:

1. **Skeleton store**, byte-exact: available when
   `SkeletonStoreConsumer.SetSkeletonStore()` was called.
2. **Re-parse original content**: re-parses the original HTML with a DOM walker,
   patches translations into the tree, and renders back. Requires
   `OriginalContentSetter.SetOriginalContent()` or
   `SourcePathSetter.SetSourcePath()`.
3. **Block-only output**: reconstructs a document from the content model and the
   structural layer. This is the cross-format path (DocLang, Docling or DOCX
   content rendered as clean HTML), and the last resort when no original content
   is available.

## Files

| File | Role |
| --- | --- |
| `core/format/skeleton.go` | SkeletonStore type, entry format, interfaces, the wiring seam |
| `core/format/skeleton_stream.go` | `StreamSkeletonWrite`, the shared streaming writer helper |
| `core/format/capturereader.go` | `CaptureReader`, the sliding window for `encoding/xml` readers |
| `core/formats/html/tokenreader.go` | Single-pass tokenizer reader |
| `core/formats/html/reader.go` | Dispatch: skeleton store → tokenizer, else → DOM |
| `core/formats/html/writer.go` | Skeleton mode, re-parse fallback, block-only fallback |
| `core/formats/html/roundtrip_test.go` | Byte-exact, translation, and attribute round-trip tests |
| `core/formats/json/streamscanner.go`, `tokenstream.go` | The streaming JSON tokenizer and the seam the walk reads through |
| `core/flow/filerunner.go` | `streamingFeed` and `wireSkeleton`: the per-run streaming and skeleton decision |
| `host/toolrun.go` | `NewWiredSkeleton` on the tool path, failing the file on error |
