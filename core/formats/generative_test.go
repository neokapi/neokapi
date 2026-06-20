package formats_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

// TestWriterGenerativeCapability is the declarative source of truth for which
// formats can be cross-format conversion targets (AD-005 "Writer output modes").
// The registry probes each built-in writer's GenerativeWriter capability once at
// registration and records it on FormatInfo — resolvable without loading any
// plugin. Generative writers serialize a whole document from the content model;
// skeleton-bound (packaged/binary) writers only round-trip into their own file.
func TestWriterGenerativeCapability(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	// Skeleton-bound: packaged / binary formats that need the original file.
	skeletonBound := []string{"openxml", "odf", "idml", "icml", "mif", "epub", "image"}
	// Generative: document / interchange / catalog writers that build standalone.
	generative := []string{
		"doclang", "markdown", "html", "asciidoc", "plaintext",
		"xliff", "xliff2", "po", "tmx", "klf", "json", "yaml",
	}

	for _, id := range skeletonBound {
		info := reg.FormatInfo(registry.FormatID(id))
		if info == nil || !info.HasWriter {
			t.Errorf("%s: expected a registered writer", id)
			continue
		}
		if info.Generative {
			t.Errorf("%s: expected skeleton-bound (Generative=false), got Generative=true", id)
		}
	}
	for _, id := range generative {
		info := reg.FormatInfo(registry.FormatID(id))
		if info == nil || !info.HasWriter {
			t.Errorf("%s: expected a registered writer", id)
			continue
		}
		if !info.Generative {
			t.Errorf("%s: expected Generative=true (valid conversion target)", id)
		}
	}
}
