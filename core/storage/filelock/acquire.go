package filelock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// ErrHeldHere is the error Acquire returns for a lock the calling goroutine
// already holds.
var ErrHeldHere = errors.New("already being written by this process")

// The lock is per open file, so a goroutine that asks again for a lock it
// holds would wait for itself forever. holders records, for each lock path
// Acquire holds, the goroutine that holds it. Acquire refuses at once a path
// its own goroutine holds. Another goroutine of the process waits for the
// lock, as another process does: the holder may finish and let it in.
var holders = struct {
	sync.Mutex
	by map[string]uint64
}{by: map[string]uint64{}}

// Acquire opens the lock file at path, takes the lock on it, and returns the
// held Lock, on a file the path still names. A sweep may remove the file
// while Acquire waits for the lock on it, and a lock on a removed file
// excludes nobody, so Acquire then opens the path again. Each acquisition
// marks the file as used now, which keeps Sweep away from it for its age.
// So Acquire opens the path again at most once in practice: the file it
// opens again is new, and a sweep removes only a file untouched for its age.
// It keeps trying until ctx is done, because a sweep with a short age can
// remove a new file more than once, and the lock is still taken in the end.
//
// A path the calling goroutine already holds through Acquire is refused at
// once with ErrHeldHere, rather than waited for.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	key := holderKey(path)
	me := goroutineID()
	holders.Lock()
	owner, held := holders.by[key]
	holders.Unlock()
	if held && owner == me {
		return nil, fmt.Errorf("take the lock on %s: %w: a write of it started while this goroutine was writing it, and would wait for itself", path, ErrHeldHere)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
			holders.Lock()
			holders.by[key] = me
			holders.Unlock()
			l.holder = key
			return l, nil
		}
		l.Unlock()
		_ = l.Close()
	}
}

// forgetHolder drops the record of l's holder, before the lock is given
// back, so the record never names a goroutine that holds nothing.
func (l *Lock) forgetHolder() {
	if l.holder == "" {
		return
	}
	holders.Lock()
	delete(holders.by, l.holder)
	holders.Unlock()
	l.holder = ""
}

// holderKey is the key of a lock path in holders.
func holderKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// goroutineID is the number the runtime gives the calling goroutine, read
// from the first line of its stack ("goroutine 18 [running]:").
func goroutineID() uint64 {
	var buf [64]byte
	b := buf[:runtime.Stack(buf[:], false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		b = b[:i]
	}
	id, _ := strconv.ParseUint(string(b), 10, 64)
	return id
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
// and that no one holds. It looks at no more than limit entries, starting
// after the first start entries in directory order, and returns how many it
// removed and where the next sweep starts: after the entries it looked at
// and left, or at 0 once it reached the end. A file someone holds, or that changed
// while the sweep looked at it, stays.
func Sweep(dir string, maxAge time.Duration, start, limit int) (removed, next int, err error) {
	d, err := os.Open(dir)
	if err != nil {
		return 0, 0, err
	}
	defer d.Close()
	for skipped := 0; skipped < start; {
		names, err := d.Readdirnames(min(start-skipped, 1024))
		skipped += len(names)
		if errors.Is(err, io.EOF) || len(names) == 0 {
			// The directory shrank below start: begin again from the top.
			return Sweep(dir, maxAge, 0, limit)
		}
		if err != nil {
			return 0, 0, err
		}
	}
	names, err := d.Readdirnames(limit)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, 0, err
	}
	for _, name := range names {
		if !strings.HasSuffix(name, ".lock") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < maxAge {
			continue
		}
		if sweepOne(path, maxAge) {
			removed++
		}
	}
	// The entries removed no longer take a place in the directory.
	next = start + len(names) - removed
	if len(names) < limit {
		next = 0
	}
	return removed, next, nil
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
// was last swept, and whose content is where the next sweep starts.
const sweptMarker = ".swept"

// SweepDue sweeps dir (Sweep with maxAge and limit) when it was last swept
// longer than every ago, or never. Each sweep starts where the last one
// stopped, so a directory larger than limit is swept in turn. The marker is
// moved first, so processes that take locks at the same moment rarely sweep
// together; when they do, each removal is still safe.
func SweepDue(dir string, every, maxAge time.Duration, limit int) {
	marker := filepath.Join(dir, sweptMarker)
	start := 0
	if info, err := os.Stat(marker); err == nil {
		if time.Since(info.ModTime()) < every {
			return
		}
		if b, err := os.ReadFile(marker); err == nil {
			start, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
	}
	if err := os.WriteFile(marker, []byte(strconv.Itoa(start)), 0o600); err != nil {
		return
	}
	_, next, err := Sweep(dir, maxAge, max(start, 0), limit)
	if err != nil {
		return
	}
	_ = os.WriteFile(marker, []byte(strconv.Itoa(next)), 0o600)
}
