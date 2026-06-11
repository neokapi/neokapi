---
sidebar_position: 2
title: Use neokapi from Go
description: A minimal end-to-end Go program that uses the neokapi framework as a library — register the built-in formats, read a file into the streaming content model, run a built-in tool, walk the Blocks, and write bilingual XLIFF.
keywords: [neokapi, go library, quickstart, framework, content model, format reader, format writer, pipeline, golang]
---

# Use neokapi from Go

neokapi is a Go framework first. The [`kapi` CLI and desktop app](/kapi/overview)
and [Kapi React](/react/introduction) are surfaces built on top of it, but the
same content model, format readers and writers, tools, and streaming pipeline
are a Go library you can import directly. This page is the shortest path from
`go get` to a working program that reads a file, transforms it, and writes a
translated file.

If you want the concepts behind the code first, read
[Architecture](/framework/architecture), the
[Content Model](/framework/content-model), and [Tools](/framework/tools). This
page assumes only that you have those open in another tab.

## Install

The framework module is `github.com/neokapi/neokapi`. Add it to your module:

```bash
go get github.com/neokapi/neokapi
```

## A complete program

The program below reads a small JSON localization file, runs the built-in
`pseudo-translate` [tool](/framework/tools) to fill in a target, walks the
resulting [Blocks](/framework/content-model), and writes the stream back out as
bilingual XLIFF 2.x. Every symbol is part of the public framework surface.

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
)

const sourceJSON = `{
  "greeting": "Hello, world",
  "farewell": "Goodbye"
}`

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()

	const (
		sourceLocale = model.LocaleID("en-US")
		targetLocale = model.LocaleID("fr-FR")
		outputPath   = "messages.xlf"
	)

	// 1. Build a format registry and register every built-in reader/writer.
	//    The registry maps a format id (e.g. "json", "xliff2") to a factory.
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// 2. Create a reader for the source format and a writer for the output
	//    format. Here we read JSON and write bilingual XLIFF 2.x.
	reader, err := reg.NewReader("json")
	if err != nil {
		return fmt.Errorf("new json reader: %w", err)
	}
	defer reader.Close()

	writer, err := reg.NewWriter("xliff2")
	if err != nil {
		return fmt.Errorf("new xliff2 writer: %w", err)
	}
	defer writer.Close()

	// 3. Open the source document. A RawDocument carries the bytes, the
	//    source/target locales, and an io.ReadCloser the reader streams from.
	doc := &model.RawDocument{
		URI:          "messages.json",
		SourceLocale: sourceLocale,
		TargetLocale: targetLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(sourceJSON))),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmt.Errorf("open document: %w", err)
	}

	// 4. Pick a built-in tool. pseudo-translate writes a target for each
	//    Block by transforming the source text.
	pseudo := tools.NewPseudoTranslateTool(&tools.PseudoConfig{
		TargetLocale: targetLocale,
		Prefix:       "[",
		Suffix:       "]",
	})

	// 5. Configure the writer's output and target locale.
	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(targetLocale)

	// 6. Wire a streaming pipeline: reader -> tool -> inspect -> writer.
	//    Each stage runs in its own goroutine, connected by buffered channels
	//    of *model.Part, exactly as the executor does internally.
	toolIn := make(chan *model.Part, 64)   // reader -> tool
	writerIn := make(chan *model.Part, 64) // tool   -> inspect
	inspected := make(chan *model.Part, 64) // inspect -> writer

	g, gctx := errgroup.WithContext(ctx)

	// Reader stage: stream Parts out of the format reader. Each PartResult
	// pairs a *Part with an optional error.
	g.Go(func() error {
		defer close(toolIn)
		for result := range reader.Read(gctx) {
			if result.Error != nil {
				return fmt.Errorf("read: %w", result.Error)
			}
			select {
			case toolIn <- result.Part:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	// Tool stage: a tool's Process consumes Parts from its input channel,
	// transforms the ones it handles (here: Blocks), and relays the rest.
	g.Go(func() error {
		defer close(writerIn)
		return pseudo.Process(gctx, toolIn, writerIn)
	})

	// Inspection stage: walk the content model (Blocks, their source text,
	// and the target the tool just wrote) before handing Parts to the writer.
	g.Go(func() error {
		defer close(inspected)
		for part := range writerIn {
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					fmt.Printf("block %-10s source=%q target=%q\n",
						block.ID, block.SourceText(), block.TargetText(targetLocale))
				}
			}
			select {
			case inspected <- part:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	// Writer stage: reconstruct the document from the Part stream.
	g.Go(func() error {
		return writer.Write(gctx, inspected)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	fmt.Fprintf(os.Stdout, "wrote %s\n", outputPath)
	return nil
}
```

Running it prints what each Block looks like after the tool and writes
`messages.xlf`:

```text
block tu1        source="Hello, world" target="[Ĥéļļö, ŵöŕļđ]"
block tu2        source="Goodbye" target="[Ĝööđƃýé]"
wrote messages.xlf
```

```xml title="messages.xlf"
<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.2" version="2.2" srcLang="en-US" trgLang="fr-FR">
  <file id="messages.json">
    <unit id="tu1" name="greeting">
      <segment>
        <source>Hello, world</source>
        <target>[Ĥéļļö, ŵöŕļđ]</target>
      </segment>
    </unit>
    <unit id="tu2" name="farewell">
      <segment>
        <source>Goodbye</source>
        <target>[Ĝööđƃýé]</target>
      </segment>
    </unit>
  </file>
</xliff>
```

This exact program lives in the repository under
[`examples/go-quickstart/`](https://github.com/neokapi/neokapi/tree/main/examples/go-quickstart)
and is built as part of the framework module.

## What each piece is

The program touches every core concept the rest of this section covers in
depth.

- **The registry** ([`core/registry`](/framework/formats)) maps a format id to a
  reader and writer factory. `formats.RegisterAll` populates it with every
  built-in [format](/framework/formats); `NewReader` / `NewWriter` hand back a
  fresh instance. The registry also detects a format from a path or MIME type
  when you don't name one explicitly.
- **The reader** turns the source file into a stream of
  [Parts](/framework/content-model). `Open` binds a `RawDocument`; `Read` returns
  a channel of `PartResult` (a `*Part` plus an optional error). A monolingual
  format like JSON emits one [Block](/framework/content-model) per translatable
  value, surrounded by layer-start / layer-end Parts that carry the document
  structure.
- **The content model** ([`core/model`](/framework/content-model)) is what flows
  on the channels. A `Part` carries a type discriminator and a `Resource`; a
  `Block` is the translatable unit, with a flat `Source []Run`, a map of
  variant-keyed `Target`s, and stand-off overlays. `block.SourceText()` projects
  the source runs to plain text; `block.SetTargetText(locale, …)` and
  `block.TargetText(locale)` read and write a target. Inline markup (HTML tags,
  ICU placeholders) lives in `Run`s, not in the text, so a tool can edit words
  without disturbing the markup.
- **The tool** ([`core/tools`](/framework/tools)) is a stage that satisfies the
  `Process(ctx, in, out)` contract: it consumes Parts, transforms the ones it
  handles, and relays the rest. `pseudo-translate` writes a target for each
  Block; swap it for `word-count`, `case-transform`, or any other built-in, or
  chain several together.
- **The pipeline** ([`core/flow`](/framework/pipeline)) is the concurrency: each
  stage is a goroutine, the stages are joined by buffered channels of Parts, and
  an `errgroup` propagates the first error and cancels the rest. The example
  wires the chain by hand to show the mechanics; for batches of files there is a
  higher-level executor (below).

## Running flows instead of wiring channels

Wiring the channels by hand, as above, is the clearest way to see how Parts
move — but you rarely need to. For a single file, `flow.NewFileRunner` runs the
whole read → process → write pipeline (format detection, reader/writer creation,
tool chain, output) for you:

```go
runner := flow.NewFileRunner(flow.FileRunnerConfig{
	FormatReg:    reg,
	SourceLocale: "en-US",
})
err := runner.RunFile(ctx, "pseudo", []tool.Tool{pseudo},
	"messages.json", "messages.out.json", "fr-FR")
```

For batches of files run in parallel, `flow.NewExecutor` takes a built flow and a
slice of items and runs them concurrently, bounded by `MaxConcurrency`. See
[Pipeline](/framework/pipeline) for the executor options and the concurrency
model, and [Flows](/framework/flows) for composing named tool chains.

## Where to go next

- [Content Model](/framework/content-model) — Parts, Blocks, Runs, Targets, and
  overlays in depth.
- [Formats](/framework/formats) — the built-in readers and writers, detection,
  and the generated [Format Reference](/formats).
- [Tools](/framework/tools) — the tool interface, `BaseTool` dispatch, and the
  generated [Tool Reference](/tools).
- [Pipeline](/framework/pipeline) and [Flows](/framework/flows) — the executor,
  channels, backpressure, and named compositions.
- [Implementing a Tool](/contribute/tools) and
  [Implementing a Format](/contribute/formats) — extend the framework with your
  own stages and readers/writers.
```

