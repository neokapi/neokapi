package host

import (
	"context"
	"fmt"
	"net/rpc"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	"github.com/asgeirf/gokapi/plugin/shared"
)

// ToolRPCClient implements tool.Tool by delegating to a plugin process over net/rpc.
type ToolRPCClient struct {
	client *rpc.Client
	info   *shared.ToolInfoResult
}

// ensure interface compliance
var _ tool.Tool = (*ToolRPCClient)(nil)

// Name returns the tool's unique identifier from the remote plugin.
func (c *ToolRPCClient) Name() string {
	info, err := c.fetchInfo()
	if err != nil {
		return ""
	}
	return info.Name
}

// Description returns the tool's description from the remote plugin.
func (c *ToolRPCClient) Description() string {
	info, err := c.fetchInfo()
	if err != nil {
		return ""
	}
	return info.Description
}

// Process reads Parts from the input channel, sends them to the remote plugin
// for processing, and writes the results to the output channel.
func (c *ToolRPCClient) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// Collect all input parts.
	var collected []*model.Part
	for {
		select {
		case p, ok := <-in:
			if !ok {
				goto process
			}
			collected = append(collected, p)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

process:
	args := shared.ProcessArgs{
		Parts: shared.PartsToDTO(collected),
	}
	var result shared.ProcessResult
	if err := c.client.Call("Plugin.Process", &args, &result); err != nil {
		return fmt.Errorf("rpc Process: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("plugin Process: %s", result.Error)
	}

	// Emit processed parts.
	for _, dto := range result.Parts {
		part := shared.DTOToPart(dto)
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Config returns nil since remote plugins manage their own config.
func (c *ToolRPCClient) Config() tool.ToolConfig {
	return nil
}

// SetConfig is a no-op for remote plugins.
func (c *ToolRPCClient) SetConfig(_ tool.ToolConfig) error {
	return nil
}

// fetchInfo caches and returns the plugin's ToolInfoResult.
func (c *ToolRPCClient) fetchInfo() (*shared.ToolInfoResult, error) {
	if c.info != nil {
		return c.info, nil
	}
	var info shared.ToolInfoResult
	if err := c.client.Call("Plugin.ToolInfo", new(struct{}), &info); err != nil {
		return nil, fmt.Errorf("rpc ToolInfo: %w", err)
	}
	c.info = &info
	return c.info, nil
}
