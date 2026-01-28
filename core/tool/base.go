package tool

import (
	"context"
	"fmt"

	"github.com/asgeirf/gokapi/core/model"
)

// PartHandler is the function signature for handling a single Part.
type PartHandler func(part *model.Part) (*model.Part, error)

// BaseTool provides default pass-through behavior and event dispatch.
// Embed in concrete tools and override Handle* methods as needed.
type BaseTool struct {
	ToolName        string
	ToolDescription string
	Cfg             ToolConfig

	// Override these handlers in concrete tools.
	HandleBlockFn      PartHandler
	HandleDataFn       PartHandler
	HandleMediaFn      PartHandler
	HandleLayerStartFn PartHandler
	HandleLayerEndFn   PartHandler
	HandleGroupStartFn PartHandler
	HandleGroupEndFn   PartHandler
	HandleBatchStartFn PartHandler
	HandleBatchEndFn   PartHandler
}

// Name returns the tool's identifier.
func (b *BaseTool) Name() string { return b.ToolName }

// Description returns the tool's description.
func (b *BaseTool) Description() string { return b.ToolDescription }

// Config returns the current configuration.
func (b *BaseTool) Config() ToolConfig { return b.Cfg }

// SetConfig applies a new configuration after validation.
func (b *BaseTool) SetConfig(cfg ToolConfig) error {
	if cfg == nil {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	b.Cfg = cfg
	return nil
}

// Process dispatches each Part to the appropriate handler.
func (b *BaseTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			result, err := b.dispatch(part)
			if err != nil {
				return err
			}
			select {
			case out <- result:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (b *BaseTool) dispatch(part *model.Part) (*model.Part, error) {
	switch part.Type {
	case model.PartBlock:
		return b.handleBlock(part)
	case model.PartData:
		return b.handleData(part)
	case model.PartMedia:
		return b.handleMedia(part)
	case model.PartLayerStart:
		return b.handleLayerStart(part)
	case model.PartLayerEnd:
		return b.handleLayerEnd(part)
	case model.PartGroupStart:
		return b.handleGroupStart(part)
	case model.PartGroupEnd:
		return b.handleGroupEnd(part)
	case model.PartBatchStart:
		return b.handleBatchStart(part)
	case model.PartBatchEnd:
		return b.handleBatchEnd(part)
	default:
		return part, nil
	}
}

func (b *BaseTool) handleBlock(part *model.Part) (*model.Part, error) {
	if b.HandleBlockFn != nil {
		return b.HandleBlockFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleData(part *model.Part) (*model.Part, error) {
	if b.HandleDataFn != nil {
		return b.HandleDataFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleMedia(part *model.Part) (*model.Part, error) {
	if b.HandleMediaFn != nil {
		return b.HandleMediaFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleLayerStart(part *model.Part) (*model.Part, error) {
	if b.HandleLayerStartFn != nil {
		return b.HandleLayerStartFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleLayerEnd(part *model.Part) (*model.Part, error) {
	if b.HandleLayerEndFn != nil {
		return b.HandleLayerEndFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleGroupStart(part *model.Part) (*model.Part, error) {
	if b.HandleGroupStartFn != nil {
		return b.HandleGroupStartFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleGroupEnd(part *model.Part) (*model.Part, error) {
	if b.HandleGroupEndFn != nil {
		return b.HandleGroupEndFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleBatchStart(part *model.Part) (*model.Part, error) {
	if b.HandleBatchStartFn != nil {
		return b.HandleBatchStartFn(part)
	}
	return part, nil
}

func (b *BaseTool) handleBatchEnd(part *model.Part) (*model.Part, error) {
	if b.HandleBatchEndFn != nil {
		return b.HandleBatchEndFn(part)
	}
	return part, nil
}
