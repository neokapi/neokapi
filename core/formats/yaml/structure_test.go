package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
)

func del(key string) format.StructuralEdit {
	return format.StructuralEdit{Op: format.StructuralDeleteBlock, Key: key}
}

func insAfter(anchor, key, value string) format.StructuralEdit {
	return format.StructuralEdit{Op: format.StructuralInsertBlock, Key: key, Anchor: anchor, Value: value}
}

func insBefore(anchor, key, value string) format.StructuralEdit {
	e := insAfter(anchor, key, value)
	e.Before = true
	return e
}

const structureDoc = `# Navigation strings
nav:
  # The home link
  home: Home
  cart: Cart # shown in the header
  # Remove after 2.0
  legacy: Old
  help: |
    Read the guide
    before you order.
footer: "Footer"
`

func TestEditStructure(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		edits []format.StructuralEdit
		want  string
	}{
		{
			name:  "removing a key keeps the comment above it",
			doc:   structureDoc,
			edits: []format.StructuralEdit{del("nav.legacy")},
			want: `# Navigation strings
nav:
  # The home link
  home: Home
  cart: Cart # shown in the header
  # Remove after 2.0
  help: |
    Read the guide
    before you order.
footer: "Footer"
`,
		},
		{
			name:  "removing a key removes the comment on its own line",
			doc:   structureDoc,
			edits: []format.StructuralEdit{del("nav.cart")},
			want: `# Navigation strings
nav:
  # The home link
  home: Home
  # Remove after 2.0
  legacy: Old
  help: |
    Read the guide
    before you order.
footer: "Footer"
`,
		},
		{
			name:  "removing a key removes every line of its value",
			doc:   structureDoc,
			edits: []format.StructuralEdit{del("nav.help")},
			want: `# Navigation strings
nav:
  # The home link
  home: Home
  cart: Cart # shown in the header
  # Remove after 2.0
  legacy: Old
footer: "Footer"
`,
		},
		{
			name:  "removing the last key of a mapping leaves the mapping empty",
			doc:   "nav:\n  only: x\nother: y\n",
			edits: []format.StructuralEdit{del("nav.only")},
			want:  "nav: {}\nother: y\n",
		},
		{
			name:  "a key after another goes on the line after its value, before the next key's comment",
			doc:   structureDoc,
			edits: []format.StructuralEdit{insAfter("nav.cart", "nav.checkout", "Checkout")},
			want: `# Navigation strings
nav:
  # The home link
  home: Home
  cart: Cart # shown in the header
  checkout: Checkout
  # Remove after 2.0
  legacy: Old
  help: |
    Read the guide
    before you order.
footer: "Footer"
`,
		},
		{
			name:  "a key before another goes on the line before it",
			doc:   structureDoc,
			edits: []format.StructuralEdit{insBefore("nav.home", "nav.top", "Top")},
			want: `# Navigation strings
nav:
  # The home link
  top: Top
  home: Home
  cart: Cart # shown in the header
  # Remove after 2.0
  legacy: Old
  help: |
    Read the guide
    before you order.
footer: "Footer"
`,
		},
		{
			name:  "a key after a value of several lines",
			doc:   structureDoc,
			edits: []format.StructuralEdit{insAfter("nav.help", "nav.faq", "FAQ")},
			want: `# Navigation strings
nav:
  # The home link
  home: Home
  cart: Cart # shown in the header
  # Remove after 2.0
  legacy: Old
  help: |
    Read the guide
    before you order.
  faq: FAQ
footer: "Footer"
`,
		},
		{
			name:  "a key with no anchor goes last",
			doc:   "a: A\nb: B",
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "c", Value: "C"}},
			want:  "a: A\nb: B\nc: C\n",
		},
		{
			name:  "a key into an empty document",
			doc:   "# nothing yet\n",
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "a", Value: "A"}},
			want:  "# nothing yet\na: A\n",
		},
		{
			name: "values that would not read back as text written plain are quoted",
			doc:  "a: A\n",
			edits: []format.StructuralEdit{
				insAfter("a", "b", "true"), insAfter("b", "c", "42"), insAfter("c", "d", "key: value"),
				insAfter("d", "e", " padded"), insAfter("e", "f", "two\nlines"), insAfter("f", "g", "# not a comment"),
			},
			want: "a: A\nb: \"true\"\nc: \"42\"\nd: \"key: value\"\ne: \" padded\"\nf: \"two\\nlines\"\ng: \"# not a comment\"\n",
		},
		{
			name:  "a later edit sees the keys an earlier one added",
			doc:   "x: X\n",
			edits: []format.StructuralEdit{insAfter("x", "a", "A"), insAfter("a", "b", "B"), insBefore("a", "c", "C")},
			want:  "x: X\nc: C\na: A\nb: B\n",
		},
		{
			name:  "CRLF line breaks",
			doc:   "nav:\r\n  a: A\r\n  b: B\r\n",
			edits: []format.StructuralEdit{insAfter("nav.a", "nav.n", "N"), del("nav.b")},
			want:  "nav:\r\n  a: A\r\n  n: N\r\n",
		},
		{
			name:  "a quoted key",
			doc:   "\"a key\": A\n",
			edits: []format.StructuralEdit{insAfter("a key", "true", "T")},
			want:  "\"a key\": A\n\"true\": T\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWriter().EditStructure([]byte(tt.doc), tt.edits)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestEditStructureRefuses(t *testing.T) {
	const doc = `nav:
  cart: Cart
flow: {a: A, b: B}
list:
  - item
  - k: v
    j: w
base: &base
  t: T
copy: *base
anchored: &v text
`
	tests := []struct {
		name   string
		edit   format.StructuralEdit
		reason format.StructureReason
	}{
		{"a key with that path exists", insAfter("nav.cart", "nav.cart", "x"), format.StructureExists},
		{"a mapping sits at the key", insAfter("flow.a", "nav", "x"), format.StructureExists},
		{"a key in a flow mapping", del("flow.a"), format.StructureUnsupported},
		{"a sequence item", del("list[0]"), format.StructureUnsupported},
		{"a key on a sequence item's dash line", del("list[1].k"), format.StructureUnsupported},
		{"a key reached through an alias", del("copy.t"), format.StructureUnsupported},
		{"a value other keys alias", del("anchored"), format.StructureUnsupported},
		{"the anchor is missing", insAfter("nav.nope", "nav.x", "x"), format.StructureNotFound},
		{"the new key is not in the anchor's mapping", insAfter("nav.cart", "other.x", "x"), format.StructureUnsupported},
		{"no text value", del("nav"), format.StructureUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWriter().EditStructure([]byte(doc), []format.StructuralEdit{tt.edit})
			var se *format.StructureError
			require.ErrorAs(t, err, &se, "want a StructureError, got %v", err)
			assert.Equal(t, tt.reason, se.Reason, se.Message)
		})
	}
}

func TestEditStructureNeedsAnAnchorInAFileOfSeveralDocuments(t *testing.T) {
	_, err := NewWriter().EditStructure([]byte("a: A\n---\nb: B\n"), []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "c", Value: "C"}})
	var se *format.StructureError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, format.StructureUnsupported, se.Reason)

	got, err := NewWriter().EditStructure([]byte("a: A\n---\nb: B\n"), []format.StructuralEdit{insAfter("b", "c", "C")})
	require.NoError(t, err)
	assert.Equal(t, "a: A\n---\nb: B\nc: C\n", string(got))
}

func TestWriterDeclaresStructure(t *testing.T) {
	assert.Equal(t, []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}, format.StructuralOps(NewWriter()))
}
