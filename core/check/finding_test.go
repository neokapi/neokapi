package check

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeight(t *testing.T) {
	tests := []struct {
		name string
		f    Finding
		want int
	}{
		{"failing", Finding{Fails: true}, FailingWeight},
		{"reporting", Finding{}, ReportingWeight},
		{"suggested weighs nothing even when marked failing", Finding{Fails: true, Suggested: true}, 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Weight(tt.f), tt.name)
	}
}

func TestFindingJSONUsesCategory(t *testing.T) {
	f := Finding{
		Category:     "terminology",
		Fails:        true,
		Message:      "forbidden term",
		Suggestion:   "use this instead",
		OriginalText: "leverage",
		Metadata:     map[string]string{"rule": "vocab-1"},
	}
	b, err := json.Marshal(f)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"category":"terminology"`)
	assert.Contains(t, string(b), `"fails":true`)

	var back Finding
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, f.Category, back.Category)
	assert.Equal(t, f.Fails, back.Fails)
	assert.Equal(t, "vocab-1", back.Metadata["rule"])
}
