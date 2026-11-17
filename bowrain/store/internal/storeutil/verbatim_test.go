package storeutil

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerbatimInJSON: text the helper calls verbatim is a substring of its
// own JSON encoding, and text it refuses is not.
func TestVerbatimInJSON(t *testing.T) {
	for _, text := range []string{"powerful", "game-changing", "it's worth", "innhaldsminne", "—", "R&D", `say "this"`, `a\b`, "x<y", "line break", "tab\there", ""} {
		enc, err := json.Marshal([]map[string]string{{"text": "before " + text + " after"}})
		require.NoError(t, err)
		verbatim := text != "" && strings.Contains(string(enc), text)
		assert.Equal(t, verbatim, VerbatimInJSON(text), "text %q encodes as %s", text, enc)
	}
}
