package model

// TextRange represents a character offset range within a text.
// Used by annotations to identify precise positions of terms, entities,
// and other recognized elements in source text.
type TextRange struct {
	Start int // inclusive start offset (0-based)
	End   int // exclusive end offset
}

// Length returns the number of characters in the range.
func (r TextRange) Length() int {
	return r.End - r.Start
}

// IsEmpty returns true if the range has zero length.
func (r TextRange) IsEmpty() bool {
	return r.Start >= r.End
}

// Contains returns true if the given offset falls within this range.
func (r TextRange) Contains(offset int) bool {
	return offset >= r.Start && offset < r.End
}

// Overlaps returns true if this range overlaps with another range.
func (r TextRange) Overlaps(other TextRange) bool {
	return r.Start < other.End && other.Start < r.End
}

// Extract returns the substring at this range from the given text.
// Returns empty string if the range is out of bounds.
func (r TextRange) Extract(text string) string {
	runes := []rune(text)
	if r.Start < 0 || r.End > len(runes) || r.Start >= r.End {
		return ""
	}
	return string(runes[r.Start:r.End])
}
