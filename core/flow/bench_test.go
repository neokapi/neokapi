package flow_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// BenchmarkExecutor_SingleTool measures throughput of the flow executor
// with a single passthrough tool processing 100 parts per iteration.
func BenchmarkExecutor_SingleTool(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		f, _ := flow.NewFlow("bench-single").
			AddTool(&tool.BaseTool{ToolName: "passthrough"}).
			Build()

		executor := flow.NewExecutor()
		ctx := context.Background()

		in, out, wait := executor.ExecuteWithChannels(ctx, f)

		go func() {
			for i := range 100 {
				in <- &model.Part{
					Type:     model.PartBlock,
					Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Hello world sentence number %d for benchmarking", i)),
				}
			}
			close(in)
		}()

		for range out {
		}

		if err := wait(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecutor_ToolChain measures throughput of the flow executor
// with a 3-tool chain processing 100 parts per iteration.
func BenchmarkExecutor_ToolChain(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		f, _ := flow.NewFlow("bench-chain").
			AddTool(&tool.BaseTool{ToolName: "tool1"}).
			AddTool(&tool.BaseTool{ToolName: "tool2"}).
			AddTool(&tool.BaseTool{ToolName: "tool3"}).
			Build()

		executor := flow.NewExecutor()
		ctx := context.Background()

		in, out, wait := executor.ExecuteWithChannels(ctx, f)

		go func() {
			for i := range 100 {
				in <- &model.Part{
					Type:     model.PartBlock,
					Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Hello world sentence number %d for benchmarking", i)),
				}
			}
			close(in)
		}()

		for range out {
		}

		if err := wait(); err != nil {
			b.Fatal(err)
		}
	}
}
