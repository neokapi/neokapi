package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
)

// Tool processes Parts in a Flow.
type Tool interface {
	// Name returns the tool's unique identifier.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Process reads Parts from the input channel, processes them,
	// and writes results to the output channel. Process blocks until
	// input is exhausted or context is canceled. Receives and sends must
	// observe ctx. The caller owns both channels and closes the output after
	// Process returns; a tool must join its workers before returning and
	// leave channel closure to the caller. Dropped parts emit nothing.
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error

	// Config returns the current configuration.
	Config() ToolConfig

	// SetConfig applies a new configuration.
	SetConfig(cfg ToolConfig) error
}

// SchemaProvider is an optional interface for tools that declare their
// parameter schema. Tools implementing this interface enable schema-driven
// CLI flag generation, flow editor config panels, and validation.
type SchemaProvider interface {
	Schema() *schema.ComponentSchema
}
