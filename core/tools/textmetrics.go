package tools

import "strings"

// countWords counts the number of words in a text string.
// Words are sequences of non-whitespace characters separated by whitespace.
func countWords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
