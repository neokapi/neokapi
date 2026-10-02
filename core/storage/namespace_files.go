package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The database files a file system holds. Natively they are every database
// there is; in the browser they are the files a database may be read in from
// (see driver_js.go).

// sqliteMagic opens every SQLite database file.
var sqliteMagic = []byte("SQLite format 3\x00")

// journalSuffixes name the files SQLite keeps beside a database: the
// write-ahead log, its shared-memory index and the rollback journal.
var journalSuffixes = []string{"-wal", "-shm", "-journal"}

// fileExists reports whether a regular file is at path.
func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		return info.Mode().IsRegular(), nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("storage: stat %s: %w", path, err)
	}
}

// removeFiles deletes the file at path and the journal files beside it.
func removeFiles(path string) error {
	for _, suffix := range append([]string{""}, journalSuffixes...) {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("storage: remove %s: %w", path+suffix, err)
		}
	}
	return nil
}

// listFiles returns the database files at or below dir, sorted.
func listFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipAll
			}
			return err
		}
		if !d.Type().IsRegular() || isJournal(path) {
			return nil
		}
		if ok, herr := hasSQLiteHeader(path); herr != nil || !ok {
			return herr
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list databases in %s: %w", dir, err)
	}
	slices.Sort(out)
	return out, nil
}

func isJournal(path string) bool {
	for _, suffix := range journalSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func hasSQLiteHeader(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(sqliteMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return false, nil
	}
	return bytes.Equal(head, sqliteMagic), nil
}
