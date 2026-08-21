// Package storeutil holds helpers shared by the PostgreSQL ContentStore
// (package store) and the SQLite ContentStore (package sqlitestore), so the
// two backends cannot drift on stream defaulting, locale serialization,
// block-ID generation, or which rows a deletion reaches. Word counting is
// model.CountWordsInRunsJSON.
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

// BlockScopedTables are the tables whose rows are ABOUT one block, addressed by
// its id, and mean nothing without it: a reviewer's note, a proposed source
// change, a target, an annotation, an overlay, the block's own history. Both
// deletion verbs and both dialects sweep this one list, so a table added to it
// is reached by all four at once — the drift that let notes and source
// proposals outlive their blocks was four copies of one list.
//
// Deliberately absent: change_log records that a block was REMOVED, and a log
// that forgot the removal would be no log; version_blocks names the blocks a
// version froze, which is what a snapshot is; block_asset_refs has no Go writer
// or reader to orphan anything.
func BlockScopedTables() []string {
	return []string{
		"translations",
		"annotations",
		"overlays_ext",
		"block_history",
		"block_notes",
		"proposed_source_changes",
	}
}

// StreamScopedTables are the tables whose rows belong to ONE stream, addressed
// by (project_id, stream). A deleted stream is erased, not retained: the rows
// describe work on a stream that no longer exists, and a stream created again
// under the same name must start empty rather than inherit a dead one's items,
// targets, notes, decisions and change log. stream_members and stream_tags are
// absent because a foreign key on streams already cascades them, on both
// backends.
//
// unit_decisions is included and change_log with it: the ledger and the log are
// per-stream, and a recreated stream inheriting either would read as having
// decided and changed things nobody did on it.
func StreamScopedTables() []string {
	return []string{
		"items",
		// A stream owns its blocks. Before it did, deleting a stream had to
		// leave block rows behind and sweep for ones no surviving item named —
		// a reclaim pass that existed only because two streams shared a row.
		"blocks",
		"translations",
		"annotations",
		"overlays_ext",
		"block_history",
		"block_notes",
		"change_log",
		"unit_decisions",
		"proposed_source_changes",
	}
}

// ProjectScopedTablesWithoutCascade are the project's content-describing tables
// that carry a bare project_id column with no foreign key to projects, so a
// project delete has to reach them explicitly — the cascade never will.
func ProjectScopedTablesWithoutCascade() []string {
	return []string{
		"change_log",
		"block_history",
		"block_notes",
		"block_asset_refs",
		"unit_decisions",
		"proposed_source_changes",
	}
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
