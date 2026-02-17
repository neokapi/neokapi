// Package bridge implements a JVM subprocess manager that wraps Okapi Framework
// Java filters as gokapi DataFormatReader/DataFormatWriter implementations.
// Communication uses newline-delimited JSON (NDJSON) over stdin/stdout.
package bridge

import "encoding/json"

// Command is the NDJSON envelope sent from Go to the Java bridge.
type Command struct {
	Command string `json:"command"`
	Params  any    `json:"params,omitempty"`
}

// Response is the NDJSON envelope received from the Java bridge.
type Response struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// IsOK returns true if the response status is "ok".
func (r *Response) IsOK() bool {
	return r.Status == "ok"
}

// --- Command parameter types ---

// OpenParams are sent with the "open" command.
type OpenParams struct {
	FilterClass   string         `json:"filter_class"`
	URI           string         `json:"uri"`
	SourceLocale  string         `json:"source_locale"`
	Encoding      string         `json:"encoding"`
	ContentBase64 string         `json:"content_base64"`
	MimeType      string         `json:"mime_type"`
	FilterParams  map[string]any `json:"filter_params,omitempty"`
}

// InfoParams are sent with the "info" command.
type InfoParams struct {
	FilterClass string `json:"filter_class"`
}

// WriteParams are sent with the "write" command.
type WriteParams struct {
	FilterClass           string         `json:"filter_class"`
	Parts                 any            `json:"parts"`
	Locale                string         `json:"locale"`
	Encoding              string         `json:"encoding"`
	OriginalContentBase64 string         `json:"original_content_base64"`
	FilterParams          map[string]any `json:"filter_params,omitempty"`
}

// --- Response data types ---

// ReadyData is returned by the Java bridge on startup.
type ReadyData struct {
	Ready bool `json:"ready"`
}

// ReadData is returned by the "read" command.
type ReadData struct {
	Parts json.RawMessage `json:"parts"`
}

// WriteData is returned by the "write" command.
type WriteData struct {
	OutputBase64 string `json:"output_base64"`
}

// InfoData is returned by the "info" command.
type InfoData struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	MimeTypes   []string `json:"mime_types"`
	Extensions  []string `json:"extensions"`
}

// FilterEntry describes a single available Okapi filter.
type FilterEntry struct {
	FilterClass string   `json:"filter_class"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	MimeTypes   []string `json:"mime_types"`
	Extensions  []string `json:"extensions"`
}

// ListFiltersData is returned by the "list_filters" command.
type ListFiltersData struct {
	Filters []FilterEntry `json:"filters"`
}
