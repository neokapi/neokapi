// Package bridge implements a JVM subprocess manager that wraps Okapi Framework
// Java filters as neokapi DataFormatReader/DataFormatWriter implementations.
// Communication uses gRPC with proto-generated types.
package bridge

import (
	"encoding/json"
	"fmt"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
)

// OpenParams are sent with the Open RPC.
type OpenParams struct {
	FilterClass  string
	URI          string
	SourceLocale string
	TargetLocale string
	Encoding     string
	Content      []byte // Raw document bytes
	MimeType     string
	FilterParams map[string]any
	SourcePath   string // Optional absolute file path; Java reads from disk when set
}

// WriteParams are sent with the Write RPC.
type WriteParams struct {
	FilterClass     string
	Parts           []*pb.PartMessage
	Locale          string
	Encoding        string
	OriginalContent []byte
	FilterParams    map[string]any
	SourcePath      string // Optional absolute file path; Java reads from disk when set
}

// WriteStreamParams are sent with the WriteStream method.
// Same as WriteParams but without the pre-collected Parts slice — parts
// are streamed directly from a channel.
type WriteStreamParams struct {
	FilterClass     string
	Locale          string
	Encoding        string
	OriginalContent []byte // Legacy: inline original document bytes
	FilterParams    map[string]any
	SourcePath      string // Optional absolute file path; Java reads from disk when set
	OutputPath      string // When set, Java writes output directly to this path
}

// WriteStreamResult holds the output of a WriteStream call.
// When OutputPath was used, the output bytes are empty and OutputPath
// indicates where Java wrote the file directly.
type WriteStreamResult struct {
	Output     []byte // Inline output bytes (empty when OutputPath was used)
	OutputPath string // Path where output was written (empty when inline)
}

// --- Response data types ---

// InfoData is returned by the Info RPC.
type InfoData struct {
	Name        string
	DisplayName string
	MimeTypes   []string
	Extensions  []string
}

// FilterEntry describes a single available Okapi filter.
type FilterEntry struct {
	FilterClass string
	FilterID    string // Natural Okapi filter ID (e.g., "okf_html")
	Name        string
	DisplayName string
	MimeTypes   []string
	Extensions  []string
}

// ListFiltersData is returned by the ListFilters RPC.
type ListFiltersData struct {
	Filters []FilterEntry
}

// encodeFilterParams converts map[string]any to map[string]string for proto.
// Complex values are JSON-encoded. Parameters should use the hierarchical
// structure matching the filter's JSON schema (section objects with nested
// properties), not flat Okapi parameter names.
func encodeFilterParams(params map[string]any) map[string]string {
	if len(params) == 0 {
		return nil
	}
	result := make(map[string]string, len(params))
	for k, v := range params {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			data, err := json.Marshal(val)
			if err != nil {
				result[k] = fmt.Sprintf("%v", val)
			} else {
				result[k] = string(data)
			}
		}
	}
	return result
}
