package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A link target is refused a URL that runs code where the document is read,
// spelled however a browser would still read it as one.
func TestUnsafeURLScheme(t *testing.T) {
	tests := []struct {
		value  string
		scheme string
		unsafe bool
	}{
		{"javascript:alert(1)", "javascript", true},
		{"JavaScript:alert(1)", "javascript", true},
		{"  javascript:alert(1)", "javascript", true},
		{"\x01javascript:alert(1)", "javascript", true},
		{"java\tscript:alert(1)", "javascript", true},
		{"java\nscr\ript:alert(1)", "javascript", true},
		{"vbscript:msgbox(1)", "vbscript", true},
		{"VBScript:msgbox(1)", "vbscript", true},
		{"data:text/html,<script>alert(1)</script>", "data", true},
		{"data:;base64,PHNjcmlwdD4=", "data", true},
		{"DATA:text/html,x", "data", true},
		{"data:image/png;base64,iVBORw0KGgo=", "", false},
		{"data:IMAGE/gif;base64,R0lGOD==", "", false},
		{"https://example.com/guide", "", false},
		{"http://example.com/?q=javascript:alert(1)", "", false},
		{"mailto:help@example.com", "", false},
		{"/docs/guide.html", "", false},
		{"guide.html#javascript:x", "", false},
		{"./javascript:x", "", false},
		{"", "", false},
		{"jav&#x61;script:alert(1)", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			scheme, unsafe := unsafeURLScheme(tt.value)
			assert.Equal(t, tt.unsafe, unsafe)
			if tt.unsafe {
				assert.Equal(t, tt.scheme, scheme)
			}
		})
	}
}

// The attributes whose value is a URL a reader follows or loads, in any
// namespace and either case.
func TestIsLinkTarget(t *testing.T) {
	for _, name := range []string{"href", "src", "HREF", "xlink:href", "x:src"} {
		assert.True(t, isLinkTarget(name), name)
	}
	for _, name := range []string{"title", "alt", "rel", "target", "hreflang", "srcset-x", "xml:lang"} {
		assert.False(t, isLinkTarget(name), name)
	}
}
