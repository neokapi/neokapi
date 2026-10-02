package json

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

func TestEditStructure(t *testing.T) {
	const nested = "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42\n}\n"
	tests := []struct {
		name  string
		doc   string
		edits []format.StructuralEdit
		want  string
	}{
		{
			name:  "delete a member between two others",
			doc:   nested,
			edits: []format.StructuralEdit{del("nav.cart")},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "delete the last member of an object takes the comma before it",
			doc:   nested,
			edits: []format.StructuralEdit{del("nav.legacy")},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "delete the only member leaves an empty object",
			doc:   "{\n  \"nav\": {\n    \"only\": \"x\"\n  }\n}\n",
			edits: []format.StructuralEdit{del("nav.only")},
			want:  "{\n  \"nav\": {\n  }\n}\n",
		},
		{
			name:  "delete keeps a comment above the member",
			doc:   "{\n  \"a\": \"A\",\n  // about b\n  \"b\": \"B\",\n  \"c\": \"C\"\n}",
			edits: []format.StructuralEdit{del("b")},
			want:  "{\n  \"a\": \"A\",\n  // about b\n  \"c\": \"C\"\n}",
		},
		{
			name:  "delete in compact JSON",
			doc:   `{"a":"A","b":"B","c":"C"}`,
			edits: []format.StructuralEdit{del("b"), del("c")},
			want:  `{"a":"A"}`,
		},
		{
			name:  "insert after a member, in its object and at its indentation",
			doc:   nested,
			edits: []format.StructuralEdit{insAfter("nav.home", "nav.checkout", "Checkout")},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"checkout\": \"Checkout\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "insert after the last member adds the comma",
			doc:   nested,
			edits: []format.StructuralEdit{insAfter("nav.legacy", "nav.checkout", "Checkout")},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\",\n    \"checkout\": \"Checkout\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "insert before the first member",
			doc:   nested,
			edits: []format.StructuralEdit{insBefore("nav.home", "nav.top", "Top")},
			want:  "{\n  \"nav\": {\n    \"top\": \"Top\",\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "insert with no anchor goes last at the top level",
			doc:   nested,
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "footer", Value: "Footer"}},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42,\n  \"footer\": \"Footer\"\n}\n",
		},
		{
			name:  "insert into an empty document",
			doc:   "{}\n",
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "a", Value: "A"}},
			want:  "{\n  \"a\": \"A\"\n}\n",
		},
		{
			name:  "insert after a member a comment trails keeps the comment on its line",
			doc:   "{\n  \"a\": \"A\", // about a\n  \"b\": \"B\" // about b\n}",
			edits: []format.StructuralEdit{insAfter("a", "a2", "A2"), insAfter("b", "b2", "B2")},
			want:  "{\n  \"a\": \"A\", // about a\n  \"a2\": \"A2\",\n  \"b\": \"B\", // about b\n  \"b2\": \"B2\"\n}",
		},
		{
			name:  "a later edit sees the members an earlier one added",
			doc:   "{\n  \"x\": \"X\"\n}",
			edits: []format.StructuralEdit{insAfter("x", "a", "A"), insAfter("a", "b", "B"), insBefore("a", "c", "C")},
			want:  "{\n  \"x\": \"X\",\n  \"c\": \"C\",\n  \"a\": \"A\",\n  \"b\": \"B\"\n}",
		},
		{
			name:  "compact JSON keeps its layout",
			doc:   `{"a": "A", "b": "B"}`,
			edits: []format.StructuralEdit{insAfter("a", "n", "N"), insBefore("a", "m", "M")},
			want:  `{"m": "M", "a": "A", "n": "N", "b": "B"}`,
		},
		{
			name:  "CRLF line breaks",
			doc:   "{\r\n  \"a\": \"A\"\r\n}\r\n",
			edits: []format.StructuralEdit{insAfter("a", "b", "B"), insBefore("a", "z", "Z")},
			want:  "{\r\n  \"z\": \"Z\",\r\n  \"a\": \"A\",\r\n  \"b\": \"B\"\r\n}\r\n",
		},
		{
			name:  "a trailing comma stays",
			doc:   "{\n  \"a\": \"A\",\n}",
			edits: []format.StructuralEdit{insAfter("a", "b", "B")},
			want:  "{\n  \"a\": \"A\",\n  \"b\": \"B\",\n}",
		},
		{
			name:  "the value is escaped as the writer escapes an edited value",
			doc:   "{\n  \"a\": \"A\"\n}",
			edits: []format.StructuralEdit{insAfter("a", "b", "Say \"hi\"\nto a/b")},
			want:  "{\n  \"a\": \"A\",\n  \"b\": \"Say \\\"hi\\\"\\nto a\\/b\"\n}",
		},
		{
			name:  "a member in an object inside an array",
			doc:   "{\"list\": [{\"t\": \"T\", \"u\": \"U\"}]}",
			edits: []format.StructuralEdit{del("list[0].t"), insAfter("list[0].u", "list[0].v", "V")},
			want:  "{\"list\": [{\"u\": \"U\", \"v\": \"V\"}]}",
		},
		{
			name:  "insert with no anchor goes last in the object its key path names",
			doc:   nested,
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "nav.checkout", Value: "Checkout"}},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\",\n    \"checkout\": \"Checkout\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "insert builds the objects its key path names and the document lacks",
			doc:   nested,
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "account.profile.title", Value: "Profile"}},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42,\n  \"account\": {\n    \"profile\": {\n      \"title\": \"Profile\"\n    }\n  }\n}\n",
		},
		{
			name:  "insert beside an anchor builds an object in the anchor's object",
			doc:   nested,
			edits: []format.StructuralEdit{insAfter("nav.home", "nav.sub.item", "Item")},
			want:  "{\n  \"nav\": {\n    \"home\": \"Home\",\n    \"sub\": {\n      \"item\": \"Item\"\n    },\n    \"cart\": \"Cart\",\n    \"legacy\": \"Old\"\n  },\n  \"count\": 42\n}\n",
		},
		{
			name:  "an object built in compact JSON stays on the line",
			doc:   `{"a": {"b": "B"}}`,
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "a.c.d", Value: "D"}},
			want:  `{"a": {"b": "B", "c": {"d": "D"}}}`,
		},
		{
			name:  "an object that names values by flat key paths takes a flat key",
			doc:   "{\n  \"nav.home\": \"Home\",\n  \"nav.cart\": \"Cart\"\n}\n",
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "nav.checkout", Value: "Checkout"}, insAfter("nav.home", "nav.top", "Top")},
			want:  "{\n  \"nav.home\": \"Home\",\n  \"nav.top\": \"Top\",\n  \"nav.cart\": \"Cart\",\n  \"nav.checkout\": \"Checkout\"\n}\n",
		},
		{
			name:  "delete takes a comment on the member's own line",
			doc:   "{\n  \"a\": \"A\", // about a\n  \"b\": \"B\", // about b\n  \"c\": \"C\" // about c\n}",
			edits: []format.StructuralEdit{del("b"), del("c")},
			want:  "{\n  \"a\": \"A\" // about a\n}",
		},
		{
			name:  "a block comment that runs on to the next line stays whole",
			doc:   "{\n  \"a\": \"A\", /* about\n  b */\n  \"b\": \"B\"\n}",
			edits: []format.StructuralEdit{insAfter("a", "n", "N")},
			want:  "{\n  \"a\": \"A\",\n  \"n\": \"N\", /* about\n  b */\n  \"b\": \"B\"\n}",
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
	const doc = "{\n  \"nav\": {\n    \"cart\": \"Cart\"\n  },\n  \"count\": 42,\n  \"list\": [\"a\", \"b\"],\n  \"dup\": \"1\",\n  \"dup\": \"2\"\n}"
	tests := []struct {
		name   string
		edit   format.StructuralEdit
		reason format.StructureReason
	}{
		{"a member with that key exists", insAfter("nav.cart", "nav.cart", "x"), format.StructureExists},
		{"a number sits at the key", format.StructuralEdit{Op: format.StructuralInsertBlock, Key: "count", Value: "x"}, format.StructureExists},
		{"the anchor is missing", insAfter("nav.nope", "nav.x", "x"), format.StructureNotFound},
		{"the new key is not in the anchor's object", insAfter("nav.cart", "footer.x", "x"), format.StructureUnsupported},
		{"the new key's object is inside the anchor's", insAfter("count", "nav.checkout", "x"), format.StructureUnsupported},
		{"a text value sits where an object would go", format.StructuralEdit{Op: format.StructuralInsertBlock, Key: "nav.cart.x", Value: "x"}, format.StructureExists},
		{"an array sits where an object would go", format.StructuralEdit{Op: format.StructuralInsertBlock, Key: "list.x", Value: "x"}, format.StructureExists},
		{"a new value in an array", format.StructuralEdit{Op: format.StructuralInsertBlock, Key: "list[2]", Value: "x"}, format.StructureUnsupported},
		{"an array item", del("list[0]"), format.StructureUnsupported},
		{"no text value", del("count"), format.StructureUnsupported},
		{"a key written twice", del("dup"), format.StructureUnsupported},
		{"nothing to delete", del("nav.nope"), format.StructureNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWriter().EditStructure([]byte(doc), []format.StructuralEdit{tt.edit})
			var se *format.StructureError
			require.ErrorAs(t, err, &se, "want a StructureError, got %v", err)
			assert.Equal(t, tt.reason, se.Reason, se.Message)
			assert.Equal(t, 0, se.Edit)
		})
	}
}

func TestEditStructureNamesTheEditItCannotMake(t *testing.T) {
	_, err := NewWriter().EditStructure([]byte(`{"a": "A"}`), []format.StructuralEdit{insAfter("a", "b", "B"), del("zzz")})
	var se *format.StructureError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, 1, se.Edit)
}

func TestWriterDeclaresStructure(t *testing.T) {
	assert.Equal(t, []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}, format.StructuralOps(NewWriter()))
}

// A configuration that reads a block's note, id or metadata from the members
// beside it makes those members part of the block's shell, so the writer
// declares no structural operation under it and refuses one.
func TestWriterDeclaresNoStructureWhenSiblingsDescribeABlock(t *testing.T) {
	for name, set := range map[string]func(*Config){
		"noteRules":        func(c *Config) { c.NoteRules = "^description$" },
		"idRules":          func(c *Config) { c.IDRules = "^id$" },
		"useIdStack":       func(c *Config) { c.UseIDStack = true },
		"genericMetaRules": func(c *Config) { c.GenericMetaRules = "^meta$" },
		"maxwidthRules":    func(c *Config) { c.MaxwidthRules = "^max$" },
	} {
		t.Run(name, func(t *testing.T) {
			w := NewWriter()
			set(w.Config())
			assert.Empty(t, format.StructuralOps(w))
			_, err := w.EditStructure([]byte(`{"a": "A"}`), []format.StructuralEdit{del("a")})
			var se *format.StructureError
			require.ErrorAs(t, err, &se)
			assert.Equal(t, format.StructureUnsupported, se.Reason)
		})
	}
}

// Under a configuration that names blocks by full key path (/nav/home), the
// writer reads a block's name as the key path it spells.
func TestEditStructureReadsFullKeyPathNames(t *testing.T) {
	w := NewWriter()
	w.Config().UseFullKeyPath = true
	got, err := w.EditStructure([]byte("{\n  \"nav\": {\n    \"home\": \"Home\"\n  }\n}"), []format.StructuralEdit{
		{Op: format.StructuralInsertBlock, Key: "/nav/checkout", Value: "Checkout"},
		insBefore("/nav/home", "/nav/top", "Top"),
	})
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"nav\": {\n    \"top\": \"Top\",\n    \"home\": \"Home\",\n    \"checkout\": \"Checkout\"\n  }\n}", string(got))
}
