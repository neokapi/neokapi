package contextual

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// ParagraphSpans returns ordinal paragraph IDs and exact UTF-8 byte ranges
// without requiring a review request. IDs are stable only within the exact
// supplied text. Blank lines separate paragraphs; internal line endings remain.
func ParagraphSpans(text string) ([]CandidateSpan, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("paragraph text must be valid UTF-8")
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("paragraph text must not be empty")
	}
	return candidateParagraphs(text), nil
}
