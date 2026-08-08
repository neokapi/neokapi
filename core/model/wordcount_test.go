package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountWords(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"two words", "Hello world", 2},
		{"leading and trailing space", "  leading spaces  ", 2},
		{"empty", "", 0},
		{"one word", "one", 1},
		{"collapsed runs of space", "multiple   spaces   between", 3},
		{"PUA markers break words, not counted", "one\ue001two three", 3},
		{"PUA marker inside a token splits it", "wor\ue000d", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, CountWords(tt.text))
		})
	}
}

func TestCountWordsInRunsJSONMalformed(t *testing.T) {
	assert.Equal(t, 0, CountWordsInRunsJSON("not json"))
	assert.Equal(t, 0, CountWordsInRunsJSON(""))
}

// TestWordCountAgreesAcrossPaths pins the acceptance for the word-count
// consolidation: a block carrying a plural form and an inline code counts the
// same whether the count comes from the rehydrated Block (the editor API path)
// or from the serialized source runs (the store-column path). The plural
// descends into its 'other' branch and the inline code contributes nothing.
func TestWordCountAgreesAcrossPaths(t *testing.T) {
	source := []Run{
		TextR("You have "),
		PhR(PlaceholderRun{ID: "1", Type: "icu-pivot", Equiv: "count"}),
		TextR(" "),
		PluralR(PluralRun{
			Pivot: "count",
			Forms: map[PluralForm][]Run{
				"one":       {TextR("new message")},
				PluralOther: {TextR("new messages waiting")},
			},
		}),
	}
	block := &Block{ID: "b1", Translatable: true, Source: source}

	// Editor API path: Block.WordCount over rehydrated runs.
	// "You have  new messages waiting" -> 5 words (inline code dropped, plural
	// descends to 'other').
	blockCount := block.WordCount()
	assert.Equal(t, 5, blockCount)

	// Store-column path: count from the serialized source runs directly.
	sourceJSON, err := json.Marshal(source)
	require.NoError(t, err)
	jsonCount := CountWordsInRunsJSON(string(sourceJSON))

	assert.Equal(t, blockCount, jsonCount,
		"store-column and editor-API word counts must agree")
	assert.Positive(t, jsonCount, "a plural block counts its words, not zero")
}
