package tool_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// BenchmarkBaseTool_Process measures passthrough throughput of a BaseTool
// processing 1000 Block parts through a channel pair.
func BenchmarkBaseTool_Process(b *testing.B) {
	b.ReportAllocs()

	// Pre-build parts outside the loop to measure only channel/dispatch overhead.
	parts := make([]*model.Part, 1000)
	for i := range parts {
		parts[i] = &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Source text for block %d with realistic length", i)),
		}
	}

	for b.Loop() {
		t := &tool.BaseTool{ToolName: "passthrough"}

		in := make(chan *model.Part, 64)
		out := make(chan *model.Part, 64)

		go func() {
			for _, p := range parts {
				in <- p
			}
			close(in)
		}()

		go func() {
			defer close(out)
			_ = t.Process(context.Background(), in, out)
		}()

		count := 0
		for range out {
			count++
		}

		if count != len(parts) {
			b.Fatalf("expected %d parts, got %d", len(parts), count)
		}
	}
}

// BenchmarkProduce_TargetWrite measures a Produce handler that writes a target
// with inline codes and stamps it, the write every translating tool makes per
// block.
func BenchmarkProduce_TargetWrite(b *testing.B) {
	b.ReportAllocs()
	runs := []model.Run{
		model.TextR("Read the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link:hyperlink", Data: `<a href="https://example.com/guide">`, Attrs: map[string]string{"href": "https://example.com/guide"}}),
		model.TextR("shop guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link:hyperlink", Data: "</a>"}),
		model.TextR(" before you "),
		model.PcOpenR(model.PcOpenRun{ID: "2", Type: "fmt:bold", Data: "<b>"}),
		model.TextR("order"),
		model.PcCloseR(model.PcCloseRun{ID: "2", Type: "fmt:bold", Data: "</b>"}),
		model.TextR("."),
	}
	t := &tool.BaseTool{ToolName: "bench"}
	t.Produce = func(v tool.VariantView) error {
		v.SetTargetRuns("fr", v.SourceRuns())
		v.StampTargetProvenance("fr", model.TargetStatusDraft, model.Origin{Tool: "bench"})
		return nil
	}
	ctx := context.Background()
	for b.Loop() {
		blk := model.NewRunsBlock("tu1", runs)
		if _, err := t.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: blk}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParallelBlockTool measures the ParallelBlockTool with 4-way
// concurrency processing 1000 blocks that each do a lightweight transform.
func BenchmarkParallelBlockTool(b *testing.B) {
	b.ReportAllocs()

	parts := make([]*model.Part, 1000)
	for i := range parts {
		parts[i] = &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Source text for block %d with realistic length", i)),
		}
	}

	for b.Loop() {
		inner := &tool.BaseTool{
			ToolName: "uppercase",
			Produce: func(v tool.VariantView) error {
				v.SetTargetText(model.LocaleFrench, strings.ToUpper(v.SourceText()))
				return nil
			},
		}
		pt := tool.NewParallelBlockTool(inner, 4)

		in := make(chan *model.Part, 64)
		out := make(chan *model.Part, 64)

		go func() {
			for _, p := range parts {
				in <- p
			}
			close(in)
		}()

		go func() {
			defer close(out)
			_ = pt.Process(context.Background(), in, out)
		}()

		count := 0
		for range out {
			count++
		}

		if count != len(parts) {
			b.Fatalf("expected %d parts, got %d", len(parts), count)
		}
	}
}
