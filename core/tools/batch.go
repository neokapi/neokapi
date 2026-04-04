package tools

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// BatchConfig holds configuration for the batch collector tool.
type BatchConfig struct {
	Size int `schema:"title=Batch Size,description=Number of blocks to collect before forwarding as a batch,default=10,min=1"`
}

// ToolName returns the tool name this config applies to.
func (c *BatchConfig) ToolName() string { return "batch" }

// Reset restores default values.
func (c *BatchConfig) Reset() { c.Size = 10 }

// Validate checks configuration validity.
func (c *BatchConfig) Validate() error {
	if c.Size < 1 {
		return errors.New("batch: Size must be >= 1")
	}
	return nil
}

// NewBatchTool creates a tool that collects blocks into batches.
// Non-block parts pass through immediately. Blocks are buffered and
// forwarded together every Size blocks, with a BatchAnnotation on the
// last block in each batch.
func NewBatchTool(cfg *BatchConfig) tool.Tool {
	if cfg.Size < 1 {
		cfg.Size = 10
	}

	return &batchTool{
		BaseTool: tool.BaseTool{
			ToolName:        "batch",
			ToolDescription: "Collect blocks into batches for downstream batch processing",
			Cfg:             cfg,
		},
		size: cfg.Size,
	}
}

type batchTool struct {
	tool.BaseTool
	size int
}

// Process overrides BaseTool.Process to implement batching logic.
func (b *batchTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	var batch []*model.Part

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		for _, p := range batch {
			select {
			case out <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				// Flush remaining blocks on stream end
				return flush()
			}
			if part.Type != model.PartBlock {
				// Non-block parts pass through immediately
				select {
				case out <- part:
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}
			batch = append(batch, part)
			if len(batch) >= b.size {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
}
