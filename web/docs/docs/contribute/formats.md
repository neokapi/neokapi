---
sidebar_position: 3
title: Implementing a Format
description: Step-by-step guide for adding a new document format to neokapi — reader and writer Go structs, inline code handling, roundtrip fidelity tests, and registration in the format registry.
keywords: [format implementation, DataFormatReader, DataFormatWriter, neokapi, Go, format reader, roundtrip]
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
            Type: model.PartBlock,
            Resource: &model.Block{
                ID:           "b1",
                Translatable: true,
                Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Hello")}},
            },
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
                // Write translated content — see renderFragment below
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

neokapi solves this with **coded text**: inline markup is replaced by Unicode
Private Use Area (PUA) marker characters in the text string, while the original
markup is stored in a parallel `Spans` slice. This lets tools, translation
engines, and TM matching operate on plain text with positional markers, and the
writer reconstructs the original markup on output.

### The Fragment and Span Model

A `Fragment` holds text content with inline codes:

```go
type Fragment struct {
    CodedText string  // Text with PUA marker characters
    Spans     []*Span // Original markup for each marker
}
```

Three marker characters identify span types:

| Marker      | Constant                  | Unicode | Purpose                                                 |
| ----------- | ------------------------- | ------- | ------------------------------------------------------- |
| Opening     | `model.MarkerOpening`     | U+E001  | Paired opening tag (`<b>`, `**`, `<a href="...">`)      |
| Closing     | `model.MarkerClosing`     | U+E002  | Paired closing tag (`</b>`, `**`, `</a>`)               |
| Placeholder | `model.MarkerPlaceholder` | U+E003  | Self-closing element (`<br/>`, `<img>`, `\{variable\}`) |

Each `Span` stores full metadata about an inline code:

```go
type Span struct {
    SpanType    SpanType // SpanOpening, SpanClosing, or SpanPlaceholder
    Type        string   // Semantic type (e.g., "bold", "link", "image", "br")
    ID          string   // Unique identifier for matching open/close pairs
    Data        string   // Original markup verbatim (e.g., "<b>", "</b>")
    OuterData   string   // Outer context when needed
    Deletable   bool     // Can a translator remove this code?
    Cloneable   bool     // Can a translator duplicate this code?
    CanReorder  bool     // Can this code move relative to others?
    DisplayText string   // What to show in a translation editor (e.g., "[B]")
    EquivText   string   // Text equivalent (e.g., "\n" for <br>)
}
```

### How It Works

Consider this HTML paragraph:

```html
<p>Click <b>here</b> for <a href="/help">info</a></p>
```

The reader extracts the `<p>` content as a single Fragment:

```
CodedText: "Click \uE001here\uE002 for \uE001info\uE002"
Spans: [
    {SpanType: Opening,     Type: "b", ID: "b",    Data: "<b>"},
    {SpanType: Closing,     Type: "b", ID: "b",    Data: "</b>"},
    {SpanType: Opening,     Type: "a", ID: "a",    Data: "<a href=\"/help\">"},
    {SpanType: Closing,     Type: "a", ID: "a",    Data: "</a>"},
]
```

The marker characters are positional anchors — each marker in `CodedText`
corresponds to the next `Span` in the slice, in order. This means:

- `frag.Text()` returns `"Click here for info"` (markers stripped)
- `frag.HasSpans()` returns `true`
- `frag.Spans[0].Data` returns `"<b>"` (the original markup, including attributes)

Tools see the markers but skip over them. Translation engines get clean text
with positional codes. The writer replays `Span.Data` at each marker position
to reconstruct the original markup perfectly — even preserving attributes like
`class="emphasis"` or `href="/help"`.

### Building Fragments in a Reader

Use the Fragment builder API to construct coded text incrementally:

```go
frag := &model.Fragment{}

// Plain text
frag.AppendText("Click ")

// Opening tag — appends MarkerOpening + records the Span
frag.AppendSpan(&model.Span{
    SpanType: model.SpanOpening,
    Type:     "b",
    ID:       "b",
    Data:     "<b>",
})

// Text inside the tag
frag.AppendText("here")

// Closing tag — appends MarkerClosing + records the Span
frag.AppendSpan(&model.Span{
    SpanType: model.SpanClosing,
    Type:     "b",
    ID:       "b",
    Data:     "</b>",
})

frag.AppendText(" for info")
```

For self-closing elements like `<br/>` or `<img>`:

```go
frag.AppendSpan(&model.Span{
    SpanType: model.SpanPlaceholder,
    Type:     "br",
    ID:       "br",
    Data:     "<br/>",
})
```

### Three Categories of Inline Elements

When implementing a format reader, classify each inline element into one of
three categories:

| Category         | SpanType          | Examples                              | Pattern                     |
| ---------------- | ----------------- | ------------------------------------- | --------------------------- |
| **Paired tags**  | Opening + Closing | `<b>...</b>`, `**...**`, `<a>...</a>` | Wrap content with two spans |
| **Self-closing** | Placeholder       | `<br/>`, `<img>`, `<hr/>`             | Single span, no children    |
| **Block-level**  | _(not a span)_    | `<p>`, `<div>`, `<h1>`                | Boundary for a new Block    |

The reader decides what is inline vs. block-level. For HTML, this distinction
is well-defined. For other formats (Markdown, XLIFF, custom XML), you choose
the mapping based on what a translator needs to see as a contiguous unit.

### Complete Reader Example with Inline Codes

Here is how the HTML reader collects inline content from a block-level element.
This pattern applies to any format with inline markup:

```go
// collectInlineContent builds a Fragment from all text and inline
// elements inside a block-level container node.
func (r *Reader) collectInlineContent(n *html.Node) *model.Fragment {
    frag := &model.Fragment{}
    r.collectFromNode(n, frag)
    return frag
}

func (r *Reader) collectFromNode(n *html.Node, frag *model.Fragment) {
    for child := n.FirstChild; child != nil; child = child.NextSibling {
        switch child.Type {
        case html.TextNode:
            // Plain text — append directly
            frag.AppendText(child.Data)

        case html.ElementNode:
            if selfClosingElements[child.DataAtom] {
                // Self-closing: <br/>, <img>, etc. → single placeholder span
                frag.AppendSpan(&model.Span{
                    SpanType: model.SpanPlaceholder,
                    Type:     child.Data,
                    ID:       child.Data,
                    Data:     renderTag(child), // e.g., "<br/>"
                })
            } else if isInlineElement(child) {
                // Paired inline: <b>, <a>, <em>, etc.
                frag.AppendSpan(&model.Span{
                    SpanType: model.SpanOpening,
                    Type:     child.Data,
                    ID:       child.Data,
                    Data:     renderOpenTag(child), // e.g., "<a href=\"/help\">"
                })
                r.collectFromNode(child, frag) // Recurse into children
                frag.AppendSpan(&model.Span{
                    SpanType: model.SpanClosing,
                    Type:     child.Data,
                    ID:       child.Data,
                    Data:     fmt.Sprintf("</%s>", child.Data),
                })
            }
            // Block-level elements are NOT collected — they form new Blocks
        }
    }
}
```

The key insight: **recurse into inline children** to handle nested formatting
like `<b><i>bold italic</i></b>`. Each level of nesting adds its own
opening/closing span pair, and the `CodedText` naturally captures the correct
order.

### Reconstructing Markup in a Writer

The writer iterates through `CodedText` character by character. When it
encounters a marker character, it emits the corresponding `Span.Data`:

```go
func (w *Writer) renderFragment(buf *strings.Builder, frag *model.Fragment) {
    if !frag.HasSpans() {
        // No inline codes — write text directly
        buf.WriteString(frag.CodedText)
        return
    }

    spanIdx := 0
    for _, r := range frag.CodedText {
        if r == model.MarkerOpening || r == model.MarkerClosing || r == model.MarkerPlaceholder {
            // Replace marker with original markup
            if spanIdx < len(frag.Spans) {
                buf.WriteString(frag.Spans[spanIdx].Data)
                spanIdx++
            }
        } else {
            // Regular text character
            buf.WriteRune(r)
        }
    }
}
```

This approach guarantees **perfect roundtrip fidelity** — the writer doesn't
need to understand the markup format. It just replays whatever `Data` the
reader stored. An `<a href="/help" class="nav">` tag roundtrips as exactly
that string, attributes and all.

### Choosing Target vs Source Content

When writing output, the writer must choose the right content. Use target
content if a translation exists for the configured locale, otherwise fall back
to source:

```go
func (w *Writer) writeBlock(block *model.Block) {
    if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
        // Write translated content (preserving inline codes)
        for _, seg := range block.Targets[w.Locale] {
            w.renderFragment(buf, seg.Content)
        }
    } else {
        // Fall back to source
        for _, seg := range block.Source {
            w.renderFragment(buf, seg.Content)
        }
    }
}
```

### Using Skeletons for Document Structure

Block-level structure that surrounds translatable content (opening and closing
tags, whitespace, etc.) is captured in a **Skeleton**. The reader builds a
skeleton with text parts and a reference to the block content:

```go
block := &model.Block{
    ID:           "tu1",
    Translatable: true,
    Source:       []*model.Segment{{ID: "s1", Content: frag}},
    Skeleton: &model.Skeleton{
        Strategy: model.SkeletonFragmentBased,
        Parts: []model.SkeletonPart{
            &model.SkeletonText{Text: "<p>"},       // Before content
            &model.SkeletonRef{ResourceID: "tu1"},   // Content placeholder
            &model.SkeletonText{Text: "</p>\n"},     // After content
        },
    },
}
```

The writer uses the skeleton to reconstruct the document:

```go
if block.Skeleton != nil {
    for _, sp := range block.Skeleton.Parts {
        switch p := sp.(type) {
        case *model.SkeletonText:
            fmt.Fprint(w.Output, p.Text)  // Emit structure verbatim
        case *model.SkeletonRef:
            // Emit the translated/source content with inline codes
            w.renderFragment(buf, content)
        }
    }
}
```

Skeletons are critical for roundtrip fidelity of the block-level document
structure. Without them, the writer would need to re-generate all surrounding
tags, whitespace, and attributes — which risks losing information.

---

## Span Metadata Fields

The `Span` struct carries more than just the raw markup. These fields help
tools, editors, and QA checks work with inline codes intelligently:

| Field         | Purpose                                         | Example                             |
| ------------- | ----------------------------------------------- | ----------------------------------- |
| `SpanType`    | Discriminator: Opening, Closing, or Placeholder | `SpanOpening`                       |
| `Type`        | Semantic type for tool processing               | `"bold"`, `"link"`, `"image"`       |
| `ID`          | Matches opening/closing pairs                   | `"b1"` for both `<b>` and `</b>`    |
| `Data`        | Original markup for roundtrip reconstruction    | `"<a href=\"/help\">"`              |
| `OuterData`   | Outer context (e.g., CDATA wrapper)             |                                     |
| `Deletable`   | Translator can remove this code                 | `true` for optional formatting      |
| `Cloneable`   | Translator can duplicate this code              | `true` for `<b>`                    |
| `CanReorder`  | Code can move in translation                    | `true` for independent placeholders |
| `DisplayText` | UI label in translation editors                 | `"[B]"`, `"[/B]"`, `"[IMG]"`        |
| `EquivText`   | Plain text equivalent                           | `"\n"` for `<br>`                   |

Set these fields in the reader when you have the information. At minimum, set
`SpanType`, `Type`, `ID`, and `Data`. The other fields enhance the experience
for translators and tools but are optional.

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
func TestReadInlineSpans(t *testing.T) {
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

    frag := blocks[0].FirstFragment()
    require.NotNil(t, frag)

    // Plain text is correct
    assert.Equal(t, "Click here for info", frag.Text())

    // Inline codes are preserved
    assert.True(t, frag.HasSpans())
    require.Len(t, frag.Spans, 2)
    assert.Equal(t, model.SpanOpening, frag.Spans[0].SpanType)
    assert.Equal(t, "<b>", frag.Spans[0].Data)
    assert.Equal(t, model.SpanClosing, frag.Spans[1].SpanType)
    assert.Equal(t, "</b>", frag.Spans[1].Data)
}
```

Test each type of inline element your format supports:

```go
func TestReadPlaceholderSpan(t *testing.T) {
    // Self-closing elements become SpanPlaceholder
    reader := NewReader()
    reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Line one<br/>Line two</p></body></html>`,
        model.LocaleEnglish,
    ))
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    frag := blocks[0].FirstFragment()

    assert.Equal(t, "Line oneLine two", frag.Text())
    require.Len(t, frag.Spans, 1)
    assert.Equal(t, model.SpanPlaceholder, frag.Spans[0].SpanType)
    assert.Equal(t, "br", frag.Spans[0].Type)
}

func TestReadLinkSpan(t *testing.T) {
    // Links preserve href in Span.Data
    reader := NewReader()
    reader.Open(ctx, testutil.RawDocFromString(
        `<html><body><p>Visit <a href="http://example.com">our site</a></p></body></html>`,
        model.LocaleEnglish,
    ))
    defer reader.Close()

    blocks := testutil.CollectBlocks(t, reader.Read(ctx))
    frag := blocks[0].FirstFragment()

    assert.Equal(t, "Visit our site", frag.Text())
    assert.Contains(t, frag.Spans[0].Data, "href") // Attributes preserved
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

    // Build a translated Fragment with the same inline codes
    for _, p := range parts {
        if p.Type == model.PartBlock {
            block := p.Resource.(*model.Block)

            target := &model.Fragment{}
            target.AppendText("Cliquez ")
            target.AppendSpan(&model.Span{
                SpanType: model.SpanOpening,
                Type: "b", ID: "b", Data: "<b>",
            })
            target.AppendText("ici")
            target.AppendSpan(&model.Span{
                SpanType: model.SpanClosing,
                Type: "b", ID: "b", Data: "</b>",
            })
            block.SetTargetFragment(model.LocaleFrench, target)
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

Different formats map to the same Fragment/Span model in different ways:

### HTML / XML

Block-level elements (`<p>`, `<div>`, `<h1>`) are Block boundaries. Inline
elements (`<b>`, `<a>`, `<em>`, `<span>`) become Spans. Void elements
(`<br>`, `<img>`) become placeholders.

```
Input:  <p>Click <b>here</b> for <a href="/help">info</a></p>
Text:   "Click here for info"
Spans:  [<b>, </b>, <a href="/help">, </a>]
```

### Markdown

Emphasis markers (`*`, `**`, `` ` ``) become opening/closing span pairs.
Links have the URL stored in the opening span's Data field.

```
Input:  Click **here** for [info](/help)
Text:   "Click here for info"
Spans:  [**, **, [, ](/help)]
```

### XLIFF / Translation Formats

XLIFF `<bpt>`/`<ept>` (begin/end paired tag) map to Opening/Closing spans.
`<ph>` (placeholder) maps to Placeholder spans. `<it>` (isolated tag) maps
to Placeholder. The original XLIFF inline markup goes into `Span.Data`.

### Templating / Variables

Template variables like `\{name\}` or `$\{count\}` become placeholder spans.
The full variable expression goes into `Data`:

```
Input:  Hello {name}, you have {count} items
Text:   "Hello , you have  items"
Spans:  [{name} (Placeholder), {count} (Placeholder)]
```

### Formats Without Inline Codes

Formats like JSON, YAML, or .properties typically don't have inline markup.
Use `model.NewFragment(text)` to create plain text Fragments with no spans.
If these formats contain embedded HTML or Markdown, use nested Layers
(see [Architecture](/framework/architecture)) to delegate inline
handling to the appropriate sub-format reader.
