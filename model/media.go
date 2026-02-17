package model

// Media holds binary or media content (images, embedded objects).
type Media struct {
	ID         string
	MimeType   string
	Data       []byte
	URI        string // External reference if not inline
	AltText    string // Accessible alternative text
	Properties map[string]string
}

// ResourceID returns the Media's unique identifier.
func (m *Media) ResourceID() string { return m.ID }
