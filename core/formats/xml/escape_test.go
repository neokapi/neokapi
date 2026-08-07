package xml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

const escapeProbeSource = `<?xml version="1.0"?>
<root><p>Hello</p></root>
`

// TestUnrepresentableCharacterIsRejected pins the policy for a character XML
// 1.0 cannot carry: refuse the write and name the block.
//
// The alternatives both fail silently. Emitting the character produces a
// document that will not reopen — the failure surfaces at a merge or a
// re-extraction, far from the tool that caused it. Stripping it loses content
// the pipeline was told to write. A control character reaches a writer only
// because a tool put it there, which is upstream breakage worth naming.
func TestUnrepresentableCharacterIsRejected(t *testing.T) {
	for _, r := range []rune{0x00, 0x01, 0x07, 0x08, 0x0B, 0x0C, 0x0E, 0x1F} {
		t.Run(fmt.Sprintf("U+%04X", r), func(t *testing.T) {
			_, err := modifyAndWrite(t, escapeProbeSource, "Ax"+string(r)+"zB")
			require.Error(t, err)

			var unrepresentable *format.UnrepresentableValueError
			require.ErrorAs(t, err, &unrepresentable)
			require.Equal(t, "xml", unrepresentable.Format)
			require.NotEmpty(t, unrepresentable.BlockID, "the error must name the block")

			var charErr *xmlesc.UnrepresentableCharError
			require.ErrorAs(t, err, &charErr)
			require.Equal(t, r, charErr.Rune)

			require.Contains(t, err.Error(), unrepresentable.BlockID)
			require.Contains(t, strings.ToUpper(err.Error()), "U+")
		})
	}
}

// TestRepresentableCharactersSurvive is the other half: a value made only of
// characters XML admits must go through and read back as itself.
func TestRepresentableCharactersSurvive(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "markup_delimiters", text: `a < b & c > d`},
		{name: "quotes", text: `say "hello" and 'bye'`},
		{name: "cdata_end", text: `]]>`},
		{name: "astral", text: "emoji \U0001F600 tail"},
		{name: "replacement_char", text: "before � after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := modifyAndWrite(t, escapeProbeSource, tc.text)
			require.NoError(t, err)

			parts, err := spec.ReadParts(NewReader(), []byte(out))
			require.NoError(t, err, "output must reparse: %q", out)

			var got []string
			for _, part := range parts {
				if part == nil || part.Type != model.PartBlock {
					continue
				}
				if block, ok := part.Resource.(*model.Block); ok && block.Translatable {
					got = append(got, model.RenderRunsWithData(block.Source))
				}
			}
			require.Contains(t, got, tc.text, "output: %q", out)
		})
	}
}

func modifyAndWrite(t *testing.T, source, target string) (string, error) {
	t.Helper()
	reader := NewReader()
	writer := NewWriter()
	store, err := format.NewWiredSkeleton(reader, writer)
	require.NoError(t, err)
	if store != nil {
		defer store.Close()
	}
	parts, err := spec.ReadParts(reader, []byte(source))
	require.NoError(t, err)

	locale := model.LocaleID("fr")
	mutated := 0
	for _, part := range parts {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		if block, ok := part.Resource.(*model.Block); ok && block.Translatable {
			block.SetTargetText(locale, target)
			mutated++
		}
	}
	require.NotZero(t, mutated, "source produced no translatable block")

	writer.SetLocale(locale)
	out, werr := spec.WriteParts(writer, parts, []byte(source))
	return string(out), werr
}
