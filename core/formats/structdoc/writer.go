// Package structdoc provides the structural conversion writers for JSON and
// YAML: a document (DocLang, Markdown, HTML, docx, …) is serialized as an array
// of structural block records — the same shape as `kapi inspect` — rather than a
// key→value catalog. This is what `kapi convert <doc> --to json|yaml` produces.
//
// It is deliberately distinct from the catalog json/yaml writers
// (core/formats/json, core/formats/yaml), which remain the i18n round-trip
// format (key→value, byte-exact via a skeleton). A structural document has no
// catalog keys, so the catalog writers collapse it onto the empty key; these
// writers capture its structure instead.
package structdoc

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structrec"
)

// Format selects the serialization a Writer emits.
type Format int

const (
	// JSON renders an indented JSON array of records.
	JSON Format = iota
	// YAML renders a YAML sequence of records.
	YAML
)

// FormatIDJSON / FormatIDYAML are the writer ids for the structural conversion
// writers. They are not registered as detectable formats (no extension / MIME);
// the `convert` command routes its json/yaml document targets to them.
const (
	FormatIDJSON = "structdoc-json"
	FormatIDYAML = "structdoc-yaml"
)

// Writer serializes the content model's blocks as structural records.
type Writer struct {
	format.BaseFormatWriter
	fmt Format
}

// NewJSONWriter returns a structural writer emitting a JSON array of records.
func NewJSONWriter() *Writer {
	w := &Writer{fmt: JSON}
	w.FormatName = FormatIDJSON
	return w
}

// NewYAMLWriter returns a structural writer emitting a YAML sequence of records.
func NewYAMLWriter() *Writer {
	w := &Writer{fmt: YAML}
	w.FormatName = FormatIDYAML
	return w
}

// Write consumes Parts and writes the structural record array/sequence. Every
// Block with non-empty text becomes one record, numbered in stream order.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var recs []structrec.Record
	n := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush(recs)
			}
			if part == nil || part.Type != model.PartBlock {
				continue
			}
			block, ok := part.Resource.(*model.Block)
			if !ok || block == nil {
				continue
			}
			runs := w.blockRuns(block)
			if model.RunsText(runs) == "" {
				continue
			}
			n++
			recs = append(recs, structrec.FromBlock(n, block, runs))
		}
	}
}

// blockRuns returns the target runs when a locale is set and present, else the
// source runs — mirroring the catalog writers' projection. structrec.FromBlock
// renders these as placeholder-tagged text so inline codes survive.
func (w *Writer) blockRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetRuns(w.Locale)
	}
	return block.SourceRuns()
}

func (w *Writer) flush(recs []structrec.Record) error {
	var (
		out []byte
		err error
	)
	switch w.fmt {
	case YAML:
		out, err = structrec.MarshalYAML(recs)
	default:
		out, err = structrec.MarshalJSONArray(recs)
	}
	if err != nil {
		return fmt.Errorf("structdoc: marshal: %w", err)
	}
	if _, werr := w.Output.Write(out); werr != nil {
		return fmt.Errorf("structdoc: write: %w", werr)
	}
	return nil
}
