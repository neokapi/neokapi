package format_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/stretchr/testify/assert"
)

func TestSubfilterMapping(t *testing.T) {
	m := format.SubfilterMapping{
		Pattern: "*.body",
		Format:  "html",
	}
	assert.Equal(t, "*.body", m.Pattern)
	assert.Equal(t, "html", m.Format)
}

// The JSON and XML readers route their subfilter lookups through this one
// matcher, so a pattern behaves the same whichever document it is written
// against.
func TestMatchSubfilterPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"exact dotted path", "translations.body", "translations.body", true},
		{"exact path mismatch", "translations.body", "translations.title", false},
		{"star matches one segment", "*.body", "post.body", true},
		{"star does not span segments", "*.body", "a.b.body", false},
		{"star mid-pattern", "translations.*.html", "translations.nb.html", true},
		{"star mid-pattern needs the segment", "translations.*.html", "translations.html", false},
		{"literal root", "body", "body", true},
		{"pattern longer than the path", "a.b.c", "a.b", false},
		{"path longer than the pattern", "a.b", "a.b.c", false},
		{"character class", "item[0-9].text", "item3.text", true},
		{"character class out of range", "item[0-9].text", "itemx.text", false},
		{"question mark matches one character", "a?.body", "ab.body", true},
		{"malformed pattern matches nothing", "[unclosed", "[unclosed", false},
		{"empty pattern matches only an empty path", "", "", true},
		{"empty pattern rejects a path", "", "body", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, format.MatchSubfilterPattern(tt.pattern, tt.path))
		})
	}
}

func TestSubfilterAwareInterface(t *testing.T) {
	// Verify the interface exists and can be used as a type constraint.
	var _ format.SubfilterAware = (*mockSubfilterAware)(nil)
}

type mockSubfilterAware struct{}

func (m *mockSubfilterAware) SetSubfilterResolver(_ format.SubfilterResolver) {}
