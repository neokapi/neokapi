package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A URL is refused when it runs code where the document is read, spelled
// however a browser would still read it as one.
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
		// An SVG image, or any XML, can carry script.
		{"data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+", "data", true},
		{"data:IMAGE/SVG+XML,<svg onload='alert(1)'/>", "data", true},
		{"data: image/svg+xml ;charset=utf-8,<svg/>", "data", true},
		{"data:image/png;base64,iVBORw0KGgo=", "", false},
		{"data:IMAGE/gif;base64,R0lGOD==", "", false},
		{"https://example.com/guide", "", false},
		{"http://example.com/?q=javascript:alert(1)", "", false},
		{"mailto:help@example.com", "", false},
		{"/docs/guide.html", "", false},
		{"guide.html#javascript:x", "", false},
		{"./javascript:x", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			bad, unsafe := unsafeURLScheme(tt.value)
			assert.Equal(t, tt.unsafe, unsafe)
			if tt.unsafe {
				assert.Equal(t, tt.scheme, bad.scheme)
			}
		})
	}
}

// A value is judged as a reader of the document decodes it: a character
// reference (HTML, XML, and a Markdown destination a renderer passes to the
// browser) and a CommonMark backslash escape spell the same scheme as the
// character they stand for.
func TestCheckURLValueReadsReferencesAndEscapes(t *testing.T) {
	refused := []string{
		`javascript\:alert(1)`,
		`&#106;avascript:alert(1)`,
		`&#x6A;avascript:alert(1)`,
		`jav&#x61;script:alert(1)`,
		`javascript&#58;alert(1)`,
		`javascript&colon;alert(1)`,
		`&#x6A;avascript&colon;alert(1)`,
		`java&Tab;script:alert(1)`,
		`java&#9;script:alert(1)`,
		`\&#106;avascript:alert(1)`,
		`&amp;#106;avascript:alert(1)`,
		`vbscript\:msgbox(1)`,
		`data&colon;text/html,x`,
		`data:image/svg&#43;xml,<svg/>`,
	}
	for _, v := range refused {
		err := checkURLValue("href", v, oneURL, "value")
		if assert.NotNil(t, err, "%q is a script URL to a reader of the document", v) {
			assert.Equal(t, CodeInvalid, err.Code)
			assert.Equal(t, "value", err.Field)
		}
	}
	for _, v := range []string{
		`https://example.com/a?b=1&amp;c=2`,
		`https://example.com/a\_b`,
		`guide.html#a:b`,
		`%6Aavascript:alert(1)`,
		`javascript%3Aalert(1)`,
		`data:image/png;base64,iVBORw0KGgo=`,
	} {
		assert.Nil(t, checkURLValue("href", v, oneURL, "value"), v)
	}
}

// An event handler and an iframe's srcdoc hold script whatever their value.
func TestAttrScript(t *testing.T) {
	for _, name := range []string{"onclick", "onLoad", "ONERROR", "svg:onload", "onanimationstart", "srcdoc", "SrcDoc"} {
		assert.True(t, attrScript(name), name)
	}
	for _, name := range []string{"on", "on-call", "on_time", "href", "title", "role", "x:on2"} {
		assert.False(t, attrScript(name), name)
	}
}

// Every attribute whose value a reader follows, loads or submits to as a URL
// is checked, in any namespace and case, and so is each URL of a list.
func TestAttrRunsCode(t *testing.T) {
	refused := []struct{ name, value string }{
		{"href", "javascript:alert(1)"},
		{"xlink:href", "javascript:alert(1)"},
		{"SRC", "javascript:alert(1)"},
		{"action", "javascript:alert(1)"},
		{"formaction", "javascript:alert(1)"},
		{"data", "javascript:alert(1)"},
		{"cite", "javascript:alert(1)"},
		{"poster", "javascript:alert(1)"},
		{"background", "javascript:alert(1)"},
		{"to", "javascript:alert(1)"},
		{"from", "javascript:alert(1)"},
		{"by", "javascript:alert(1)"},
		{"values", "https://example.com/;javascript:alert(1)"},
		{"srcset", "a.png 1x, javascript:alert(1) 2x"},
		{"srcset", "a.png 1x,javascript:alert(1) 2x"},
		{"ping", "https://example.com/p javascript:alert(1)"},
		{"onclick", "track()"},
		{"onclick", ""},
		{"srcdoc", "<p>hi</p>"},
	}
	for _, tt := range refused {
		err := attrRunsCode(tt.name, tt.value, map[string]string{tt.name: tt.value}, "name", "value")
		if assert.NotNil(t, err, "%s=%q", tt.name, tt.value) {
			assert.Equal(t, CodeInvalid, err.Code)
		}
	}
	assert.Equal(t, "name", attrRunsCode("onclick", "x", nil, "name", "value").Field, "an event handler is refused by its name")
	assert.Equal(t, "value", attrRunsCode("href", "javascript:x", nil, "name", "value").Field, "a URL is refused by its value")

	for _, tt := range []struct{ name, value string }{
		{"href", "https://example.com/"},
		{"formaction", "/send"},
		{"srcset", "data:image/png;base64,iVBORw0KGgo= 1x, b.png 2x"},
		{"values", "0;1;0"},
		{"to", "1"},
		{"title", "javascript:alert(1)"},
		{"rel", "javascript:alert(1)"},
	} {
		assert.Nil(t, attrRunsCode(tt.name, tt.value, map[string]string{tt.name: tt.value}, "name", "value"), "%s=%q", tt.name, tt.value)
	}
}

// An SVG animation writes its to, from, by or values into the attribute its
// attributeName names, so pointing it at an href checks the values it holds.
func TestAttrRunsCodeAnimationRetarget(t *testing.T) {
	others := map[string]string{"attributeName": "href", "to": "javascript:alert(1)"}
	err := attrRunsCode("attributeName", "href", others, "name", "value")
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "to attribute")
	assert.NotNil(t, attrRunsCode("attributeName", "xlink:href", map[string]string{"attributeName": "xlink:href", "values": "javascript:alert(1)"}, "name", "value"))
	assert.Nil(t, attrRunsCode("attributeName", "fill", map[string]string{"attributeName": "fill", "to": "red"}, "name", "value"))
	assert.Nil(t, attrRunsCode("attributeName", "href", map[string]string{"attributeName": "href", "to": "#b"}, "name", "value"))
}
