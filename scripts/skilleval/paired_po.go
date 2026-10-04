package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// pairedPOEntry is one entry of a PO catalog as a reader of it sees it: the
// comments above it, and each keyword's string with its continuation lines
// joined.
type pairedPOEntry struct {
	Comments []string
	Fields   map[string]string
}

// pairedPOEntries reads a PO catalog's entries in order. A string spelled
// over several quoted lines reads as the one string they join to, so two
// files that wrap an entry differently hold the same entries.
func pairedPOEntries(body []byte) ([]pairedPOEntry, error) {
	var entries []pairedPOEntry
	var current *pairedPOEntry
	var field string
	flush := func() {
		if current != nil && len(current.Fields) > 0 {
			entries = append(entries, *current)
		}
		current, field = nil, ""
	}
	for n, raw := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "#"):
			if current != nil && len(current.Fields) > 0 {
				flush()
			}
			if current == nil {
				current = &pairedPOEntry{Fields: map[string]string{}}
			}
			current.Comments = append(current.Comments, line)
		case strings.HasPrefix(line, `"`):
			if current == nil || field == "" {
				return nil, fmt.Errorf("line %d: a string continues no keyword", n+1)
			}
			value, err := strconv.Unquote(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			current.Fields[field] += value
		default:
			keyword, rest, ok := strings.Cut(line, " ")
			if !ok {
				return nil, fmt.Errorf("line %d: %q is not a keyword and a string", n+1, line)
			}
			value, err := strconv.Unquote(strings.TrimSpace(rest))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n+1, err)
			}
			if current == nil {
				current = &pairedPOEntry{Fields: map[string]string{}}
			}
			if _, seen := current.Fields[keyword]; seen {
				return nil, fmt.Errorf("line %d: %s twice in one entry", n+1, keyword)
			}
			field = keyword
			current.Fields[field] = value
		}
	}
	flush()
	return entries, nil
}

// pairedPOEntriesMatch compares two catalogs entry by entry and names the
// first difference.
func pairedPOEntriesMatch(want, got []pairedPOEntry) (bool, string, error) {
	if len(want) != len(got) {
		return false, fmt.Sprintf("%d entries, the reference has %d", len(got), len(want)), nil
	}
	for i := range want {
		if !slices.Equal(want[i].Comments, got[i].Comments) {
			return false, fmt.Sprintf("entry %d: comments differ", i+1), nil
		}
		keys := make([]string, 0, len(want[i].Fields))
		for k := range want[i].Fields {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if got[i].Fields[k] != want[i].Fields[k] {
				return false, fmt.Sprintf("entry %d: %s is %q, want %q", i+1, k, pairedClip(got[i].Fields[k]), pairedClip(want[i].Fields[k])), nil
			}
		}
		if len(got[i].Fields) != len(want[i].Fields) {
			return false, fmt.Sprintf("entry %d: %d keywords, the reference has %d", i+1, len(got[i].Fields), len(want[i].Fields)), nil
		}
	}
	return true, "", nil
}
