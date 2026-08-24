package translatability

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		element string
		want    Class
		why     string
	}{
		{"p", Yes, "prose"},
		{"h1", Yes, "prose"},
		{"li", Yes, "prose"},
		{"figcaption", Yes, "prose"},

		{"code", No, "a command or identifier, not copy"},
		{"kbd", No, "keys the reader presses"},
		{"samp", No, "sample output"},
		{"var", No, "a variable name"},
		{"pre", No, "preformatted"},
		{"script", No, "code"},
		{"style", No, "code"},
		{"textarea", No, "machine input"},

		{"div", Container, "flow content"},
		{"section", Container, "flow content"},

		// Every React component lands here, which is the point: the caller
		// promotes it and reports the promotion rather than skipping it.
		{"TabItem", Container, "an unmapped component"},
		{"Tabs", Container, "an unmapped component"},
		{"SomethingNobodyHasSeen", Container, "an unmapped component"},
	}
	for _, tc := range tests {
		t.Run(tc.element, func(t *testing.T) {
			assert.Equal(t, tc.want, Classify(tc.element), tc.why)
		})
	}
}

func TestIsTranslatableAttribute(t *testing.T) {
	// HTML and ARIA attributes carry visible text on anything.
	assert.True(t, IsTranslatableAttribute("alt", "img"))
	assert.True(t, IsTranslatableAttribute("title", "div"))
	assert.True(t, IsTranslatableAttribute("aria-label", "button"))
	assert.True(t, IsTranslatableAttribute("placeholder", "input"))

	// Component prop conventions count only on PascalCase components: on a
	// plain element these names are usually DOM props or data fields.
	assert.True(t, IsTranslatableAttribute("label", "TabItem"))
	assert.True(t, IsTranslatableAttribute("description", "PageHeader"))
	assert.False(t, IsTranslatableAttribute("label", "option"),
		"a plain <option label=…> is a DOM prop, not copy")
	assert.False(t, IsTranslatableAttribute("description", "meta"))

	assert.False(t, IsTranslatableAttribute("href", "a"))
	assert.False(t, IsTranslatableAttribute("value", "TabItem"))
	assert.False(t, IsTranslatableAttribute("label", ""))
}

func TestIsInline(t *testing.T) {
	assert.True(t, IsInline("em"))
	assert.True(t, IsInline("strong"))
	assert.False(t, IsInline("div"))
}

func TestIsComponent(t *testing.T) {
	assert.True(t, IsComponent("TabItem"))
	assert.False(t, IsComponent("div"))
	assert.False(t, IsComponent(""))
}

// The embedded table is generated from the TypeScript source; if it were empty
// or truncated every Classify call would answer Container and nothing would be
// protected, so assert the shape rather than trusting the build.
func TestEmbeddedTableIsPopulated(t *testing.T) {
	s := get()
	assert.NotEmpty(t, s.nonTranslatable)
	assert.NotEmpty(t, s.translatable)
	assert.NotEmpty(t, s.inline)
	assert.NotEmpty(t, s.htmlAttrs)
	assert.NotEmpty(t, s.componentAttrs)

	// The four that decide inline code, named explicitly so a regeneration that
	// drops them fails here rather than in a translated document.
	for _, e := range []string{"code", "kbd", "samp", "var"} {
		assert.True(t, s.nonTranslatable[e], "%s must stay non-translatable", e)
	}
}
