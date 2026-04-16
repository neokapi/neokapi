package sqlitestore

import (
	"encoding/json"
	"unicode"
)

// countWordsFromSourceJSON counts words from serialized source segments
// JSON. Segments carry a Runs slice — text runs contribute their text
// verbatim; other run kinds contribute nothing. Unicode space
// boundaries define word breaks.
func countWordsFromSourceJSON(sourceJSON string) int {
	var segments []struct {
		Runs []struct {
			Text *struct {
				Text string `json:"text"`
			} `json:"text,omitempty"`
		} `json:"Runs"`
	}
	if err := json.Unmarshal([]byte(sourceJSON), &segments); err != nil {
		return 0
	}
	count := 0
	for _, seg := range segments {
		for _, r := range seg.Runs {
			if r.Text != nil {
				count += countWords(r.Text.Text)
			}
		}
	}
	return count
}

// extractTargetLocales returns locale keys from a targets_json string
// (which is a JSON object mapping locale → segments) without deserializing
// the segment values. Only locales with non-empty segment arrays are included.
func extractTargetLocales(targetsJSON string) []string {
	var targets map[string]json.RawMessage
	if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
		return nil
	}
	var locales []string
	for locale, raw := range targets {
		if len(raw) > 2 {
			locales = append(locales, locale)
		}
	}
	return locales
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
