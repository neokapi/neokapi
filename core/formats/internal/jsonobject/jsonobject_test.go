package jsonobject

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokens reads plain JSON with string values and no escapes, enough for the
// layouts these tests exercise; the formats hand over their own scanners'
// tokens.
func tokens(t *testing.T, src string) []Token {
	t.Helper()
	var out []Token
	i := 0
	for {
		start := i
		for i < len(src) && strings.ContainsRune(" \t\r\n", rune(src[i])) {
			i++
		}
		prefix := src[start:i]
		if i == len(src) {
			return append(out, Token{Kind: EOF, Prefix: prefix})
		}
		kinds := map[byte]Kind{'{': ObjectStart, '}': ObjectEnd, '[': ArrayStart, ']': ArrayEnd, ':': Colon, ',': Comma}
		if k, ok := kinds[src[i]]; ok {
			out = append(out, Token{Kind: k, Prefix: prefix, Raw: src[i : i+1]})
			i++
			continue
		}
		require.Equal(t, byte('"'), src[i], "test tokenizer reads strings only, at %d", i)
		end := strings.IndexByte(src[i+1:], '"') + i + 2
		out = append(out, Token{Kind: String, Prefix: prefix, Raw: src[i:end], Text: src[i+1 : end-1]})
		i = end
	}
}

func parse(t *testing.T, src string) *Doc {
	t.Helper()
	d, err := Parse([]byte(src), tokens(t, src))
	require.NoError(t, err)
	return d
}

func TestFindNamesMembersAsTheReaderDoes(t *testing.T) {
	d := parse(t, `{"a": {"b": "x"}, "a.b": "y", "l": [{"c": "z"}]}`)
	assert.Len(t, d.Find("a.b"), 2, "a flat key and a nested one share a path")
	require.Len(t, d.Find("l[0].c"), 1)
	assert.Equal(t, "l[0]", d.Find("l[0].c")[0].Object.Path)
	assert.Empty(t, d.Find("l[0]"))
}

func TestParseRefusesTokensThatDoNotSpellTheDocument(t *testing.T) {
	src := `{"a": "x"}`
	toks := tokens(t, src)
	_, err := Parse([]byte(src+" "), toks)
	require.Error(t, err)
	_, err = Parse([]byte(`{"a" "x"}`), tokens(t, `{"a" "x"}`))
	require.Error(t, err)
}

func TestLayouts(t *testing.T) {
	tests := []struct {
		name string
		src  string
		edit func(d *Doc) []byte
		want string
	}{
		{"delete the first of two on the brace line", `{ "a": "A", "b": "B" }`,
			func(d *Doc) []byte { return d.Delete(d.Find("a")[0]) }, `{ "b": "B" }`},
		{"delete the only member", `{ "a": "A" }`,
			func(d *Doc) []byte { return d.Delete(d.Find("a")[0]) }, `{ }`},
		{"insert before the only member", `{"a": "A"}`,
			func(d *Doc) []byte { return d.InsertBefore(d.Find("a")[0], `"z"`, `"Z"`) }, `{"z": "Z", "a": "A"}`},
		{"append to an empty nested object keeps its indentation", "{\n  \"o\": {}\n}",
			func(d *Doc) []byte { return d.Append(d.ObjectsAt("o")[0], `"k"`, `"v"`) }, "{\n  \"o\": {\n    \"k\": \"v\"\n  }\n}"},
		{"a nested object on lines of its own, one step deeper per level", "{\r\n\t\"a\": \"A\"\r\n}",
			func(d *Doc) []byte { return d.Append(d.Root, `"b"`, d.Nest(d.Root, []string{`"c"`, `"d"`}, `"D"`)) },
			"{\r\n\t\"a\": \"A\",\r\n\t\"b\": {\r\n\t\t\"c\": {\r\n\t\t\t\"d\": \"D\"\r\n\t\t}\r\n\t}\r\n}"},
		{"a nested object in an object on one line", `{"a":"A"}`,
			func(d *Doc) []byte { return d.Append(d.Root, `"b"`, d.Nest(d.Root, []string{`"c"`}, `"C"`)) }, `{"a":"A","b":{"c":"C"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.edit(parse(t, tt.src))))
		})
	}
}

func TestObjectsAtAndDottedKeys(t *testing.T) {
	d := parse(t, `{"nav": {"home": "H"}, "flat.key": "F", "l": [{"c": "z"}]}`)
	require.Len(t, d.ObjectsAt(""), 1)
	assert.Same(t, d.Root, d.ObjectsAt("")[0])
	require.Len(t, d.ObjectsAt("nav"), 1)
	assert.Len(t, d.ObjectsAt("l[0]"), 1)
	assert.Empty(t, d.ObjectsAt("nav.home"))
	assert.True(t, d.Root.DottedKeys())
	assert.False(t, d.ObjectsAt("nav")[0].DottedKeys())
}
