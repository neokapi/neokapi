package formats

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/registry"
)

// TestEOFNewlinePreserved asserts that whether a document ends with a line
// break is a property of the document, not of the writer.
//
// A writer that always terminates the last line adds a newline to a file
// authored without one; a writer that never does strips one the author wrote.
// Either way every byte after the change belongs to a diff nobody asked for,
// which is what makes a translated file unreviewable — and neither shows up in
// a round-trip test whose fixture happens to use the convention the writer
// prefers.
func TestEOFNewlinePreserved(t *testing.T) {
	reg := registry.NewFormatRegistry()
	RegisterAll(reg)

	for _, tc := range escapeSweepFormats() {
		if reason, ok := eofNewlineSkips[tc.id]; ok {
			t.Run(tc.id, func(t *testing.T) { t.Skipf("%s: %s", tc.id, reason) })
			continue
		}
		t.Run(tc.id, func(t *testing.T) {
			base := strings.TrimRight(tc.source, "\n")
			for _, variant := range []struct {
				name string
				src  string
			}{
				{name: "with_trailing_newline", src: base + "\n"},
				{name: "without_trailing_newline", src: base},
			} {
				t.Run(variant.name, func(t *testing.T) {
					out := untouchedRoundTrip(t, reg, tc.id, []byte(variant.src))
					if !bytes.Equal(out, []byte(variant.src)) {
						t.Fatalf("%s: untouched round-trip changed the document\nwant %q\ngot  %q",
							tc.id, variant.src, string(out))
					}
				})
			}
		})
	}
}

// eofNewlineSkips names the formats whose probe document cannot drop its final
// line break without becoming a different document.
var eofNewlineSkips = map[string]string{
	"srt": "a cue is terminated by the blank line that follows it",
	"vtt": "a cue is terminated by the blank line that follows it",
	"po":  "an entry is terminated by the line break after its last string",
}

func untouchedRoundTrip(t *testing.T, reg *registry.FormatRegistry, id string, src []byte) []byte {
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
	parts, err := spec.ReadParts(reader, src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	out, err := spec.WriteParts(writer, parts, src)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return out
}
