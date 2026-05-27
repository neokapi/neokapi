---
sidebar_position: 14
title: Skeleton Store and Streaming HTML
description: Implementation note for AD-005 — details of the SkeletonStore temp-file-backed binary store and the tokenizer-based HTML reader/writer that uses skeleton entries to faithfully reconstruct documents.
keywords: [SkeletonStore, streaming HTML, skeleton, tokenizer, HTML reader, implementation note, neokapi]
---

import { StreamDiagram } from "@site/src/components/diagram";

# Skeleton Store and Streaming HTML

Implementation details for the `SkeletonStore` framework type and the
tokenizer-based HTML reader/writer that uses it. Parent AD:
[AD-005](/contribute/architecture/005-format-system) (skeleton strategies).

## SkeletonStore (`core/format/skeleton.go`)

A temp-file-backed binary store for document skeleton data. The reader writes
entries during extraction; the writer reads them during reconstruction. The
pipeline (tools) never sees the skeleton — it only carries blocks.

### Binary format

Each entry is:

```
[type:1 byte] [length:4 bytes big-endian] [data:N bytes]
```

| Type byte | Meaning | Data contents              |
| --------- | ------- | -------------------------- |
| `0`       | Text    | Non-translatable raw bytes |
| `1`       | Ref     | Block ID as UTF-8 string   |
| `2`       | Lang    | Source-locale `lang`/`xml:lang` attribute value (raw bytes between the quotes), spliced for language retargeting |

The `Lang` entry lets a writer retarget the document language: when the stored
value matches the document's source locale it emits the target locale,
otherwise it emits the stored value verbatim. Writers that do not understand
the type must treat it as inert (emitting nothing would drop the attribute
value). Only the HTML reader emits `Lang` today; other formats never see it,
and because their entry-type switches have no `default` case the addition is
purely additive.

The format is append-only during writing and sequential during reading. After
`Flush()`, the file is seeked to the beginning and entries are read with
`Next()` until `io.EOF`.

### API

```go
func NewSkeletonStore() (*SkeletonStore, error)    // creates temp file in os.TempDir()
func (s *SkeletonStore) WriteText(data []byte) error // skips empty data
func (s *SkeletonStore) WriteRef(blockID string) error
func (s *SkeletonStore) WriteLang(value string) error // language-attribute value for retargeting
func (s *SkeletonStore) Flush() error                // flushes buffered writer, seeks to 0
func (s *SkeletonStore) Next() (SkeletonEntry, error) // returns io.EOF at end
func (s *SkeletonStore) Close() error                 // removes temp file
```

`WriteText` skips empty byte slices to avoid writing no-op entries.

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

### Flow executor wiring

The skeleton store must be wired **before** `reader.Read()` is called, since
the reader writes skeleton entries during reading. This requires creating the
writer early (before reading), which is a change from the original flow where
the writer was created after reading.

Three call sites wire the skeleton store:

- `cli/flow.go` — `runSingleFile()`
- `cli/toolrun.go` — `processOneFile()`
- `kapi/cmd/kapi/mcp_tools.go` — `executeFlow()`

All three follow the same pattern:

```go
// Create writer early so we can wire skeleton store before reading.
writer, err := formatReg.NewWriter(registryName)

// Wire skeleton store if both reader and writer support it.
if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
    if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
        store, err := format.NewSkeletonStore()
        if err == nil {
            defer store.Close()
            emitter.SetSkeletonStore(store)
            consumer.SetSkeletonStore(store)
        }
    }
}

// Now read — the reader writes skeleton entries during this call.
for result := range reader.Read(ctx) { ... }
```

## HTML tokenizer reader (`core/formats/html/tokenreader.go`)

Single-pass reader using Go's `html.Tokenizer` (from `golang.org/x/net/html`).
No `html.Parse()`, no DOM tree, no pre-scan pass. Writes skeleton entries as
it processes tokens.

### Element classification

When the tokenizer enters a block-level element, it needs to know whether the
element is a **container** (has block-level children) or a **leaf block**
(contains only inline content). Instead of a pre-scan pass over the entire
document, the reader **forward-scans** from the current position through the
element's buffered content:

- If any direct child is a block-level start tag → **container** (mixed
  content mode — the element's start/end tags go to skeleton, children are
  processed recursively)
- If no block children found by the end tag → **leaf block** (content is
  extracted as a translatable block with inline spans)

The forward scan skips inline element subtrees (tracking depth) and only
checks direct children. After classification, the scanner's buffered tokens
are replayed for processing.

### Token processing

| Token type                                                   | Action                                                                                             |
| ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------- |
| Non-translatable element start (e.g., `<script>`, `<style>`) | Write raw bytes to skeleton, consume until close tag                                               |
| Block-level element start (container)                        | Write start tag to skeleton, process children recursively                                          |
| Block-level element start (leaf)                             | Extract translatable attributes as skeleton refs, buffer content, build a `[]Run` for the block    |
| Inline element start/end                                     | Part of leaf block content → becomes a paired-code run (`PcOpen`/`PcClose`)                        |
| Text token                                                   | Part of leaf block content → appended as a `TextRun`                                               |
| Comment                                                      | Written to skeleton (non-translatable)                                                             |
| Doctype                                                      | Written to skeleton                                                                                |

### Translatable attributes

For elements with translatable attributes (e.g., `title`, `alt`, `content`
on meta tags), the reader splits the raw tag bytes at attribute value
boundaries to create interleaved skeleton text and ref entries:

<StreamDiagram
  title={'<p title="Tooltip">'}
  items={[
    { kind: "skeleton.WriteText", detail: `'<p title="'`, role: "meta" },
    { kind: "skeleton.WriteRef", detail: '"tu1"', role: "block", note: 'block for "Tooltip"' },
    { kind: "skeleton.WriteText", detail: `'">'`, role: "meta" },
  ]}
/>

The `findAttrValueRange` function locates the byte range of an attribute
value within the raw tag bytes by scanning for `attrKey=` followed by a
quote character.

`lang` / `xml:lang` attribute values are handled the same way, but spliced as
a typed `SkeletonLang` (byte `2`) entry rather than verbatim text
(`extractLangFromToken`), so the writer can retarget the document language on
output instead of emitting the source-locale value (mirrors Okapi's HTML
filter).

### Run sequence building

For leaf block elements, tokens between start and end tag are collected and
built into a `[]model.Run` (via the HTML `runBuilder` —
`core/formats/html/run_builder.go`):

- Text tokens → append a `TextRun` (`AddText`, which coalesces adjacent text)
- Inline element open/close → a paired `PcOpenRun` / `PcCloseRun` (sharing an
  `ID`) with `Data = string(raw)` (preserves original quote style, attribute
  order, whitespace)
- Self-closing inline → a `PlaceholderRun`
- Comments within inline content → a `PlaceholderRun`

### Memory profile

| Component             | Memory                           |
| --------------------- | -------------------------------- |
| Tokenizer             | ~4KB internal buffer (streaming) |
| Forward scan          | ~1–10 tokens replay buffer       |
| Run sequence building | ~1–10KB (one leaf block)         |
| Skeleton store        | Temp file on disk                |
| Pipeline              | Blocks only (~5% of document)    |
| **Peak per document** | **~100KB**                       |

Compared to the DOM-based approach: ~4–20MB per document (two full DOM trees
for reader + writer).

## HTML writer skeleton mode (`core/formats/html/writer.go`)

When a skeleton store is available, the writer reads entries sequentially and
fills in block content. No tokenizer, no DOM, no state machine:

```go
func (w *Writer) writeFromSkeleton(
    store *format.SkeletonStore,
    blocks map[string]*model.Block,
    sourceLocale model.LocaleID,
    needsLangRewrite bool,
) error {
    for {
        entry, err := store.Next()
        if errors.Is(err, io.EOF) { break }
        if err != nil { return err }
        switch entry.Type {
        case format.SkeletonText:
            if _, err := w.Output.Write(entry.Data); err != nil {
                return err
            }
        case format.SkeletonRef:
            if block, ok := blocks[string(entry.Data)]; ok {
                text := w.getBlockText(block)
                // (block-ref substitution + HTML encoding elided)
                if _, err := io.WriteString(w.Output, text); err != nil {
                    return err
                }
            }
        case format.SkeletonLang:
            // Retarget the document language: when the stored source-locale
            // lang matches, emit the writer's target locale; else verbatim.
            lang := string(entry.Data)
            if needsLangRewrite && sameLanguage(lang, sourceLocale.String()) {
                lang = w.Locale.String()
            }
            if _, err := io.WriteString(w.Output, lang); err != nil {
                return err
            }
        }
    }
    return nil
}
```

### Writer fallback chain

The writer tries three modes in order:

1. **Skeleton store** (byte-exact, ~4KB memory) — available when
   `SkeletonStoreConsumer.SetSkeletonStore()` was called
2. **Re-parse original content** — re-parses the original HTML with a DOM
   walker, patches translations into the tree, renders back. Requires
   `OriginalContentSetter.SetOriginalContent()` or
   `SourcePathSetter.SetSourcePath()`
3. **Block-only fallback** — outputs only block text content, no HTML
   structure. Last resort when no original content is available.

## Files

| File                                  | Role                                                    |
| ------------------------------------- | ------------------------------------------------------- |
| `core/format/skeleton.go`             | SkeletonStore type, binary format, interfaces           |
| `core/format/skeleton_test.go`        | Unit tests (roundtrip, empty skip, large data)          |
| `core/formats/html/tokenreader.go`    | Single-pass tokenizer reader (~1100 lines)              |
| `core/formats/html/reader.go`         | Dispatch: skeleton store → tokenizer, else → DOM        |
| `core/formats/html/writer.go`         | Skeleton mode + re-parse fallback + block-only fallback |
| `core/formats/html/roundtrip_test.go` | Byte-exact, translation, and attribute roundtrip tests  |
| `cli/flow.go`                         | Skeleton store wiring in `runSingleFile()`              |
| `cli/toolrun.go`                      | Skeleton store wiring in `processOneFile()`             |
| `kapi/cmd/kapi/mcp_tools.go`          | Skeleton store wiring in `executeFlow()`                |
