package formats

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// TestSourceLocaleSpellingKeepsTheSource asserts that an untouched write
// emits the same document whichever spelling of a source locale the blocks
// carry, including a tag that is not well-formed BCP-47 but that the locale
// gate accepts. A reader copies the document's source locale onto its blocks,
// and a project home gives the project's source locale to every block a
// reader left with no language, so the tag reaches every writer. A writer
// emits the block's source where it writes no target, and a source looked up
// by a key the tag did not reach is written empty without an error.
func TestSourceLocaleSpellingKeepsTheSource(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range escapeSweepFormats() {
		t.Run(tc.id, func(t *testing.T) {
			src := []byte(tc.source)
			want := sourceLocaleRoundTrip(t, reg, tc.id, src, "en")
			for _, loc := range []model.LocaleID{"en-u-en-en-t-nu", "AA-u-01-01-u-00-00", "en-u-en-en-u-ca-ca"} {
				if got := sourceLocaleRoundTrip(t, reg, tc.id, src, loc); !bytes.Equal(got, want) {
					t.Errorf("%s read in %q:\nwant %q\ngot  %q", tc.id, loc, want, got)
				}
			}
		})
	}
}

// sourceLocaleRoundTrip reads src as a document in loc, gives loc to every
// block the reader left with no language, as a project home does, and writes
// the parts back through a wired skeleton.
func sourceLocaleRoundTrip(t *testing.T, reg *registry.FormatRegistry, id string, src []byte, loc model.LocaleID) []byte {
	t.Helper()
	reader, err := reg.NewReader(registry.FormatID(id))
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	writer, err := reg.NewWriter(registry.FormatID(id))
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	store, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		t.Fatalf("wire skeleton: %v", err)
	}
	if store != nil {
		defer store.Close()
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{
		SourceLocale: loc,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(src)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		t.Fatalf("open: %v", err)
	}
	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			t.Fatalf("read: %v", pr.Error)
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b.SourceLocale == "" {
			b.SourceLocale = loc
		}
		parts = append(parts, pr.Part)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	out, err := spec.WriteParts(writer, parts, src)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return out
}
