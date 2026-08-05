package formats_test

// EPUB reads its chapter XHTML through the HTML reader — the behavioural half of
// the SubfilterResolver wiring.
//
// core/format/subfilter.go promises that a reader implementing SubfilterAware
// "will have their resolver set by the pipeline before Open is called". That
// promise went unkept for the whole life of the interface: SetSubfilterResolver
// was called only from per-format tests, which inject the resolver by hand and
// therefore cannot catch its absence. epub/reader.go routes XHTML through the
// HTML reader only "when a subfilter resolver is available", so every production
// read took the fallback path: <h1>/<h2> flattened to plain paragraphs and
// <head><title> was promoted to body content.
//
// This test reads through the registry with no hand-injected resolver, the way
// production does. The injection itself is unit-tested in
// core/registry/subfilter_inject_test.go.

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawDoc wraps document bytes as a RawDocument. A fresh reader per use is
// required: readers consume their input.
func rawDoc(data []byte, path, formatID string) *model.RawDocument {
	return &model.RawDocument{
		URI:          path,
		Encoding:     "UTF-8",
		SourceLocale: model.LocaleEnglish,
		FormatID:     formatID,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestEPUBRoutesXHTMLThroughHTMLReader(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	r, err := reg.NewReader("epub")
	require.NoError(t, err)

	data, err := os.ReadFile("epub/testdata/minimal.epub")
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, r.Open(ctx, rawDoc(data, "minimal.epub", "epub")))
	t.Cleanup(func() { _ = r.Close() })

	type heading struct {
		text  string
		level int
	}
	var headings []heading
	var texts []string
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		b, ok := pr.Part.Resource.(*model.Block)
		if !ok {
			continue
		}
		if text := b.SourceText(); text != "" {
			texts = append(texts, text)
		}
		if b.SemanticRole() == model.RoleHeading {
			headings = append(headings, heading{b.SourceText(), b.HeadingLevel()})
		}
	}

	assert.Equal(t, []heading{{"Welcome", 1}, {"Conclusion", 2}}, headings,
		"epub chapter <h1>/<h2> should carry heading roles and their levels")

	// Body prose still arrives, in reading order — the delegation must not cost
	// content. The chapter <title> values are extracted too, which is the HTML
	// reader behaving normally: a document title is translatable content, not
	// something the subfilter should suppress.
	assert.Subset(t, texts, []string{
		"Welcome", "This is the first paragraph.", "This is the second paragraph.",
		"Conclusion", "Final thoughts here.",
	})
}
