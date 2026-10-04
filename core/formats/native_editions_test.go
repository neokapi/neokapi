package formats

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// A reader marks each edition it files from the document's bytes as native:
// a monolingual document holds the edition it was read in alone, a bilingual
// one holds its target beside it, and the first native edition is the one the
// engine treats as authoritative.
func TestReadersMarkTheEditionsTheDocumentHolds(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	en, fr := model.EditionKey{Locale: "en"}, model.EditionKey{Locale: "fr"}
	tests := []struct {
		format string
		src    string
		want   []model.EditionKey
	}{
		{"markdown", "Hello world.\n", []model.EditionKey{en}},
		{"xliff", `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="a.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1"><source>Hello</source><target>Bonjour</target></trans-unit>
    </body>
  </file>
</xliff>
`, []model.EditionKey{en, fr}},
		{"xliff2", `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1"><segment><source>Hello</source><target>Bonjour</target></segment></unit>
  </file>
</xliff>
`, []model.EditionKey{en, fr}},
		{"po", "msgid \"\"\nmsgstr \"\"\n\"Language: fr\\n\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n", []model.EditionKey{en, fr}},
	}
	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			blocks := readFormatBlocks(t, reg, tc.format, tc.src, "en", "fr")
			require.NotEmpty(t, blocks)
			var b *model.Block
			for _, cand := range blocks {
				if cand.Translatable && cand.SourceText() != "" {
					b = cand
					break
				}
			}
			require.NotNil(t, b, "a translatable block")
			if b.SourceLocale == "" {
				b.SourceLocale = "en"
			}
			assert.Equal(t, tc.want, b.NativeEditions())
			assert.Equal(t, tc.want[0], b.Authoritative(model.AuthorityPolicy{}))
		})
	}
}

// readFormatBlocks reads src in format as a document from srcLoc to trgLoc
// and returns its blocks.
func readFormatBlocks(t *testing.T, reg *registry.FormatRegistry, format, src string, srcLoc, trgLoc model.LocaleID) []*model.Block {
	t.Helper()
	reader, err := reg.NewReader(registry.FormatID(format))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{
		SourceLocale: srcLoc,
		TargetLocale: trgLoc,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
	}
	require.NoError(t, reader.Open(ctx, doc))
	var out []*model.Block
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			out = append(out, b)
		}
	}
	require.NoError(t, reader.Close())
	return out
}
