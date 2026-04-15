//go:build !klzcache

package db

import (
	"context"
	"fmt"
)

// Open returns an unimplemented Cache stub. In Phase 1 (no
// klzcache build tag) all query methods return ErrNotImplemented.
// The Phase 4 SQLite implementation lives in a sibling file guarded
// by `//go:build klzcache`.
func Open(_ context.Context, manifestHash string) (Cache, error) {
	return &noopCache{hash: manifestHash}, nil
}

// Build is a placeholder for the Phase-4 cache-build routine. Used
// by `kapi cache warm`. In Phase 1 it returns a friendly error so
// tooling can wire the CLI entry point now without gating on
// internal/db readiness.
func Build(_ context.Context, manifestHash string) error {
	return fmt.Errorf("klz cache build requires the klzcache build tag (phase 4, see RFC 0001); manifest %s", manifestHash)
}

type noopCache struct{ hash string }

func (c *noopCache) BlockByID(_ context.Context, _ string) ([]byte, error) {
	return nil, &ErrNotImplemented{Op: "BlockByID"}
}

func (c *noopCache) SimilarSources(_ context.Context, _, _ string, _ int) ([]string, error) {
	return nil, &ErrNotImplemented{Op: "SimilarSources"}
}

func (c *noopCache) Close() error { return nil }
