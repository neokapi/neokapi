package flow_test

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

func exampleUppercaseTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName:        "uppercase",
		ToolDescription: "Uppercases source text",
		WritesSource:    true,
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			block := part.Resource.(*model.Block)
			block.SetSourceText(strings.ToUpper(block.SourceText()))
			return part, nil
		},
	}
}

func ExampleNewFlow() {
	// Build a flow with a single tool.
	f, _ := flow.NewFlow("uppercase-flow").
		AddTool(exampleUppercaseTool()).
		Build()

	fmt.Println(f.Name)
	fmt.Println(len(f.Tools))
	// Output:
	// uppercase-flow
	// 1
}

func ExampleNewExecutor() {
	// Create an executor that processes documents sequentially
	// with a channel buffer of 32 and fail-fast enabled.
	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(1),
		flow.WithChannelSize(32),
		flow.WithFailFast(true),
	)

	// The executor is ready to run flows over batch items.
	_ = executor
	fmt.Println("executor created")
	// Output:
	// executor created
}
