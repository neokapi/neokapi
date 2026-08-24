---
sidebar_position: 3
title: Implementing a Format
description: Step-by-step guide for adding a new document format to neokapi — reader and writer Go structs, inline code (Run) handling, roundtrip fidelity tests, and registration in the format registry.
keywords: [format implementation, DataFormatReader, DataFormatWriter, neokapi, Go, format reader, runs, roundtrip]
---

# ▒ Îḿþļéḿéñţîñĝ à Ñéŵ Ƒöŕḿàţ ▒

▒ Ţĥîš ĝüîđé éẋþļàîñš ĥöŵ ţö àđđ à ñéŵ đöçüḿéñţ ƒöŕḿàţ ţö ñéöķàþî, ƒŕöḿ ƃàšîç
ŕéàđéŕš àñđ ŵŕîţéŕš ţö ƒüļļ îñļîñé çöđé šüþþöŕţ ŵîţĥ ŕöüñđţŕîþ ƒîđéļîţý. ▒

## ▒ Šţŕüçţüŕé ▒

▒ Çŕéàţé à þàçķàĝé üñđéŕ `çöŕé/ƒöŕḿàţš/` ŵîţĥ ţĥéšé ƒîļéš: ▒

```
core/formats/myformat/
├── reader.go          # DataFormatReader implementation
├── writer.go          # DataFormatWriter implementation
├── config.go          # Format-specific configuration
├── reader_test.go     # Extraction and roundtrip tests
└── testdata/          # Sample files for testing
```

## ▒ Ŕéàđéŕ ▒

▒ Ţĥé ŕéàđéŕ ḿüšţ îḿþļéḿéñţ `ƒöŕḿàţ.ĐàţàƑöŕḿàţŔéàđéŕ`. Éḿƃéđ `ƒöŕḿàţ.ƂàšéƑöŕḿàţŔéàđéŕ`
ƒöŕ šĥàŕéđ ƃéĥàṽîöŕ: ▒

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

▒ Ţĥé éẋàḿþļé àƃöṽé éḿîţš þļàîñ ţéẋţ. Ḿöšţ ŕéàļ-ŵöŕļđ ƒöŕḿàţš çöñţàîñ îñļîñé
ḿàŕķüþ (ƃöļđ, ļîñķš, îḿàĝéš) ţĥàţ ḿüšţ ƃé þŕéšéŕṽéđ ţĥŕöüĝĥ ţĥé þîþéļîñé —
šéé [Îñļîñé Çöđé Ĥàñđļîñĝ](#inline-code-handling) ƃéļöŵ. ▒

## ▒ Ŵŕîţéŕ ▒

▒ Ţĥé ŵŕîţéŕ ḿüšţ îḿþļéḿéñţ `ƒöŕḿàţ.ĐàţàƑöŕḿàţŴŕîţéŕ`. Éḿƃéđ `ƒöŕḿàţ.ƂàšéƑöŕḿàţŴŕîţéŕ`: ▒

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

## ▒ Îñļîñé Çöđé Ĥàñđļîñĝ ▒

▒ Ḿöšţ đöçüḿéñţ ƒöŕḿàţš çöñţàîñ îñļîñé ḿàŕķüþ — ƃöļđ, îţàļîç, ļîñķš, îḿàĝéš,
ļîñé ƃŕéàķš, ṽàŕîàƃļéš, þļàçéĥöļđéŕš. Ţĥé ƒŕàḿéŵöŕķ ḿüšţ þŕéšéŕṽé ţĥîš ḿàŕķüþ
ţĥŕöüĝĥ ţĥé éñţîŕé þîþéļîñé (éẋţŕàçţîöñ, çöñţéñţ-ḿéḿöŕý ļööķüþ, ḾŢ, ÀÎ
ţŕàñšļàţîöñ, ǪÀ, ŕéçöñšţŕüçţîöñ) ŵîţĥöüţ çöŕŕüþţîöñ. ▒

▒ ñéöķàþî šöļṽéš ţĥîš ŵîţĥ ţĥé **Ŕüñ** ḿöđéļ: à ƃļöçķ'š çöñţéñţ îš à ƒļàţ
`[]ḿöđéļ.Ŕüñ` šéǫüéñçé. Ţéẋţ ţŕàṽéļš àš `ŢéẋţŔüñ`š; îñļîñé ḿàŕķüþ ƃéçöḿéš
îñļîñé-çöđé ŕüñš (`ÞçÖþéñ`/`ÞçÇļöšé` ƒöŕ þàîŕéđ ţàĝš, `Þĥ` ƒöŕ šéļƒ-çļöšîñĝ
ţöķéñš) ţĥàţ çàŕŕý ţĥé öŕîĝîñàļ ḿàŕķüþ îñ à `Đàţà` ƒîéļđ. Ţĥîš ļéţš ţööļš,
ţŕàñšļàţîöñ éñĝîñéš, àñđ çöñţéñţ-ḿéḿöŕý ḿàţçĥîñĝ þŕöĵéçţ ţĥé ŕüñš ţö þļàîñ ţéẋţ, àñđ ţĥé
ŵŕîţéŕ ŕéçöñšţŕüçţš ţĥé öŕîĝîñàļ ḿàŕķüþ ƃý ŕé-éḿîţţîñĝ éàçĥ ŕüñ'š `Đàţà`. ▒

### ▒ Ţĥé Ŕüñ Ḿöđéļ ▒

▒ À `Ŕüñ` îš à đîšçŕîḿîñàţéđ üñîöñ — éẋàçţļý öñé öƒ îţš þöîñţéŕ ƒîéļđš îš šéţ: ▒

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

▒ Ţĥé ţĥŕéé îñļîñé-çöđé ŕüñš ýöü ŕéàçĥ ƒöŕ ḿöšţ àŕé: ▒

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

▒ `ŔüñÇöñšţŕàîñţš` îš ţĥé éđîţîñĝ-þöļîçý ţŕîþļé: ▒

```go
type RunConstraints struct {
    Deletable   bool // translator may remove this code
    Cloneable   bool // translator may duplicate this code
    Reorderable bool // this code may move relative to others
}
```

### ▒ Ĥöŵ Îţ Ŵöŕķš ▒

▒ Çöñšîđéŕ ţĥîš ĤŢḾĻ þàŕàĝŕàþĥ: ▒

```html
<p>Click <b>here</b> for <a href="/help">info</a></p>
```

▒ Ţĥé ŕéàđéŕ éẋţŕàçţš ţĥé `<þ>` çöñţéñţ àš à šîñĝļé šéĝḿéñţ ŵĥöšé `Ŕüñš` àŕé: ▒

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

▒ Ţĥé ŕüñš àŕé öŕđéŕéđ, àñđ à `ÞçÇļöšé` šĥàŕéš îţš `ÎĐ` ŵîţĥ ţĥé ḿàţçĥîñĝ
`ÞçÖþéñ`. Ţĥîš ḿéàñš: ▒

- ▒ `ƃļöçķ.ŠöüŕçéŢéẋţ()` ŕéţüŕñš `"Çļîçķ ĥéŕé ƒöŕ îñƒö"` (îñļîñé-çöđé ŕüñš çöñţŕîƃüţé ñöţĥîñĝ) ▒
- ▒ `ƃļöçķ.ŠöüŕçéŔüñš()` çöñţàîñš ţĥé `ÞçÖþéñ`/`ÞçÇļöšé` þàîŕš àƃöṽé ▒
- ▒ ţĥé šéçöñđ ŕüñ'š `ÞçÖþéñ.Đàţà` îš `"<ƃ>"` (ţĥé öŕîĝîñàļ ḿàŕķüþ, îñçļüđîñĝ àţţŕîƃüţéš) ▒

▒ Ţööļš þŕöĵéçţ ţĥé ŕüñš ţö þļàîñ ţéẋţ àñđ šķîþ ţĥé îñļîñé çöđéš. Ţŕàñšļàţîöñ
éñĝîñéš ĝéţ çļéàñ ţéẋţ ŵîţĥ öþàǫüé ţöķéñš. Ţĥé ŵŕîţéŕ ŕé-éḿîţš éàçĥ ŕüñ'š
`Đàţà` ţö ŕéçöñšţŕüçţ ţĥé öŕîĝîñàļ ḿàŕķüþ þéŕƒéçţļý — éṽéñ þŕéšéŕṽîñĝ
àţţŕîƃüţéš ļîķé `çļàšš="éḿþĥàšîš"` öŕ `ĥŕéƒ="/ĥéļþ"`. ▒

### ▒ Ţĥŕéé Çàţéĝöŕîéš öƒ Îñļîñé Éļéḿéñţš ▒

▒ Ŵĥéñ îḿþļéḿéñţîñĝ à ƒöŕḿàţ ŕéàđéŕ, çļàššîƒý éàçĥ îñļîñé éļéḿéñţ îñţö öñé öƒ
ţĥŕéé çàţéĝöŕîéš: ▒

| Category         | Run kind             | Examples                              | Pattern                                |
| ---------------- | -------------------- | ------------------------------------- | -------------------------------------- |
| **Paired tags**  | `PcOpen` + `PcClose` | `<b>...</b>`, `**...**`, `<a>...</a>` | Wrap content with two runs (shared ID) |
| **Self-closing** | `Ph`                 | `<br/>`, `<img>`, `<hr/>`             | Single run, no children                |
| **Block-level**  | _(not a run)_        | `<p>`, `<div>`, `<h1>`                | Boundary for a new Block               |

▒ Ţĥé ŕéàđéŕ đéçîđéš ŵĥàţ îš îñļîñé ṽš. ƃļöçķ-ļéṽéļ. Ƒöŕ ĤŢḾĻ, ţĥîš đîšţîñçţîöñ
îš ŵéļļ-đéƒîñéđ. Ƒöŕ öţĥéŕ ƒöŕḿàţš (Ḿàŕķđöŵñ, ẊĻÎƑƑ, çüšţöḿ ẊḾĻ), ýöü çĥööšé
ţĥé ḿàþþîñĝ ƃàšéđ öñ ŵĥàţ à ţŕàñšļàţöŕ ñééđš ţö šéé àš à çöñţîĝüöüš üñîţ. ▒

### ▒ Çöḿþļéţé Ŕéàđéŕ Éẋàḿþļé ŵîţĥ Îñļîñé Çöđéš ▒

▒ Ĥéŕé îš ĥöŵ à ŕéàđéŕ çöļļéçţš îñļîñé çöñţéñţ ƒŕöḿ à ƃļöçķ-ļéṽéļ éļéḿéñţ îñţö à
`[]ḿöđéļ.Ŕüñ` šļîçé. Ţĥîš þàţţéŕñ àþþļîéš ţö àñý ƒöŕḿàţ ŵîţĥ îñļîñé ḿàŕķüþ: ▒

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

▒ Ţĥé ķéý îñšîĝĥţ: **ŕéçüŕšé îñţö îñļîñé çĥîļđŕéñ** ţö ĥàñđļé ñéšţéđ ƒöŕḿàţţîñĝ
ļîķé `<ƃ><î>ƃöļđ îţàļîç</î></ƃ>`. Éàçĥ ļéṽéļ öƒ ñéšţîñĝ àþþéñđš îţš öŵñ
`ÞçÖþéñ`/`ÞçÇļöšé` þàîŕ, àñđ ţĥé ƒļàţ ŕüñ šéǫüéñçé ñàţüŕàļļý çàþţüŕéš ţĥé
çöŕŕéçţ öŕđéŕ. Àţţàçĥ ţĥé çöļļéçţéđ ŕüñš ţö à ƃļöçķ ŵîţĥ
`ƃļöçķ.ŠéţŠöüŕçéŔüñš(ŕüñš)`, öŕ ƃüîļđ ţĥé ƃļöçķ đîŕéçţļý ŵîţĥ
`ḿöđéļ.ÑéŵŔüñšƂļöçķ(îđ, ŕüñš)`. ▒

### ▒ Ŕéçöñšţŕüçţîñĝ Ḿàŕķüþ îñ à Ŵŕîţéŕ ▒

▒ Ţĥé ŵŕîţéŕ ŵàļķš ţĥé ŕüñ šéǫüéñçé àñđ éḿîţš éàçĥ ŕüñ'š çöñţéñţ: ļîţéŕàļ ţéẋţ ƒöŕ
`ŢéẋţŔüñ`š, ţĥé çàþţüŕéđ `Đàţà` ƒöŕ îñļîñé-çöđé ŕüñš. Ţĥé ƒŕàḿéŵöŕķ þŕöṽîđéš
`ḿöđéļ.ŔéñđéŕŔüñšŴîţĥĐàţà` ƒöŕ éẋàçţļý ţĥîš — ţĥé çàñöñîçàļ ŕéñđéŕîñĝ þàţĥ ţĥé
ĤŢḾĻ, ẊḾĻ, àñđ Ḿàŕķđöŵñ ŵŕîţéŕš àļļ üšé: ▒

```go
func (w *Writer) renderRuns(buf *strings.Builder, runs []model.Run) {
    // RenderRunsWithData emits TextRun content verbatim and re-emits the
    // captured Data for every inline-code run (Ph, PcOpen, PcClose, Sub).
    buf.WriteString(model.RenderRunsWithData(runs))
}
```

▒ Ţĥîš àþþŕöàçĥ ĝüàŕàñţééš **þéŕƒéçţ ŕöüñđţŕîþ ƒîđéļîţý** — ţĥé ŵŕîţéŕ đöéšñ'ţ
ñééđ ţö üñđéŕšţàñđ ţĥé ḿàŕķüþ ƒöŕḿàţ. Îţ ĵüšţ ŕéþļàýš ŵĥàţéṽéŕ `Đàţà` ţĥé
ŕéàđéŕ šţöŕéđ. Àñ `<à ĥŕéƒ="/ĥéļþ" çļàšš="ñàṽ">` ţàĝ ŕöüñđţŕîþš àš éẋàçţļý
ţĥàţ šţŕîñĝ, àţţŕîƃüţéš àñđ àļļ. ▒

### ▒ Çĥööšîñĝ Ţàŕĝéţ ṽš Šöüŕçé Çöñţéñţ ▒

▒ Ŵĥéñ ŵŕîţîñĝ öüţþüţ, ţĥé ŵŕîţéŕ ḿüšţ çĥööšé ţĥé ŕîĝĥţ çöñţéñţ. Üšé ţĥé ţàŕĝéţ
ŕüñš îƒ à ţŕàñšļàţîöñ éẋîšţš ƒöŕ ţĥé çöñƒîĝüŕéđ ļöçàļé, öţĥéŕŵîšé ƒàļļ ƃàçķ ţö
ţĥé šöüŕçé ŕüñš: ▒

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

### ▒ Üšîñĝ Šķéļéţöñš ƒöŕ Đöçüḿéñţ Šţŕüçţüŕé ▒

▒ Ƃļöçķ-ļéṽéļ šţŕüçţüŕé ţĥàţ šüŕŕöüñđš ţŕàñšļàţàƃļé çöñţéñţ (öþéñîñĝ àñđ çļöšîñĝ
ţàĝš, ŵĥîţéšþàçé, éţç.) îš çàþţüŕéđ îñ à **Šķéļéţöñ**. Ţĥé ŕéàđéŕ ƃüîļđš à
šķéļéţöñ ŵîţĥ ţéẋţ þàŕţš àñđ à ŕéƒéŕéñçé ţö ţĥé ƃļöçķ çöñţéñţ: ▒

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

▒ Ţĥé ŵŕîţéŕ üšéš ţĥé šķéļéţöñ ţö ŕéçöñšţŕüçţ ţĥé đöçüḿéñţ: ▒

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

▒ Šķéļéţöñš àŕé çŕîţîçàļ ƒöŕ ŕöüñđţŕîþ ƒîđéļîţý öƒ ţĥé ƃļöçķ-ļéṽéļ đöçüḿéñţ
šţŕüçţüŕé. Ŵîţĥöüţ ţĥéḿ, ţĥé ŵŕîţéŕ ŵöüļđ ñééđ ţö ŕé-ĝéñéŕàţé àļļ šüŕŕöüñđîñĝ
ţàĝš, ŵĥîţéšþàçé, àñđ àţţŕîƃüţéš — ŵĥîçĥ ŕîšķš ļöšîñĝ îñƒöŕḿàţîöñ. ▒

---

## ▒ Ŕüñ Ḿéţàđàţà Ƒîéļđš ▒

▒ Àñ îñļîñé-çöđé ŕüñ çàŕŕîéš ḿöŕé ţĥàñ ĵüšţ ţĥé ŕàŵ ḿàŕķüþ. Ţĥéšé ƒîéļđš ĥéļþ
ţööļš, éđîţöŕš, àñđ ǪÀ çĥéçķš ŵöŕķ ŵîţĥ îñļîñé çöđéš îñţéļļîĝéñţļý: ▒

| Field         | Purpose                                          | Example                                  |
| ------------- | ------------------------------------------------ | ---------------------------------------- |
| _discriminator_ | Which field is set: `PcOpen`, `PcClose`, or `Ph` | a `PcOpen`                            |
| `Type`        | Semantic type for tool processing                | `"fmt:bold"`, `"fmt:link"`, `"var"`      |
| `ID`          | Matches an opening run to its closing run        | `"1"` shared by the `<b>`/`</b>` pair    |
| `Data`        | Original markup for roundtrip reconstruction     | `"<a href=\"/help\">"`                   |
| `Disp`        | UI label in translation editors                  | `"[B]"`, `"[/B]"`, `"[IMG]"`             |
| `Equiv`       | Plain text equivalent                            | `"\n"` for `<br>`                        |
| `Constraints` | Editing policy (`Deletable`/`Cloneable`/`Reorderable`) | non-deletable for a `{count}` variable |

▒ Šéţ ţĥéšé ƒîéļđš îñ ţĥé ŕéàđéŕ ŵĥéñ ýöü ĥàṽé ţĥé îñƒöŕḿàţîöñ. Àţ ḿîñîḿüḿ, šéţ
ţĥé đîšçŕîḿîñàţöŕ, `Ţýþé`, `ÎĐ`, àñđ `Đàţà`. Ţĥé öţĥéŕ ƒîéļđš éñĥàñçé ţĥé
éẋþéŕîéñçé ƒöŕ ţŕàñšļàţöŕš àñđ ţööļš ƃüţ àŕé öþţîöñàļ. ▒

---

## ▒ Çöñƒîĝüŕàţîöñ ▒

▒ `ĐàţàƑöŕḿàţÇöñƒîĝ` ŕéǫüîŕéš ƒöüŕ ḿéţĥöđš — `ƑöŕḿàţÑàḿé()`, `Ŕéšéţ()`,
`Ṽàļîđàţé()`, àñđ `ÀþþļýḾàþ(ṽàļüéš ḿàþ[šţŕîñĝ]àñý) éŕŕöŕ`. `ÀþþļýḾàþ` àþþļîéš
çöñƒîĝ ṽàļüéš ƒŕöḿ à ḿàþ àñđ ŕéĵéçţš üñķñöŵñ ķéýš àñđ ţýþé ḿîšḿàţçĥéš. Ţĥé
`ƒöŕḿàţ.ÀþþļýḾàþṼîàĴŠÖÑ` ĥéļþéŕ (`çöŕé/ƒöŕḿàţ/àþþļýḿàþ.ĝö`) îḿþļéḿéñţš ţĥîš ƒöŕ
šţŕüçţ çöñƒîĝš ṽîà ĴŠÖÑ ḿàŕšĥàļ/üñḿàŕšĥàļ ŵîţĥ `ĐîšàļļöŵÜñķñöŵñƑîéļđš`, ŵĥîļé
çöñƒîĝš ŵîţĥ çöḿþļéẋ þàŕšîñĝ çàñ ĥàñđ-ŵŕîţé à šŵîţçĥ-ƃàšéđ `ÀþþļýḾàþ` (šéé
`çöŕé/ƒöŕḿàţš/ĵšöñ/çöñƒîĝ.ĝö`). Ţĥé ĥéļþéŕ ŕéǫüîŕéš ţĥé šţŕüçţ'š ƒîéļđš ţö
çàŕŕý ḿàţçĥîñĝ `ýàḿļ`/`ĵšöñ` ţàĝš ƒöŕ ţĥé îñçöḿîñĝ ķéýš. ▒

```go
type Config struct {
    Encoding string `yaml:"encoding" json:"encoding"`
}

func (c *Config) FormatName() string { return "myformat" }
func (c *Config) Reset()             { c.Encoding = "UTF-8" }
func (c *Config) Validate() error    { return nil }

func (c *Config) ApplyMap(values map[string]any) error {
    return format.ApplyMapViaJSON(c, values)
}
```

## ▒ Ŕéĝîšţŕàţîöñ ▒

▒ Àđđ ýöüŕ ƒöŕḿàţ îñšîđé `ƒöŕḿàţš.ŔéĝîšţéŕÀļļ()` îñ `çöŕé/ƒöŕḿàţš/ŕéĝîšţéŕ.ĝö`.
`ŔéĝîšţéŕŔéàđéŕ` ţàķéš ţĥé ƒöŕḿàţ ñàḿé, à ŕéàđéŕ ƒàçţöŕý, à `ƑöŕḿàţŠîĝñàţüŕé`
ƒöŕ đéţéçţîöñ, àñđ à đîšþļàý ñàḿé; `ŔéĝîšţéŕŴŕîţéŕ` ţàķéš ţĥé ñàḿé àñđ à ŵŕîţéŕ
ƒàçţöŕý: ▒

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

## ▒ Ţéšţîñĝ ▒

### ▒ Éẋţŕàçţîöñ Ţéšţš ▒

▒ Ṽéŕîƒý ţĥàţ ţĥé ŕéàđéŕ çöŕŕéçţļý îđéñţîƒîéš ţŕàñšļàţàƃļé çöñţéñţ àñđ îñļîñé
çöđéš: ▒

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
    // (There is no Segment type — segmentation is an opt-in overlay, F-02.)
    runs := blocks[0].SourceRuns()
    require.Len(t, runs, 4) // "Click ", <b>, "here", </b> + trailing text coalesces
    require.NotNil(t, runs[1].PcOpen)
    assert.Equal(t, "<b>", runs[1].PcOpen.Data)
    require.NotNil(t, runs[3].PcClose)
    assert.Equal(t, "</b>", runs[3].PcClose.Data)
    assert.Equal(t, runs[1].PcOpen.ID, runs[3].PcClose.ID) // shared ID
}
```

▒ Ţéšţ éàçĥ ţýþé öƒ îñļîñé éļéḿéñţ ýöüŕ ƒöŕḿàţ šüþþöŕţš: ▒

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

### ▒ Ŕöüñđţŕîþ Ţéšţš ▒

▒ Ţĥé ĝöļđ šţàñđàŕđ: ŕéàđ à ƒîļé, ŵŕîţé îţ ƃàçķ, çöḿþàŕé ŵîţĥ ţĥé öŕîĝîñàļ.
Ţĥîš þŕöṽéš ţĥàţ îñļîñé çöđéš šüŕṽîṽé ţĥé ƒüļļ ŕéàđ-ŵŕîţé çýçļé: ▒

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

### ▒ Ţŕàñšļàţîöñ Ŕöüñđţŕîþ Ţéšţš ▒

▒ Ṽéŕîƒý ţĥàţ ţŕàñšļàţéđ çöñţéñţ ŵîţĥ îñļîñé çöđéš ŵŕîţéš çöŕŕéçţļý: ▒

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

▒ Šéé [Ţéšţîñĝ](/contribute/testing) ƒöŕ ḿöŕé þàţţéŕñš. ▒

---

## ▒ Îñļîñé Çöđé Þàţţéŕñš ƃý Ƒöŕḿàţ ▒

▒ Đîƒƒéŕéñţ ƒöŕḿàţš ḿàþ ţö ţĥé šàḿé Ŕüñ ḿöđéļ îñ đîƒƒéŕéñţ ŵàýš: ▒

### ▒ ĤŢḾĻ / ẊḾĻ ▒

▒ Ƃļöçķ-ļéṽéļ éļéḿéñţš (`<þ>`, `<đîṽ>`, `<ĥ1>`) àŕé Ƃļöçķ ƃöüñđàŕîéš. Îñļîñé
éļéḿéñţš (`<ƃ>`, `<à>`, `<éḿ>`, `<šþàñ>`) ƃéçöḿé `ÞçÖþéñ`/`ÞçÇļöšé` þàîŕš. Ṽöîđ
éļéḿéñţš (`<ƃŕ>`, `<îḿĝ>`) ƃéçöḿé `Þĥ` ŕüñš. ▒

```
Input:  <p>Click <b>here</b> for <a href="/help">info</a></p>
Text:   "Click here for info"
Runs:   [text, PcOpen <b>, text, PcClose </b>, text, PcOpen <a href="/help">, text, PcClose </a>]
```

### ▒ Ḿàŕķđöŵñ ▒

▒ Éḿþĥàšîš ḿàŕķéŕš (`*`, `**`, `` ` ``) ƃéçöḿé `ÞçÖþéñ`/`ÞçÇļöšé` þàîŕš. Ļîñķš
ĥàṽé ţĥé ÜŔĻ šţöŕéđ îñ ţĥé öþéñîñĝ ŕüñ'š `Đàţà` ƒîéļđ. ▒

```
Input:  Click **here** for [info](/help)
Text:   "Click here for info"
Runs:   [text, PcOpen **, text, PcClose **, text, PcOpen [, text, PcClose ](/help)]
```

### ▒ ẊĻÎƑƑ / Ţŕàñšļàţîöñ Ƒöŕḿàţš ▒

▒ ẊĻÎƑƑ `<þç>` ḿàþš ţö à `ÞçÖþéñ`/`ÞçÇļöšé` þàîŕ, `<ƃþţ>`/`<éþţ>` (ƃéĝîñ/éñđ
þàîŕéđ ţàĝ) ļîķéŵîšé. `<þĥ>` (þļàçéĥöļđéŕ) àñđ `<îţ>` (îšöļàţéđ ţàĝ) ḿàþ ţö à
`Þĥ` ŕüñ. Ţĥé öŕîĝîñàļ ẊĻÎƑƑ îñļîñé ḿàŕķüþ ĝöéš îñţö ţĥé ŕüñ'š `Đàţà`. ▒

### ▒ Ţéḿþļàţîñĝ / Ṽàŕîàƃļéš ▒

▒ Ţéḿþļàţé ṽàŕîàƃļéš ļîķé `\{name\}` öŕ `$\{count\}` ƃéçöḿé `Þĥ` ŕüñš. Ţĥé ƒüļļ
ṽàŕîàƃļé éẋþŕéššîöñ ĝöéš îñţö `Đàţà`: ▒

```
Input:  Hello {name}, you have {count} items
Text:   "Hello , you have  items"
Runs:   [text, Ph {name}, text, Ph {count}, text]
```

### ▒ Ƒöŕḿàţš Ŵîţĥöüţ Îñļîñé Çöđéš ▒

▒ Ƒöŕḿàţš ļîķé ĴŠÖÑ, ÝÀḾĻ, öŕ .þŕöþéŕţîéš ţýþîçàļļý đöñ'ţ ĥàṽé îñļîñé ḿàŕķüþ.
Üšé `ḿöđéļ.ÑéŵƂļöçķ(îđ, ţéẋţ)` ţö çŕéàţé à ƃļöçķ ŵîţĥ à šîñĝļé þļàîñ `ŢéẋţŔüñ`.
Îƒ ţĥéšé ƒöŕḿàţš çöñţàîñ éḿƃéđđéđ ĤŢḾĻ öŕ Ḿàŕķđöŵñ, üšé ñéšţéđ Ļàýéŕš
(šéé [Àŕçĥîţéçţüŕé](/framework/architecture)) ţö đéļéĝàţé îñļîñé
ĥàñđļîñĝ ţö ţĥé àþþŕöþŕîàţé šüƃ-ƒöŕḿàţ ŕéàđéŕ. ▒
