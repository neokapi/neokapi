---
sidebar_position: 3
title: Implementing a Format
description: Step-by-step guide for adding a new document format to neokapi — reader and writer Go structs, inline code (Run) handling, roundtrip fidelity tests, and registration in the format registry.
keywords: [format implementation, DataFormatReader, DataFormatWriter, neokapi, Go, format reader, runs, roundtrip]
---

# Implementing a New Format

This guide explains how to add a new document format to neokapi, from basic
readers and writers to full inline code support with roundtrip fidelity.

## Structure

Create a package under `core/formats/` with these files:

```
core/formats/myformat/
├── reader.go          # DataFormatReader implementation
├── writer.go          # DataFormatWriter implementation
├── config.go          # Format-specific configuration
├── reader_test.go     # Extraction and roundtrip tests
└── testdata/          # Sample files for testing
```

## Reader

The reader must implement `format.DataFormatReader`. Embed `format.BaseFormatReader`
for shared behavior:

```go
package myformat

import (
    "context"
    "github.com/neokapi/neokapi/core/format"
    "github.com/neokapi/neokapi/core/model"
)

type Reader struct {
    format.BaseFormatReader
}

func NewReader() *Reader {
    return &Reader{
        BaseFormatReader: format.BaseFormatReader{
            FormatName:        "myformat",
            FormatDisplayName: "My Format",
            FormatMimeType:    "application/x-myformat",
            FormatExtensions:  []string{".myf"},
        },
    }
}

func (r *Reader) Signature() format.FormatSignature {
    return format.FormatSignature{
        MIMETypes:  []string{"application/x-myformat"},
        Extensions: []string{".myf"},
    }
}

func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
    if doc == nil || doc.Reader == nil {
        return fmt.Errorf("myformat: nil document or reader")
    }
    r.Doc = doc
    return nil
}

func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
    ch := make(chan model.PartResult, 64)
    go func() {
        defer close(ch)

        // 1. Emit PartLayerStart
        ch <- model.PartResult{Part: &model.Part{
            Type:     model.PartLayerStart,
            Resource: &model.Layer{ID: "doc1", Format: "myformat"},
        }}

        // 2. Emit Blocks for translatable content
        ch <- model.PartResult{Part: &model.Part{
            Type:     model.PartBlock,
            Resource: model.NewBlock("b1", "Hello"),
        }}

        // 3. Emit PartLayerEnd
        ch <- model.PartResult{Part: &model.Part{
            Type:     model.PartLayerEnd,
            Resource: &model.Layer{ID: "doc1", Format: "myformat"},
        }}
    }()
    return ch
}

func (r *Reader) Close() error {
    if r.Doc != nil && r.Doc.Reader != nil {
        return r.Doc.Reader.Close()
    }
    return nil
}
```

The example above emits plain text. Most real-world formats contain inline
markup (bold, links, images) that must be preserved through the pipeline —
see [Inline Code Handling](#inline-code-handling) below.

## Writer

The writer must implement `format.DataFormatWriter`. Embed `format.BaseFormatWriter`:

```go
type Writer struct {
    format.BaseFormatWriter
}

func NewWriter() *Writer {
    return &Writer{
        BaseFormatWriter: format.BaseFormatWriter{FormatName: "myformat"},
    }
}

func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-parts:
            if !ok {
                return nil
            }
            switch part.Type {
            case model.PartBlock:
                block := part.Resource.(*model.Block)
                // Write translated content — see renderRuns below
            case model.PartData:
                // Write structural content verbatim
            }
        }
    }
}
```

---

## Inline Code Handling

Most document formats contain inline markup — bold, italic, links, images,
line breaks, variables, placeholders. A localization framework must preserve
this markup through the entire pipeline (extraction, TM lookup, MT, AI
translation, QA, reconstruction) without corruption.

neokapi solves this with the **Run** model: a block's content is a flat
`[]model.Run` sequence. Text travels as `TextRun`s; inline markup becomes
inline-code runs (`PcOpen`/`PcClose` for paired tags, `Ph` for self-closing
tokens) that carry the original markup in a `Data` field. This lets tools,
translation engines, and TM matching project the runs to plain text, and the
writer reconstructs the original markup by re-emitting each run's `Data`.

### The Run Model

A `Run` is a discriminated union — exactly one of its pointer fields is set:

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

The three inline-code runs you reach for most are:

```go
// PlaceholderRun — a self-closing token (<br/>, {count}, an icon).
type PlaceholderRun struct {
    ID          string          // unique within the run sequence
    Type        string          // semantic type (e.g., "fmt:linebreak", "var")
    SubType     string          // optional refinement
    Data        string          // original markup verbatim (e.g., "<br/>")
    Equiv       string          // plain-text equivalent (e.g., "\n")
    Disp        string          // editor display label (e.g., "[BR]")
    Constraints *RunConstraints // deletable / cloneable / reorderable
}

// PcOpenRun — the opening half of a paired code. PcCloseRun mirrors it
// (sharing ID) but omits Disp and Constraints — the close inherits the
// opener's behavior.
type PcOpenRun struct {
    ID          string
    Type        string          // e.g., "fmt:bold", "fmt:link"
    SubType     string
    Data        string          // e.g., "<b>", "<a href=\"/help\">"
    Equiv       string
    Disp        string
    Constraints *RunConstraints
}
```

`RunConstraints` is the editing-policy triple:

```go
type RunConstraints struct {
    Deletable   bool // translator may remove this code
    Cloneable   bool // translator may duplicate this code
    Reorderable bool // this code may move relative to others
}
```

### How It Works

Consider this HTML paragraph:

```html
<p>Click <b>here</b> for <a href="/help">info</a></p>
```

The reader extracts the `<p>` content as a single segment whose `Runs` are:

```
[
    {Text: "Click "},
    {PcOpen:  {ID: "1", Type: "fmt:bold", Data: "<b>"}},
    {Text: "here"},
    {PcClose: {ID: "1", Type: "fmt:bold", Data: "</b>"}},
    {Text: " for "},
    {PcOpen:  {ID: "2", Type: "fmt:link", Data: "<a href=\"/help\">"}},
    {Text: "info"},
    {PcClose: {ID: "2", Type: "fmt:link", Data: "</a>"}},
]
```

The runs are ordered, and a `PcClose` shares its `ID` with the matching
`PcOpen`. This means:

- `block.SourceText()` returns `"Click here for info"` (inline-code runs contribute nothing)
- `block.SourceRuns()` contains the `PcOpen`/`PcClose` pairs above
- the second run's `PcOpen.Data` is `"<b>"` (the original markup, including attributes)

Tools project the runs to plain text and skip the inline codes. Translation
engines get clean text with opaque tokens. The writer re-emits each run's
`Data` to reconstruct the original markup perfectly — even preserving
attributes like `class="emphasis"` or `href="/help"`.

### Three Categories of Inline Elements

When implementing a format reader, classify each inline element into one of
three categories:

| Category         | Run kind             | Examples                              | Pattern                                |
| ---------------- | -------------------- | ------------------------------------- | -------------------------------------- |
| **Paired tags**  | `PcOpen` + `PcClose` | `<b>...</b>`, `**...**`, `<a>...</a>` | Wrap content with two runs (shared ID) |
| **Self-closing** | `Ph`                 | `<br/>`, `<img>`, `<hr/>`             | Single run, no children                |
| **Block-level**  | _(not a run)_        | `<p>`, `<div>`, `<h1>`                | Boundary for a new Block               |

The reader decides what is inline vs. block-level. For HTML, this distinction
is well-defined. For other formats (Markdown, XLIFF, custom XML), you choose
the mapping based on what a translator needs to see as a contiguous unit.

### Complete Reader Example with Inline Codes

Here is how a reader collects inline content from a block-level element into a
`[]model.Run` slice. This pattern applies to any format with inline markup:

```go
// collectInlineContent builds a run sequence from all text and inline
// elements inside a block-level container node.
func (r *Reader) collectInlineContent(n *html.Node) []model.Run {
    var runs []model.Run
    r.collectFromNode(n, &runs)
    return runs
}

// appendText coalesces adjacent text so consecutive chunks stay one TextRun.
func appendText(runs *[]model.Run, text string) {
    if text == "" {
        return
    }
    if n := len(*runs); n > 0 && (*runs)[n-1].Text != nil {
        (*runs)[n-1].Text.Text += text
        return
    }
    *runs = append(*runs, model.Run{Text: &model.TextRun{Text: text}})
}

func (r *Reader) collectFromNode(n *html.Node, runs *[]model.Run) {
    for child := n.FirstChild; child != nil; child = child.NextSibling {
        switch child.Type {
        case html.TextNode:
            // Plain text — coalesce into the run sequence
            appendText(runs, child.Data)

        case html.ElementNode:
            if selfClosingElements[child.DataAtom] {
                // Self-closing: <br/>, <img>, etc. → a Ph run
                *runs = append(*runs, model.Run{Ph: &model.PlaceholderRun{
                    ID:   r.nextID(),
                    Type: child.Data,
                    Data: renderTag(child), // e.g., "<br/>"
                }})
            } else if isInlineElement(child) {
                // Paired inline: <b>, <a>, <em>, etc.
                id := r.nextID()
                *runs = append(*runs, model.Run{PcOpen: &model.PcOpenRun{
                    ID:   id,
                    Type: child.Data,
                    Data: renderOpenTag(child), // e.g., "<a href=\"/help\">"
                }})
                r.collectFromNode(child, runs) // Recurse into children
                *runs = append(*runs, model.Run{PcClose: &model.PcCloseRun{
                    ID:   id, // shares ID with its PcOpen
                    Type: child.Data,
                    Data: fmt.Sprintf("</%s>", child.Data),
                }})
            }
            // Block-level elements are NOT collected — they form new Blocks
        }
    }
}
```

The key insight: **recurse into inline children** to handle nested formatting
like `<b><i>bold italic</i></b>`. Each level of nesting appends its own
`PcOpen`/`PcClose` pair, and the flat run sequence naturally captures the
correct order. Attach the collected runs to a block with
`block.SetSourceRuns(runs)`, or build the block directly with
`model.NewRunsBlock(id, runs)`.

### Reconstructing Markup in a Writer

The writer walks the run sequence and emits each run's content: literal text for
`TextRun`s, the captured `Data` for inline-code runs. The framework provides
`model.RenderRunsWithData` for exactly this — the canonical rendering path the
HTML, XML, and Markdown writers all use:

```go
func (w *Writer) renderRuns(buf *strings.Builder, runs []model.Run) {
    // RenderRunsWithData emits TextRun content verbatim and re-emits the
    // captured Data for every inline-code run (Ph, PcOpen, PcClose, Sub).
    buf.WriteString(model.RenderRunsWithData(runs))
}
```

This approach guarantees **perfect roundtrip fidelity** — the writer doesn't
need to understand the markup format. It just replays whatever `Data` the
reader stored. An `<a href="/help" class="nav">` tag roundtrips as exactly
that string, attributes and all.

### Choosing Target vs Source Content

When writing output, the writer must choose the right content. Use the target
runs if a translation exists for the configured locale, otherwise fall back to
the source runs:

```go
func (w *Writer) writeBlock(block *model.Block) {
    if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
        // Write translated content (preserving inline codes)
        w.renderRuns(buf, block.TargetRuns(w.Locale))
    } else {
        // Fall back to source
        w.renderRuns(buf, block.SourceRuns())
    }
}
```

### Using Skeletons for Document Structure

Block-level structure that surrounds translatable content (opening and closing
tags, whitespace, etc.) is captured in a **Skeleton**. The reader builds a
skeleton with text parts and a reference to the block content:

```go
block := model.NewRunsBlock("tu1", runs)
block.Skeleton = &model.Skeleton{
    Strategy: model.SkeletonFragmentBased,
    Parts: []model.SkeletonPart{
        &model.SkeletonText{Text: "<p>"},      // Before content
        &model.SkeletonRef{ResourceID: "tu1"}, // Content placeholder
        &model.SkeletonText{Text: "</p>\n"},   // After content
    },
}
```

The writer uses the skeleton to reconstruct the document:

```go
if block.Skeleton != nil {
    for _, sp := range block.Skeleton.Parts {
        switch p := sp.(type) {
        case *model.SkeletonText:
            fmt.Fprint(w.Output, p.Text) // Emit structure verbatim
        case *model.SkeletonRef:
            // Emit the translated/source runs with inline codes
            w.renderRuns(buf, runs)
        }
    }
}
```

Skeletons are critical for roundtrip fidelity of the block-level document
structure. Without them, the writer would need to re-generate all surrounding
tags, whitespace, and attributes — which risks losing information.

---

## Run Metadata Fields

An inline-code run carries more than just the raw markup. These fields help
tools, editors, and QA checks work with inline codes intelligently:

| Field         | Purpose                                          | Example                                  |
| ------------- | ------------------------------------------------ | ---------------------------------------- |
| _discriminator_ | Which field is set: `PcOpen`, `PcClose`, or `Ph` | a `PcOpen`                            |
| `Type`        | Semantic type for tool processing                | `"fmt:bold"`, `"fmt:link"`, `"var"`      |
| `ID`          | Matches an opening run to its closing run        | `"1"` shared by the `<b>`/`</b>` pair    |
| `Data`        | Original markup for roundtrip reconstruction     | `"<a href=\"/help\">"`                   |
| `Disp`        | UI label in translation editors                  | `"[B]"`, `"[/B]"`, `"[IMG]"`             |
| `Equiv`       | Plain text equivalent                            | `"\n"` for `<br>`                        |
| `Constraints` | Editing policy (`Deletable`/`Cloneable`/`Reorderable`) | non-deletable for a `{count}` variable |

Set these fields in the reader when you have the information. At minimum, set
the discriminator, `Type`, `ID`, and `Data`. The other fields enhance the
experience for translators and tools but are optional.

---

## Configuration

```go
type Config struct {
    Encoding string `yaml:"encoding"`
}

func (c *Config) FormatName() string { return "myformat" }
func (c *Config) Reset()             { c.Encoding = "UTF-8" }
func (c *Config) Validate() error    { return nil }
```

## Registration

Add your format inside `formats.RegisterAll()` in `core/formats/register.go`.
`RegisterReader` takes the format name, a reader factory, a `FormatSignature`
for detection, and a display name; `RegisterWriter` takes the name and a writer
factory:

```go
// In RegisterAll(reg *registry.FormatRegistry, opts ...RegisterOptions):
reg.RegisterReader("myformat",
    func() format.DataFormatReader { return myformat.NewReader() },
    format.FormatSignature{
        MIMETypes:  []string{"application/x-myformat"},
        Extensions: []string{".myf"},
    }, "My Format")
reg.RegisterWriter("myformat", func() format.DataFormatWriter {
    return myformat.NewWriter()
})
```

---

## Testing

### Extraction Tests

Verify that the reader correctly identifies translatable content and inline
codes:

```go
func TestReadInlineRuns(t *testing.T) {
    ctx := context.Background()
    reader := NewReader()
    err := reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Click <b>here</b> for info</p></body></html>`,
        model.LocaleEnglish,
    ))
    require.NoError(t, err)
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    require.GreaterOrEqual(t, len(blocks), 1)

    // Plain text is the source runs with inline markup stripped.
    assert.Equal(t, "Click here for info", blocks[0].SourceText())

    // Inline codes are preserved as a PcOpen/PcClose pair on the source runs.
    // (There is no Segment type — segmentation is an opt-in overlay, AD-002.)
    runs := blocks[0].SourceRuns()
    require.Len(t, runs, 4) // "Click ", <b>, "here", </b> + trailing text coalesces
    require.NotNil(t, runs[1].PcOpen)
    assert.Equal(t, "<b>", runs[1].PcOpen.Data)
    require.NotNil(t, runs[3].PcClose)
    assert.Equal(t, "</b>", runs[3].PcClose.Data)
    assert.Equal(t, runs[1].PcOpen.ID, runs[3].PcClose.ID) // shared ID
}
```

Test each type of inline element your format supports:

```go
func TestReadPlaceholderRun(t *testing.T) {
    // Self-closing elements become Ph runs
    reader := NewReader()
    reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Line one<br/>Line two</p></body></html>`,
        model.LocaleEnglish,
    ))
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    runs := blocks[0].SourceRuns()

    assert.Equal(t, "Line oneLine two", blocks[0].SourceText())
    // The <br/> is a single Ph run between the two text runs.
    require.NotNil(t, runs[1].Ph)
    assert.Equal(t, "br", runs[1].Ph.Type)
}

func TestReadLinkRun(t *testing.T) {
    // Links preserve href in PcOpen.Data
    reader := NewReader()
    reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Visit <a href="http://example.com">our site</a></p></body></html>`,
        model.LocaleEnglish,
    ))
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    runs := blocks[0].SourceRuns()

    assert.Equal(t, "Visit our site", blocks[0].SourceText())
    require.NotNil(t, runs[1].PcOpen)
    assert.Contains(t, runs[1].PcOpen.Data, "href") // Attributes preserved
}
```

### Roundtrip Tests

The gold standard: read a file, write it back, compare with the original.
This proves that inline codes survive the full read-write cycle:

```go
func TestRoundTrip(t *testing.T) {
    original, err := os.ReadFile("testdata/sample.myf")
    require.NoError(t, err)

    ctx := context.Background()
    reader := NewReader()
    err = reader.Open(ctx, testutil.RawDocFromReader(
        bytes.NewReader(original), "testdata/sample.myf", model.LocaleEnglish))
    require.NoError(t, err)
    parts := testutil.CollectParts(t, reader.Read(ctx))
    reader.Close()

    var buf bytes.Buffer
    writer := NewWriter()
    writer.SetOutputWriter(&buf)
    writer.Write(ctx, testutil.PartsToChannel(parts))
    writer.Close()

    assert.Equal(t, string(original), buf.String())
}
```

### Translation Roundtrip Tests

Verify that translated content with inline codes writes correctly:

```go
func TestTranslationRoundTrip(t *testing.T) {
    ctx := context.Background()
    reader := NewReader()
    reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Click <b>here</b></p></body></html>`,
        model.LocaleEnglish,
    ))
    parts := testutil.CollectParts(t, reader.Read(ctx))
    reader.Close()

    // Build a translated run sequence with the same inline codes.
    for _, p := range parts {
        if p.Type == model.PartBlock {
            block := p.Resource.(*model.Block)

            block.SetTargetRuns(model.LocaleFrench, []model.Run{
                {Text: &model.TextRun{Text: "Cliquez "}},
                {PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
                {Text: &model.TextRun{Text: "ici"}},
                {PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
            })
        }
    }

    var buf bytes.Buffer
    writer := NewWriter()
    writer.SetOutputWriter(&buf)
    writer.SetLocale(model.LocaleFrench)
    writer.Write(ctx, testutil.PartsToChannel(parts))

    assert.Contains(t, buf.String(), "Cliquez <b>ici</b>")
}
```

See [Testing](/contribute/testing) for more patterns.

---

## Inline Code Patterns by Format

Different formats map to the same Run model in different ways:

### HTML / XML

Block-level elements (`<p>`, `<div>`, `<h1>`) are Block boundaries. Inline
elements (`<b>`, `<a>`, `<em>`, `<span>`) become `PcOpen`/`PcClose` pairs. Void
elements (`<br>`, `<img>`) become `Ph` runs.

```
Input:  <p>Click <b>here</b> for <a href="/help">info</a></p>
Text:   "Click here for info"
Runs:   [text, PcOpen <b>, text, PcClose </b>, text, PcOpen <a href="/help">, text, PcClose </a>]
```

### Markdown

Emphasis markers (`*`, `**`, `` ` ``) become `PcOpen`/`PcClose` pairs. Links
have the URL stored in the opening run's `Data` field.

```
Input:  Click **here** for [info](/help)
Text:   "Click here for info"
Runs:   [text, PcOpen **, text, PcClose **, text, PcOpen [, text, PcClose ](/help)]
```

### XLIFF / Translation Formats

XLIFF `<pc>` maps to a `PcOpen`/`PcClose` pair, `<bpt>`/`<ept>` (begin/end
paired tag) likewise. `<ph>` (placeholder) and `<it>` (isolated tag) map to a
`Ph` run. The original XLIFF inline markup goes into the run's `Data`.

### Templating / Variables

Template variables like `\{name\}` or `$\{count\}` become `Ph` runs. The full
variable expression goes into `Data`:

```
Input:  Hello {name}, you have {count} items
Text:   "Hello , you have  items"
Runs:   [text, Ph {name}, text, Ph {count}, text]
```

### Formats Without Inline Codes

Formats like JSON, YAML, or .properties typically don't have inline markup.
Use `model.NewBlock(id, text)` to create a block with a single plain `TextRun`.
If these formats contain embedded HTML or Markdown, use nested Layers
(see [Architecture](/framework/architecture)) to delegate inline
handling to the appropriate sub-format reader.
