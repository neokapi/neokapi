package model

// Media holds binary or media content (images, embedded objects).
//
// Three storage modes are supported, checked in priority order:
// BlobKey (server-managed blob storage) > URI (external reference) > Data (inline bytes).
type Media struct {
	ID         string
	MimeType   string
	Data       []byte // Inline binary (small assets, pipeline-internal)
	BlobKey    string // Content-addressed key in BlobStore (large assets)
	URI        string // External reference (CDN URL, SAS URL)
	Filename   string // Original filename
	AltText    string // Accessible alternative text
	Size       int64  // Size in bytes
	Properties map[string]string
}

// ResourceID returns the Media's unique identifier.
func (m *Media) ResourceID() string { return m.ID }
