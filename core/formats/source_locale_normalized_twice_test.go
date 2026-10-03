package formats_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// sourceLocaleNormalizedTwice is a source locale NormalizeLocale does not
// settle in one step: it gives "aa-u-00-u-00-00", and that gives
// "aa-u-00-u-00". A writer reads the source as the block's authoritative
// edition, whose key carries the once-normalized locale, so that key has to
// reach the source although normalizing it again changes it.
const sourceLocaleNormalizedTwice model.LocaleID = "AA-u-00-00-u-00-00"

// convertWith reads input as from and writes every part through to, wired to
// a skeleton store when the two are one format and withSkeleton is set. stamp,
// when set, sees every block once the reader has finished.
func convertWith(t *testing.T, from, to registry.FormatID, input []byte, withSkeleton bool, stamp func(*model.Block)) string {
	t.Helper()
	ctx := t.Context()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader(from)
	require.NoError(t, err)
	writer, err := reg.NewWriter(to)
	require.NoError(t, err)
	if withSkeleton {
		store, err := format.NewWiredSkeleton(reader, writer)
		require.NoError(t, err)
		if store != nil {
			defer store.Close()
		}
	}

	doc := &model.RawDocument{
		URI:          "doc." + string(from),
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(input)),
	}
	require.NoError(t, reader.Open(ctx, doc))
	var parts []*model.Part
	for res := range reader.Read(ctx) {
		require.NoError(t, res.Error)
		if res.Part != nil {
			parts = append(parts, res.Part)
		}
	}
	require.NoError(t, reader.Close())
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && stamp != nil {
			stamp(b)
		}
	}

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	if ocs, ok := writer.(format.OriginalContentSetter); ok && from == to {
		ocs.SetOriginalContent(input)
	}
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, writer.Write(ctx, ch))
	require.NoError(t, writer.Close())
	return buf.String()
}

// Every writer that reads the source through the authoritative edition writes
// the same bytes whatever the block's source locale is spelled as.
func TestASourceLocaleNormalizedTwiceChangesNoWrittenByte(t *testing.T) {
	cases := []struct {
		format registry.FormatID
		input  string
		file   string
	}{
		{format: "html", input: `<html><body><p>Hello world</p><div>Bare text <b>bold</b> <p>Para</p></div></body></html>`},
		{format: "asciidoc", input: "= Title here\n\nHello world.\n\nSecond *bold* line.\n"},
		{format: "doclang", file: "doclang/testdata/sample.dclg.xml"},
		{format: "epub", file: "epub/testdata/minimal.epub"},
		{format: "i18next", file: "i18next/testdata/interpolation_en.json"},
		{format: "kbf", file: "jsx/testdata/kapi-translated.kbf.json"},
	}
	stamp := func(b *model.Block) { b.SourceLocale = sourceLocaleNormalizedTwice }
	for _, tc := range cases {
		input := []byte(tc.input)
		if tc.file != "" {
			var err error
			input, err = os.ReadFile(filepath.FromSlash(tc.file))
			require.NoError(t, err)
		}
		for _, withSkeleton := range []bool{true, false} {
			name := string(tc.format)
			if !withSkeleton {
				name += "/no skeleton"
			}
			t.Run(name, func(t *testing.T) {
				want := convertWith(t, tc.format, tc.format, input, withSkeleton, nil)
				require.NotEmpty(t, want)
				assert.Equal(t, want, convertWith(t, tc.format, tc.format, input, withSkeleton, stamp))
			})
		}
	}
}

// The ARB reader files the catalogue's @@locale on each block as written, so
// the spelling reaches a writer from the file alone.
func TestWritersWriteAnARBSourceWhoseLocaleNormalizesTwice(t *testing.T) {
	doc := []byte(`{"@@locale": "AA-u-00-00-u-00-00", "greeting": "Hello world"}`)
	for _, to := range []registry.FormatID{"asciidoc", "doclang", "html"} {
		t.Run(string(to), func(t *testing.T) {
			var locales []model.LocaleID
			out := convertWith(t, "arb", to, doc, false, func(b *model.Block) { locales = append(locales, b.SourceLocale) })
			require.Equal(t, []model.LocaleID{sourceLocaleNormalizedTwice}, locales)
			assert.Contains(t, out, "Hello world")
		})
	}
}
