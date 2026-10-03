package host

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/model"
)

// streamEntryBlocks reads a single archive entry (addressed by a `container!entry`
// locator) and streams its Blocks — the read backbone for kcat and kgrep on
// one inner file. Only that entry is read (random-access for ZIP, scan for TAR);
// the whole archive is never loaded.
func (a *App) streamEntryBlocks(ctx context.Context, loc entryLocator, fn func(index int, b *model.Block) error) (string, error) {
	content, _, err := container.OpenEntry(loc.Archive, loc.Entry)
	if err != nil {
		return "", fmt.Errorf("%s!%s: %w", loc.Archive, loc.Entry, err)
	}
	fmtName, reader, err := a.openReader(loc.Entry, bytes.NewReader(content))
	if err != nil {
		return fmtName, fmt.Errorf("%s!%s: %w", loc.Archive, loc.Entry, err)
	}
	defer reader.Close()

	doc := &model.RawDocument{
		URI:          loc.Archive + "!" + loc.Entry,
		SourceLocale: model.LocaleID(a.SourceLocale()),
		Encoding:     a.InputEncoding(),
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

// copyFile streams src to dst, for a backup, without buffering the whole file
// in memory.
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
