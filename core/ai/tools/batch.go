package tools

import (
	"slices"

	"golang.org/x/sync/errgroup"
)

// chunkBlocks splits items into consecutive batches of at most size. A size
// below 1 is treated as 1 (one item per batch). It is a thin wrapper around
// slices.Chunk so the AI tools share one batching definition.
func chunkBlocks[T any](items []T, size int) [][]T {
	if size < 1 {
		size = 1
	}
	return slices.Collect(slices.Chunk(items, size))
}

// goBatches runs fn for each batch with up to concurrency batches in flight,
// returning the first error any batch reports (errgroup semantics). A
// concurrency below 1 runs sequentially. fn receives the batch index so callers
// can scatter results into a pre-sized slot without a mutex. Batches are not
// cancelled on a sibling's error, matching the hand-rolled sem+WaitGroup loop
// this replaces; cancellation stays the caller's responsibility via the context
// they close over in fn.
func goBatches[T any](batches [][]T, concurrency int, fn func(idx int, batch []T) error) error {
	var g errgroup.Group
	g.SetLimit(max(concurrency, 1))
	for idx, batch := range batches {
		g.Go(func() error { return fn(idx, batch) })
	}
	return g.Wait()
}
