package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A refused remove_edition says why in the terms of the format refused: a
// format kapi writes back is told its writer takes no block out of a file,
// with no claim about where the format keeps its languages, and a format
// kapi only reads is told the operation is not supported.
func TestDescription_RemoveEditionRefusalSaysWhy(t *testing.T) {
	remove := Op{Kind: KindRemoveEdition}
	tests := []struct {
		name    string
		facts   FormatFacts
		says    string
		saysNot []string
	}{
		{name: "a format whose writer takes no block out", facts: FormatFacts{Name: "markdown", Editable: true},
			says: "the markdown writer takes no block out of a file", saysNot: []string{"file of its own"}},
		{name: "a format that keeps every language in one file", facts: FormatFacts{Name: "xcstrings", Editable: true},
			says: "the xcstrings writer takes no block out of a file", saysNot: []string{"file of its own", "each translation"}},
		{name: "a format kapi only reads", facts: FormatFacts{Name: "pdf"},
			says: "the pdf format does not support remove_edition", saysNot: []string{"writer"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := DescribeFormat(tc.facts)
			require.Equal(t, EditionsPerFile, d.Editions)
			err := d.supports(remove)
			require.NotNil(t, err)
			assert.Equal(t, CodeUnsupported, err.Code)
			assert.Equal(t, string(KindRemoveEdition), err.Capability)
			assert.Contains(t, err.Message, tc.says)
			for _, s := range tc.saysNot {
				assert.NotContains(t, err.Message, s)
			}
		})
	}
}
