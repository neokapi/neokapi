package model

// DisplayHint provides rendering guidance for UI tools working with blocks.
type DisplayHint struct {
	Preview     string // Short preview of surrounding context
	Context     string // Description of where this block appears
	MaxLength   int    // Maximum character count for the translation (0 = unlimited)
	ContentType string // Hint about content type (e.g. "heading", "button", "paragraph")
}
