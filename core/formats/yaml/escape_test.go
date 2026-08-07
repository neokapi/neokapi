package yaml

import (
	"fmt"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"
)

// TestModifiedValueReEscapes walks the YAML 1.2 §5.7 escape table: a translated
// value carrying each character must come back out of a YAML parser intact.
//
// Skeleton replay hides the whole table. An untouched scalar is written back as
// its original bytes, so the encoder only ever sees a value something changed —
// and a raw control byte inside a double-quoted scalar makes the document
// unparseable while the writer reports success.
func TestModifiedValueReEscapes(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		// escape is the form encodeDoubleQuoted emits for the character.
		escape string
		// forcesQuoting marks a character outside YAML's printable production:
		// no plain, single-quoted or block scalar can hold it, so the encoder
		// must abandon the source's style whatever it was.
		forcesQuoting bool
	}{
		{name: "null", r: 0x00, escape: `\0`, forcesQuoting: true},
		{name: "bell", r: 0x07, escape: `\a`, forcesQuoting: true},
		{name: "backspace", r: 0x08, escape: `\b`, forcesQuoting: true},
		{name: "vertical_tab", r: 0x0B, escape: `\v`, forcesQuoting: true},
		{name: "form_feed", r: 0x0C, escape: `\f`, forcesQuoting: true},
		{name: "carriage_return", r: 0x0D, escape: `\r`, forcesQuoting: true},
		{name: "escape", r: 0x1B, escape: `\e`, forcesQuoting: true},
		{name: "next_line", r: 0x85, escape: `\N`, forcesQuoting: true},
		{name: "start_of_heading", r: 0x01, escape: `\x01`, forcesQuoting: true},
		{name: "device_control_4", r: 0x14, escape: `\x14`, forcesQuoting: true},
		{name: "unit_separator", r: 0x1F, escape: `\x1f`, forcesQuoting: true},
		{name: "delete", r: 0x7F, escape: `\x7f`, forcesQuoting: true},
		{name: "c1_application_program_command", r: 0x9F, escape: `\x9f`, forcesQuoting: true},
		// Carried literally by the other styles; escaped only where the
		// document already uses a double-quoted scalar.
		{name: "tab", r: 0x09, escape: `\t`},
		{name: "line_feed", r: 0x0A, escape: `\n`},
		{name: "double_quote", r: '"', escape: `\"`},
		{name: "backslash", r: '\\', escape: `\\`},
	}

	// Every source scalar style, so the encoder is exercised where it has to
	// abandon the recorded style for a double-quoted one.
	styles := map[string]string{
		"plain":          "msg: hello\n",
		"single_quoted":  "msg: 'hello'\n",
		"double_quoted":  "msg: \"hello\"\n",
		"literal_block":  "msg: |\n  hello\n",
		"folded_block":   "msg: >\n  hello\n",
		"nested_mapping": "outer:\n  msg: hello\n",
	}

	for styleName, source := range styles {
		t.Run(styleName, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					want := "Ax" + string(tc.r) + "zB"
					out := modifyAndWrite(t, source, want)

					if tc.forcesQuoting || styleName == "double_quoted" {
						require.Contains(t, out, tc.escape,
							"expected the YAML escape for U+%04X in %q", tc.r, out)
					}

					var got map[string]any
					require.NoError(t, yamlv3.Unmarshal([]byte(out), &got),
						"output must reparse: %q", out)

					// A block scalar's clip chomping restores the terminating
					// line break the style itself carries, so the value comes
					// back with it whatever the writer does.
					reparsed := modifiedValues(t, out)
					require.Truef(t,
						slices.Contains(reparsed, want) || slices.Contains(reparsed, want+"\n"),
						"value must survive: want %q, got %q from %q", want, reparsed, out)
				})
			}
		})
	}
}

// modifyAndWrite drives read → set target → write over a skeleton, which is the
// path a translated document takes.
func modifyAndWrite(t *testing.T, source, target string) string {
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
	out, err := spec.WriteParts(writer, parts, []byte(source))
	require.NoError(t, err)
	return string(out)
}

func modifiedValues(t *testing.T, source string) []string {
	t.Helper()
	parts, err := spec.ReadParts(NewReader(), []byte(source))
	require.NoError(t, err, "reparse")
	var out []string
	for _, part := range parts {
		if part == nil || part.Type != model.PartBlock {
			continue
		}
		if block, ok := part.Resource.(*model.Block); ok && block.Translatable {
			out = append(out, model.RenderRunsWithData(block.Source))
		}
	}
	return out
}

// TestUntouchedScalarKeepsItsStyle guards the other side of the fix: forcing a
// double-quoted scalar is reserved for a value that needs an escape, so a
// document nothing edited still comes back byte-for-byte.
func TestUntouchedScalarKeepsItsStyle(t *testing.T) {
	sources := []string{
		"msg: hello\n",
		"msg: 'hello'\n",
		"msg: \"hello\"\n",
		"msg: |\n  hello\n",
		"msg: >\n  hello\n",
	}
	for i, source := range sources {
		t.Run(fmt.Sprintf("source_%d", i), func(t *testing.T) {
			reader := NewReader()
			writer := NewWriter()
			store, err := format.NewWiredSkeleton(reader, writer)
			require.NoError(t, err)
			if store != nil {
				defer store.Close()
			}
			parts, err := spec.ReadParts(reader, []byte(source))
			require.NoError(t, err)
			out, err := spec.WriteParts(writer, parts, []byte(source))
			require.NoError(t, err)
			require.Equal(t, source, string(out))
		})
	}
}
