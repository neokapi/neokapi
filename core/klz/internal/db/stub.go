//go:build !klzcache

package db

import (
	"context"
	"errors"
	"time"
)

// Source is the iterator the Phase-4 cache builder consumes. Exposed
// here so callers compile identically with or without the klzcache
// tag. The stub build ignores it.
type Source interface {
	Blocks() ([]BlockRow, error)
	Targets() ([]TargetRow, error)
	SourceLocale() string
}

// BlockRow mirrors the tagged-build type so callers can construct
// Source implementations without branching on build tags.
type BlockRow struct {
	ID                   string
	DocumentPath         string
	Hash                 string
	Type                 string
	Component            string
	JSXPath              string
	OptionalPlaceholders int
	RequiredPlaceholders int
	SourceText           string
	SourceJSON           string
	Context              string
}

// TargetRow mirrors the tagged-build type.
type TargetRow struct {
	BlockID      string
	Locale       string
	TargetJSON   string
	Status       string
	Origin       string
	OriginDetail string
}

// GCReport mirrors the tagged-build type so callers can read the
// report struct fields directly.
type GCReport struct {
	Root           string
	TotalEntries   int
	TotalBytes     int64
	EvictedEntries int
	EvictedBytes   int64
}

// Entry mirrors the tagged-build type.
type Entry struct {
	Path  string
	Hash  string
	Bytes int64
	Atime time.Time
	Mtime time.Time
}

// Open returns an unimplemented Cache stub. Without the klzcache tag
// every query returns ErrNotImplemented so callers can still link
// against the internal/db package in pure-JSON builds.
func Open(_ context.Context, manifestHash string) (Cache, error) {
	return &noopCache{hash: manifestHash}, nil
}

// Build is a placeholder in untagged builds. It returns a friendly
// error so `kapi cache warm` has something to print.
func Build(_ context.Context, manifestHash string, _ Source) error {
	return errors.New("klz cache build requires the klzcache build tag (see RFC 0001); manifest " + manifestHash)
}

// GC is a placeholder in untagged builds.
func GC(_ context.Context, _ int64, _ time.Duration) (GCReport, error) {
	return GCReport{Root: CacheRoot()}, errors.New("klz cache gc requires the klzcache build tag (see RFC 0001)")
}

// Entries is a placeholder in untagged builds.
func Entries() ([]Entry, error) { return nil, nil }

type noopCache struct{ hash string }

func (c *noopCache) BlockByID(_ context.Context, _ string) ([]byte, error) {
	return nil, &ErrNotImplemented{Op: "BlockByID"}
}

func (c *noopCache) SimilarSources(_ context.Context, _, _ string, _ int) ([]string, error) {
	return nil, &ErrNotImplemented{Op: "SimilarSources"}
}

func (c *noopCache) Close() error { return nil }
