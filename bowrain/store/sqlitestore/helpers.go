package sqlitestore

import (
	"encoding/json"
	"unicode"
)

// countWordsFromSourceJSON counts words from the serialized source runs JSON.
// source_json now holds the block's flat []model.Run sequence directly. Text
// runs serialize as `{"text":"..."}` per Framework AD-002, so we decode the
// text key as a bare string. Other run kinds contribute nothing. Unicode space
// boundaries define word breaks.
func countWordsFromSourceJSON(sourceJSON string) int {
	var runs []struct {
		Text *string `json:"text,omitempty"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &runs); err != nil {
		return 0
	}
	count := 0
	for _, r := range runs {
		if r.Text != nil {
			count += countWords(*r.Text)
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
