package model

// Layer is a top-level structural grouping: a document, a section, or embedded content.
// Layers can nest — embedded content (HTML inside JSON, CDATA in XML) becomes
// a child Layer with its own DataFormat.
type Layer struct {
	ID             string
	Name           string
	Format         string // DataFormat ID (e.g., "html", "json", "xml"). Empty = same as parent.
	Locale         LocaleID
	Encoding       string
	MimeType       string
	LineBreak      string
	IsMultilingual bool
	ParentID       string // ID of the parent Layer (empty for root)
	Properties     map[string]string
	Overlays       []Facet // stand-off facet carrier for layer-scoped format metadata
	HasBOM         bool    // Whether the document has a byte order mark
}

// ResourceID returns the Layer's unique identifier.
func (l *Layer) ResourceID() string { return l.ID }

// IsRoot returns true if this is a root (document-level) Layer.
func (l *Layer) IsRoot() bool { return l.ParentID == "" }

// IsEmbedded returns true if this Layer represents embedded content with a different format.
func (l *Layer) IsEmbedded() bool { return l.ParentID != "" && l.Format != "" }
