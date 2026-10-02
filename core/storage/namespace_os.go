//go:build !js

package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Natively a database is a file on disk, with its journal files beside it.

func dbExists(path string) (bool, error) { return fileExists(path) }

func dbRemove(path string) error { return removeFiles(path) }

// dbRename clears the destination's journal files first, because SQLite would
// replay a stale log it found beside the moved database, then moves the
// source's log ahead of the database itself.
func dbRename(from, to string) error {
	for _, suffix := range journalSuffixes {
		if err := os.Remove(to + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("storage: clear %s: %w", to+suffix, err)
		}
	}
	for _, suffix := range journalSuffixes {
		if err := os.Rename(from+suffix, to+suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("storage: move %s: %w", from+suffix, err)
		}
	}
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("storage: move %s: %w", from, err)
	}
	return nil
}

func dbList(dir string) ([]string, error) { return listFiles(dir) }
