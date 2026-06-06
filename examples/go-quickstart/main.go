// Command go-quickstart is a minimal end-to-end example of using neokapi as a
// Go library: register the built-in formats, read a source file into the
// streaming content model, run a built-in tool, walk the resulting Blocks, and
// write the stream back out as bilingual XLIFF 2.x.
//
// Run it:
//
//	go run ./examples/go-quickstart
//
// It writes ./messages.xlf next to the working directory.
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

// A small JSON localization file to process. In a real program this would be a
// file on disk; here we keep it inline so the example is self-contained.
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
	//    Block by transforming the source text — useful for testing
	//    localization readiness without a real translation engine.
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
	toolIn := make(chan *model.Part, 64)    // reader -> tool
	writerIn := make(chan *model.Part, 64)  // tool   -> inspect
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
