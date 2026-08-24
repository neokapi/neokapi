---
sidebar_position: 1
title: Implementing Formats
description: Implementation note for E-02 — step-by-step instructions for writing new neokapi format readers and writers, or migrating Okapi Java filters, including terminology mapping from Okapi to neokapi concepts.
keywords: [implementing formats, format reader, format writer, Okapi migration, DataFormatReader, neokapi]
---

# ▒ Îḿþļéḿéñţîñĝ Ƒöŕḿàţš ▒

▒ Šţéþ-ƃý-šţéþ ĝüîđé ƒöŕ îḿþļéḿéñţîñĝ ñéŵ ñéöķàþî ƒöŕḿàţ ŕéàđéŕš/ŵŕîţéŕš öŕ
ḿîĝŕàţîñĝ éẋîšţîñĝ Öķàþî ƒîļţéŕš. Þàŕéñţ ÀĐ:
[É-02](/contribute/architecture/engine/e-02-format-system). ▒

▒ :::ñöţé Çàñöñîçàļ ţüţöŕîàļ
Ƒöŕ ţĥé éñđ-ţö-éñđ "àđđ à ƒöŕḿàţ" ŵàļķţĥŕöüĝĥ, ƒöļļöŵ
[Îḿþļéḿéñţîñĝ à Ƒöŕḿàţ](/contribute/formats). Ţĥîš ñöţé ƒöçüšéš öñ ţĥé
šķéļéţöñ-šţöŕé, ŵŕîţéŕ-ƒàļļƃàçķ, àñđ Öķàþî-þöŕţîñĝ îñţéŕñàļš ţĥàţ šîţ ƃéñéàţĥ
ţĥàţ ţüţöŕîàļ. Ḿàîñţàîñéŕš: ţĥé ḿàţüŕîţý ƃàŕ à ƒöŕḿàţ ḿüšţ çļéàŕ ļîṽéš îñ
`đöçš/îñţéŕñàļš/ƒöŕḿàţ-ḿàţüŕîţý.ḿđ`, àñđ ţĥé çöñšöļîđàţéđ éñĝîñé ŕéƒéŕéñçé îñ
`đöçš/îñţéŕñàļš/ƒöŕḿàţ-éñĝîñééŕîñĝ.ḿđ`.
::: ▒

## ▒ Ţéŕḿîñöļöĝý Ḿàþþîñĝ ƒŕöḿ Öķàþî ▒

| Okapi (Java)                      | neokapi (Go)               |
| --------------------------------- | -------------------------- |
| Filter                            | DataFormat (Reader/Writer) |
| Step                              | Tool                       |
| Pipeline                          | Flow                       |
| PipelineDriver                    | Executor                   |
| Event                             | Part                       |
| TextUnit                          | Block                      |
| TextFragment                      | Run sequence (`[]Run`)     |
| Code                              | Run                        |
| StartDocument / EndDocument       | Layer (root)               |
| StartSubDocument / StartSubFilter | Child Layer                |

## ▒ Ƒîļé Šţŕüçţüŕé ▒

▒ Çŕéàţé à þàçķàĝé üñđéŕ `çöŕé/ƒöŕḿàţš/<ñàḿé>/` ŵîţĥ ţĥŕéé ƒîļéš: ▒

```
core/formats/<name>/
├── config.go       # Config struct with Reset(), Validate(), ApplyMap()
├── reader.go       # DataFormatReader implementation
├── writer.go       # DataFormatWriter implementation
├── reader_test.go  # Reader tests
├── writer_test.go  # Writer or roundtrip tests
└── testdata/       # Test input files
```

## ▒ Çöñƒîĝ ▒

▒ Éṽéŕý ƒöŕḿàţ ĥàš à `Çöñƒîĝ` šţŕüçţ îḿþļéḿéñţîñĝ `ƒöŕḿàţ.ĐàţàƑöŕḿàţÇöñƒîĝ`: ▒

```go
type Config struct {
    // Format-specific options...
    // Use compiled regex caches for regex-based config (see json/config.go).
}

func (c *Config) FormatName() string { return "<name>" }

func (c *Config) Reset() {
    *c = Config{
        // Set defaults here. Use zero values intentionally —
        // bool defaults to false, so use "nonFoo" naming when
        // you want the default behavior to be "foo".
    }
}

func (c *Config) Validate() error {
    // Return non-nil error for invalid combinations.
    return nil
}

// ApplyMap applies config values from a generic map (used by CLI/presets).
func (c *Config) ApplyMap(values map[string]any) error {
    for key, val := range values {
        switch key {
        case "someOption":
            // type-assert and assign
        default:
            return fmt.Errorf("<name>: unknown parameter: %s", key)
        }
    }
    return nil
}
```

▒ **Ŕéƒéŕéñçé**: `çöŕé/ƒöŕḿàţš/ĵšöñ/çöñƒîĝ.ĝö` (çöḿþļéẋ çöñƒîĝ ŵîţĥ ŕéĝéẋ çàçĥéš),
`çöŕé/ƒöŕḿàţš/þļàîñţéẋţ/çöñƒîĝ.ĝö` (ḿîñîḿàļ çöñƒîĝ). ▒

## ▒ Ŕéàđéŕ ▒

▒ Éḿƃéđ `ƒöŕḿàţ.ƂàšéƑöŕḿàţŔéàđéŕ` àñđ îḿþļéḿéñţ `ƒöŕḿàţ.ĐàţàƑöŕḿàţŔéàđéŕ`: ▒

```go
type Reader struct {
    format.BaseFormatReader
    cfg           *Config
    skeletonStore *format.SkeletonStore
    skelBuf       bytes.Buffer // coalescing buffer for skeleton text
}

var _ format.SkeletonStoreEmitter = (*Reader)(nil)

func NewReader() *Reader {
    cfg := &Config{}
    cfg.Reset()
    return &Reader{
        BaseFormatReader: format.BaseFormatReader{
            FormatName:        "<name>",
            FormatDisplayName: "<Display Name>",
            FormatMimeType:    "application/<name>",
            FormatExtensions:  []string{".<ext>"},
            Cfg:               cfg,
        },
        cfg: cfg,
    }
}

func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
    r.skeletonStore = store
}
```

▒ `ƂàšéƑöŕḿàţŔéàđéŕ` šüþþļîéš `Ñàḿé`/`ĐîšþļàýÑàḿé`/`Çöñƒîĝ`/`ŠéţÇöñƒîĝ`. Ýöü ḿüšţ
šţîļļ îḿþļéḿéñţ ţĥé ţĥŕéé ḿéţĥöđš îţ đöéš **ñöţ** þŕöṽîđé — `Šîĝñàţüŕé`, `Öþéñ`,
àñđ `Çļöšé`: ▒

```go
func (r *Reader) Signature() format.FormatSignature {
    return format.FormatSignature{
        MIMETypes:  []string{"application/<name>"},
        Extensions: []string{".<ext>"},
    }
}

// Open validates and stashes the document; it does NOT parse. Parse errors are
// surfaced on the channel in Read (as PartResult.Error), never returned here.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
    if doc == nil || doc.Reader == nil {
        return errors.New("<name>: nil document or reader")
    }
    r.Doc = doc
    return nil
}

func (r *Reader) Close() error {
    if r.Doc != nil && r.Doc.Reader != nil {
        return r.Doc.Reader.Close()
    }
    return nil
}
```

### ▒ Ŕéàđ Ḿéţĥöđ Þàţţéŕñ ▒

▒ Ţĥé `Ŕéàđ` ḿéţĥöđ öþéñš à ĝöŕöüţîñé ţĥàţ šéñđš `ḿöđéļ.ÞàŕţŔéšüļţ` ṽàļüéš öñ à
çĥàññéļ. Îţ ḿüšţ éḿîţ `ÞàŕţĻàýéŕŠţàŕţ` ƒîŕšţ, ţĥéñ ƃļöçķš/đàţà, ţĥéñ
`ÞàŕţĻàýéŕÉñđ`: ▒

```go
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
    ch := make(chan model.PartResult, 64)
    go func() {
        defer close(ch)
        r.readContent(ctx, ch)
    }()
    return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
    // 1. Emit PartLayerStart
    layer := &model.Layer{
        ID:     "doc",
        Name:   filepath.Base(r.Doc.URI),
        Format: "<name>",
        Locale: r.Doc.SourceLocale,
    }
    ch <- model.PartResult{Part: &model.Part{
        Type:     model.PartLayerStart,
        Resource: layer,
    }}

    // 2. Parse input, emit blocks and data
    //    (see Skeleton Store Integration below)

    // 3. Flush skeleton store
    r.skelFlush()
    if r.skeletonStore != nil {
        if err := r.skeletonStore.Flush(); err != nil {
            ch <- model.PartResult{Error: fmt.Errorf("<name>: flush skeleton: %w", err)}
            return
        }
    }

    // 4. Emit PartLayerEnd
    ch <- model.PartResult{Part: &model.Part{
        Type:     model.PartLayerEnd,
        Resource: layer,
    }}
}
```

### ▒ Ƃļöçķ Çŕéàţîöñ ▒

```go
block := model.NewBlock(blockID, sourceText)
block.Name = blockName
block.Properties["<format>.keypath"] = keyPath // format-specific metadata

ch <- model.PartResult{Part: &model.Part{
    Type:     model.PartBlock,
    Resource: block,
}}
```

### ▒ Šüƃƒîļţéŕ Šüþþöŕţ ▒

▒ Îƒ ţĥé ƒöŕḿàţ çàñ çöñţàîñ éḿƃéđđéđ çöñţéñţ (é.ĝ., ĤŢḾĻ šţŕîñĝš îñšîđé ĴŠÖÑ),
îḿþļéḿéñţ `ƒöŕḿàţ.ŠüƃƒîļţéŕÀŵàŕé`: ▒

```go
var _ format.SubfilterAware = (*Reader)(nil)

func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
    r.resolver = resolver
}
```

▒ Ŵĥéñ éñçöüñţéŕîñĝ éḿƃéđđéđ çöñţéñţ, çŕéàţé à çĥîļđ ļàýéŕ: ▒

```go
subReader, err := r.resolver.ResolveReader(subFormatName)
// Open subReader with the embedded content as a RawDocument
// Emit PartLayerStart for child, forward sub-parts, emit PartLayerEnd
```

▒ Ţĥŕéé öƃļîĝàţîöñš çöḿé ŵîţĥ à çĥîļđ ļàýéŕ, àñđ éàçĥ îš à ŵàý ţĥé ţŕàñšļàţéđ
çĥîļđ šîļéñţļý ƒàîļš ţö ŕéàçĥ ţĥé ƒîļé ŵĥéñ îţ îš šķîþþéđ. ▒

▒ **Ŵŕîţé à šķéļéţöñ ŕéƒ ƒöŕ îţ.** À đéļéĝàţéđ šþàñ îš à ŕàñĝé ļîķé àñý öţĥéŕ
ƃļöçķ'š: éḿîţ `ļàýéŕ:<îđ>` ŵĥéŕé ţĥé ḿéḿƃéŕ'š ƃýţéš ŵéŕé. À ŕéàđéŕ ţĥàţ éḿîţš
ţĥé çĥîļđ ļàýéŕ àñđ ñö ŕéƒ þŕöđüçéš à ţŕàñšļàţéđ šüƃ-đöçüḿéñţ ţĥàţ ţĥé ŵŕîţéŕ
ţĥéñ đŕöþš — ţĥé éẋîţ çöđé îš 0, ţĥé ƒîļé îš ŵŕîţţéñ, àñđ ţĥé ŵöŕķ îš öñļý îñ
ţĥé šţöŕé, šö à ļàţéŕ ḿéŕĝé ŕéþöŕţš îţ đöñé. ▒

▒ **Đéļéĝàţé ŵĥàţ ţĥé ƒöŕḿàţ àçţüàļļý îš.** Šüƃ-ƒîļţéŕîñĝ îš ƒöŕ çöñţéñţ îñ
*àñöţĥéŕ* ƒöŕḿàţ — ĤŢḾĻ îñšîđé à ĴŠÖÑ šţŕîñĝ, ẊĤŢḾĻ îñ àñ ÉÞÜƂ šþîñé. Ĥàñđîñĝ à
ƒöŕḿàţ îţš öŵñ ḿàŕķüþ ţö à ĝéñéŕîç ŕéàđéŕ öƒ ţĥé šàḿé ƒàḿîļý đîšçàŕđš ţĥé
ƒöŕḿàţ'š éẋţŕàçţîöñ ŕüļéš àñđ îţš çöñƒîĝ àļöñĝ ŵîţĥ ţĥéḿ, àñđ ţĥéŕé îš ñöţĥîñĝ
ţĥé ŵŕîţéŕ šîđé çàñ đö ţö ŕéþàîŕ ţĥàţ. Üþšţŕéàḿ Öķàþî đŕàŵš ţĥé šàḿé ļîñé:
`ÖþéñÖƒƒîçéƑîļţéŕ` đîšþàţçĥéš éàçĥ šţŕéàḿ öƒ àñ ÖĐƑ þàçķàĝé ţö îţš öŵñ
`ÖĐƑƑîļţéŕ`, ñéṽéŕ ţö `öķƒ_ẋḿļ`. ▒

▒ **Þüţ ţĥé çĥîļđ ƃàçķ îñ ţĥé çàŕŕîéŕ îţ çàḿé öüţ öƒ.** Ţĥé šüƃ-ŕéàđéŕ îš ĥàñđéđ
*đéçöđéđ* çöñţéñţ, šö ĥöŵ ţĥé þàŕéñţ šþéļļéđ îţ îš ñöţ ŕéçöṽéŕàƃļé ƒŕöḿ ţĥé
çĥîļđ'š öüţþüţ. Ŕéçöŕđ ţĥé çàŕŕîéŕ öñ ţĥé çĥîļđ ļàýéŕ àš ţĥé ŕéàđéŕ šééš îţ àñđ
ĥàṽé ţĥé ŵŕîţéŕ ĥöñöüŕ îţ — ƒöŕ ẊḾĻ, à ÇĐÀŢÀ šéçţîöñ ŕéţüŕñš àš à ÇĐÀŢÀ šéçţîöñ
ŵîţĥ îţš đéļîḿîţéŕš ļéƒţ îñ ţĥé šķéļéţöñ àñđ ţĥé çĥîļđ ŵŕîţţéñ ƃéţŵééñ ţĥéḿ
ṽéŕƃàţîḿ, àñđ éšçàþéđ çĥàŕàçţéŕ đàţà ŕéţüŕñš éšçàþéđ. ẊḾĻ 1.0 §2.7 ḿàķéš ţĥé
ţŵö ţĥé šàḿé çöñţéñţ, šö çöñṽéŕţîñĝ éîţĥéŕ îñţö ţĥé öţĥéŕ ŵöüļđ ŕéŵŕîţé éṽéŕý
šüçĥ éļéḿéñţ ƒöŕ ñöţĥîñĝ; §2.4 ḿàķéš ţĥé éšçàþîñĝ öƃļîĝàţöŕý ƒöŕ ţĥé šéçöñđ, öŕ
ţĥé ḿàŕķüþ ţĥé šüƃ-ŕéàđéŕ ĥàñđéđ ƃàçķ çļöšéš ţĥé éļéḿéñţ îţ šîţš îñ. Öķàþî
éñçöđéš éẋàçţļý ţĥîš đîšţîñçţîöñ îñ ţĥé þàŕéñţ éñçöđéŕ îţ ĥàñđš ţĥé šüƃ-ƒîļţéŕ:
`ñüļļ` ƒöŕ à ÇĐÀŢÀ šüƃƒîļţéŕ ("ŵé đöñ'ţ éñçöđé çđàţà") àñđ àñ `ẊḾĻÉñçöđéŕ` ƒöŕ
à ÞÇĐÀŢÀ öñé (`ÀƃšţŕàçţḾàŕķüþƑîļţéŕ.ĥàñđļéÇđàţàŠéçţîöñ` /
`ĥàñđļéÀţţŕîƃüţéŠüƃƒîļţéŕîñĝ`). ▒

▒ À ḿéḿƃéŕ ñöţĥîñĝ ţŕàñšļàţéđ šĥöüļđ ĝö ƃàçķ àš ţĥé ḿéḿƃéŕ. Ţĥé šüƃ-ŵŕîţéŕ
šéŕîàļîžéš ƒŕöḿ ţĥé çöñţéñţ ḿöđéļ, šö þüţţîñĝ àñ üñţöüçĥéđ ḿéḿƃéŕ ţĥŕöüĝĥ îţ
ŕéŵŕîţéš ḿàŕķüþ ñö ŕüñ ĥàđ ŕéàšöñ ţö çĥàñĝé — ŵĥîçĥ îš ŵĥý ţĥé ÉÞÜƂ ŵŕîţéŕ
šþļîçéš îñţö ţĥé éñţŕý'š öŕîĝîñàļ ƃýţéš àñđ ţĥé ẊḾĻ ŵŕîţéŕ ŕéţüŕñš ţĥé ḿéḿƃéŕ'š
öŵñ çöñţéñţ ŵĥéñ ñö ƃļöçķ îñ ţĥé çĥîļđ ļàýéŕ ĥöļđš à ţàŕĝéţ. ▒

## ▒ Ŵŕîţéŕ ▒

▒ Éḿƃéđ `ƒöŕḿàţ.ƂàšéƑöŕḿàţŴŕîţéŕ` àñđ îḿþļéḿéñţ `ƒöŕḿàţ.ĐàţàƑöŕḿàţŴŕîţéŕ`: ▒

```go
type Writer struct {
    format.BaseFormatWriter
    cfg           *Config
    skeletonStore *format.SkeletonStore
}

var _ format.SkeletonStoreConsumer = (*Writer)(nil)

func NewWriter() *Writer {
    cfg := &Config{}
    cfg.Reset()
    return &Writer{
        BaseFormatWriter: format.BaseFormatWriter{FormatName: "<name>"},
        cfg:              cfg,
    }
}

func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
    w.skeletonStore = store
}
```

### ▒ Ŵŕîţé Ḿéţĥöđ Þàţţéŕñ ▒

▒ Ţĥé ŵŕîţéŕ çöļļéçţš àļļ ƃļöçķš ƒŕöḿ ţĥé çĥàññéļ, ţĥéñ ŕéçöñšţŕüçţš ţĥé
đöçüḿéñţ. Îţ šĥöüļđ šüþþöŕţ à ƒàļļƃàçķ çĥàîñ: ▒

```go
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
    blocksByID := make(map[string]*model.Block)

    // 1. Drain channel, collect blocks
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-parts:
            if !ok {
                goto done
            }
            if part.Type == model.PartBlock {
                if block, ok := part.Resource.(*model.Block); ok {
                    blocksByID[block.ID] = block
                }
            }
        }
    }

done:
    // 2. Reconstruct using fallback chain
    if w.skeletonStore != nil {
        return w.writeFromSkeleton(w.skeletonStore, blocksByID)
    }
    return w.writeFromBlocks(blocksByID) // fallback
}
```

### ▒ Šķéļéţöñ Šţöŕé Ŕéçöñšţŕüçţîöñ ▒

```go
func (w *Writer) writeFromSkeleton(
    store *format.SkeletonStore,
    blocks map[string]*model.Block,
) error {
    for {
        entry, err := store.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("<name> writer: read skeleton: %w", err)
        }
        switch entry.Type {
        case format.SkeletonText:
            if _, err := w.Output.Write(entry.Data); err != nil {
                return err
            }
        case format.SkeletonRef:
            refID := string(entry.Data)
            if block, ok := blocks[refID]; ok {
                text := w.encodeValue(block) // format-specific encoding
                if _, err := io.WriteString(w.Output, text); err != nil {
                    return err
                }
            }
        }
    }
    return nil
}
```

### ▒ Ŵŕîţé-šîđé þöšţ-þŕöçéššîñĝ: ţĥé ñö-ŕéĝéẋ çöñṽéñţîöñ ▒

▒ À ƒöŕḿàţ ŵŕîţéŕ ḾÜŠŢ ÑÖŢ ŕéĝéẋ- öŕ ƃýţé-ŕéŵŕîţé îţš àļŕéàđý-šéŕîàļîžéđ
öüţþüţ ţö çöḿþéñšàţé ƒöŕ à ḿöđéļîñĝ ĝàþ. Ţĥàţ þöšţ-þŕöçéššîñĝ îš ƃŕîţţļé
(îţ þàţţéŕñ-ḿàţçĥéš šéŕîàļîžéđ ḿàŕķüþ), çöüþļéš ţö éḿîššîöñ öŕđéŕîñĝ, àñđ
ĥîđéš ţĥé ƒàçţ ţĥàţ ţĥé ḿöđéļ îš ḿîššîñĝ à þŕîḿîţîṽé. Ţĥé üñîƒîéđ þàţţéŕñ
ţĥàţ éṽéŕý ŵŕîţéŕ ƒöļļöŵš îñšţéàđ: ▒

1. ▒ **Šķéļéţöñ-šţöŕé éḿîššîöñ.** Ţĥé ŕéàđéŕ šţöŕéš ñöñ-ţŕàñšļàţàƃļé ƃýţéš
   ṽéŕƃàţîḿ; ţĥé ŵŕîţéŕ ŕéþļàýš ţĥéḿ àñđ šþļîçéš öñļý ţŕàñšļàţéđ šļöţš, šö
   ţĥé ŵŕîţéŕ îñţŕöđüçéš ñö šţŕüçţüŕàļ đîṽéŕĝéñçé ţö "ƒîẋ üþ" àƒţéŕŵàŕđ. ▒
2. ▒ **Šýḿḿéţŕîç çöḿþàŕé-ţîḿé çàñöñîçàļîžàţîöñ.** Çöšḿéţîç đîƒƒéŕéñçéš ƃéţŵééñ
   ţŵö ŵŕîţéŕš (àţţŕîƃüţé öŕđéŕ, ñàḿéšþàçé đéçļš, šéļƒ-çļöšîñĝ ṽš
   öþéñ/çļöšé, îñšîĝñîƒîçàñţ ŵĥîţéšþàçé) àŕé çàñçéļļéđ ƃý ţĥé šĥàŕéđ
   `ẊḾĻÇàñöñîçàļ` ñöŕḿàļîžéŕ (`çļî/þàŕîţý/ŕöüñđţŕîþ/ñöŕḿàļîžéŕš.ĝö`), àþþļîéđ
   ţö **ƃöţĥ** `ĝöţ` àñđ `ŕéƒ`. Ŕéàçĥîñĝ ţĥé `çàñöñ` ţîéŕ — ñöţ `ƃýţé` — îš
   ţĥé ñöŕḿ àñđ îš šüƒƒîçîéñţ. ▒
3. ▒ **Šţŕüçţüŕàļ ḿéŕĝéš àš çàñöñîçàļîžàţîöñ, ñöţ ŵŕîţé-šîđé ŕéŵŕîţîñĝ.**
   "Ḿéŕĝé àđĵàçéñţ éǫüîṽàļéñţ éļéḿéñţš" ƃéļöñĝš îñ ţĥé ñöŕḿàļîžéŕ (àþþļîéđ
   šýḿḿéţŕîçàļļý ţö ƃöţĥ šîđéš), ñöţ îñ ţĥé ŵŕîţéŕ (àþþļîéđ ţö öñé šîđé ṽîà
   ŕéĝéẋ). îđḿļ'š `ḾéŕĝéÀđĵàçéñţÇŠŔš` îš ţĥé ţéḿþļàţé. ▒

▒ Þéŕ-ṽàļüé **éšçàþîñĝ öƒ ţéẋţ çöñţéñţ ƃéƒöŕé îţš ƒîŕšţ éḿîššîöñ** (ƃàçķšļàšĥ
/ ǫüöţé / ñéŵļîñé / đéļîḿîţéŕ éñçöđîñĝ) îš ñöţ þöšţ-þŕöçéššîñĝ àñđ îš ƒîñé. ▒

▒ **Ţĥé öñé šàñçţîöñéđ éẋçéþţîöñ** îš ƒàîţĥƒüļļý ŕéþŕöđüçîñĝ à ţŕàñšƒöŕḿ ţĥàţ
Öķàþî *îţšéļƒ* þéŕƒöŕḿš öñ ƃýţéš ţĥé ŕéàđéŕ çàþţüŕéđ öþàǫüéļý, ŵĥéŕé ñö
šýḿḿéţŕîç ñöŕḿàļîžéŕ çàñ ŕéàçĥ. öþéñẋḿļ'š ĐŕàŵîñĝḾĻ đéƒàüļţ-ŕüñ ĥöîšţ
(`öþţîḿîšéĐḾĻƂļöçķÞŕöþéŕţîéš` îñ `đḿļ_šţýļé_öþţîḿîžàţîöñ.ĝö`) îš ţĥé çüŕŕéñţ
ţéḿþļàţé: ţĥé ŴḾĻ ŕéàđéŕ çàþţüŕéš ţĥé éñţîŕé `<ŵ:đŕàŵîñĝ>` þàýļöàđ àš öþàǫüé
ẊḾĻ àñđ ŕéþļàýš îţ ṽéŕƃàţîḿ, šö ţĥé öñļý þļàçé ţö ḿîŕŕöŕ Öķàþî'š
`ŠţýļéÖþţîḿîšàţîöñ.Đéƒàüļţ` ĥöîšţ öƒ çöḿḿöñ `<à:ŕÞŕ>` îñţö `<à:þÞŕ><à:đéƒŔÞŕ>`
îš àñ àļŵàýš-öñ þöšţ-šķéļéţöñ ƒļüšĥ. Ţĥîš îš ŕéþŕöđüçţîöñ, ñöţ çöḿþéñšàţîöñ:
ţĥé ŕéƒéŕéñçé öüţþüţ àļŕéàđý çöñţàîñš ţĥé ĥöîšţ, àñđ ƃéçàüšé ţĥé þàýļöàđ îš
öþàǫüé ţö ţĥé çöḿþàŕàţöŕ îţ çàññöţ ƃé çàñçéļļéđ öñ ƃöţĥ šîđéš. À ŵŕîţéŕ ţĥàţ
ķééþš šüçĥ à ţŕàñšƒöŕḿ ḾÜŠŢ đöçüḿéñţ ţĥé Öķàþî çļàšš/ḿéţĥöđ îţ ḿîŕŕöŕš, šö à
ŕéàđéŕ çàñ ţéļļ ŕéþŕöđüçţîöñ ƒŕöḿ çöḿþéñšàţîöñ. ▒

> ▒ Ţĥé ŴöŕđþŕöçéššîñĝḾĻ šîđé đöéš **ñöţ** ǫüàļîƒý: ñàţîṽé îš ƒàîţĥƒüļ àñđ
> éḿîţš šöüŕçé `<ŵ:ŕÞŕ>` îñļîñé ŵîţĥ ñö šýñţĥéšîšéđ þàŕàĝŕàþĥ šţýļéš. Ţĥé
> ƒöŕḿéŕ Ŵöŕđ Šţýļé Öþţîḿîšàţîöñ (ŴŠÖ) þöšţ-þàšš ţĥàţ ḿîḿîçķéđ Öķàþî'š çöḿþàçţ
> þŠţýļé ƒöŕḿ ĥàš ƃééñ đéļéţéđ; éǫüîṽàļéñçé ŵîţĥ Öķàþî'š çöḿþàçţ öüţþüţ îš
> îñšţéàđ þŕöṽéđ ƃý àñ éƒƒéçţîṽé-ŕÞŕ ñöŕḿàļîžéŕ îñ ţĥé þàŕîţý çöḿþàŕàţöŕ. ▒

▒ Ƒöŕḿàţš àļŕéàđý çöñṽéŕţéđ ţö ţĥîš çöñṽéñţîöñ: ĥţḿļ (ĐÖḾ `šéţÀţţŕ` îñšţéàđ öƒ
ļàñĝ ŕéĝéẋ), ţĥé ŕéĝéẋ ƒöŕḿàţ (þŕéƒîẋ/çàþţüŕé/šüƒƒîẋ àššéḿƃļý), ŵîķî (šţöŕéđ
ĥéàđéŕ ļéṽéļ), àñđ öþéñẋḿļ (šţŕüçţüŕàļ `<ŵ:ŕ>` éñṽéļöþé éḿîššîöñ + ƃýţé-šþļîçé
ŕüñ ḿéŕĝéš ŕéþļàçîñĝ ţĥé þöšţ-šéŕîàļîžàţîöñ ƒüšé ŕéĝéẋéš). Ŵĥéñ à šţŕüçţüŕàļ
ƒîẋ îš ĝéñüîñéļý îḿþŕàçţîçàļ, þŕéƒéŕ à đöçüḿéñţéđ `đîṽ`-ţîéŕ đîṽéŕĝéñçé öŕ à
ţŕàçķéđ ƒöļļöŵ-üþ îššüé öṽéŕ à ñéŵ ŵŕîţé-šîđé ŕéĝéẋ. ▒

## ▒ Šķéļéţöñ Šţöŕé Îñţéĝŕàţîöñ ▒

▒ Ţĥé ŠķéļéţöñŠţöŕé (`çöŕé/ƒöŕḿàţ/šķéļéţöñ.ĝö`) éñàƃļéš ƃýţé-éẋàçţ ŕöüñđţŕîþ öƒ
đöçüḿéñţš. Ţĥé ŕéàđéŕ ŵŕîţéš šķéļéţöñ éñţŕîéš àš îţ þàŕšéš; ţĥé ŵŕîţéŕ ŕéàđš
ţĥéḿ ţö ŕéçöñšţŕüçţ ţĥé öüţþüţ. Ţööļš îñ ƃéţŵééñ öñļý šéé ƃļöçķš — ţĥéý ñéṽéŕ
ţöüçĥ ţĥé šķéļéţöñ. ▒

▒ Šéé [Šķéļéţöñ Šţöŕé](/contribute/implementation/engine/skeleton-store) ƒöŕ ƃîñàŕý ƒöŕḿàţ àñđ ÀÞÎ
đéţàîļš. ▒

### ▒ Ŕéàđéŕ Šîđé: Çöàļéšçîñĝ Ƃüƒƒéŕ Þàţţéŕñ ▒

▒ Đö ÑÖŢ ŵŕîţé öñé šķéļéţöñ éñţŕý þéŕ ţöķéñ. Üšé à `ƃýţéš.Ƃüƒƒéŕ` ţö àççüḿüļàţé
ţĥé šķéļéţöñ ţéẋţ ƃéţŵééñ ƃļöçķ ŕéƒéŕéñçéš, ţĥéñ ƒļüšĥ ƃéƒöŕé éàçĥ ŕéƒ: ▒

```go
// skelText appends text to the coalescing buffer.
func (r *Reader) skelText(s string) {
    if r.skeletonStore != nil {
        r.skelBuf.WriteString(s)
    }
}

// skelRef flushes accumulated text, then writes a block reference.
func (r *Reader) skelRef(id string) {
    if r.skeletonStore != nil {
        if r.skelBuf.Len() > 0 {
            r.skeletonStore.WriteText(r.skelBuf.Bytes())
            r.skelBuf.Reset()
        }
        r.skeletonStore.WriteRef(id)
    }
}

// skelFlush writes any remaining buffered text.
func (r *Reader) skelFlush() {
    if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
        r.skeletonStore.WriteText(r.skelBuf.Bytes())
        r.skelBuf.Reset()
    }
}
```

▒ Ţĥîš ŕéđüçéš šķéļéţöñ éñţŕîéš ƒŕöḿ ~Ñ (öñé þéŕ ţöķéñ) ţö ~2Ƃ+1 (ŵĥéŕé Ƃ îš ţĥé
ñüḿƃéŕ öƒ ţŕàñšļàţàƃļé ƃļöçķš). Ƒöŕ éẋàḿþļé, à ĴŠÖÑ ƒîļé ŵîţĥ 50 šţŕîñĝš
þŕöđüçéš ~101 éñţŕîéš îñšţéàđ öƒ ~10,000. ▒

### ▒ Ŵĥàţ Ĝöéš Ŵĥéŕé ▒

| Content                                           | Skeleton             | Block / Data                                  |
| ------------------------------------------------- | -------------------- | --------------------------------------------- |
| Structural tokens (`\{`, `}`, `[`, `]`, `,`, `:`) | Text                 | --                                            |
| Whitespace, formatting                            | Text                 | --                                            |
| Object keys                                       | Text                 | --                                            |
| Translatable string values                        | Ref (block ID)       | Source text                                   |
| Non-translatable contextual values (code, captions, formulas, do-not-translate, config-excluded) | Ref (block ID) | `Block{Translatable:false}` + `SemanticRole` |
| Comments / non-content metadata                   | Text or Ref          | `Data` (`PartData`) or a `NoteAnnotation`     |
| Embedded/subfiltered content                      | Ref (`layer:<path>`) | Child layer                                   |

▒ Ţĥé ļàšţ ţŵö ŕöŵš àŕé ţĥé **çöñţéñţ-ƒîđéļîţý šüŕƒàçîñĝ** çöñṽéñţîöñ
([É-02](/contribute/architecture/engine/e-02-format-system), đéƒàüļţ-ÖÑ
þéŕ-ƒöŕḿàţ öþţ-öüţ `éẋţŕàçţÑöñŢŕàñšļàţàƃļéÇöñţéñţ`): çöñţéẋţüàļ ñöñ-ţŕàñšļàţàƃļé
çöñţéñţ îš šüŕƒàçéđ ƒöŕ ĻĻḾ/ŔÀĜ îñĝéšţîöñ îñšţéàđ öƒ ƃéîñĝ ƃüŕîéđ îñ šķéļéţöñ.
Ţĥé ƃöđý šţîļļ ŕîđéš à šķéļéţöñ **Ŕéƒ** šö ţĥé ŕöüñđ-ţŕîþ šţàýš ƃýţé-éẋàçţ àñđ
ţĥé ŵŕîţéŕ ŕé-éḿîţš îţ ƒŕöḿ ţĥé (ñöñ-ţŕàñšļàţàƃļé) ƃļöçķ; ḾŢ šķîþš îţ ƃéçàüšé
`Ţŕàñšļàţàƃļé` îš ƒàļšé. Ŵîţĥ ţĥé ƒļàĝ öƒƒ, ţĥéšé ŕöŵš çöļļàþšé ƃàçķ ţö þļàîñ
šķéļéţöñ `Ţéẋţ` — ţĥé çöñƒîĝüŕàţîöñ þàŕîţý þîñš
([À-02](/contribute/architecture/assurance/a-02-parity)). Šéé
[Çöñţéñţ-Ƒîđéļîţý Šüŕƒàçîñĝ](/contribute/implementation/engine/content-fidelity) ƒöŕ ţĥé
îḿþļéḿéñţàţîöñ ŕéçîþé. Ţŕàñšļàţàƃļé þŕöšé éḿƃéđđéđ *îñšîđé* àñ öþàǫüé þàýļöàđ
(é.ĝ. `<ḿ:ñöŕ/>` ţéẋţ îñ à Ŵöŕđ éǫüàţîöñ) üšéš ţĥé **šüƃ-šķéļéţöñ** þàţţéŕñ
([Šķéļéţöñ Šţöŕé](/contribute/implementation/engine/skeleton-store)). ▒

▒ Ţĥé šķéļéţöñ ŕéƒ ŕéþļàçéš ţĥé **éñţîŕé éñçöđéđ ṽàļüé** (é.ĝ., îñçļüđîñĝ ĴŠÖÑ
ǫüöţéš), àñđ ţĥé ŵŕîţéŕ îš ŕéšþöñšîƃļé ƒöŕ ŕé-éñçöđîñĝ ţĥé ƃļöçķ ţéẋţ îñ ţĥé
ƒöŕḿàţ'š éñçöđîñĝ (é.ĝ., ĴŠÖÑ šţŕîñĝ éšçàþîñĝ). ▒

### ▒ Ŵŕîţéŕ Ƒàļļƃàçķ Çĥàîñ ▒

▒ Àļŵàýš îḿþļéḿéñţ à ƒàļļƃàçķ ƒöŕ ŵĥéñ ñö šķéļéţöñ šţöŕé îš ŵîŕéđ (é.ĝ., ŵĥéñ
ţĥé ƒöŕḿàţ îš üšéđ öüţšîđé ţĥé ƒļöŵ éẋéçüţöŕ): ▒

1. ▒ **Šķéļéţöñ šţöŕé** — ƃýţé-éẋàçţ ŕéçöñšţŕüçţîöñ (þŕéƒéŕŕéđ) ▒
2. ▒ **Ŕé-þàŕšé öŕîĝîñàļ** — ŕé-ţöķéñîžé ƒŕöḿ šàṽéđ öŕîĝîñàļ çöñţéñţ, šüƃšţîţüţé
   ƃļöçķš ƃý þàţĥ (ĝööđ ƒîđéļîţý, ŕéǫüîŕéš ĥöļđîñĝ öŕîĝîñàļ îñ ḿéḿöŕý) ▒
3. ▒ **Ƃüîļđ ƒŕöḿ ƃļöçķš** — ŕéçöñšţŕüçţ ƒŕöḿ ƃļöçķš àļöñé (ļöŵéšţ ƒîđéļîţý,
   àļŵàýš ŵöŕķš) ▒

▒ Ţĥé ĴŠÖÑ ŵŕîţéŕ îḿþļéḿéñţš àļļ ţĥŕéé. Ţĥé ĤŢḾĻ ŵŕîţéŕ îḿþļéḿéñţš šķéļéţöñ +
ŕé-þàŕšé. Šîḿþļéŕ ƒöŕḿàţš ḿàý öñļý ñééđ šķéļéţöñ + ƃüîļđ-ƒŕöḿ-ƃļöçķš. ▒

## ▒ Ŕéĝîšţŕàţîöñ ▒

▒ Ŕéĝîšţéŕ ţĥé ƒöŕḿàţ îñ `çöŕé/ƒöŕḿàţš/ŕéĝîšţéŕ.ĝö`: ▒

```go
import <name>fmt "github.com/neokapi/neokapi/core/formats/<name>"

// In RegisterAll(reg *registry.FormatRegistry, opts ...RegisterOptions):
// RegisterReader takes (name, factory, FormatSignature, displayName).
reg.RegisterReader("<name>",
    func() format.DataFormatReader { return <name>fmt.NewReader() },
    format.FormatSignature{
        MIMETypes:  []string{"application/<name>"},
        Extensions: []string{".<ext>"},
    }, "<Display Name>")
reg.RegisterWriter("<name>", func() format.DataFormatWriter { return <name>fmt.NewWriter() })
```

▒ Üšé àñ îḿþöŕţ àļîàš îƒ ţĥé þàçķàĝé ñàḿé çöñƒļîçţš ŵîţĥ à Ĝö ƃüîļţîñ (é.ĝ.,
`ẋḿļƒḿţ`, `çšṽƒḿţ`). ▒

## ▒ Ţéšţîñĝ ▒

### ▒ Ţéšţ Þàţţéŕñš ▒

▒ Üšé `ĝîţĥüƃ.çöḿ/šţŕéţçĥŕ/ţéšţîƒý` (àššéŕţ/ŕéǫüîŕé). Ţàƃļé-đŕîṽéñ ţéšţš àŕé
ţĥé šţàñđàŕđ þàţţéŕñ. Þļàçé ţéšţ đàţà îñ à `ţéšţđàţà/` šüƃđîŕéçţöŕý. ▒

#### ▒ Ŕöüñđţŕîþ Ţéšţ (ƃýţé-éẋàçţ) ▒

▒ Ŕéàđ à ƒîļé, þàšš ƃļöçķš ţĥŕöüĝĥ üñçĥàñĝéđ, ŵŕîţé öüţþüţ, çöḿþàŕé: ▒

```go
func roundtrip(t *testing.T, input string) string {
    t.Helper()
    reader := NewReader()
    writer := NewWriter()
    // Open reader with input, drain parts, feed to writer
    // Assert output == input (byte-exact)
}
```

#### ▒ Šķéļéţöñ Ŕöüñđţŕîþ Ţéšţ ▒

▒ Šàḿé àš ŕöüñđţŕîþ ƃüţ ŵîţĥ à ŠķéļéţöñŠţöŕé ŵîŕéđ ƃéţŵééñ ŕéàđéŕ àñđ ŵŕîţéŕ: ▒

```go
func roundtripWithSkeleton(t *testing.T, input string) string {
    t.Helper()
    reader := NewReader()
    writer := NewWriter()
    store, err := format.NewSkeletonStore()
    require.NoError(t, err)
    defer store.Close()
    reader.SetSkeletonStore(store)
    writer.SetSkeletonStore(store)
    // Open reader, drain parts, flush store, feed blocks to writer
    // Assert output == input (byte-exact)
}
```

#### ▒ Ţŕàñšļàţîöñ Ŕöüñđţŕîþ Ţéšţ ▒

▒ Ŕéàđ, ḿöđîƒý ƃļöçķ ţàŕĝéţš, ŵŕîţé, ṽéŕîƒý ţŕàñšļàţéđ ṽàļüéš àþþéàŕ: ▒

```go
func TestTranslation(t *testing.T) {
    // Read input
    // Set target text on blocks
    // Write with skeleton store
    // Verify output has translated values in correct positions
}
```

### ▒ Ŵĥàţ ţö Ţéšţ ▒

- ▒ **Ƃýţé-éẋàçţ ŕöüñđţŕîþ**: Îñþüţ == öüţþüţ ŵĥéñ ñö ţŕàñšļàţîöñ îš àþþļîéđ ▒
- ▒ **Šķéļéţöñ ƃýţé-éẋàçţ ŕöüñđţŕîþ**: Šàḿé, ƃüţ ŵîţĥ ŠķéļéţöñŠţöŕé ŵîŕéđ ▒
- ▒ **Ţŕàñšļàţîöñ ŕöüñđţŕîþ**: Ţŕàñšļàţéđ ţéẋţ àþþéàŕš àţ çöŕŕéçţ þöšîţîöñš ▒
- ▒ **Ŵĥîţéšþàçé/ƒöŕḿàţţîñĝ þŕéšéŕṽàţîöñ**: Îñđéñţàţîöñ, ţŕàîļîñĝ ñéŵļîñéš,
  çöḿḿéñţš (îƒ ţĥé ƒöŕḿàţ šüþþöŕţš ţĥéḿ) ▒
- ▒ **Çöñƒîĝ ṽàŕîàţîöñš**: Éàçĥ çöñƒîĝ öþţîöñ ŵîţĥ ŕéþŕéšéñţàţîṽé îñþüţš ▒
- ▒ **Éđĝé çàšéš**: Éḿþţý ƒîļéš, Üñîçöđé, éšçàþé šéǫüéñçéš, ñéšţéđ šţŕüçţüŕéš ▒
- ▒ **Šüƃƒîļţéŕ ŕöüñđţŕîþ**: Éḿƃéđđéđ çöñţéñţ šüŕṽîṽéš éẋţŕàçţîöñ àñđ
  ŕéçöñšţŕüçţîöñ ▒

### ▒ Þöŕţîñĝ Öķàþî Ţéšţš ▒

▒ Ŵĥéñ ḿîĝŕàţîñĝ àñ Öķàþî ƒîļţéŕ, þöŕţ îţš ţéšţ îñṽéñţöŕý: ▒

1. ▒ Ƒîñđ ţĥé Öķàþî ƒîļţéŕ'š ţéšţ çļàšš (é.ĝ., `ĴŠÖÑƑîļţéŕŢéšţ.ĵàṽà`) ▒
2. ▒ Çöþý ţéšţ îñþüţ ƒîļéš ţö `ţéšţđàţà/` ▒
3. ▒ Çŕéàţé ţàƃļé-đŕîṽéñ ţéšţš ḿàþþîñĝ ţö éàçĥ Öķàþî ţéšţ çàšé ▒
4. ▒ Çöñṽéŕţ Ĵàṽà àššéŕţîöñš ţö Ĝö àššéŕţ/ŕéǫüîŕé çàļļš ▒
5. ▒ Ţĥé Öķàþî ĝöļđ ƒîļéš (`.ĝöļđ` šüƒƒîẋ) ƃéçöḿé éẋþéçţéđ öüţþüţš ▒

▒ Öķàþî ţéšţ þàţţéŕñš ḿàþ ţö ñéöķàþî àš: ▒

| Okapi Pattern                   | neokapi Equivalent                                        |
| ------------------------------- | --------------------------------------------------------- |
| `testRoundTrip(input)`          | `roundtrip(t, input)` / `roundtripWithSkeleton(t, input)` |
| `testExtraction(input, events)` | Read + assert block count, text, properties               |
| `testOutput(input, gold)`       | Read + write + compare against expected output            |
| `testDoubleExtraction(input)`   | Read, write, read again, compare blocks                   |

## ▒ Ŕéƒéŕéñçé Îḿþļéḿéñţàţîöñš ▒

| Format                                      | Best for learning                                        | Key patterns                                                                 |
| ------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------------------------- |
| **JSON** (`core/formats/json/`)             | Key-value formats, regex-based config, subfilter support | Token walking, coalescing skeleton, 3-mode writer fallback, extensive config |
| **HTML** (`core/formats/html/`)             | Markup/streaming formats, tokenizer-based parsing        | Tokenizer dispatch, inline spans, per-block skeletons (`model.Block.Skeleton`) |
| **Plaintext** (`core/formats/plaintext/`)   | Minimal format, starting point                           | Simplest possible reader/writer                                              |
| **XLIFF** (`core/formats/xliff/`)           | Bilingual exchange formats                               | SkeletonStore (coalescing buffer in reader, `writeFromSkeleton` in writer), segment/target handling |
| **Properties** (`core/formats/properties/`) | Line-oriented key-value formats                          | Line parsing, escape handling                                                |

## Checklist

Before submitting a new format:

- [ ] `config.go` — Config with `Reset()`, `Validate()`, `ApplyMap()`
- [ ] `reader.go` — Embeds `BaseFormatReader`, implements `SkeletonStoreEmitter`
- [ ] `writer.go` — Embeds `BaseFormatWriter`, implements `SkeletonStoreConsumer`
- [ ] Reader emits `PartLayerStart` → blocks/data → `PartLayerEnd`
- [ ] Skeleton store: coalescing buffer in reader, `writeFromSkeleton` in writer
- [ ] Writer fallback chain (skeleton → re-parse or build-from-blocks)
- [ ] No write-side regex/byte post-processing of serialized output (see [the no-regex convention](#write-side-post-processing-the-no-regex-convention)); any Okapi-reproduction exception documents the mirrored class/method
- [ ] Registered in `core/formats/register.go`
- [ ] Byte-exact roundtrip tests (with and without skeleton store)
- [ ] Translation roundtrip tests
- [ ] Config option tests
- [ ] `go test ./core/formats/<name>/...` passes
- [ ] `make lint` passes
