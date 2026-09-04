package markdown_test

import "testing"

// TestDocsShapesRoundTrip covers the three shapes that made eight pages of the
// repo's own docs fall back to an opaque region under the MDX reader (#1870):
// an inline code span wrapping across a list-item or blockquote continuation
// line, an HTML entity in link text or prose, and a task-list marker. Each
// must round-trip byte-for-byte through the markdown format itself.
func TestDocsShapesRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"code span wrapping a list continuation", "- An item with `code that\n  wraps` across the break.\n"},
		{"code span wrapping a blockquote continuation", "> A quoted line with `code that\n> wraps` across the break.\n"},
		{"code span wrapping in bold", "**only when `reader.Name() ==\n  writer.Name()`**\n"},
		{"entity in link text", "see [Ship gates &amp; CI](/kapi/recipes/ship-gates-and-ci).\n"},
		{"entity in bold", "Ship gates **&amp; CI** here.\n"},
		{"numeric entities", "Ship gates &#38; CI &#x26; here.\n"},
		{"task list", "- [ ] `config.go` with `Reset()`, `Validate()`, `ApplyMap()`\n- [x] Tag is annotated.\n- [X] Capitalised marker.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSkeletonByteExact(t, tc.input)
		})
	}
}
