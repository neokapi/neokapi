// Package storeutil holds helpers shared by the PostgreSQL ContentStore
// (package store) and the SQLite ContentStore (package sqlitestore), so the
// two backends cannot drift on stream defaulting, locale serialization, or
// block-ID generation. Word counting is model.CountWordsInRunsJSON.
package storeutil

import (
	"strings"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// NewBlockID generates a short random block ID.
func NewBlockID() string { return id.New() }

// DefaultStream returns "main" when stream is empty.
func DefaultStream(stream string) string {
	if stream == "" {
		return "main"
	}
	return stream
}

// JoinLocales serializes locales as a comma-separated column value.
func JoinLocales(locales []model.LocaleID) string {
	parts := make([]string, len(locales))
	for i, l := range locales {
		parts[i] = string(l)
	}
	return strings.Join(parts, ",")
}

// SplitLocales parses a comma-separated column value into locale IDs.
func SplitLocales(s string) []model.LocaleID {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	locales := make([]model.LocaleID, len(parts))
	for i, p := range parts {
		locales[i] = model.LocaleID(strings.TrimSpace(p))
	}
	return locales
}
