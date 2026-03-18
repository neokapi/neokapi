package service

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// readJSON decodes a JSON response body into dst.
func readJSON(r io.Reader, dst interface{}) error {
	return json.NewDecoder(r).Decode(dst)
}

// stringReader wraps a string as an io.Reader.
func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

// jsonStringArray serializes a []string as a JSON array.
func jsonStringArray(ss []string) string {
	b, err := json.Marshal(ss)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// jsonString serializes a string as a JSON string (with quotes and escaping).
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return string(b)
}
