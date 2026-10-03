package arb

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

const structureCatalog = `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "cart": "Cart",
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  }
}
`

func TestEditStructure(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		edits []format.StructuralEdit
		want  string
	}{
		{
			name:  "a message after one with metadata goes after the metadata",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{insAfter("greeting", "farewell", "Goodbye {name}")},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "farewell": "Goodbye {name}",
  "cart": "Cart",
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  }
}
`,
		},
		{
			name:  "a message before another goes right before it",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{insBefore("cart", "checkout", "Checkout")},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "checkout": "Checkout",
  "cart": "Cart",
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  }
}
`,
		},
		{
			name:  "a message before one whose metadata comes first goes before the metadata",
			doc:   "{\n  \"@a\": {\"description\": \"x\"},\n  \"a\": \"A\"\n}",
			edits: []format.StructuralEdit{insBefore("a", "b", "B")},
			want:  "{\n  \"b\": \"B\",\n  \"@a\": {\"description\": \"x\"},\n  \"a\": \"A\"\n}",
		},
		{
			name:  "removing a message removes its metadata",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{del("greeting")},
			want: `{
  "@@locale": "en",
  "cart": "Cart",
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  }
}
`,
		},
		{
			name:  "removing the last message and its metadata",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{del("legacy")},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "cart": "Cart"
}
`,
		},
		{
			name:  "removing a message with no metadata",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{del("cart")},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  }
}
`,
		},
		{
			name:  "a message with no anchor goes last, after the last metadata",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{{Op: format.StructuralInsertBlock, Key: "footer", Value: "Footer \"text\""}},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "cart": "Cart",
  "legacy": "Old",
  "@legacy": {
    "description": "Remove after 2.0"
  },
  "footer": "Footer \"text\""
}
`,
		},
		{
			name:  "a message replacing another in its place",
			doc:   structureCatalog,
			edits: []format.StructuralEdit{insAfter("legacy", "current", "New"), del("legacy")},
			want: `{
  "@@locale": "en",
  "greeting": "Hello {name}",
  "@greeting": {
    "description": "Shown on the home page",
    "placeholders": {
      "name": {}
    }
  },
  "cart": "Cart",
  "current": "New"
}
`,
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
	tests := []struct {
		name   string
		edit   format.StructuralEdit
		reason format.StructureReason
	}{
		{"a message id that marks metadata", insAfter("cart", "@cart", "x"), format.StructureUnsupported},
		{"a message with that id exists", insAfter("cart", "legacy", "x"), format.StructureExists},
		{"the anchor is metadata", insAfter("@greeting", "x", "x"), format.StructureNotFound},
		{"the anchor is global metadata", insAfter("@@locale", "x", "x"), format.StructureNotFound},
		{"the anchor is missing", insAfter("nope", "x", "x"), format.StructureNotFound},
		{"removing a message that is not there", del("nope"), format.StructureNotFound},
		{"removing metadata", del("@greeting"), format.StructureNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWriter().EditStructure([]byte(structureCatalog), []format.StructuralEdit{tt.edit})
			var se *format.StructureError
			require.ErrorAs(t, err, &se, "want a StructureError, got %v", err)
			assert.Equal(t, tt.reason, se.Reason, se.Message)
		})
	}
}

func TestWriterDeclaresStructure(t *testing.T) {
	assert.Equal(t, []string{format.StructuralDeleteBlock, format.StructuralInsertBlock}, format.StructuralOps(NewWriter()))
}
