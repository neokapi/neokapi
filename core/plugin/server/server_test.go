package server_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/server"
	"github.com/neokapi/neokapi/core/plugin/shared"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolRPCServerToolInfo(t *testing.T) {
	impl := &tool.BaseTool{
		ToolName:        "test-tool",
		ToolDescription: "A test tool",
	}

	srv := server.ToolRPCServer{Impl: impl}

	var resp shared.ToolInfoResult
	err := srv.ToolInfo(struct{}{}, &resp)
	require.NoError(t, err)
	assert.Equal(t, "test-tool", resp.Name)
	assert.Equal(t, "A test tool", resp.Description)
}

func TestToolRPCServerProcess(t *testing.T) {
	impl := &tool.BaseTool{
		ToolName:        "upper-tool",
		ToolDescription: "Uppercases target",
	}
	impl.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block := part.Resource.(*model.Block)
		block.SetTargetText(model.LocaleFrench, "TRANSLATED")
		return part, nil
	}

	srv := server.ToolRPCServer{Impl: impl}

	block := model.NewBlock("tu1", "Hello")
	inputParts := []shared.PartDTO{shared.PartToDTO(&model.Part{Type: model.PartBlock, Resource: block})}

	args := &shared.ProcessArgs{Parts: inputParts}
	var resp shared.ProcessResult
	err := srv.Process(args, &resp)
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	require.Len(t, resp.Parts, 1)

	resultPart := shared.DTOToPart(resp.Parts[0])
	resultBlock := resultPart.Resource.(*model.Block)
	assert.Equal(t, "TRANSLATED", resultBlock.TargetText(model.LocaleFrench))
}

func TestToolRPCServerProcessError(t *testing.T) {
	impl := &tool.BaseTool{
		ToolName:        "fail-tool",
		ToolDescription: "Always fails",
	}
	impl.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		return nil, assert.AnError
	}

	srv := server.ToolRPCServer{Impl: impl}

	block := model.NewBlock("tu1", "Hello")
	inputParts := []shared.PartDTO{shared.PartToDTO(&model.Part{Type: model.PartBlock, Resource: block})}

	args := &shared.ProcessArgs{Parts: inputParts}
	var resp shared.ProcessResult
	err := srv.Process(args, &resp)
	require.NoError(t, err) // RPC call itself succeeds
	assert.NotEmpty(t, resp.Error)
}

func TestToolRPCServerProcessPassthrough(t *testing.T) {
	// Tool with no handlers should pass everything through.
	impl := &tool.BaseTool{
		ToolName:        "noop-tool",
		ToolDescription: "Does nothing",
	}

	srv := server.ToolRPCServer{Impl: impl}

	block := model.NewBlock("tu1", "Hello")
	data := &model.Data{ID: "d1", Name: "skel"}
	inputParts := []shared.PartDTO{
		shared.PartToDTO(&model.Part{Type: model.PartBlock, Resource: block}),
		shared.PartToDTO(&model.Part{Type: model.PartData, Resource: data}),
	}

	args := &shared.ProcessArgs{Parts: inputParts}
	var resp shared.ProcessResult
	err := srv.Process(args, &resp)
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	assert.Len(t, resp.Parts, 2)
}

func TestToolServerPluginServer(t *testing.T) {
	impl := &tool.BaseTool{
		ToolName:        "test-tool",
		ToolDescription: "test",
	}

	plugin := &server.ToolServerPlugin{Impl: impl}
	rpcServer, err := plugin.Server(nil)
	require.NoError(t, err)
	require.NotNil(t, rpcServer)

	_, ok := rpcServer.(*server.ToolRPCServer)
	assert.True(t, ok)
}

func TestToolServerPluginClientErrors(t *testing.T) {
	plugin := &server.ToolServerPlugin{Impl: &tool.BaseTool{}}
	_, err := plugin.Client(nil, nil)
	require.Error(t, err)
}
