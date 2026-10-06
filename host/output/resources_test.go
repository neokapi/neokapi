package output

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceListOutputNouns pins the heading, empty line and total of each
// named-resource kind, so no kind renders as a doubled plural ("termss").
func TestResourceListOutputNouns(t *testing.T) {
	entry := ResourceListEntry{Name: "product", Path: "x", Size: 2048, Modified: time.Unix(0, 0)}
	tests := []struct {
		kind               string
		title, none, total string
	}{
		{"terms", "Named terms stores:", "No named terms stores found.", "Total: 1 terms store(s)"},
		{"memory", "Named content memories:", "No named content memories found.", "Total: 1 content memory(ies)"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			for _, key := range []string{"title", "none", "total"} {
				_, ok := sources["resources."+tt.kind+"."+key]
				require.True(t, ok, "missing chrome key resources.%s.%s", tt.kind, key)
			}

			var empty bytes.Buffer
			require.NoError(t, ResourceListOutput{Kind: tt.kind}.FormatText(&empty))
			assert.Equal(t, tt.none+"\n", empty.String())

			var full bytes.Buffer
			require.NoError(t, ResourceListOutput{Kind: tt.kind, Resources: []ResourceListEntry{entry}, Total: 1}.FormatText(&full))
			out := full.String()
			assert.Contains(t, out, tt.title)
			assert.Contains(t, out, tt.total)
			assert.NotContains(t, out, tt.kind+"s ")
			assert.NotContains(t, out, tt.kind+"s:")
		})
	}
}
