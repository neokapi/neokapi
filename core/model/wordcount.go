package model

import (
	"encoding/json"
	"unicode"
)

// CountWords counts whitespace-delimited words in text, treating Unicode
// Private Use Area runes (U+E000–U+F8FF) as word breaks rather than word
// characters. Those runes are the inline-span markers the content model embeds
// in coded text, so counting them as letters would inflate the total.
//
// This is the single word-counting rule shared by the block model, the content
// stores, and the check tools, so one project can never report two different
// word counts for the same content.
func CountWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || isPUAMarker(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// CountWordsInRunsJSON counts words in a serialized []Run source sequence
// without rehydrating a Block. The runs are flattened with RunsText — text
// verbatim, inline codes dropped, plural/select descending into their 'other'
// branch — so a structured block counts its words rather than zero. Malformed
// JSON counts as zero.
func CountWordsInRunsJSON(runsJSON string) int {
	var runs []Run
	if err := json.Unmarshal([]byte(runsJSON), &runs); err != nil {
		return 0
	}
	return CountWords(RunsText(runs))
}

// isPUAMarker reports whether r is in the Unicode Private Use Area the content
// model uses to encode inline spans in coded text.
func isPUAMarker(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}
