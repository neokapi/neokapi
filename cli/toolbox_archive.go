package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
)

// streamEntryBlocks reads a single archive entry (addressed by a `container!entry`
// locator) and streams its Blocks — the read backbone for kcat/kgrep/inspect on
// one inner file. Only that entry is read (random-access for ZIP, scan for TAR);
// the whole archive is never loaded.
func (a *App) streamEntryBlocks(ctx context.Context, loc EntryLocator, fn func(index int, b *model.Block) error) (string, error) {
	content, _, err := container.OpenEntry(loc.Archive, loc.Entry)
	if err != nil {
		return "", fmt.Errorf("%s!%s: %w", loc.Archive, loc.Entry, err)
	}
	fmtName := a.resolveFormatName(loc.Entry, content)
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return fmtName, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	defer reader.Close()

	doc := &model.RawDocument{
		URI:          loc.Archive + "!" + loc.Entry,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return fmtName, fmt.Errorf("open %s!%s: %w", loc.Archive, loc.Entry, err)
	}
	index := 0
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return fmtName, res.Error
		}
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b != nil {
			if err := fn(index, b); err != nil {
				return fmtName, err
			}
			index++
		}
	}
	return fmtName, nil
}

// editBytes runs the tool over an in-memory document and returns the rewritten
// bytes. It mirrors editDocument's reader→tool→writer pipeline (skeleton wired
// for byte-faithful round-trip) but on bytes, so it can drive a single archive
// entry. Returns an error if the format has no writer.
func (a *App) editBytes(ctx context.Context, name string, content []byte, t *tool.BaseTool, writeLocale model.LocaleID) ([]byte, error) {
	fmtName := a.resolveFormatName(name, content)
	reader, err := a.FormatReg.NewReader(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("no reader for format %q: %w", fmtName, err)
	}
	writer, err := a.FormatReg.NewWriter(registry.FormatID(fmtName))
	if err != nil {
		return nil, fmt.Errorf("%q is not editable (no writer)", fmtName)
	}
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
			if store, serr := format.NewSkeletonStore(); serr == nil {
				defer store.Close()
				emitter.SetSkeletonStore(store)
				consumer.SetSkeletonStore(store)
			}
		}
	}

	doc := &model.RawDocument{
		URI:          name,
		SourceLocale: model.LocaleID(a.SourceLang),
		Encoding:     a.Encoding,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	var outParts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			reader.Close()
			return nil, res.Error
		}
		if res.Part == nil {
			continue
		}
		p, aerr := t.ApplyContext(ctx, res.Part)
		if aerr != nil {
			reader.Close()
			return nil, aerr
		}
		if p != nil {
			outParts = append(outParts, p)
		}
	}
	reader.Close()

	var buf bytes.Buffer
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, err
	}
	if ocs, ok := writer.(format.OriginalContentSetter); ok {
		ocs.SetOriginalContent(content)
	}
	writer.SetEncoding(a.Encoding)
	writer.SetLocale(writeLocale)

	ch := make(chan *model.Part, len(outParts)+1)
	for _, p := range outParts {
		ch <- p
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		return nil, fmt.Errorf("write %s: %w", name, err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// editArchiveEntry edits a single entry addressed by a `container!entry` locator.
// In place (-i): the edited entry is spliced back into the archive (every other
// member byte-for-byte). Otherwise the edited entry's content is written to out
// (you addressed one file, so you get that file's edited text).
func (a *App) editArchiveEntry(ctx context.Context, loc EntryLocator, t *tool.BaseTool, writeLocale model.LocaleID, inPlace bool, backupSuffix string, out io.Writer) error {
	content, _, err := container.OpenEntry(loc.Archive, loc.Entry)
	if err != nil {
		return fmt.Errorf("%s!%s: %w", loc.Archive, loc.Entry, err)
	}
	edited, err := a.editBytes(ctx, loc.Entry, content, t, writeLocale)
	if err != nil {
		return err
	}
	if !inPlace {
		_, werr := out.Write(edited)
		return werr
	}
	if backupSuffix != "" {
		if err := copyFile(loc.Archive, loc.Archive+backupSuffix); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}
	return writeAtomic(loc.Archive, func(f *os.File) error {
		return container.Transform(loc.Archive, f, func(name string, _ func() ([]byte, error)) ([]byte, bool, error) {
			if sameEntry(name, loc.Entry) {
				return edited, true, nil
			}
			return nil, false, nil
		})
	})
}

// editArchiveAll edits every eligible entry of a whole container (`ksed PAT
// bundle.zip`). In place (-i) it repacks the archive; otherwise the repacked
// archive is streamed to out. Binary/interchange/nested entries pass through.
func (a *App) editArchiveAll(ctx context.Context, path string, t *tool.BaseTool, writeLocale model.LocaleID, inPlace bool, backupSuffix string, out io.Writer) error {
	proc := func(name string, read func() ([]byte, error)) ([]byte, bool, error) {
		if _, eligible := a.containerEntryFormat(name); !eligible {
			return nil, false, nil
		}
		content, rerr := read()
		if rerr != nil {
			return nil, false, rerr
		}
		edited, eerr := a.editBytes(ctx, name, content, t, writeLocale)
		if eerr != nil {
			return nil, false, fmt.Errorf("%s!%s: %w", path, name, eerr)
		}
		return edited, true, nil
	}
	if !inPlace {
		return container.Transform(path, out, proc)
	}
	if backupSuffix != "" {
		if err := copyFile(path, path+backupSuffix); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}
	return writeAtomic(path, func(f *os.File) error {
		return container.Transform(path, f, proc)
	})
}

// sameEntry compares two archive entry paths up to slash normalisation and a
// leading "./".
func sameEntry(a, b string) bool {
	norm := func(s string) string { return strings.TrimPrefix(filepath.ToSlash(s), "./") }
	return norm(a) == norm(b)
}

// copyFile streams src to dst (used for archive backups; never buffers the whole
// file in memory).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
