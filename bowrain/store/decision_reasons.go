package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// encodeReasons is a decision's reasons as the store writes them: a JSON
// array, "[]" when there are none.
func encodeReasons(reasons []string) string {
	if len(reasons) == 0 {
		return "[]"
	}
	b, err := json.Marshal(reasons)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// reasonsInto scans a stored reasons column into dst, nil when it holds none.
func reasonsInto(dst *[]string) sql.Scanner { return reasonsScanner{dst} }

type reasonsScanner struct{ dst *[]string }

func (r reasonsScanner) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*r.dst = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("reasons: unexpected %T", src)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("reasons: %w", err)
	}
	if len(out) == 0 {
		out = nil
	}
	*r.dst = out
	return nil
}
