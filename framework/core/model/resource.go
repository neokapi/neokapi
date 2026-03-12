package model

// Resource is the interface satisfied by all content payloads carried by a Part.
type Resource interface {
	ResourceID() string
}
