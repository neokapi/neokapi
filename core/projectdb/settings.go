package projectdb

import (
	"context"
	"fmt"
)

// A project setting is a team choice about the project that is neither content
// nor governance: a saved view of it, say. It belongs with the project rather
// than with one checkout, so it lives in the context store beside the terms and
// the voice profiles, where every checkout of the project reads one answer and
// whatever carries the context store between machines carries it too.
//
// A setting is one value under a name, stored as metadata of the context store
// under the "setting." namespace. The caller owns the value's encoding.

// SettingSavedFilters holds the saved filters a project's team shares: named
// narrowings of the project to some collections, paths and languages. The
// desktop app writes it, as a JSON array.
const SettingSavedFilters = "saved-filters"

// settingKeyPrefix namespaces project settings among the context store's
// metadata keys.
const settingKeyPrefix = "setting."

// Setting reads one project setting. ok is false when it was never written.
// Returns ErrNoStore on a build with no file-backed store.
func (d *DB) Setting(ctx context.Context, name string) (value string, ok bool, err error) {
	value, ok, err = d.ContextMeta(ctx, settingKeyPrefix+name)
	if err != nil {
		return "", false, fmt.Errorf("projectdb: read setting %q: %w", name, err)
	}
	return value, ok, nil
}

// PutSetting writes one project setting, replacing any previous value.
func (d *DB) PutSetting(ctx context.Context, name, value string) error {
	if err := d.PutContextMeta(ctx, settingKeyPrefix+name, value); err != nil {
		return fmt.Errorf("projectdb: write setting %q: %w", name, err)
	}
	return nil
}
