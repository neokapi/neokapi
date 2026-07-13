package designtokens

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestScanDeprecatedStreamParity asserts the streaming $deprecated scan produces
// exactly the same maps as the buffered scanDeprecated across nesting, arrays,
// the $description branch, and boolean (non-string) $deprecated.
func TestScanDeprecatedStreamParity(t *testing.T) {
	docs := []string{
		`{"color":{"$value":"#fff","$type":"color","$deprecated":"use accent"}}`,
		`{"g":{"$description":"grp","token":{"$value":"1","$type":"dimension","$deprecated":"gone","$description":"old dim"}}}`,
		`{"a":{"$value":"1","$type":"x","$deprecated":true}}`, // boolean $deprecated — not collected
		`{"list":[{"$value":"1","$type":"x","$deprecated":"d0"},{"$value":"2","$type":"x","$deprecated":"d1","$description":"desc"}]}`,
		`{"deep":{"a":{"b":{"$value":"1","$type":"x","$deprecated":"nested"}}}}`,
		`{"plain":{"$value":"1","$type":"x"}}`, // no $deprecated
	}
	for _, cfgDesc := range []bool{true, false} {
		for _, doc := range docs {
			rb := NewReader()
			rb.cfg.ExtractDescriptions = cfgDesc
			rb.scanDeprecated([]byte(doc))

			rs := NewReader()
			rs.cfg.ExtractDescriptions = cfgDesc
			rs.scanDeprecatedStream(strings.NewReader(doc))

			assert.Equal(t, rb.deprecatedNoteByDesc, rs.deprecatedNoteByDesc, "noteByDesc mismatch (ExtractDescriptions=%v) for %s", cfgDesc, doc)
			assert.Equal(t, rb.deprecatedDataText, rs.deprecatedDataText, "dataText mismatch (ExtractDescriptions=%v) for %s", cfgDesc, doc)
		}
	}
}
