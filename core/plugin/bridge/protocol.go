// Package bridge implements a JVM subprocess manager that wraps Okapi Framework
// Java filters as gokapi DataFormatReader/DataFormatWriter implementations.
// Communication uses gRPC with proto-generated types.
package bridge

import (
	"encoding/json"
	"fmt"

	pb "github.com/gokapi/gokapi/core/plugin/proto/v2"
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
// Complex values are JSON-encoded.
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
