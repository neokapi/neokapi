---
sidebar_position: 3
title: Implementing a Format
---

# Implementing a New Format

This guide explains how to add a new document format to gokapi.

## Structure

Create a package under `formats/` with three files:

```
formats/myformat/
├── reader.go     # DataFormatReader implementation
├── writer.go     # DataFormatWriter implementation
├── config.go     # Format-specific configuration
└── reader_test.go, writer_test.go  # Tests with testdata/
```

## Reader

The reader must implement `format.DataFormatReader`. Embed `format.BaseFormatReader` for shared behavior:

```go
package myformat

import (
    "context"
    "github.com/gokapi/gokapi/core/format"
    "github.com/gokapi/gokapi/core/model"
)

type Reader struct {
    format.BaseFormatReader
}

func NewReader() *Reader {
    return &Reader{
        BaseFormatReader: format.NewBaseFormatReader(
            "myformat",
            "My Format Filter",
            "application/x-myformat",
            []string{".myf"},
        ),
    }
}

func (r *Reader) Signature() format.FormatSignature {
    return format.FormatSignature{
        MIMETypes:  []string{"application/x-myformat"},
        Extensions: []string{".myf"},
    }
}

func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
    // Parse the document, prepare for streaming
    return nil
}

func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
    ch := make(chan model.PartResult, 64)
    go func() {
        defer close(ch)

        // Emit PartLayerStart
        ch <- model.PartResult{Part: &model.Part{
            Type:     model.PartLayerStart,
            Resource: &model.Layer{ID: "doc1", Format: "myformat"},
        }}

        // Emit Blocks for translatable content
        ch <- model.PartResult{Part: &model.Part{
            Type: model.PartBlock,
            Resource: &model.Block{
                ID:           "b1",
                Translatable: true,
                Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Hello")}},
            },
        }}

        // Emit PartLayerEnd
        ch <- model.PartResult{Part: &model.Part{
            Type:     model.PartLayerEnd,
            Resource: &model.Layer{ID: "doc1", Format: "myformat"},
        }}
    }()
    return ch
}

func (r *Reader) Close() error { return nil }
```

## Writer

The writer must implement `format.DataFormatWriter`. Embed `format.BaseFormatWriter`:

```go
type Writer struct {
    format.BaseFormatWriter
}

func NewWriter() *Writer {
    return &Writer{
        BaseFormatWriter: format.NewBaseFormatWriter("myformat"),
    }
}

func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
    for part := range parts {
        switch part.Type {
        case model.PartBlock:
            block := part.Resource.(*model.Block)
            // Write translated content
        case model.PartData:
            data := part.Resource.(*model.Data)
            // Write structural content
        }
    }
    return nil
}
```

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

Add your format to `formats/register.go`:

```go
func init() {
    registry.DefaultFormatRegistry.RegisterReader("myformat", func() format.DataFormatReader {
        return myformat.NewReader()
    })
    registry.DefaultFormatRegistry.RegisterWriter("myformat", func() format.DataFormatWriter {
        return myformat.NewWriter()
    })
}
```

## Testing

Follow the roundtrip test pattern: read a file, write it back, compare with the original.

```go
func TestRoundTrip(t *testing.T) {
    original, err := os.ReadFile("testdata/sample.myf")
    require.NoError(t, err)

    reader := NewReader()
    err = reader.Open(ctx, testutil.RawDocFromFile("testdata/sample.myf", "en"))
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

See [Testing](/docs/developer/testing) for more patterns.
