package wiki_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/wiki"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native wiki reader. Failures pinpoint the feature and example so the
// spec doubles as documentation and verification.
//
// Wiki is a single-variant filter (the native reader has a `Variant`
// config knob for MediaWiki vs DokuWiki, but the okf_wiki bridge id
// covers DokuWiki only). The spec leaves the variant tag empty.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return wiki.NewReader() },
	}
	r.Run(t)
}
