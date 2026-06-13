//go:build parity

package formats

import (
	"github.com/neokapi/neokapi/core/format"
	plaintextfmt "github.com/neokapi/neokapi/core/formats/plaintext"
	pofmt "github.com/neokapi/neokapi/core/formats/po"
	propertiesfmt "github.com/neokapi/neokapi/core/formats/properties"
)

// parityWriters maps a bridge filter id to its non-default DataFormatWriter
// factory. A writer is a `func() format.DataFormatWriter` and therefore CANNOT
// live in spec.yaml — this registry is the one irreducibly-Go piece of the
// per-format parity config left after #852 moved the rest (bridge filter
// class, tikal, skips) into the spec.yaml grammar. Only formats whose parity
// run exercises the round-trip / tikal corner need an entry; everything else
// is read-only on the parity side and needs no writer.
//
// Keep this list minimal: a row here is hand-maintained Go, the exact thing
// #852 set out to shrink. When a writer's only consumer is the parity
// round-trip, it belongs here, not in the spec.yaml.
var parityWriters = map[string]func() format.DataFormatWriter{
	"okf_properties": func() format.DataFormatWriter { return propertiesfmt.NewWriter() },
	"okf_po":         func() format.DataFormatWriter { return pofmt.NewWriter() },
	"okf_plaintext":  func() format.DataFormatWriter { return plaintextfmt.NewWriter() },
}

// parityWriterFor returns the registered writer factory for a format id, or
// nil when the format runs read-only on the parity side.
func parityWriterFor(id string) func() format.DataFormatWriter {
	return parityWriters[id]
}
