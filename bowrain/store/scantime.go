package store

import (
	"fmt"
	"time"
)

// scanTime reads a timestamp column that may arrive either as a driver time
// value or as text.
//
// The stores in this package write timestamps as RFC3339 strings, which
// PostgreSQL parses into its TIMESTAMPTZ columns and hands back as time.Time.
// SQLite has no date type: it keeps the string, and its driver only converts
// what a column's declared type says is a date. A column declared TEXT therefore
// comes back as a string and blows up a *time.Time scan — which is how a task
// queue could be written happily and then fail to read back a single row.
//
// scanTime accepts both, so a store's read path does not depend on which
// backend it was handed.
type scanTime struct {
	Time  time.Time
	Valid bool
}

// timeLayouts are the shapes a timestamp column can hold: what this package
// writes (RFC3339 with and without fractional seconds) and what SQLite's own
// datetime() default produces.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

// Scan implements sql.Scanner.
func (s *scanTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		s.Time, s.Valid = time.Time{}, false
		return nil
	case time.Time:
		s.Time, s.Valid = v, true
		return nil
	case []byte:
		return s.parse(string(v))
	case string:
		return s.parse(v)
	default:
		return fmt.Errorf("scan timestamp: unsupported type %T", src)
	}
}

func (s *scanTime) parse(raw string) error {
	if raw == "" {
		s.Time, s.Valid = time.Time{}, false
		return nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			s.Time, s.Valid = t.UTC(), true
			return nil
		}
	}
	return fmt.Errorf("scan timestamp: cannot parse %q", raw)
}
