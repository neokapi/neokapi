package filehome

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// source is where a document's bytes are: a file, or a member of an archive.
// data, when set, holds the bytes instead: the document as a structural edit
// left it, before anything is written.
type source struct {
	path  string
	entry string
	data  []byte
}

func (s source) String() string {
	if s.entry != "" {
		return filepath.Base(s.path) + "!" + s.entry
	}
	return filepath.Base(s.path)
}

// exists reports whether the source's file is there.
func (s source) exists() bool {
	if s.data != nil {
		return true
	}
	_, err := os.Stat(s.path)
	return err == nil
}

// entryBytes reads an archive member.
func (s source) entryBytes() ([]byte, error) {
	if s.data != nil {
		return s.data, nil
	}
	data, _, err := container.OpenEntry(s.path, s.entry)
	return data, err
}

// with returns the source holding data in place of its bytes.
func (s source) with(data []byte) source {
	s.data = data
	return s
}

// pass is one read of a document through its format, with or without a write.
type pass struct {
	src    source
	format Binding
	locale model.LocaleID
	// target is the language of the translation a bilingual file holds, for
	// a reader that has to be told it; empty for any other file.
	target model.LocaleID
	// encoding is the encoding the reader reads in.
	encoding string
	// fn sees every block, in document order. Returning change.ErrStop ends a
	// read early.
	fn func(*model.Block) error
	// out receives the document as the writer writes it; nil reads only.
	out io.Writer
	// writeLocale is the locale the writer writes: empty for the document's
	// own edition, a target locale to materialize that edition.
	writeLocale model.LocaleID
	// writerSource, for a write, is the file the writer may re-read the
	// original from; empty is the source itself.
	writerSource source
}

// run makes the pass. A read with a writer available wires the writer's
// skeleton store, because some readers model a document differently with one
// (the HTML reader numbers attribute blocks after their paragraph), and a
// read that addresses an edit must see the blocks the write sees.
func (p pass) run(ctx context.Context) (err error) {
	reader, err := p.format.NewReader()
	if err != nil {
		return &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: fmt.Sprintf("no reader for %s: %v", p.format.Name, err)}
	}
	writer, werr := p.format.NewWriter()
	if werr != nil {
		writer = nil
		if p.out != nil {
			reader.Close()
			return &change.Error{Code: change.CodeUnsupported, Capability: "write",
				Message: fmt.Sprintf("the %s format has no writer, so %s cannot be edited", p.format.Name, p.src)}
		}
	}

	ws := p.writerSource
	if ws.path == "" {
		ws = p.src
	}
	// The writer of a same-format round trip rebuilds the document from the
	// original: by path when it can re-read the file, from its bytes when it
	// cannot.
	var original []byte
	if p.out != nil {
		if sps, ok := writer.(format.SourcePathSetter); ok && ws.entry == "" && ws.data == nil {
			abs, aerr := filepath.Abs(ws.path)
			if aerr != nil {
				reader.Close()
				return aerr
			}
			sps.SetSourcePath(abs)
		} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
			if original, err = readAll(ws); err != nil {
				reader.Close()
				return err
			}
			ocs.SetOriginalContent(original)
		}
	}

	doc := &model.RawDocument{URI: p.src.String(), SourceLocale: p.locale, TargetLocale: p.target, Encoding: p.encoding}
	var file *os.File
	switch {
	case p.src.data != nil:
		setBytes(doc, p.src.data)
	case p.src.entry != "":
		data, eerr := p.src.entryBytes()
		if eerr != nil {
			reader.Close()
			return eerr
		}
		setBytes(doc, data)
	case original != nil && ws.data == nil && ws.path == p.src.path && ws.entry == p.src.entry:
		setBytes(doc, original)
	default:
		f, oerr := os.Open(p.src.path)
		if oerr != nil {
			reader.Close()
			return oerr
		}
		file = f
		defer file.Close()
		info, serr := f.Stat()
		if serr != nil {
			reader.Close()
			return serr
		}
		// The handle belongs to the pass, not the reader: a reader that closed
		// it would pull the file out from under its own random-access view.
		doc.Reader = io.NopCloser(safeio.DefaultBudget().Reader(f))
		doc.ReaderAt, doc.Size = f, info.Size()
	}

	if p.out == nil {
		return p.read(ctx, reader, writer, doc)
	}
	// The binding configured the writer's encoding with the rest of its
	// output options; only the locale it writes is the pass's.
	writer.SetLocale(p.writeLocale)
	if format.IsStreamingReader(reader) && format.IsStreamingWriter(writer) && original == nil {
		return p.stream(ctx, reader, writer, doc)
	}
	return p.buffered(ctx, reader, writer, doc)
}

func setBytes(doc *model.RawDocument, data []byte) {
	doc.Reader = io.NopCloser(safeio.DefaultBudget().Reader(bytes.NewReader(data)))
	doc.ReaderAt, doc.Size = bytes.NewReader(data), int64(len(data))
}

// readAll reads a source whole, within the byte budget.
func readAll(s source) ([]byte, error) {
	if s.data != nil {
		return s.data, nil
	}
	if s.entry != "" {
		return s.entryBytes()
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(safeio.DefaultBudget().Reader(f))
}

// read streams the document's blocks to fn. The skeleton store wired for the
// writer the read does not use keeps nothing (format.NewWiredReadSkeleton).
func (p pass) read(ctx context.Context, reader format.DataFormatReader, writer format.DataFormatWriter, doc *model.RawDocument) error {
	var store *format.SkeletonStore
	if writer != nil {
		store = format.NewWiredReadSkeleton(reader, writer)
	}
	// The reader closes before the skeleton store it writes into.
	defer func() {
		reader.Close()
		if store != nil {
			_ = store.Close()
		}
	}()
	if err := format.OpenDocument(ctx, reader, doc); err != nil {
		return fmt.Errorf("open %s: %w", p.src, err)
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := reader.Read(rctx)
	defer drain(results, cancel)
	for res := range results {
		if res.Error != nil {
			return res.Error
		}
		b := blockOf(res.Part)
		if b == nil || p.fn == nil {
			continue
		}
		if err := p.visit(b); err != nil {
			if errors.Is(err, change.ErrStop) {
				return nil
			}
			return err
		}
	}
	return ctx.Err()
}

// stream reads and writes concurrently through a streaming skeleton store, so
// neither the input nor the block stream is held whole.
func (p pass) stream(ctx context.Context, reader format.DataFormatReader, writer format.DataFormatWriter, doc *model.RawDocument) error {
	store := format.NewWiredStreamingSkeleton(reader, writer)
	if err := format.OpenDocument(ctx, reader, doc); err != nil {
		reader.Close()
		if store != nil {
			_ = store.Close()
		}
		return fmt.Errorf("open %s: %w", p.src, err)
	}
	if err := writer.SetOutputWriter(p.out); err != nil {
		reader.Close()
		return err
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	parts := make(chan *model.Part, 64)
	var readErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		results := reader.Read(rctx)
		defer func() {
			if rec := recover(); rec != nil {
				readErr = fmt.Errorf("read %s: panicked: %v\n%s", p.src, rec, debug.Stack())
			}
			close(parts)
			if store != nil {
				store.CloseWrite()
			}
			drain(results, cancel)
			reader.Close()
		}()
		for res := range results {
			if res.Error != nil {
				readErr = res.Error
				return
			}
			if res.Part == nil {
				continue
			}
			if b := blockOf(res.Part); b != nil && p.fn != nil {
				if err := p.visit(b); err != nil {
					readErr = err
					return
				}
			}
			select {
			case parts <- res.Part:
			case <-rctx.Done():
				return
			}
		}
	}()
	writeErr := writer.Write(ctx, parts)
	if writeErr != nil {
		cancel()
		for range parts { //nolint:revive // drain so the reader goroutine can finish
		}
	}
	<-done
	closeErr := writer.Close()
	if store != nil {
		_ = store.Close()
	}
	return firstErr(readErr, writeErr, closeErr, ctx.Err())
}

// buffered reads every part, then writes them: the path for a reader or
// writer that cannot stream.
func (p pass) buffered(ctx context.Context, reader format.DataFormatReader, writer format.DataFormatWriter, doc *model.RawDocument) error {
	store, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		reader.Close()
		return fmt.Errorf("cannot edit %s: %w", p.src, err)
	}
	if store != nil {
		defer store.Close()
	}
	if err := format.OpenDocument(ctx, reader, doc); err != nil {
		reader.Close()
		return fmt.Errorf("open %s: %w", p.src, err)
	}
	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return res.Error
		}
		if res.Part == nil {
			continue
		}
		if b := blockOf(res.Part); b != nil && p.fn != nil {
			if err := p.visit(b); err != nil {
				reader.Close()
				return err
			}
		}
		parts = append(parts, res.Part)
	}
	reader.Close()
	if err := writer.SetOutputWriter(p.out); err != nil {
		return err
	}
	ch := make(chan *model.Part, len(parts)+1)
	for _, part := range parts {
		ch <- part
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		return fmt.Errorf("write %s: %w", p.src, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close %s: %w", p.src, err)
	}
	return nil
}

// visit hands fn a block the reader produced. A block the reader left with
// no language is given the document's, as every block of a project is read
// in its source language, so its own edition answers to that language as
// well as to the empty edition key.
func (p pass) visit(b *model.Block) error {
	if b.SourceLocale == "" {
		b.SourceLocale = p.locale
	}
	return p.fn(b)
}

// blockOf is the block a part carries, or nil.
func blockOf(part *model.Part) *model.Block {
	if part == nil {
		return nil
	}
	b, _ := part.Resource.(*model.Block)
	return b
}

// drain cancels a reader's read and empties its channel, so the reader's
// goroutine can finish.
func drain(results <-chan model.PartResult, cancel context.CancelFunc) {
	cancel()
	for range results { //nolint:revive // drain
	}
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
