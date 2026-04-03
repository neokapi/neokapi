package flow

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"golang.org/x/sync/errgroup"
)

// BatchFile describes a single file in a batch execution.
type BatchFile struct {
	// Parts is the pre-read content of the file.
	Parts []*model.Part

	// URI identifies the file (used for error messages and collector context).
	URI string

	// TargetLocale is the target locale for this file.
	TargetLocale model.LocaleID
}

// BatchResult holds the output from processing a single file.
type BatchResult struct {
	URI   string
	Parts []*model.Part
}

// BatchConfig configures the BatchExecutor.
type BatchConfig struct {
	FileConcurrency int         // max files processed in parallel (default: 1)
	ChannelSize     int         // per-pipeline channel buffer size (default: 64)
	SharedResources []io.Closer // resources shared across files (closed at end)
	FailFast        bool        // cancel remaining on first error (default: true)
}

// BatchExecutor processes multiple files through a tool chain with configurable
// file-level concurrency and shared tool instances.
type BatchExecutor struct {
	config BatchConfig
}

// NewBatchExecutor creates a BatchExecutor with the given config.
func NewBatchExecutor(config BatchConfig) *BatchExecutor {
	if config.FileConcurrency <= 0 {
		config.FileConcurrency = 1
	}
	if config.ChannelSize <= 0 {
		config.ChannelSize = 64
	}
	return &BatchExecutor{config: config}
}

// BatchOption is a functional option for configuring a BatchExecutor.
type BatchOption func(*BatchConfig)

// WithFileConcurrency sets the max files processed in parallel.
// 0 means runtime.NumCPU().
func WithFileConcurrency(n int) BatchOption {
	return func(c *BatchConfig) {
		if n == 0 {
			c.FileConcurrency = runtime.NumCPU()
		} else {
			c.FileConcurrency = n
		}
	}
}

// WithBatchChannelSize sets the per-pipeline channel buffer size.
func WithBatchChannelSize(n int) BatchOption {
	return func(c *BatchConfig) {
		if n > 0 {
			c.ChannelSize = n
		}
	}
}

// WithSharedResources registers resources that are shared across files
// and closed after all files complete.
func WithSharedResources(resources ...io.Closer) BatchOption {
	return func(c *BatchConfig) {
		c.SharedResources = append(c.SharedResources, resources...)
	}
}

// WithBatchFailFast controls whether to cancel remaining files on first error.
func WithBatchFailFast(b bool) BatchOption {
	return func(c *BatchConfig) {
		c.FailFast = b
	}
}

// NewBatchExecutorWithOptions creates a BatchExecutor with functional options.
func NewBatchExecutorWithOptions(opts ...BatchOption) *BatchExecutor {
	cfg := BatchConfig{
		FileConcurrency: 1,
		ChannelSize:     64,
		FailFast:        true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return NewBatchExecutor(cfg)
}

// Execute processes multiple files through the given tool factories with
// configurable file-level concurrency. Each file gets its own tool chain
// created from the factories. Results are returned in input order.
func (b *BatchExecutor) Execute(ctx context.Context, toolFactories []ToolFactory, files []BatchFile, collectors ...Collector) ([]BatchResult, error) {
	// Ensure shared resources are cleaned up.
	defer func() {
		for _, r := range b.config.SharedResources {
			r.Close()
		}
	}()

	if len(files) == 0 {
		return nil, nil
	}

	results := make([]BatchResult, len(files))
	var mu sync.Mutex // protects collector calls

	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, b.config.FileConcurrency)

	for i, file := range files {
		if b.config.FailFast {
			select {
			case <-ctx.Done():
				break
			default:
			}
		}

		sem <- struct{}{} // acquire slot
		g.Go(func() error {
			defer func() { <-sem }() // release slot

			parts, err := b.processFile(ctx, toolFactories, file)
			if err != nil {
				return fmt.Errorf("processing %s: %w", file.URI, err)
			}

			results[i] = BatchResult{
				URI:   file.URI,
				Parts: parts,
			}

			// Feed collectors.
			if len(collectors) > 0 {
				item := &FlowItem{
					Input: &model.RawDocument{
						URI: file.URI,
					},
					TargetLocale: file.TargetLocale,
				}
				mu.Lock()
				for _, c := range collectors {
					if err := c.Collect(ctx, item, parts); err != nil {
						mu.Unlock()
						return fmt.Errorf("collector error for %s: %w", file.URI, err)
					}
				}
				mu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

// processFile creates a tool chain from factories and processes a single file.
func (b *BatchExecutor) processFile(ctx context.Context, toolFactories []ToolFactory, file BatchFile) ([]*model.Part, error) {
	if len(toolFactories) == 0 {
		return file.Parts, nil
	}

	// Create fresh tool instances from factories.
	tools := make([]tool.Tool, len(toolFactories))
	for i, factory := range toolFactories {
		t, err := factory()
		if err != nil {
			return nil, fmt.Errorf("tool factory %d: %w", i, err)
		}
		tools[i] = t
	}

	// Build and execute the pipeline.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	channels := make([]chan *model.Part, len(tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, b.config.ChannelSize)
	}

	for i, t := range tools {
		inCh := channels[i]
		outCh := channels[i+1]

		g.Go(func() error {
			defer close(outCh)
			if err := t.Process(ctx, inCh, outCh); err != nil {
				return fmt.Errorf("tool %s: %w", t.Name(), err)
			}
			return nil
		})
	}

	// Feed input parts.
	go func() {
		for _, p := range file.Parts {
			select {
			case channels[0] <- p:
			case <-ctx.Done():
				break
			}
		}
		close(channels[0])
	}()

	// Collect output.
	outCh := channels[len(channels)-1]
	var parts []*model.Part
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range outCh {
			parts = append(parts, p)
		}
	}()

	err := g.Wait()
	<-done

	if err != nil {
		return nil, err
	}

	return parts, nil
}
