package html

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Open Graph is RDFa and spells its key `property="og:title"`, not `name=`.
// translatableMetaNames has listed the og: keys since it was written, and the
// lookup read only `name`, so those entries could never match: a landing page's
// tab title and social card went out in the source language in every locale,
// and the allowlist said otherwise.
func TestMetaKeyReadsPropertyForOpenGraph(t *testing.T) {
	cases := []struct {
		name     string
		attrName string
		property string
		want     string
	}{
		{"open graph uses property", "", "og:title", "og:title"},
		{"twitter cards use name", "twitter:title", "", "twitter:title"},
		{"og spelled with name is accepted too", "og:description", "", "og:description"},
		{"an ordinary meta name", "description", "", "description"},
		{"name wins when both are present", "description", "og:title", "description"},
		{"case is folded", "", "OG:Title", "og:title"},
		{"neither is a key at all", "", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, metaKey(tc.attrName, tc.property))
		})
	}
}

// And the point of the key: the entries in the table are now reachable.
func TestOpenGraphContentIsTranslatable(t *testing.T) {
	for _, key := range []string{"og:title", "og:description", "og:site_name",
		"twitter:title", "twitter:description"} {
		assert.True(t, translatableMetaNames[metaKey("", key)],
			"%s is listed as translatable and must be reachable through property=", key)
	}
	assert.False(t, translatableMetaNames[metaKey("", "og:url")],
		"a URL is not prose")
	assert.False(t, translatableMetaNames[metaKey("", "og:image")])
}
