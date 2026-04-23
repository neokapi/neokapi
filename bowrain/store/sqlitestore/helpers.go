package sqlitestore

import (
	"encoding/json"
	"unicode"
)

// countWordsFromSourceJSON counts words from serialized source segments
// JSON. Text runs serialize as `{"text":"..."}` per Framework AD-002, so we decode
// the text key as a bare string. Other run kinds contribute nothing.
// Unicode space boundaries define word breaks.
func countWordsFromSourceJSON(sourceJSON string) int {
	var segments []struct {
		Runs []struct {
			Text *string `json:"text,omitempty"`
		} `json:"Runs"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &segments); err != nil {
		return 0
	}
	count := 0
	for _, seg := range segments {
		for _, r := range seg.Runs {
			if r.Text != nil {
				count += countWords(*r.Text)
			}
		}
	}
	return count
}

// countWords counts whitespace-delimited words, skipping PUA marker runes.
func countWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) || isMarker(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

// isMarker checks if a rune is a Unicode Private Use Area marker
// used by the content model for inline span encoding.
func isMarker(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}
