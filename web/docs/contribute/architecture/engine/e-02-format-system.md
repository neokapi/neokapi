---
id: e-02-format-system
sidebar_position: 2
title: "E-02: The format system"
description: "Formats are pluggable reader/writer pairs that convert between on-disk files and the Part stream, with one skeleton store for byte-exact write-back and a three-way reader output policy that surfaces non-translatable context as content."
keywords: [neokapi, architecture decision, format system, DataFormatReader, DataFormatWriter, skeleton, streaming, detection, content fidelity, non-translatable content]
---

# E-02: The format system

## Summary

Formats are pluggable readers and writers that convert between on-disk
representations and the Part stream. The framework ships a broad set of built-in
formats under `core/formats/`, each implementing `DataFormatReader` and
`DataFormatWriter` on top of shared `BaseFormatReader` / `BaseFormatWriter`
embeds. A single `FormatRegistry` exposes a factory-based lookup that serves
built-in and plugin formats uniformly. Detection cascades through extension,
priority, and content sniffing. Byte-exact write-back rides one skeleton store
that every reader and writer pair wires the same way, and a reader classifies
its input **three** ways: translatable content, pure structure, and
non-translatable-but-meaningful context that is *surfaced* rather than buried.

## Context

The framework must read a large variety of file formats and write them back with
byte-exact fidelity: every newline, every entity reference, every attribute
quote style. Formats vary widely in structure: linear text (plain text,
Markdown), tree-structured markup (HTML, XML, OpenXML), line-oriented key-value
(Java properties, Apple strings), grid-based (CSV, spreadsheets), and
exchange-specific (XLIFF, TMX, Gettext).

Formats also frequently contain embedded content in another format (HTML inside
JSON, Markdown inside CSV), and the reader/writer contract must accommodate that
recursion without special cases.

A second consumer complicates the picture. The translation path wants only the
prose a human would translate; model ingestion wants *all* textual context:
code listings, captions, alt-text, formulas, do-not-translate strings,
config-excluded values, and comments. A two-way classification, where a fragment
becomes either a translatable Block or opaque skeleton bytes, makes everything
the first consumer skips invisible to the second.

## Decision

### Reader and writer interfaces

These interfaces implement the `file` *source* and *sink* binding in
[E-04](e-04-flows-and-io-binding.md). Other bindings (the project store, a
`.kpz` workspace, interchange import/export) feed and drain the same `Part`
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
`model.PartResult{Part, Error}`: the reader produces Parts until the document is
exhausted or an error occurs, then closes the channel. `Close` releases held
resources.

The writer lifecycle is `SetOutput → Write → Close`. `Write` consumes a channel
of `*model.Part` until the channel closes, producing output on the writer's
destination.

Every read and write runs inside a span on the framework's observation seam
(`format.read`, `format.write`;
[E-01](e-01-processing-engine.md#observation-seam)), which is a no-op unless a
host registers a tracer.

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
time, so these exist only to control *output* style; they are writer
configuration rather than a pipeline stage. They are set under the reserved `output` key of the ordinary
per-format config (`defaults.formats[<id>].config` in a `kapi.yaml` recipe);
`format.SplitOutputConfig` strips that key before per-format reader/writer config
is applied, and the base writer wraps its output stream with the post-encode chain
(newline conversion → BOM policy → charset encoding), so every writer that embeds
the base inherits the behaviour with no per-format code.

### Built-in formats

The built-in formats under `core/formats/` span several families:

- **Markup**: HTML, XML, Markdown / MDX, AsciiDoc, and structured-document
  formats.
- **Translation exchange**: XLIFF 1.2 / 2.x, TMX, Gettext PO/MO.
- **Structured data**: JSON, YAML, CSV/TSV, and design-token and app
  message-catalog variants (`xcstrings`, `arb`, `i18next`, `resx`, Android
  strings, Apple strings, …).
- **Office and publishing**: OpenXML (`.docx`, `.xlsx`, `.pptx`), ODF, EPUB, and
  related packaged formats.
- **Subtitle and media**: SRT, VTT, plus audio, video and image readers. PDF
  and source code arrive as plugin formats (`kapi-pdfium`, `kapi-sourcecode`;
  [E-05](e-05-plugin-system.md), [E-08](e-08-document-structure-tiers.md)),
  with their config definitions under `core/formats/` so each format has one
  schema wherever its reader runs.

The full, authoritative list of registered formats (with extensions, MIME types,
and per-format options) is the generated [Format Reference](/formats). It is
derived from the live registry, so it never drifts from the code.

A format package under `core/formats/<name>/` typically contains `reader.go`,
`writer.go`, and `config.go`; a read-only format has no writer.
`formats.RegisterAll(reg, opts...)` in `core/formats/register.go` registers
every built-in factory, and the host calls it when it builds its registry.

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

Two tiers of registration make built-in and plugin formats indistinguishable to
callers:

1. **Built-ins**: registered by `formats.RegisterAll` when the host starts.
2. **Plugin formats**: registered by `pluginhost.RegisterModeCFormats` from the
   `formats` capability each plugin's `manifest.json` declares, read from disk
   during discovery without launching a subprocess. The manifest seeds the
   format's metadata (name, extensions, MIME types, the `generative` and
   `interchange` capabilities); the reader and writer factories dial the
   plugin's Mode-C daemon on demand ([E-05](e-05-plugin-system.md)). A plugin
   format with the same name as a built-in overrides it, because installing a
   plugin for a format is an explicit signal to prefer it; two plugins claiming
   one name keep the first registered.

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

kapi's own on-disk conventions use **compound suffixes** (`.kbf.json`,
`.memory.json`, `.terms.json`, `.overlays.json`, `.overlays.jsonl`) so the
marker survives while the file still reads as the JSON it is. `path/filepath.Ext`
reports `.json` for all of them, so extension-driven code goes through
`format.Ext` / `TrimExt` / `Stem` instead, which return the most specific
registered suffix.

### The skeleton store

Non-translatable content is preserved for write-back by one mechanism:
`format.SkeletonStore`, an append-only sequence of typed entries the reader
emits during extraction and the writer replays during reconstruction. The
pipeline never sees it; tools carry only blocks. A reader that produces one
implements `SkeletonStoreEmitter`, a writer that consumes one
`SkeletonStoreConsumer`, and nearly every built-in format does both. A skeleton
is the typed scaffolding of one format, so `format.WireSkeleton` connects a
reader to a writer only when they are the same format (see "Skeletons are typed
per format" below).

The store has several backings behind one API: a temp file
(`NewSkeletonStore`), memory (`NewMemorySkeletonStore`), a concurrent channel
for streaming pairs (`NewStreamingSkeletonStore`), and a persisted file that
outlives the process (`NewSkeletonStoreAt` / `OpenSkeletonStore`,
`NewSkeletonStoreFromBytes`), which is how a `.kpz` workspace carries a skeleton
between `extract` and `merge`. `format.NewWiredSkeleton(reader, writer)` is the
one seam every runner uses to give a same-format round trip its store. It
returns no store for a pair with no skeleton path, and it returns an error when
a store is needed and cannot be created; that error is never swallowed, because
silently reconstructing from the content model would drop structure the reader
could not model while the command still reported success.

Two entry types make an untouched document round-trip byte for byte even where
extraction normalized its text. A reader that collapses whitespace on the way in
writes the source bytes beside the reference (`SkeletonOriginal`); the writer
replays those bytes while the block is unedited and renders the block only once
it has changed. `SkeletonTrimmed` does the same for the bytes a reader trimmed
after a block. Extraction still yields clean text, and a no-op round trip is the
identity.

A writer given no store falls back in a fixed order: re-parse the original
document (`OriginalContentSetter` / `SourcePathSetter`) and patch translations
into it, or, failing that, build the document from the blocks alone. The
fallbacks trade fidelity for availability and exist for the cross-format and
no-source cases; a same-format run always has the store. See
[Skeleton store and streaming](/contribute/implementation/engine/skeleton-store)
for the entry format, the sub-skeleton and the streaming protocol.

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
   after it is fully written: O(blocks) memory. A reader and writer that both
   declare streaming (**`StreamingReader`** + **`format.StreamingWriter`**)
   instead share a **concurrent, channel-backed skeleton store**: the reader
   appends entries while the writer pops them, consuming each `SkeletonRef`'s
   block from the Part stream *on demand* (`format.StreamSkeletonWrite`). Because
   a streaming reader emits refs and their blocks in the same order, the
   pending-block window stays small. **Output is byte-identical to the buffered
   skeleton path**: the same entries in the same order, just consumed
   interleaved instead of after a flush. Both capabilities are bare markers
   (`StreamingReader()` / `StreamingWriter()`), probed via `IsStreamingReader` /
   `IsStreamingWriter`; a writer signals it took the streaming path by checking
   `SkeletonStore.IsStreaming()` in `Write`.

A format streams when both its reader and its writer declare the markers. The
record- and entry-oriented formats do, holding only the in-progress unit, and
the generated [Format Reference](/formats) says which. The file runner decides
once per run, in two steps (`streamingFeed`, then `wireSkeleton`): a streaming
reader with no pre-read source is fed concurrently, and a streaming writer then
receives the channel-backed store; any other pair receives the temp-file store.
Each streaming writer's `SkeletonRef` rendering is factored into a shared helper
so the buffered and streaming skeleton paths are byte-identical.

A format that must materialise its whole input to parse it (a packaged
zip-backed format, a DOM-building markup reader) does not declare the
capability, and a uniform fallback keeps its output byte-identical. The container
path drives one archive entry through `FileRunner.RunStream` (bytes in, bytes
out, no temp file); a streaming-capable inner format is not even buffered whole
([E-04](e-04-flows-and-io-binding.md)).

### Writer output modes: generative vs skeleton-bound

A skeleton is **format-specific**: it is the non-translatable scaffolding of
*one* file, captured by that format's reader. So a writer's ability to produce
output depends on whether it can build a whole document from the content model
alone, or only by injecting translated text back into a skeleton it was given.
Two **orthogonal** capabilities capture this:

- **Generative**: the writer can serialize a complete, valid document from the
  content model (roles, runs, structure) with no skeleton.
- **Skeleton-consuming**: the writer uses a skeleton *when given one*, for
  byte-exact fidelity, via the `SkeletonStoreConsumer` interface. It says the
  writer will use a skeleton, and says nothing about whether it needs one.

These compose into three writer classes:

- **Generative document and data writers** (`generative`, not interchange). HTML
  is the archetype: with the source file's skeleton it round-trips losslessly,
  and **without** one it still writes a clean document, so it is also a target
  for content that arrived from a different format. Markdown, DocLang, AsciiDoc,
  plain text, and the data and catalog formats behave the same. **These are the
  `convert` targets.**
- **Bilingual interchange writers** (`generative` **and** `interchange`). XLIFF,
  PO, TMX, MO, and KBF are generative *files*, but they belong to the
  extract → translate → merge loop, not to document conversion: `kapi extract`
  captures the source skeleton so `kapi merge` can round-trip translations back
  into the *original* format. A `convert`-produced interchange file carries no
  skeleton and cannot be merged back (a dead end), so interchange formats are
  **excluded as `convert` targets** and reached via `extract`/`merge`
  ([M-01](../multilingual/m-01-bilingual-interop.md)).
- **Skeleton-bound writers** (not generative). Packaged formats (OpenXML, ODF,
  EPUB, image) wrap content in a fixed package that cannot be regenerated from
  the model; they only ever write back into their *own* skeleton. Same-format and
  merge writers, never a cross-format target.

**Cross-format conversion** ([S-04](../surfaces/s-04-toolbox.md)) reconstructs the
target from the content model and never carries a foreign skeleton into the
writer. A writer is a valid conversion *target* iff it is **generative and not
interchange**. Both are **declared writer capabilities**: the writer states what
it can write via `GenerativeWriter.Generative()` (the inverse of
`BaseFormatWriter.RequiresSkeleton`) and `InterchangeWriter.IsInterchange()`. The
registry records them on `FormatInfo.Generative` / `FormatInfo.Interchange`:
probed once from the built-in writer at registration, and for plugin formats taken
from the cached manifest's `generative` / `interchange` capabilities, so
conversion, the [Conversion Lab](/lab/convert), and `kapi formats` read one
authoritative source **without loading any plugin**. Neither is derived from
`SkeletonStoreConsumer` (nearly every writer consumes a skeleton if offered, so
that bit does not distinguish a target) nor probed empirically.

Two derived flags sit beside them. `FormatInfo.RoundTrip`, probed from
`SkeletonStoreConsumer`, says the writer reconstructs from a skeleton, so an edit
changes only the edited text. `FormatInfo.Editable` (a reader and a writer, and
not interchange) is the set a caller-supplied content edit can target.

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
registration; a plugin format declares none. `kapi formats` and the writer
selection therefore read one authoritative source without loading any plugin. A
writer that implements nothing carries no marks.

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
a span that does not, and a term running from before a `<pc>` to after it is
exactly what an `Anchor` is built to express. Because the two markers are
independent nodes, each is placed wherever its own boundary lands, including
inside an element whose partner sits outside.

A span a segmentation cannot carry, one straddling two `<segment>` elements,
is recorded on the writer (`UnplacedTermMarks`) rather than half-drawn.

**Skeletons are typed per format.** A `SkeletonStore` carries an `OriginFormat`
stamp, and `format.WireSkeleton(store, reader, writer)` connects a reader's
skeleton emission to a writer **only when they are the same format**, so the
rule that a skeleton from format A is foreign to format B's writer is enforced
centrally, not left to each call site. A cross-format conversion therefore never
feeds a foreign skeleton into the target writer; that writer takes the generative
content-model route every writer shares.

### Reader output policy: three destinations

The skeleton is not the only home for non-translatable content. A reader
classifies each fragment three ways:

| Fragment | Destination |
| --- | --- |
| Translatable prose | `Block{Translatable: true}`; the pipeline processes it |
| Pure structure (delimiters, quoting, whitespace) | skeleton bytes |
| Non-translatable but meaningful context | **surfaced**; see the two channels below |

The third category (code, verbatim and literal text, captions, alt-text,
formulas, do-not-translate strings, config-excluded values, comments) is
surfaced rather than collapsed into the second. It becomes content an ingestion
consumer can read, while staying outside the translation payload.

This introduces no new content-model type. The `Translatable` flag, the
`SemanticRole` taxonomy, `Data`, and notes are all the content model's
([F-02](../foundations/f-02-content-model.md)); the skeleton and sub-skeleton
mechanisms are this AD's.

#### Two surfacing channels

What a fragment *is* determines which channel carries it. Renderable content,
text that has a place in the rendered document, becomes a content block;
out-of-band annotation, text *about* the document, becomes data or a note.

| Channel | Carrier | Used for | Round-trip |
| --- | --- | --- | --- |
| Renderable contextual content | `Block{Translatable:false}` + `SemanticRole` + skeleton ref | code blocks, literal text, captions, alt-text, formulas, do-not-translate strings, config-excluded values | verbatim bytes stay in the skeleton; the surfaced body rides a skeleton ref, so the writer replays the original exactly |
| Comment / metadata context | `Data` part or a note annotation | developer and translator comments, review annotations, editorial notes | the comment bytes round-trip verbatim through the skeleton; the surfaced copy is informational only |

A block from the first channel carries the role that names its kind (alt-text as
`RoleCaption`, a code listing as `RoleCode`, an equation as `RoleFormula`, a
non-translatable cell as `RoleTableCell`), and is flagged so translation skips it
([E-07](e-07-model-providers.md)). The second channel keeps comment context as
*data*: a comment is not part of the rendered text, so promoting it
to a content block would misrepresent the document's structure; it stays a `Data`
part or a note that ingestion can read and the editor can show.

#### The comment layer

A check reads comments as prose through a third path that leaves both channels
as they are. The comment layer (`core/comment`) locates each comment with a
half-open byte span and an inclusive line range, and builds a block from it for
checking. Those blocks never enter a reader's part stream, so round-trip and
parity are unaffected.

A file no format reader covers reaches the layer through a language provider.
The Go provider uses `go/parser` for positions and `go/doc/comment` for the
interior, from the standard library. It classifies directives, generated files,
the cgo preamble and example output as not addressable, and turns code blocks
and references to declarations into placeholders, so a check reads sentences
only. The parser consumes a list item's marker, so each item opens with a
placeholder of type `list:item` (`comment.TypeListItem`) that keeps one item's
prose apart from the next. A provider is not a format: nothing registers one in the format registry,
and a recipe declares such files with `comments: true` on a content item, which
every path that reads values then passes over (`ResolvedFile.CommentsOnly`).

A plugin can supply language providers too. The sourcecode plugin lists the
languages it reads in its manifest, such as TypeScript, Python and CSS, and the
host registers a provider for each language's extensions that sends a file's
bytes to the plugin and reads back the located comments. The plugin parses each
file with the language's tree-sitter grammar, so a marker inside a string, a
template literal, a regular expression, JSX text, a heredoc or an unquoted CSS
`url()` is content. It sets aside the directives the language's tools read, such
as `eslint-disable`, `# noqa`, `# shellcheck` and the shebang, every comment in
a file whose header says a generator owns it, and blank comment lines. In
TypeScript and JavaScript a `/** */` block is a doc comment only when nothing
but whitespace separates it from a declaration, and its tags, inline links and
code spans are placeholders. In Rust `///` and `/** */` document the item after
them and `//!` and `/*! */` the module they sit in. Java's Javadoc and C#'s XML
documentation comments document the declaration after them, and their HTML and
XML tags are placeholders. C and C++ files are also read by a lexical scan that
shares nothing with the grammar, and a file is located only when the tree
reports a comment at exactly the spans the scan reads. A file whose macros leave
syntax errors in the tree is still located, and one where the grammar misplaces
any comment is not. A comment has the subject `comment` and documents nothing
when the declaration it documents, or one its subject path names, holds a
syntax error. A Ruby file
the sourcecode format reads has its declared comments read through the same
plugin's comment provider, beside the format's strings: a format that supplies
no comments of its own falls back to the provider for the file's language. A file the grammar cannot parse whole is not
located, and its comment check did not run. The canary comes from
the plugin's manifest and goes through the plugin beside every real file. When no
installed plugin reads a declared file's language, a check over the project
reports the file unread and names the plugin to install.

A format whose reader parses a file can supply that file's comments too. Such a
provider scans the bytes and refuses a document whose comments its scan and the
format's parser disagree about, so a comment is never placed approximately. The
YAML format scans for comments outside quoted scalars and block scalar bodies,
and holds the scan to the comment lines the YAML parser reports.

Every format whose reader reads a plain XML file shares the XML format's
provider, registered under each format's own name. It scans for `<!-- -->`
outside tags, CDATA sections, processing instructions and declarations, and
holds each comment to the offsets `encoding/xml` reports. It refuses a document
the parser rejects, such as one holding `--` inside a comment, which the XML
specification forbids, and one with a comment inside its DOCTYPE, which the
parser reads as part of the declaration. Each comment is named for the element
it sits on, as in `comment/resources/string[greeting]`. A line of commented-out
markup becomes a placeholder, and the suppressions Prettier, the JetBrains IDEs
and ReSharper read are directives. A format that keeps its XML inside an archive
supplies no comments, because a comment there has no span in the file.

The HTML format reads comments by the HTML tokenizer's rules. It scans for
`<!--` outside tags, doctypes, bogus comments and the text of raw text elements
such as `<script>`, `<style>` and `<textarea>`, closes a comment on `-->` or
`--!>`, and reads `<!-->` and `<!--->` as empty comments. It holds each comment
to the offsets the `x/net/html` tokenizer reports, and to the comments the HTML
parser finds, which reads SVG and MathML content by rules of its own. It refuses
a document the three disagree on, and one that ends inside a comment. A comment
is named for the element that follows it, as in `comment/p[greeting]`.
Conditional comments, server-side includes, markdownlint instructions and the
markers React writes into a page it renders on the server are directives.

The Markdown format reads a document as its reader does, with front matter set
aside and the body parsed by the Markdown parser. A comment is an inline HTML
comment, or a comment inside an HTML block, which the HTML format's scan
locates. Every `<!--` in the body must open one of those or sit in content, such
as a code span, a fenced or indented code block, an HTML tag or a backslash
escape, or the document is refused. So is a comment closing on `--!>`, which the
Markdown parser and an HTML parser close in different places. A comment is named
for the section it sits in, as in `comment/install/from-homebrew`. The
Docusaurus `truncate` marker, the region kapi writes for a voice pointer, the
markers of a generated region and markdownlint and Prettier instructions are
directives.

The MDX format reads a document with its reader's own scan, which separates
Markdown from ESM statements, JSX elements and top-level expressions. A comment
is a top-level expression that holds one `/* */` comment, such as
`{/* A note. */}`. Every `{/*` and `<!--` in the file must open such a comment,
sit inside one, or sit in content: code or front matter in a Markdown span, which
the Markdown format reads, code in the Markdown children of a JSX element, or an
ESM statement. A document with an expression
comment inside a JSX element or a paragraph, an HTML comment, which MDX does not
allow, or an expression holding more than one comment is refused. A file whose
first comment says `DO NOT EDIT` belongs to its generator, and its comments are
set aside as generated. Prettier, Docusaurus `truncate`, markdownlint and ESLint
instructions are directives.

The PO format reads a comment as a line that opens with `#`, other than an
obsolete entry's `#~` line, and holds its scan to the comments the PO reader
parses. Translator comments are prose, named for the entry they sit in, as in
`comment/menu/Goodbye`. An extracted comment (`#.`) is rewritten by the next
extraction and set aside as generated, and references, flags and a previous
msgid are read by gettext's tools and set aside as directives. A file the
reader reads transcoded, such as one in UTF-16, is refused, since a span in the
text the reader reads is not a span in the file.

The properties format reads a comment as a line that opens with `#` or `!`
outside a value's continuation, holds its scan to the lines the properties
reader reads as comments, and names each comment for the key that follows it.
Okapi's extraction directives, such as `#_skip`, and IntelliJ's
`# suppress inspection` are directives.

For a file its reader parses, `comments: true` adds the comment blocks to the
reader's blocks for checking, and the file converges through its reader
unchanged. `comments: {only: true}` makes the comment blocks the file's whole
content: the format's provider locates them, the reader never reads the values,
and a check that asks for reader validation reports it unsupported. One
predicate, `ResolvedFile.CommentsOnly`, holds for such a file and for a file no
reader covers, and every path that reads values passes over the file by it:
extraction and drift detection (`project.ExtractToBlockStore`,
`project.CompareSourceStamps`), convergence, flow runs, merge, `kapi extract`,
the source and target units behind coverage, the plan and the ship gates, the
implicit inputs of `kapi stats` and `kapi inspect`, and the scan a push reads.
A check reads such a file through `checkFormats.commentsOnly` and a source
unit narrowed by `VerifyUnit.OnlyComments`, for which `readSource` reads no
value. Loading rejects `only` beside a target, target languages, a redaction, or
a reader's config or preset. A declared format that supplies no comments leaves the comment check
not run, and a format with no comment formatter reports the formatter as
unsupported.

Every provider passes one conformance suite, `core/comment/commenttest`. A
provider's test supplies fixtures and its own scan of the same bytes, made
without the provider, and the suite holds what the provider located to that
scan. Each comment sits in exactly one addressable comment or one exclusion. A
span begins at the comment's opening marker and ends with the comment, and its
line range is counted from the bytes. The runs keep no marker, a marker inside a
string or other literal context is never a comment, and the provider's canary
is located with its doubled word flagged by the hygiene check.

A provider sets aside the directives its language's toolchain reads. The
markers a project's own tools read, such as the `okapi-skip:` lines a contract
audit collects, are declared in the recipe, under `defaults.comments.directives`
or an item's `comments: {directives: [...]}`, and the layer sets them aside for
every provider in the same way (`comment.Locate`). A comment line whose text,
after its comment marker and leading whitespace, starts with a declared marker
is an exclusion with `ReasonDirective` and the marker as its form. A marker
inside a comment splits it. The provider reads the file a second time with the
marker lines blanked, each piece keeps the subject of the comment it came from,
and the layer holds that reading to the first byte for byte: away from the
markers it must locate the same comments with the same bytes, and inside a split
comment its pieces and the markers must hold every byte once. A reading that
disagrees leaves the file's comments unlocated. What a comment documents comes
from the first reading, because blanking a line changes what sits between a doc
comment and its declaration, and a provider may attach it differently in the
second. A provider's share of this is `LineText`, which reads
one comment line and removes its marker. A line inside a delimited comment that
runs over several lines holds no marker of its own, so no declared directive
marks it.

A recipe can also place a file's comments at a governance point of their own,
with an item's `comments: {channel: ...}` or `defaults.comments.channel`. A
check then holds each comment block to the voice and terms of that point and
each block the reader extracts to the item's point
([C-02](/contribute/architecture/context/c-02-coordinates-and-governance)). The
layer marks its blocks (`comment.IsBlock`), which is how a check tells the two
apart.

A block also records whether its comment documents a declaration
(`comment.doc`) and whether it documents the package or module its file belongs
to (`comment.package`, `comment.PackageDoc`). The Go provider and the sourcecode
plugin's Java provider give that comment the subject `package`, and the Rust
provider gives a file's inner doc comment the subject `module`. No other
provider has one. The comment limits a voice profile sets
([Checks](/framework/checks#the-check-families)) read both marks to choose the
limit a comment is held to. `File.LineKinds` classifies each line of a file
from the spans a provider located, as code, comment, package doc, set aside or
blank, and the density limit counts a change's added lines with it.

A comment can also be rewritten. A provider that implements `comment.Rewriter`
renders prose into its language's comment syntax, and `comment.Rewrite` holds
the result to `comment.Contain` before anything is written. Contain requires
every byte before and after the comment's span to be identical. The provider
must locate the rewritten file with the same comments and set-aside lines, moved
by the rewrite and otherwise equal. The rewritten comment must sit on the same
subject, with the same deprecation marker and the same placeholders: code
blocks, links, references and list items. Where the language has a formatter,
it must agree with the rewritten comment and with every comment it agreed with
before, and it must leave as they are the lines of the file it left as they were
before. The last rule holds code a formatter aligns with a comment, such as the
body of a one-line function after a `/* */` comment in its signature. A rewrite
that fails any of these is refused with a reason, and nothing is written.

The Go provider rewrites line comments and `/* */` comments. It keeps a line
comment's indentation and line ending, and writes each line with the marker
gofmt uses. It reflows a paragraph or list item only when one of its lines is
wider than the comment's widest line, or 80 columns, whichever is wider. A
comment in the position gofmt reformats as a doc comment goes through
`go/doc/comment`'s printer, as gofmt does. Directives, generated files, the cgo
preamble and example output are not addressable, and a refusal names what the
file sets aside.

A `/* */` comment is written in the layout it already has, which
`comment.ParseLayout` reads from its bytes: the delimiters as written, such as
`/**`, the space beside them, whether text starts on the opener's line or ends
on the closer's, the prefix each line below the opener's opens with, such as a
line of ` * `, how an empty line between paragraphs is written, the blank lines
above and below the text, and the line ending. The text of the comment is what
remains, and the Go provider reads the comment's prose from that same text, so
a check and a rewrite read one comment the same way. Rendering a comment's own
text in its layout reproduces its bytes. A delimited comment ends at the first
`*/`, so text holding `*/`, or text whose first character completes one with
the prefix before it, is refused as `terminator`. No escape for `*/` exists
inside a Go comment, and a rewrite that changed the text to avoid it would write
prose nobody wrote. A comment made of several comments, such as a `/* */`
comment beside line comments in one group, has no one layout and is refused as
`layout`. `TestProseP3_go` rewrites every Go comment in the repository with its
own prose, and `TestProseP4_go` does the same for every `/* */` comment in the
repository and in the Go toolchain's source tree, and requires each file to
stay byte-identical.

A comment in a language the sourcecode plugin reads is written when the plugin's
manifest declares that language writable (`rewrite` on a `capabilities.comments`
entry). The host writes it, never the plugin. It reads the comment's layout from
the file's bytes through the markers the manifest names, `comment.ParseLayout`
for a delimited comment and `comment.ParseLineLayout` for a group of line
comments, renders the text into that layout and splices it in. `Contain` then
has the plugin locate the rewritten file through `LocateComments`, so the check
a rewrite is held to runs in the plugin's grammar and shares no code with the
renderer. The text is not reflowed, and the entry's `width` does not apply.

Such a rewrite is held to the formatter the file's project uses, which runs in
the host because its configuration and ignore files belong to the project and
the plugin sees only bytes. The host looks for the marker files each formatter
the manifest lists, such as `.prettierrc` or a `vite.config.ts` that loads
vite-plus, in the file's directory and the directories above it, and runs the
nearest one's command on the file's bytes, from `node_modules/.bin` or `PATH`.
The path it gives the formatter has its symbolic links resolved, since a
formatter matches its ignore files against that path. Before its output is
trusted, the formatter must rewrite the manifest's formatter canary at the
file's own path. A formatter that is not configured, not installed, or leaves the
canary as it is did not run: `Contain` returns `comment.ErrFormatterNotRun`, and
the entry reports did-not-run with the reason `formatter` and writes nothing. A
check of a file in such a language does not run the formatter, which it reports
as unsupported.

Running that formatter runs code the project controls. The project can install
the executable in `node_modules/.bin`, and a formatter installed anywhere loads
the project's configuration: prettier imports a JavaScript configuration file and
the plugins a `.prettierrc` names, and the vite-plus `oxfmt` evaluates
`vite.config.ts`. So the formatter is an exec site under execution trust
([E-06](e-06-execution-trust.md)), decided when an edit would run it. Its record
is keyed by the configuration file that selected it, and its digest holds the
formatter's name and command, the resolved executable's bytes and that file's
bytes, so a new executable or an edited configuration asks again. Modules either
one imports are outside the digest. `kapi apply` runs the formatter under
`KAPI_TRUST_EXEC`, on a recorded allow, or after asking a person at a terminal,
whose answer it records, and a change-set read from standard input leaves no one
to ask. MCP `apply_edits` runs it only on a recorded allow, since a project's own
MCP client configuration can set the server's environment. Without trust the edit
did not run, with the reason `formatter`, and no formatter process starts. When
the host looks for the executable on `PATH`, it skips an entry that is not an
absolute path, since such an entry names a directory relative to the working
directory, and a not-installed detail lists the entries it skipped. A Go comment
is held to `go/format`, the library gofmt is built on, inside the kapi binary, so
a Go edit runs no formatter executable.

TypeScript, TSX and JavaScript are declared writable. A JSDoc block's tags,
their types and names, and its `{@link}` references are placeholders, so a
rewrite that drops or adds one is refused as `structure`. An entry naming a file
whose comments only a plugin reads, when no such plugin is installed, did not
run, with the reason `no-reader` and the command that installs the plugin.
`TestProseP3_typescript`, `TestProseP3_tsx` and `TestProseP3_javascript` rewrite
every comment of their language in the repository with its own prose, through
the plugin, and require each file to stay byte-identical.

Each comment form their code uses is rewritten in the layout it is written in:
groups of line comments and comments after code, `/* */` comments on one line
or several, JSDoc blocks including those whose text starts on the opener's line,
and comments inside JSX (`{/* */}`). Text holding `*/` is refused in each
delimited form and written in a line comment, where it closes nothing. A legacy
HTML-like comment (`<!--`) in a script opens with no delimiter the manifest
declares, so it has no layout to keep and is refused as `layout`; no code in
this repository writes one. `TestProseP4_typescript`, `TestProseP4_tsx` and
`TestProseP4_javascript` rewrite each form with exact bytes, with the plugin
reading the result and oxfmt agreeing with the file.

`kapi apply` and MCP `apply_edits` reach the rewrite through a `comment` entry,
addressed by file and the id a check reports. A check gives each finding on a
comment `location.comment_sha256`, the SHA-256 of the comment's bytes
(`comment.Fingerprint`), and the entry carries it back as the guard: a comment
whose bytes differ is refused as changed, and one that only moved to other
lines is rewritten where it now sits. An entry may carry the prose as it was
read, `current_text`, instead, and an entry with neither guard is rejected
before any entry is applied. Before the first comment of a language is written
in a run, `comment.VerifyRewriter` runs a write canary: a known comment
rewritten with its own prose must stay byte-identical, and a known-bad text, a
rewrite guarded by a fingerprint the comment does not have, and a splice one
byte before the comment must each be refused. A language with `/* */` comments
adds one to the canary, which must also rewrite to itself, and text holding its
closer must be refused as `terminator`. When the
canary fails, no comment of that language is written. A file's edits apply from
its last comment to its first, the file is read again before it is written, and
a check scoped to the written change comes back with the result.

#### Default on, via an inverted opt-out

Surfacing is the **default**, controlled per format by a single boolean,
`extractNonTranslatableContent`, exposed as a schema property in the generated
format reference and accepted in `ApplyMap` under that key. The implementation is
an **inverted private field**:

```go
// zero value false ⇒ surfacing ON (the opt-out default)
disableNonTranslatableContent bool

func (c *Config) ExtractNonTranslatableContent() bool     { return !c.disableNonTranslatableContent }
func (c *Config) SetExtractNonTranslatableContent(v bool) { c.disableNonTranslatableContent = !v }
```

The inversion means a freshly zero-valued config (a new format that has not yet
learned about the flag, a caller that constructs a config without calling
`Reset`) surfaces content automatically, because the *disable* bit must be set
explicitly to turn it off. The safe-for-ingestion behaviour needs no
configuration; opting out is explicit. The off switch exists for two callers:
the parity harness, which pins the bridge-matching configuration, and
validation-only or pure-passthrough flows that want nothing but skeleton.

A format may also **scope** what counts as meaningful context. The design-tokens
reader composes the generic JSON reader but calls
`SetExtractNonTranslatableContent(false)` on that inner config: a token's
`$value`, `$type`, and `$extensions` are structured machine data (colours,
dimensions, font names) rather than contextual prose, so design tokens surface only
`$description` as translatable prose and let everything else pass through as
non-translatable structure. The convention is uniform; each reader decides which
of its fragments are genuinely *context* rather than inert data.

Which formats expose the flag, and exactly what each surfaces, is generated into
the [Format Reference](/formats) rather than enumerated here;
[content-fidelity](/contribute/implementation/engine/content-fidelity) is the
recipe for adding surfacing to a format.

#### Round-trip, translation-skip, and parity all still hold

Surfacing is additive over the existing guarantees.

- **Byte-exact round-trip.** The verbatim source bytes never leave the skeleton.
  A surfaced renderable block stands in for the rendered body via a skeleton ref,
  or a **sub-skeleton**: verbatim segments interleaved with refs to translatable
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
  contract is "same semantic config → same results" rather than "same
  defaults": the parity runner duck-types `interface{ SetExtractNonTranslatableContent(bool) }`
  on the reader's config and forces it **false** before reading, so the native
  stream matches the bridge. The roles and properties a surfaced block carries are
  additionally **parity-safe carriers** (the canonical projection excludes
  `SemanticRole`, `Properties`, `Annotations`, and the placeholder `Equiv`/`Disp`),
  but the flag rather than the projection is what keeps the surfaced *parts
  themselves* out of the head-to-head
  ([A-02](../assurance/a-02-parity.md)).

The `SkeletonStore` also supports the **sub-skeleton** in its own right: verbatim
segments of an otherwise-opaque payload interleaved with refs to translatable
spans inside it. This is how translatable prose embedded in an opaque structure,
the natural-language text inside a Word equation, is translated while the
surrounding math is replayed byte-for-byte
([M-04](../multilingual/m-04-math-and-equations.md); see
[Skeleton Store](/contribute/implementation/engine/skeleton-store)).

### Reader-assigned identity

A reader mints each block's `ID` from what the format offers (an XLIFF unit id,
a key path, a positional name) and keeps it unique within the document through
`model.IDBuilder`: the first block to claim an id keeps it, a repeat is
separated with an ordinal (`id#2`), and the document's own spelling is recorded
under `PropDocumentID` so the writer can put it back. A container that delegates
a member to another reader (an archive entry, an embedded HTML string, an ODF
part) qualifies the member's ids with the child layer's id through
`model.QualifyMemberID`, joined by `MemberIDSeparator` (`_`), so two members'
`tu1` are two ids and the qualified id still satisfies an XLIFF `xs:NMTOKEN`.
These ids are file-local; how they become durable identity across reads is
[F-03](../foundations/f-03-identity.md).

### Deciding translatability

Which text belongs to the translator is decided once per family rather than per
reader. The markup readers (`markdown`, `mdx`) classify elements with the W3C
HTML5 translatability table in `core/translatability`, generated from the same
source the React i18n transform uses, so an element nobody has classified is a
container whose direct text is promoted rather than dropped: prose inside an
unfamiliar component is translated and reported, never lost in silence. XML
readers apply ITS 2.0 rules and local attributes through `core/its`. Text a
reader classifies as code (a code span, `<kbd>`, `<samp>`) stays a run marked
`NoTranslate` ([F-02](../foundations/f-02-content-model.md)), which every
translation path leaves untouched, and a span the model cannot take is skipped
and reported rather than costing the page its translation.

### Subfilters and nested layers

Format readers can emit child Layers when they encounter embedded content in a
different format (HTML inside JSON, Markdown inside CSV). The child reader is
resolved via a `SubfilterResolver` injected by the `FormatRegistry`. The
mechanism is defined in [F-02](../foundations/f-02-content-model.md); format
readers implement `SubfilterAware` and declare patterns in their config.

### Implementing a new format

1. Create `core/formats/<name>/` with `reader.go`, `writer.go`, and `config.go`.
2. Implement `DataFormatReader` by embedding `BaseFormatReader` and providing the
   format-specific parse logic.
3. Implement `DataFormatWriter` by embedding `BaseFormatWriter` and providing the
   format-specific serialize logic.
4. Populate every field on each inline-code run for any inline markup: `ID`,
   `Type`/`SubType`, `Data`, `Disp`, `Equiv`, `Constraints`
   ([F-02](../foundations/f-02-content-model.md)).
5. Emit and consume the skeleton store (`SkeletonStoreEmitter` /
   `SkeletonStoreConsumer`), assign ids through `model.IDBuilder`, and declare
   the streaming markers if the reader and writer can both honour them.
6. Register the reader and writer factories in `formats.RegisterAll`
   (`core/formats/register.go`).
7. If the format can host embedded content, implement `SubfilterAware` and accept
   `Subfilters []SubfilterMapping` in the config.

See [Implementing Formats](/contribute/implementation/engine/implementing-formats) for a
walkthrough, and
[Skeleton store and streaming](/contribute/implementation/engine/skeleton-store)
for the store's details.

## Consequences

- Format readers emit the same streaming Part protocol regardless of source
  format, so tools never need format-specific code.
- Format writers replay `Run.Data` verbatim
  ([F-02](../foundations/f-02-content-model.md)), so write-back fidelity is
  inherited from the content model.
- Built-in and plugin formats coexist in one registry; the pipeline treats them
  identically.
- The extension/priority/content cascade resolves most files without user
  configuration; ambiguous cases fall back to an explicit format flag.
- One skeleton store covers the full span of file formats from streaming text
  to zip-packaged markup, and the streaming markers give record-oriented formats
  bounded memory without changing anyone's output bytes.
- New formats plug in by adding a directory and registering in `RegisterAll`;
  no changes to the engine are needed.
- The three-way reader output policy serves ingestion and translation from one
  parse, and the inverted opt-out means a format that has not thought about the
  question still surfaces context rather than hiding it.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): the Parts readers produce and writers consume; the Run model that drives write-back fidelity; the `SemanticRole` taxonomy
- [E-01: The processing engine](e-01-processing-engine.md): how readers and writers plug into the pipeline
- [E-03: The tool system](e-03-tool-system.md): the tools that sit between reader and writer
- [E-04: Flows and I/O binding](e-04-flows-and-io-binding.md): readers and writers as the `file` binding; other bindings feed the same stream
- [E-05: The plugin system](e-05-plugin-system.md): how plugin and bridge formats register
- [M-04: Math and equations](../multilingual/m-04-math-and-equations.md): the sub-skeleton inside an opaque payload
- [A-02: Parity](../assurance/a-02-parity.md): the "same semantic config → same results" contract and the parity-safe carriers
- [Implementing Formats](/contribute/implementation/engine/implementing-formats): implementation walkthrough
- [Skeleton store and streaming](/contribute/implementation/engine/skeleton-store): the entry format, the wiring seam, the streaming protocol
- [content-fidelity](/contribute/implementation/engine/content-fidelity): the recipe for surfacing non-translatable context
- [F-03: Identity](../foundations/f-03-identity.md): how reader-assigned ids become durable identity
