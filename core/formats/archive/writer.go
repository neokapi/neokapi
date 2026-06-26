package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for archive containers. It reconstructs the
// archive from the original bytes — copying every entry verbatim except the
// sub-filtered ones, which are re-serialised through their own format writer and
// spliced back in. This keeps non-translatable entries byte-for-byte identical.
type Writer struct {
	format.BaseFormatWriter
	resolver        format.SubfilterResolver
	originalContent []byte
	sourcePath      string
}

var (
	_ format.SubfilterAware        = (*Writer)(nil)
	_ format.OriginalContentSetter = (*Writer)(nil)
	_ format.SourcePathSetter      = (*Writer)(nil)
)

// NewWriter creates an archive writer bound to the resolver used to reconstruct
// sub-filtered entries. RequiresSkeleton is true: the writer cannot synthesise a
// container from the content model alone — it needs the original archive — so
// "archive" is never offered as a cross-format conversion target, and an archive
// entry nested inside another archive passes through rather than recursing.
func NewWriter(resolver format.SubfilterResolver) *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:       "archive",
			RequiresSkeleton: true,
		},
		resolver: resolver,
	}
}

// SetSubfilterResolver overrides the resolver used to reconstruct entries.
func (w *Writer) SetSubfilterResolver(resolver format.SubfilterResolver) {
	w.resolver = resolver
}

// SetOriginalContent provides the original archive bytes for round-trip fidelity.
func (w *Writer) SetOriginalContent(content []byte) { w.originalContent = content }

// SetSourcePath records the path to the original archive so reconstruction can
// re-open it from disk instead of holding a full in-memory copy.
func (w *Writer) SetSourcePath(path string) { w.sourcePath = path }

// Write consumes Parts and writes a reconstructed archive. Sub-filtered child
// layers are collected and re-serialised; everything else is copied from the
// original container.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	replacements := make(map[string][]byte)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.reconstruct(replacements)
			}
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && isArchiveChild(layer) {
					body, written, err := w.writeChildLayer(ctx, layer, parts)
					if err != nil {
						return fmt.Errorf("archive: writing entry %s: %w", layer.Name, err)
					}
					if written {
						replacements[layer.Name] = body
					}
					continue
				}
			}
			// Root layer markers, Data parts, and any stray blocks are ignored —
			// the original archive is the authoritative source for them.
		}
	}
}

// isArchiveChild reports whether a layer is an archive entry envelope.
func isArchiveChild(layer *model.Layer) bool {
	return layer.Properties != nil && layer.Properties["subfilter.source"] == "archive"
}

// writeChildLayer collects the entry's sub-document parts (up to the matching
// PartLayerEnd) and re-serialises them through the entry's own format writer.
// The returned written flag is false when no sub-writer is available, signalling
// the caller to copy the original entry unchanged.
func (w *Writer) writeChildLayer(ctx context.Context, layer *model.Layer, parts <-chan *model.Part) (body []byte, written bool, err error) {
	var childParts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil, false, fmt.Errorf("unexpected end of stream in entry %s", layer.Name)
			}
			if part.Type == model.PartLayerEnd {
				if end, ok := part.Resource.(*model.Layer); ok && end.ID == layer.ID {
					goto collected
				}
			}
			childParts = append(childParts, part)
		}
	}

collected:
	if w.resolver == nil {
		return nil, false, nil
	}
	subWriter, rErr := w.resolver.ResolveWriter(layer.Format)
	if rErr != nil {
		return nil, false, nil
	}
	if sa, ok := subWriter.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(w.resolver)
	}

	var buf bytes.Buffer
	if err := subWriter.SetOutputWriter(&buf); err != nil {
		return nil, false, err
	}
	subWriter.SetLocale(w.Locale)

	childCh := make(chan *model.Part, len(childParts))
	for _, p := range childParts {
		childCh <- p
	}
	close(childCh)

	if err := subWriter.Write(ctx, childCh); err != nil {
		return nil, false, err
	}
	if err := subWriter.Close(); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// reconstruct re-emits the archive, replacing sub-filtered entries.
func (w *Writer) reconstruct(replacements map[string][]byte) error {
	data, err := w.sourceBytes()
	if err != nil {
		return err
	}
	switch detectKind(data) {
	case kindZip:
		return w.reconstructZip(data, replacements)
	case kindTar:
		return w.reconstructTar(w.Output, data, replacements)
	case kindTarGz:
		gzr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("archive: opening source gzip: %w", err)
		}
		tarData, err := io.ReadAll(gzr)
		gzr.Close()
		if err != nil {
			return fmt.Errorf("archive: decompressing source gzip: %w", err)
		}
		gz := gzip.NewWriter(w.Output)
		if err := w.reconstructTar(gz, tarData, replacements); err != nil {
			gz.Close()
			return err
		}
		return gz.Close()
	default:
		return errors.New("archive: unrecognised container for reconstruction")
	}
}

// sourceBytes returns the original archive bytes, preferring the on-disk source
// path (avoiding a second in-memory copy) and falling back to the held bytes.
func (w *Writer) sourceBytes() ([]byte, error) {
	if w.sourcePath != "" {
		data, err := os.ReadFile(w.sourcePath)
		if err != nil {
			return nil, fmt.Errorf("archive: reading source %q: %w", w.sourcePath, err)
		}
		return data, nil
	}
	if w.originalContent != nil {
		return w.originalContent, nil
	}
	return nil, errors.New("archive: original content required for reconstruction")
}

// reconstructZip copies the ZIP, replacing sub-filtered entries. Untouched
// entries are copied with zip.Writer.Copy, preserving their raw compressed bytes
// and metadata exactly.
func (w *Writer) reconstructZip(data []byte, replacements map[string][]byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("archive: opening source zip: %w", err)
	}
	zw := zip.NewWriter(w.Output)
	for _, f := range zr.File {
		repl, ok := replacements[f.Name]
		if !ok {
			if err := zw.Copy(f); err != nil {
				return fmt.Errorf("archive: copying %s: %w", f.Name, err)
			}
			continue
		}
		hdr := f.FileHeader
		hdr.CompressedSize64 = 0
		hdr.UncompressedSize64 = 0
		hdr.CRC32 = 0
		fw, err := zw.CreateHeader(&hdr)
		if err != nil {
			return fmt.Errorf("archive: writing %s: %w", f.Name, err)
		}
		if _, err := fw.Write(repl); err != nil {
			return fmt.Errorf("archive: writing %s: %w", f.Name, err)
		}
	}
	return zw.Close()
}

// reconstructTar copies the TAR, replacing sub-filtered entries (adjusting the
// header size). Entry order, modes, and timestamps are preserved.
func (w *Writer) reconstructTar(out io.Writer, data []byte, replacements map[string][]byte) error {
	tr := tar.NewReader(bytes.NewReader(data))
	tw := tar.NewWriter(out)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("archive: reading source tar: %w", err)
		}
		repl, replace := replacements[hdr.Name]
		outHdr := *hdr
		if replace {
			outHdr.Size = int64(len(repl))
		}
		if err := tw.WriteHeader(&outHdr); err != nil {
			return fmt.Errorf("archive: writing header %s: %w", hdr.Name, err)
		}
		if replace {
			if _, err := tw.Write(repl); err != nil {
				return fmt.Errorf("archive: writing %s: %w", hdr.Name, err)
			}
		} else if outHdr.FileInfo().Mode().IsRegular() {
			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("archive: copying %s: %w", hdr.Name, err)
			}
		}
	}
	return tw.Close()
}
