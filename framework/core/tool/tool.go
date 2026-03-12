package tool

import (
	"context"

	"github.com/gokapi/gokapi/core/model"
)

// Tool processes Parts in a Flow.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Process reads Parts from the input channel, processes them,
	// and writes results to the output channel. Process blocks until
	// input is exhausted or context is canceled.
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error

	// Config returns the current configuration.
	Config() ToolConfig

	// SetConfig applies a new configuration.
	SetConfig(cfg ToolConfig) error
}
