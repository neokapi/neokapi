package safeio

import (
	"context"
	"os"
	"strconv"
	"sync"

	"golang.org/x/sync/semaphore"
)

// In-flight-bytes admission control for file-level fan-out.
//
// The per-document [Budget] bounds what one read may consume, but a caller
// that fans out across files (the CLI processes up to NumCPU files at once)
// multiplies that bound: the real memory envelope is concurrency × per-file
// peak. An [Admission] caps the *total* bytes admitted into flight across a
// fan-out, sized in bytes rather than file slots, so many small files still
// run wide while a few huge files serialize.
const (
	// DefaultMaxInflightBytes is the default total in-flight byte budget for
	// a file fan-out. 2 GiB is twice the per-document [DefaultMaxBytes]: wide
	// enough that ordinary localization batches never queue, while bounding
	// the worst case to roughly two maximal documents in flight at once.
	DefaultMaxInflightBytes int64 = 2 << 30 // 2 GiB

	// UnknownFileWeight is the admission weight assumed for an input whose
	// on-disk size cannot be determined (stdin, a stat error, a non-regular
	// file). 64 MiB is far above the typical localization document — erring
	// large keeps an unknown-size flood conservative — while small enough
	// that a handful of unknowns still run concurrently under the default
	// budget.
	UnknownFileWeight int64 = 64 << 20 // 64 MiB

	// MaxInflightBytesEnv names the environment variable that overrides
	// [DefaultMaxInflightBytes]. Unset, zero, or unparsable values fall back
	// to the default; a negative value disables admission control entirely.
	MaxInflightBytesEnv = "KAPI_MAX_INFLIGHT_BYTES"
)

// Admission is a weighted byte semaphore gating how many bytes a fan-out may
// hold in flight at once. Acquire the estimated size of each unit of work
// (see [FileWeight]) before processing it and call the returned release when
// done.
//
// A nil *Admission means "no limit": Acquire succeeds immediately with a
// no-op release, so call sites need no nil checks.
type Admission struct {
	sem   *semaphore.Weighted
	total int64
}

// NewAdmission returns an Admission bounding total in-flight bytes to
// totalBytes. A totalBytes <= 0 returns nil, meaning no limit.
func NewAdmission(totalBytes int64) *Admission {
	if totalBytes <= 0 {
		return nil
	}
	return &Admission{sem: semaphore.NewWeighted(totalBytes), total: totalBytes}
}

// NewAdmissionFromEnv returns the Admission configured by
// [MaxInflightBytesEnv] (a byte count):
//
//   - unset, empty, zero, or unparsable → [DefaultMaxInflightBytes]
//   - negative → nil (admission control disabled)
//   - positive → that many bytes
func NewAdmissionFromEnv() *Admission {
	v := os.Getenv(MaxInflightBytesEnv)
	if v == "" {
		return NewAdmission(DefaultMaxInflightBytes)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n == 0 {
		return NewAdmission(DefaultMaxInflightBytes)
	}
	return NewAdmission(n) // negative → nil (unlimited)
}

// Total returns the configured in-flight byte budget, or 0 when a is nil
// (no limit).
func (a *Admission) Total() int64 {
	if a == nil {
		return 0
	}
	return a.total
}

// Acquire blocks until want bytes of the budget are available (or ctx is
// done) and returns a release function that must be called exactly once when
// the work finishes; calling it more than once is a safe no-op.
//
// want is clamped to the admission total: a file bigger than the whole budget
// acquires the full budget and runs alone — it is never rejected and can
// never deadlock. Its per-document safeio [Budget] still governs the actual
// read. A want <= 0 (an empty file) is clamped to 0 and admitted immediately
// once no larger waiter is queued ahead of it.
//
// On a nil receiver, Acquire returns a no-op release and a nil error.
func (a *Admission) Acquire(ctx context.Context, want int64) (release func(), err error) {
	if a == nil {
		return func() {}, nil
	}
	if want > a.total {
		want = a.total
	}
	if want < 0 {
		want = 0
	}
	if err := a.sem.Acquire(ctx, want); err != nil {
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() { a.sem.Release(want) })
	}, nil
}

// FileWeight estimates the admission weight of one input file: its on-disk
// size. When the size cannot be determined — "-" (stdin), a stat error, or a
// non-regular file (a pipe or device reports no meaningful size) — it returns
// [UnknownFileWeight]. The estimate deliberately ignores in-memory expansion
// (parsed parts cost more than raw bytes); the on-disk size is a cheap,
// monotone proxy that is what the fan-out actually loads up front.
func FileWeight(path string) int64 {
	if path == "" || path == "-" {
		return UnknownFileWeight
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return UnknownFileWeight
	}
	return info.Size()
}
