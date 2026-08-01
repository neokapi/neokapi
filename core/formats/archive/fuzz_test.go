package archive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// There is deliberately no FuzzRoundTripArchive. The archive format is
// inspection-only — `TestArchiveIsReadOnly` asserts the registry has no archive
// writer, and the package doc states containers are handled through the
// container binding rather than by rewriting the archive. A round-trip target
// here would have nothing to write with, so the honest coverage is the read
// contract alone.

// fuzzRegistry is built once and shared. Registering every format per iteration
// would dominate the run, and the registry is only read from during a parse.
var fuzzRegistry = sync.OnceValue(func() *registry.FormatRegistry {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	return reg
})

// fuzzReadArchive drives the archive reader over input without ever calling
// t.Fatal, so it is safe on arbitrary fuzz input.
//
// The resolver is the real FormatRegistry, not a stub, and that is the point:
// archive dispatches each entry to whichever reader the detector picks, so this
// target routes arbitrary bytes into every registered format's reader through
// the same path a user's container takes.
func fuzzReadArchive(ctx context.Context, uri string, input []byte) (hadErr bool) {
	reader, err := fuzzRegistry().NewReader("archive")
	if err != nil {
		return true
	}
	doc := &model.RawDocument{URI: uri, SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(bytes.NewReader(input))}
	if err := reader.Open(ctx, doc); err != nil {
		return true
	}
	defer reader.Close()
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			hadErr = true
		}
	}
	return hadErr
}

func fuzzMakeZip(entries map[string][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

func fuzzMakeTarGz(entries map[string][]byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			return nil
		}
		if _, err := tw.Write(body); err != nil {
			return nil
		}
	}
	if tw.Close() != nil || gz.Close() != nil {
		return nil
	}
	return buf.Bytes()
}

// FuzzReadArchive asserts the container reader never panics and always
// terminates on arbitrary input.
//
// The seeds are containers that are structurally damaged but still PARSEABLE —
// entries whose declared name and actual content disagree, entries that climb
// out of the container, entries that are themselves containers. That region
// matters more here than malformed framing: archive's hardening is that an
// unreadable entry falls back to opaque Data rather than aborting the archive,
// so the interesting failures are the ones where an entry is readable but
// wrong, not where the zip header is corrupt.
func FuzzReadArchive(f *testing.F) {
	seeds := [][]byte{
		fuzzMakeZip(map[string][]byte{"a.json": []byte(`{"k":"v"}`)}),
		fuzzMakeZip(map[string][]byte{"a.json": []byte(`{"k":"v"}`), "b.po": []byte("msgid \"x\"\nmsgstr \"y\"\n")}),
		// Extension and content disagree: a .json entry holding XML, so the
		// detector and the extension point at different readers.
		fuzzMakeZip(map[string][]byte{"a.json": []byte(`<r><s>x</s></r>`)}),
		// An entry with no extension at all — detection is content-only.
		fuzzMakeZip(map[string][]byte{"noext": []byte(`{"k":"v"}`)}),
		// An entry name that climbs out of the container.
		fuzzMakeZip(map[string][]byte{"../escape.json": []byte(`{"k":"v"}`)}),
		// An absolute entry name.
		fuzzMakeZip(map[string][]byte{"/abs.json": []byte(`{"k":"v"}`)}),
		// A nested container: an archive inside the archive.
		fuzzMakeZip(map[string][]byte{"inner.zip": fuzzMakeZip(map[string][]byte{"a.json": []byte(`{"k":"v"}`)})}),
		// An empty entry, and a directory entry.
		fuzzMakeZip(map[string][]byte{"empty.json": {}, "dir/": {}}),
		// Same shapes over the other two containers the reader accepts.
		fuzzMakeTarGz(map[string][]byte{"a.json": []byte(`{"k":"v"}`)}),
		fuzzMakeTarGz(map[string][]byte{"../escape.json": []byte(`{"k":"v"}`)}),
	}
	for _, s := range seeds {
		if s != nil {
			f.Add(s)
		}
	}
	f.Add([]byte("not an archive"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = fuzzReadArchive(t.Context(), "test://input.zip", data)
	})
}
