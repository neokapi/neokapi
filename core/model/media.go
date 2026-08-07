package model

import (
	"bytes"
	"errors"
	"io"
)

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

	// Open, when set, produces the bytes on demand instead of holding them in
	// Data. A package format's embedded media is the bulk of the file and is
	// wanted by almost nothing — a conversion to Markdown or HTML references
	// images rather than embedding them, and a check or translate flow only
	// touches text — so materializing every asset eagerly would keep tens or
	// hundreds of megabytes resident for the whole run to serve a consumer that
	// never asks.
	//
	// It is a process-local accessor: it is not serialized, does not cross the
	// plugin wire, and is dropped by any copy that leaves the reader. It stays
	// valid only while the document it came from is open, so a consumer that
	// needs the bytes to outlive that must call [Media.Materialize]. Read
	// through [Media.Bytes] or [Media.Content] rather than touching Data, so a
	// deferred slice resolves.
	Open func() (io.ReadCloser, error) `json:"-"`
}

// ResourceID returns the Media's unique identifier.
func (m *Media) ResourceID() string { return m.ID }

// HasContent reports whether the media carries bytes this process can read —
// inline or deferred. It answers without opening anything, so it is the cheap
// predicate for "is there something here to write out".
func (m *Media) HasContent() bool {
	return m != nil && (len(m.Data) > 0 || m.Open != nil)
}

// Content returns a stream over the media's bytes: inline Data when present,
// otherwise the deferred accessor. The caller closes it. Use this over [Bytes]
// when the bytes are being copied somewhere — a zip entry, an upload — so a
// large asset is never fully resident.
func (m *Media) Content() (io.ReadCloser, error) {
	if m == nil {
		return nil, errors.New("model: nil media")
	}
	if len(m.Data) > 0 {
		return io.NopCloser(bytes.NewReader(m.Data)), nil
	}
	if m.Open != nil {
		return m.Open()
	}
	return nil, errors.New("model: media has no inline or deferred content")
}

// Bytes returns the media's content in full: inline Data when present,
// otherwise the deferred accessor materialized. It is the read path for
// consumers that genuinely need a []byte — reading Data directly sees nothing
// for a deferred slice.
//
// It does not cache into Data: the caller that materializes is the one that
// decides how long the bytes live, and caching here would reintroduce exactly
// the resident copy the deferred accessor exists to avoid. Call [Materialize]
// when the bytes should stay.
func (m *Media) Bytes() ([]byte, error) {
	rc, err := m.Content()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Materialize resolves a deferred slice into inline Data so it survives the
// document that produced it. It is a no-op for media that is already inline or
// carries no deferred accessor.
func (m *Media) Materialize() error {
	if m == nil || m.Open == nil || len(m.Data) > 0 {
		return nil
	}
	data, err := m.Bytes()
	if err != nil {
		return err
	}
	m.Data = data
	m.Open = nil
	return nil
}
