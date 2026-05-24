---
sidebar_position: 15
title: Implementing Formats
description: Implementation note for AD-005 — step-by-step instructions for writing new neokapi format readers and writers, or migrating Okapi Java filters, including terminology mapping from Okapi to neokapi concepts.
keywords: [implementing formats, format reader, format writer, Okapi migration, DataFormatReader, neokapi]
---

# Implementing Formats

Step-by-step guide for implementing new neokapi format readers/writers or
migrating existing Okapi filters. Parent AD:
[AD-005](/contribute/architecture/005-format-system).

## Terminology Mapping from Okapi

| Okapi (Java)                      | neokapi (Go)               |
| --------------------------------- | -------------------------- |
| Filter                            | DataFormat (Reader/Writer) |
| Step                              | Tool                       |
| Pipeline                          | Flow                       |
| PipelineDriver                    | Executor                   |
| Event                             | Part                       |
| TextUnit                          | Block                      |
| TextFragment                      | Fragment                   |
| Code                              | Span                       |
| StartDocument / EndDocument       | Layer (root)               |
| StartSubDocument / StartSubFilter | Child Layer                |

## File Structure

Create a package under `core/formats/<name>/` with three files:

```
core/formats/<name>/
├── config.go       # Config struct with Reset(), Validate(), ApplyMap()
├── reader.go       # DataFormatReader implementation
├── writer.go       # DataFormatWriter implementation
├── reader_test.go  # Reader tests
├── writer_test.go  # Writer or roundtrip tests
└── testdata/       # Test input files
```

## Config

Every format has a `Config` struct implementing `format.DataFormatConfig`:

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

**Reference**: `core/formats/json/config.go` (complex config with regex caches),
`core/formats/plaintext/config.go` (minimal config).

## Reader

Embed `format.BaseFormatReader` and implement `format.DataFormatReader`:

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

### Read Method Pattern

The `Read` method opens a goroutine that sends `model.PartResult` values on a
channel. It must emit `PartLayerStart` first, then blocks/data, then
`PartLayerEnd`:

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

### Block Creation

```go
block := model.NewBlock(blockID, sourceText)
block.Name = blockName
block.Properties["<format>.keypath"] = keyPath // format-specific metadata

ch <- model.PartResult{Part: &model.Part{
    Type:     model.PartBlock,
    Resource: block,
}}
```

### Subfilter Support

If the format can contain embedded content (e.g., HTML strings inside JSON),
implement `format.SubfilterAware`:

```go
var _ format.SubfilterAware = (*Reader)(nil)

func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
    r.resolver = resolver
}
```

When encountering embedded content, create a child layer:

```go
subReader, err := r.resolver.ResolveReader(subFormatName)
// Open subReader with the embedded content as a RawDocument
// Emit PartLayerStart for child, forward sub-parts, emit PartLayerEnd
```

## Writer

Embed `format.BaseFormatWriter` and implement `format.DataFormatWriter`:

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

### Write Method Pattern

The writer collects all blocks from the channel, then reconstructs the
document. It should support a fallback chain:

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

### Skeleton Store Reconstruction

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

### Write-side post-processing: the no-regex convention

A format writer MUST NOT regex- or byte-rewrite its already-serialized
output to compensate for a modeling gap. That post-processing is brittle
(it pattern-matches serialized markup), couples to emission ordering, and
hides the fact that the model is missing a primitive. The unified pattern
that every writer follows instead:

1. **Skeleton-store emission.** The reader stores non-translatable bytes
   verbatim; the writer replays them and splices only translated slots, so
   the writer introduces no structural divergence to "fix up" afterward.
2. **Symmetric compare-time canonicalization.** Cosmetic differences between
   two writers (attribute order, namespace decls, self-closing vs
   open/close, insignificant whitespace) are cancelled by the shared
   `XMLCanonical` normalizer (`cli/parity/roundtrip/normalizers.go`), applied
   to **both** `got` and `ref`. Reaching the `canon` tier — not `byte` — is
   the norm and is sufficient.
3. **Structural merges as canonicalization, not write-side rewriting.**
   "Merge adjacent equivalent elements" belongs in the normalizer (applied
   symmetrically to both sides), not in the writer (applied to one side via
   regex). idml's `MergeAdjacentCSRs` is the template.

Per-value **escaping of text content before its first emission** (backslash
/ quote / newline / delimiter encoding) is not post-processing and is fine.

**The one sanctioned exception** is faithfully reproducing a transform that
Okapi *itself* performs on serialized bytes — e.g. openxml's
`AllowWordStyleOptimisation` (WSO) style synthesis and `RunProperties.minified()`
toggle collapse. These are reproduction, not compensation: the reference
output already contains them, so they cannot be moved to a symmetric
normalizer. When a writer keeps such a transform it MUST document the Okapi
class/method it mirrors, so a reader can tell reproduction from compensation.

Formats already converted to this convention: html (DOM `setAttr` instead of
lang regex), the regex format (prefix/capture/suffix assembly), wiki (stored
header level), and openxml (structural `<w:r>` envelope emission + byte-splice
run merges replacing the post-serialization fuse regexes). When a structural
fix is genuinely impractical, prefer a documented `div`-tier divergence or a
tracked follow-up issue over a new write-side regex.

## Skeleton Store Integration

The SkeletonStore (`core/format/skeleton.go`) enables byte-exact roundtrip of
documents. The reader writes skeleton entries as it parses; the writer reads
them to reconstruct the output. Tools in between only see blocks — they never
touch the skeleton.

See [Skeleton Store](/contribute/notes-internal/skeleton-store) for binary format and API
details.

### Reader Side: Coalescing Buffer Pattern

Do NOT write one skeleton entry per token. Use a `bytes.Buffer` to accumulate
non-translatable text between block references, then flush before each ref:

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

This reduces skeleton entries from ~N (one per token) to ~2B+1 (where B is the
number of translatable blocks). For example, a JSON file with 50 strings
produces ~101 entries instead of ~10,000.

### What Goes Where

| Content                                           | Skeleton             | Block       |
| ------------------------------------------------- | -------------------- | ----------- |
| Structural tokens (`\{`, `}`, `[`, `]`, `,`, `:`) | Text                 | --          |
| Whitespace, comments, formatting                  | Text                 | --          |
| Non-translatable values                           | Text                 | --          |
| Object keys                                       | Text                 | --          |
| Translatable string values                        | Ref (block ID)       | Source text |
| Embedded/subfiltered content                      | Ref (`layer:<path>`) | Child layer |

The skeleton ref replaces the **entire encoded value** (e.g., including JSON
quotes), and the writer is responsible for re-encoding the block text in the
format's encoding (e.g., JSON string escaping).

### Writer Fallback Chain

Always implement a fallback for when no skeleton store is wired (e.g., when
the format is used outside the flow executor):

1. **Skeleton store** — byte-exact reconstruction (preferred)
2. **Re-parse original** — re-tokenize from saved original content, substitute
   blocks by path (good fidelity, requires holding original in memory)
3. **Build from blocks** — reconstruct from blocks alone (lowest fidelity,
   always works)

The JSON writer implements all three. The HTML writer implements skeleton +
re-parse. Simpler formats may only need skeleton + build-from-blocks.

## Registration

Register the format in `core/formats/register.go`:

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

Use an import alias if the package name conflicts with a Go builtin (e.g.,
`xmlfmt`, `csvfmt`).

## Testing

### Test Patterns

Use `github.com/stretchr/testify` (assert/require). Table-driven tests are
the standard pattern. Place test data in a `testdata/` subdirectory.

#### Roundtrip Test (byte-exact)

Read a file, pass blocks through unchanged, write output, compare:

```go
func roundtrip(t *testing.T, input string) string {
    t.Helper()
    reader := NewReader()
    writer := NewWriter()
    // Open reader with input, drain parts, feed to writer
    // Assert output == input (byte-exact)
}
```

#### Skeleton Roundtrip Test

Same as roundtrip but with a SkeletonStore wired between reader and writer:

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

#### Translation Roundtrip Test

Read, modify block targets, write, verify translated values appear:

```go
func TestTranslation(t *testing.T) {
    // Read input
    // Set target text on blocks
    // Write with skeleton store
    // Verify output has translated values in correct positions
}
```

### What to Test

- **Byte-exact roundtrip**: Input == output when no translation is applied
- **Skeleton byte-exact roundtrip**: Same, but with SkeletonStore wired
- **Translation roundtrip**: Translated text appears at correct positions
- **Whitespace/formatting preservation**: Indentation, trailing newlines,
  comments (if the format supports them)
- **Config variations**: Each config option with representative inputs
- **Edge cases**: Empty files, Unicode, escape sequences, nested structures
- **Subfilter roundtrip**: Embedded content survives extraction and
  reconstruction

### Porting Okapi Tests

When migrating an Okapi filter, port its test inventory:

1. Find the Okapi filter's test class (e.g., `JSONFilterTest.java`)
2. Copy test input files to `testdata/`
3. Create table-driven tests mapping to each Okapi test case
4. Convert Java assertions to Go assert/require calls
5. The Okapi gold files (`.gold` suffix) become expected outputs

Okapi test patterns map to neokapi as:

| Okapi Pattern                   | neokapi Equivalent                                        |
| ------------------------------- | --------------------------------------------------------- |
| `testRoundTrip(input)`          | `roundtrip(t, input)` / `roundtripWithSkeleton(t, input)` |
| `testExtraction(input, events)` | Read + assert block count, text, properties               |
| `testOutput(input, gold)`       | Read + write + compare against expected output            |
| `testDoubleExtraction(input)`   | Read, write, read again, compare blocks                   |

## Reference Implementations

| Format                                      | Best for learning                                        | Key patterns                                                                 |
| ------------------------------------------- | -------------------------------------------------------- | ---------------------------------------------------------------------------- |
| **JSON** (`core/formats/json/`)             | Key-value formats, regex-based config, subfilter support | Token walking, coalescing skeleton, 3-mode writer fallback, extensive config |
| **HTML** (`core/formats/html/`)             | Markup/streaming formats, tokenizer-based parsing        | Tokenizer dispatch, inline spans, skeleton store                             |
| **Plaintext** (`core/formats/plaintext/`)   | Minimal format, starting point                           | Simplest possible reader/writer                                              |
| **XLIFF** (`core/formats/xliff/`)           | Bilingual exchange formats                               | Per-block skeletons (not SkeletonStore), segment handling                    |
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
