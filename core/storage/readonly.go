package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A process that may only read still has to open the database, and under WAL
// that is not a read-only act. SQLite keeps the write-ahead log's index in a
// shared-memory file beside the database, `<name>-shm`, and it creates that file
// when the first connection arrives. In a directory the process cannot write —
// a check running in a restricted sandbox, a read-only mount, a workspace owned
// by another account — the create fails and the open fails with it, although
// every byte the caller wants is sitting there readable.
//
// OpenReadOnly answers that. It first tries the database where it is, with the
// journal-mode switch skipped and every connection refusing to write. If SQLite
// still cannot open it, the database and its log are copied to a writable
// temporary directory and the copy is opened instead. The copy is a snapshot:
// it answers the reads the caller came for and is deleted when the handle
// closes.

// OpenReadOnly opens a database for reading, from a directory that may not be
// writable.
//
// The handle refuses writes: every connection carries `PRAGMA query_only=ON`,
// so a write reports "attempt to write a readonly database" rather than landing
// somewhere the caller cannot see. Close releases the handle and, where one was
// taken, deletes the snapshot.
func OpenReadOnly(dbPath string) (*DB, error) {
	if err := driverUnavailable(); err != nil {
		return nil, fmt.Errorf("open database %s for reading: %w", dbPath, err)
	}
	db, err := OpenWith(dbPath, Options{ReadOnly: true})
	if err == nil {
		return db, nil
	}
	if isMemoryDSN(dbPath) {
		return nil, err
	}
	snapshot, cleanup, serr := snapshotDatabase(dbPath)
	if serr != nil {
		return nil, fmt.Errorf("open database %s for reading: %w (snapshot: %w)", dbPath, err, serr)
	}
	db, serr = OpenWith(snapshot, Options{ReadOnly: true})
	if serr != nil {
		cleanup()
		return nil, fmt.Errorf("open database %s for reading: %w (snapshot at %s: %w)", dbPath, err, snapshot, serr)
	}
	db.path = dbPath
	db.cleanup = cleanup
	return db, nil
}

// snapshotDatabase copies a database and its write-ahead log into a fresh
// temporary directory and returns the copy's path together with the function
// that removes it.
//
// The log is copied beside the database so SQLite replays it on open and the
// snapshot shows what the last committed transaction wrote. The shared-memory
// file is deliberately NOT copied: it is an index over the log that SQLite
// rebuilds, and a stale one describes a log the copy does not have.
func snapshotDatabase(dbPath string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "kapi-store-read-")
	if err != nil {
		return "", nil, fmt.Errorf("create snapshot directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	target := filepath.Join(dir, filepath.Base(dbPath))
	if err := copyFile(dbPath, target); err != nil {
		cleanup()
		return "", nil, err
	}
	// A missing log is the ordinary case for a cleanly closed database.
	if err := copyFile(dbPath+"-wal", target+"-wal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		cleanup()
		return "", nil, err
	}
	return target, cleanup, nil
}

// copyFile copies src to dst, reporting os.ErrNotExist unwrapped when src is
// absent so a caller can tell an optional file from a failure.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.ErrNotExist
		}
		return fmt.Errorf("read %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// IsPermissionErr reports whether err is the filesystem or SQLite refusing
// because the caller may not write where the database lives.
//
// Neither driver surfaces a portable sentinel through database/sql for the
// SQLite half, so the phrasing is what there is to match on. It is used to
// decide whether to fall back to a read-only open, where a wrong answer costs a
// snapshot rather than a wrong result.
func IsPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"permission denied",
		"read-only file system",
		"readonly database",
		"attempt to write a readonly database",
		"unable to open database file",
		"operation not permitted",
	} {
		if strings.Contains(s, phrase) {
			return true
		}
	}
	return false
}
