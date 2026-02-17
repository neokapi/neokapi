package server

import (
	"context"
	"fmt"
	"net/rpc"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/core/plugin/shared"
	goplugin "github.com/hashicorp/go-plugin"
)

// ToolRPCServer wraps a tool.Tool for serving over net/rpc.
type ToolRPCServer struct {
	Impl tool.Tool
}

// ToolInfo returns the tool's metadata.
func (s *ToolRPCServer) ToolInfo(_ struct{}, resp *shared.ToolInfoResult) error {
	*resp = shared.ToolInfoResult{
		Name:        s.Impl.Name(),
		Description: s.Impl.Description(),
	}
	return nil
}

// Process processes parts through the tool.
func (s *ToolRPCServer) Process(args *shared.ProcessArgs, resp *shared.ProcessResult) error {
	// Convert DTOs to parts.
	inputParts := shared.DTOToParts(args.Parts)

	// Create channels for the tool.
	in := make(chan *model.Part, len(inputParts))
	out := make(chan *model.Part, len(inputParts)*2)

	for _, p := range inputParts {
		in <- p
	}
	close(in)

	ctx := context.Background()
	// Run process in a goroutine so we can collect output.
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Impl.Process(ctx, in, out)
		close(out)
	}()

	// Collect output parts.
	var outputParts []shared.PartDTO
	for p := range out {
		outputParts = append(outputParts, shared.PartToDTO(p))
	}

	if err := <-errCh; err != nil {
		*resp = shared.ProcessResult{Error: err.Error()}
		return nil
	}

	*resp = shared.ProcessResult{Parts: outputParts}
	return nil
}

// ToolServerPlugin is the go-plugin.Plugin implementation for serving
// a tool from a plugin process.
type ToolServerPlugin struct {
	Impl tool.Tool
}

// Server returns the RPC server for this plugin.
func (p *ToolServerPlugin) Server(broker *goplugin.MuxBroker) (interface{}, error) {
	return &ToolRPCServer{Impl: p.Impl}, nil
}

// Client is not used on the server side.
func (p *ToolServerPlugin) Client(broker *goplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return nil, fmt.Errorf("ToolServerPlugin.Client should not be called on server side")
}
