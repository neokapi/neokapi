package spectest

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// UTF8BOM is the UTF-8 encoding of U+FEFF.
const UTF8BOM = "\xef\xbb\xbf"

// BOMProbe asserts that a leading UTF-8 byte-order mark changes nothing a
// format's identity depends on, and that the mark still survives write-back.
//
// The mark is not content. Content memory and `kapi merge` key on block
// identity, so a mark folded into the first key, the first cell, or the
// front-matter delimiter orphans every match in the file and mis-keys every
// merge overlay — while the bytes round-trip exactly, so the ordinary
// round-trip test stays green. Excel writes a mark on every CSV export, which
// makes this the first thing a spreadsheet user hits.
type BOMProbe struct {
	// Format names the format under probe.
	Format string
	// NewReader and NewWriter build a fresh pair per case.
	NewReader func() format.DataFormatReader
	NewWriter func() format.DataFormatWriter
	// Source is a document with no byte-order mark.
	Source []byte
}

// blockIdentity is the identity view the probe compares: what content memory
// and merge key on.
type blockIdentity struct {
	ID   string
	Name string
	Text string
}

// Run checks identity stability and byte-exact write-back.
func (p BOMProbe) Run(t *testing.T) {
	t.Helper()
	if bytes.HasPrefix(p.Source, []byte(UTF8BOM)) {
		t.Fatalf("%s: probe source must not already carry a byte-order mark", p.Format)
	}
	withBOM := append([]byte(UTF8BOM), p.Source...)

	t.Run("identity_stable", func(t *testing.T) {
		plain := p.identities(t, p.Source)
		marked := p.identities(t, withBOM)
		if len(plain) != len(marked) {
			t.Fatalf("%s: byte-order mark changed the block count: %d without, %d with\nwithout: %+v\nwith:    %+v",
				p.Format, len(plain), len(marked), plain, marked)
		}
		for i := range plain {
			if plain[i] != marked[i] {
				t.Fatalf("%s: byte-order mark changed block %d identity\nwithout: %+v\nwith:    %+v",
					p.Format, i, plain[i], marked[i])
			}
		}
	})

	t.Run("roundtrip_byte_exact", func(t *testing.T) {
		out := p.roundtrip(t, withBOM)
		if !bytes.Equal(out, withBOM) {
			t.Fatalf("%s: byte-order mark did not survive an untouched round-trip\nwant %q\ngot  %q",
				p.Format, string(withBOM), string(out))
		}
	})
}

func (p BOMProbe) identities(t *testing.T, src []byte) []blockIdentity {
	t.Helper()
	parts, err := spec.ReadParts(p.NewReader(), src)
	if err != nil {
		t.Fatalf("%s: read: %v", p.Format, err)
	}
	var out []blockIdentity
	for _, part := range parts {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		out = append(out, blockIdentity{
			ID:   block.ID,
			Name: block.Name,
			Text: model.RenderRunsWithData(block.Source),
		})
	}
	return out
}

func (p BOMProbe) roundtrip(t *testing.T, src []byte) []byte {
	t.Helper()
	reader := p.NewReader()
	writer := p.NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	if err != nil {
		t.Fatalf("%s: wire skeleton: %v", p.Format, err)
	}
	if store != nil {
		defer store.Close()
	}
	parts, err := spec.ReadParts(reader, src)
	if err != nil {
		t.Fatalf("%s: read: %v", p.Format, err)
	}
	out, err := spec.WriteParts(writer, parts, src)
	if err != nil {
		t.Fatalf("%s: write: %v", p.Format, err)
	}
	return out
}
