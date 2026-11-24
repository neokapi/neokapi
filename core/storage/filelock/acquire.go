package filelock

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A directory that keeps one lock file per thing a lock was ever taken on
// grows without end unless something removes the unused ones. Sweep removes
// a file only while it holds the lock on it and the path still names it.
// Acquire, once it holds the lock, checks that the path still names the file
// it locked, and opens the path again when it does not.

// So a holder past its check holds the lock on the file the path names, and
// the path keeps naming that file while the lock is held, because a sweep
// cannot take the lock to remove it. Two holders on one path hold the lock on
// one file, which the kernel lets only one of them do.

// maxAcquireAttempts bounds how many times Acquire opens the path again. A
// sweep removes only a file untouched for its whole age, and the file Acquire
// opens again is new, so a second removal needs a second sweep of a file
// created a moment ago.
const maxAcquireAttempts = 32

// Acquire opens the lock file at path, takes the lock on it, and returns the
// held Lock, on a file the path still names. A sweep may remove the file
// while Acquire waits for the lock on it, and a lock on a removed file
// excludes nobody, so Acquire then opens the path again. Each acquisition
// marks the file as used now, which keeps Sweep away from it for its age.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	for range maxAcquireAttempts {
		l, err := Open(path)
		if err != nil {
			return nil, err
		}
		if err := l.Lock(ctx); err != nil {
			_ = l.Close()
			return nil, err
		}
		current, err := l.current()
		if err != nil {
			l.Unlock()
			_ = l.Close()
			return nil, err
		}
		if current {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
			return l, nil
		}
		l.Unlock()
		_ = l.Close()
	}
	return nil, fmt.Errorf("take the lock on %s: the lock file was removed %d times while waiting for it", path, maxAcquireAttempts)
}

// current reports whether the path still names the file l holds open.
func (l *Lock) current() (bool, error) {
	held, err := l.file.Stat()
	if err != nil {
		return false, err
	}
	named, err := os.Stat(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(held, named), nil
}

// Sweep removes, from dir, lock files whose time stamp is older than maxAge
// and that no one holds, looking at no more than limit entries. A file
// someone holds, or that changed while the sweep looked at it, stays. It
// returns how many files it removed.
func Sweep(dir string, maxAge time.Duration, limit int) (int, error) {
	d, err := os.Open(dir)
	if err != nil {
		return 0, err
	}
	entries, err := d.ReadDir(limit)
	_ = d.Close()
	if err != nil && len(entries) == 0 {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < maxAge {
			continue
		}
		if sweepOne(filepath.Join(dir, e.Name()), maxAge) {
			removed++
		}
	}
	return removed, nil
}

// sweepOne removes the lock file at path when it can take the lock on it
// without waiting, the path still names the file it locked, and the file is
// still older than maxAge.
func sweepOne(path string, maxAge time.Duration) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	l := &Lock{path: path, file: f}
	held, err := l.tryLock()
	if err != nil || !held {
		_ = l.Close()
		return false
	}
	current, err := l.current()
	if err != nil || !current {
		l.Unlock()
		_ = l.Close()
		return false
	}
	info, err := f.Stat()
	if err != nil || time.Since(info.ModTime()) < maxAge {
		l.Unlock()
		_ = l.Close()
		return false
	}
	return l.removeHeld() == nil
}

// tryLock takes the lock when no one holds it, and reports whether it did.
func (l *Lock) tryLock() (bool, error) {
	if !l.mu.TryLock() {
		return false, nil
	}
	held, err := tryLockFile(l.file)
	if err != nil || !held {
		l.mu.Unlock()
		return false, err
	}
	return true, nil
}

// sweptMarker is the file in a lock directory whose time stamp says when it
// was last swept.
const sweptMarker = ".swept"

// SweepDue sweeps dir (Sweep with maxAge and limit) when it was last swept
// longer than every ago, or never. The marker is moved first, so processes
// that take locks at the same moment rarely sweep together; when they do,
// each removal is still safe.
func SweepDue(dir string, every, maxAge time.Duration, limit int) {
	marker := filepath.Join(dir, sweptMarker)
	if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < every {
		return
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		return
	}
	now := time.Now()
	_ = os.Chtimes(marker, now, now)
	_, _ = Sweep(dir, maxAge, limit)
}
