package formats_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

func BenchmarkHTMLReaderConstruction(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		html.NewReader()
	}
}

func BenchmarkMarkdownReaderConstruction(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		markdown.NewReader()
	}
}
