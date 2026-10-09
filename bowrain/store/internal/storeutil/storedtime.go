package storeutil

import (
	"database/sql"
	"fmt"
	"time"
)

// SQLiteTimestampLayout is the layout SQLite's datetime('now') and
// CURRENT_TIMESTAMP write. Every sqlitestore timestamp column defaults to it,
// so a row inserted without an explicit value carries it beside the RFC3339
// values the store writes itself.
const SQLiteTimestampLayout = "2006-01-02 15:04:05"

// storedTimeLayouts are the layouts a stored timestamp may carry, tried in
// order. Go's parser accepts a fractional second whether or not the layout
// names one, so RFC3339Nano covers both time.RFC3339 and time.RFC3339Nano
// writes.
var storedTimeLayouts = [...]string{time.RFC3339Nano, SQLiteTimestampLayout}

// ParseStoredTime parses a timestamp read back from one of the store's own
// rows. An empty string (a NULL or never-set column) is not an error: it
// yields the zero time. A non-empty value that matches no layout is stored
// corruption, and the error names the column so the scanning function can
// propagate it rather than substitute the zero time, which would read as
// "never" in ordering, staleness and "last changed" views. The error reports
// the RFC3339 failure, the layout the store writes.
func ParseStoredTime(column, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	var firstErr error
	for _, layout := range storedTimeLayouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return time.Time{}, fmt.Errorf("parse %s: %w", column, firstErr)
}

// ParseOptionalStoredTime is ParseStoredTime for a nullable column read into
// an optional time: a NULL or empty value yields nil, and a corrupt value is
// an error.
func ParseOptionalStoredTime(column string, s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := ParseStoredTime(column, s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
