package flow_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceRecorder(t *testing.T) {
	t.Run("basic event recording", func(t *testing.T) {
		rec := flow.NewTraceRecorder()

		rec.Record(flow.TraceEnter, "tool-1", "block-1", nil)
		rec.Record(flow.TraceExit, "tool-1", "block-1", map[string]any{"changed": true})
		rec.Record(flow.TraceEnter, "tool-2", "block-1", nil)

		events := rec.Events()
		require.Len(t, events, 3)

		assert.Equal(t, flow.TraceEnter, events[0].Type)
		assert.Equal(t, "tool-1", events[0].NodeID)
		assert.Equal(t, "block-1", events[0].PartID)
		assert.Nil(t, events[0].Meta)

		assert.Equal(t, flow.TraceExit, events[1].Type)
		assert.Equal(t, true, events[1].Meta["changed"])

		assert.Equal(t, flow.TraceEnter, events[2].Type)
		assert.Equal(t, "tool-2", events[2].NodeID)

		// Timestamps should be non-negative and non-decreasing.
		assert.GreaterOrEqual(t, events[0].TS, int64(0))
		assert.GreaterOrEqual(t, events[1].TS, events[0].TS)
		assert.GreaterOrEqual(t, events[2].TS, events[1].TS)
	})

	t.Run("snapshot capture", func(t *testing.T) {
		rec := flow.NewTraceRecorder()

		block := model.NewBlock("b1", "Hello World")
		part := &model.Part{Type: model.PartBlock, Resource: block}

		// Capture initial snapshot.
		rec.SnapshotPart(part, "", "initial")

		// Simulate tool processing: add target text.
		block.SetTargetText(model.LocaleFrench, "Bonjour le monde")

		// Capture after-node snapshot.
		rec.SnapshotPart(part, "pseudo", "pseudo")

		snapshots := rec.Snapshots()
		require.Contains(t, snapshots, "b1")

		ss := snapshots["b1"]
		assert.Equal(t, "Block", ss.Initial.Type)
		assert.Equal(t, "Hello World", ss.Initial.SourceText)
		assert.Equal(t, "", ss.Initial.TargetText)
		assert.Equal(t, "Hello World", ss.Initial.Summary)

		require.Contains(t, ss.AfterNode, "pseudo")
		after := ss.AfterNode["pseudo"]
		assert.Equal(t, "Block", after.Type)
		assert.Equal(t, "Hello World", after.SourceText)
		assert.Equal(t, "Bonjour le monde", after.TargetText)
	})

	t.Run("events returns a copy", func(t *testing.T) {
		rec := flow.NewTraceRecorder()
		rec.Record(flow.TraceEnter, "n1", "p1", nil)

		events1 := rec.Events()
		events2 := rec.Events()

		// Modifying one copy should not affect the other.
		events1[0].Type = "modified" // intentionally assign invalid value to test copy isolation
		assert.Equal(t, flow.TraceEnter, events2[0].Type)
	})
}

func TestSnapshotFromPart(t *testing.T) {
	tests := []struct {
		name       string
		part       *model.Part
		wantType   string
		wantSumm   string
		wantSrc    string
		wantTarget string
	}{
		{
			name: "Block with short text",
			part: &model.Part{
				Type:     model.PartBlock,
				Resource: model.NewBlock("b1", "Hello"),
			},
			wantType: "Block",
			wantSumm: "Hello",
			wantSrc:  "Hello",
		},
		{
			name: "Block with long text truncates summary",
			part: &model.Part{
				Type:     model.PartBlock,
				Resource: model.NewBlock("b2", "This is a very long text that exceeds forty characters total length"),
			},
			wantType: "Block",
			wantSumm: "This is a very long text that exceeds fo...",
			wantSrc:  "This is a very long text that exceeds forty characters total length",
		},
		{
			name: "Block with target text",
			part: func() *model.Part {
				block := model.NewBlock("b3", "Hello")
				block.SetTargetText(model.LocaleFrench, "Bonjour")
				return &model.Part{Type: model.PartBlock, Resource: block}
			}(),
			wantType:   "Block",
			wantSumm:   "Hello",
			wantSrc:    "Hello",
			wantTarget: "Bonjour",
		},
		{
			name: "LayerStart",
			part: &model.Part{
				Type:     model.PartLayerStart,
				Resource: &model.Layer{ID: "doc1", Name: "Document"},
			},
			wantType: "LayerStart",
			wantSumm: "Layer: Document",
		},
		{
			name: "LayerEnd",
			part: &model.Part{
				Type:     model.PartLayerEnd,
				Resource: &model.Layer{ID: "doc1", Name: "Document"},
			},
			wantType: "LayerEnd",
			wantSumm: "end layer doc1",
		},
		{
			name: "Data",
			part: &model.Part{
				Type:     model.PartData,
				Resource: &model.Data{ID: "d1", Name: "structure"},
			},
			wantType: "Data",
			wantSumm: "Data: structure",
		},
		{
			name: "Media",
			part: &model.Part{
				Type:     model.PartMedia,
				Resource: &model.Media{ID: "m1", MimeType: "image/png"},
			},
			wantType: "Media",
			wantSumm: "Media: image/png",
		},
		{
			name: "GroupStart",
			part: &model.Part{
				Type:     model.PartGroupStart,
				Resource: &model.GroupStart{ID: "g1", Name: "section"},
			},
			wantType: "GroupStart",
			wantSumm: "Group: section",
		},
		{
			name: "GroupEnd",
			part: &model.Part{
				Type:     model.PartGroupEnd,
				Resource: &model.GroupEnd{ID: "g1"},
			},
			wantType: "GroupEnd",
			wantSumm: "end group g1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := flow.NewTraceRecorder()
			rec.SnapshotPart(tt.part, "", "initial")

			snapshots := rec.Snapshots()
			id := tt.part.Resource.ResourceID()
			require.Contains(t, snapshots, id)

			snap := snapshots[id].Initial
			assert.Equal(t, tt.wantType, snap.Type)
			assert.Equal(t, tt.wantSumm, snap.Summary)
			if tt.wantSrc != "" {
				assert.Equal(t, tt.wantSrc, snap.SourceText)
			}
			if tt.wantTarget != "" {
				assert.Equal(t, tt.wantTarget, snap.TargetText)
			}
		})
	}
}

func TestTracingTool(t *testing.T) {
	t.Run("wraps pass-through tool", func(t *testing.T) {
		inner := &tool.BaseTool{
			ToolName:        "pass-through",
			ToolDescription: "does nothing",
		}

		rec := flow.NewTraceRecorder()
		traced := flow.NewTracingTool(inner, "node-1", rec)

		assert.Equal(t, "pass-through", traced.Name())
		assert.Equal(t, "does nothing", traced.Description())

		f, err := flow.NewFlow("test").AddTool(traced).Build()
		require.NoError(t, err)
		executor := flow.NewExecutor()
		ctx := t.Context()

		in, out, wait := executor.ExecuteWithChannels(ctx, f)

		// Feed parts.
		go func() {
			for i := range 3 {
				block := model.NewBlock(fmt.Sprintf("b%d", i), fmt.Sprintf("Text %d", i))
				rec.SnapshotPart(&model.Part{Type: model.PartBlock, Resource: block}, "", "initial")
				in <- &model.Part{Type: model.PartBlock, Resource: block}
			}
			close(in)
		}()

		// Collect output.
		var results []*model.Part
		for p := range out {
			results = append(results, p)
		}
		err = wait()
		require.NoError(t, err)

		assert.Len(t, results, 3)

		// Verify events: 3 enter + 3 exit = 6 events.
		events := rec.Events()
		assert.Len(t, events, 6)

		// Check enter/exit alternation per part.
		for i := range 3 {
			partID := fmt.Sprintf("b%d", i)
			enterIdx := -1
			exitIdx := -1
			for j, ev := range events {
				if ev.PartID == partID && ev.Type == flow.TraceEnter {
					enterIdx = j
				}
				if ev.PartID == partID && ev.Type == flow.TraceExit {
					exitIdx = j
				}
			}
			assert.GreaterOrEqual(t, enterIdx, 0, "enter event for %s", partID)
			assert.GreaterOrEqual(t, exitIdx, 0, "exit event for %s", partID)
			assert.Less(t, enterIdx, exitIdx, "enter before exit for %s", partID)
		}
	})

	t.Run("wraps uppercase tool with snapshots", func(t *testing.T) {
		inner := &tool.BaseTool{
			ToolName: "upper",
			Translate: func(v tool.TargetView) error {
				if v.Translatable() {
					v.SetTargetText(model.LocaleFrench, strings.ToUpper(v.SourceText()))
				}
				return nil
			},
		}

		rec := flow.NewTraceRecorder()
		traced := flow.NewTracingTool(inner, "upper-node", rec)

		f, err := flow.NewFlow("test").AddTool(traced).Build()
		require.NoError(t, err)
		executor := flow.NewExecutor()
		ctx := t.Context()

		in, out, wait := executor.ExecuteWithChannels(ctx, f)

		block := model.NewBlock("b1", "hello world")
		rec.SnapshotPart(&model.Part{Type: model.PartBlock, Resource: block}, "", "initial")

		go func() {
			in <- &model.Part{Type: model.PartBlock, Resource: block}
			close(in)
		}()

		var results []*model.Part
		for p := range out {
			results = append(results, p)
		}
		err = wait()
		require.NoError(t, err)

		require.Len(t, results, 1)

		// Verify the after-node snapshot captured the target text.
		snapshots := rec.Snapshots()
		require.Contains(t, snapshots, "b1")
		ss := snapshots["b1"]
		assert.Equal(t, "", ss.Initial.TargetText)
		require.Contains(t, ss.AfterNode, "upper-node")
		assert.Equal(t, "HELLO WORLD", ss.AfterNode["upper-node"].TargetText)
	})

	t.Run("multi-tool chain with tracing", func(t *testing.T) {
		rec := flow.NewTraceRecorder()

		tool1 := flow.NewTracingTool(&tool.BaseTool{ToolName: "step1"}, "node-1", rec)
		tool2 := flow.NewTracingTool(&tool.BaseTool{ToolName: "step2"}, "node-2", rec)

		f, err := flow.NewFlow("multi").AddTool(tool1).AddTool(tool2).Build()
		require.NoError(t, err)
		executor := flow.NewExecutor()
		ctx := t.Context()

		in, out, wait := executor.ExecuteWithChannels(ctx, f)

		go func() {
			block := model.NewBlock("b1", "test")
			rec.SnapshotPart(&model.Part{Type: model.PartBlock, Resource: block}, "", "initial")
			in <- &model.Part{Type: model.PartBlock, Resource: block}
			close(in)
		}()

		var results []*model.Part
		for p := range out {
			results = append(results, p)
		}
		err = wait()
		require.NoError(t, err)

		assert.Len(t, results, 1)

		// Should have 4 events: enter node-1, exit node-1, enter node-2, exit node-2.
		events := rec.Events()
		assert.Len(t, events, 4)

		assert.Equal(t, flow.TraceEnter, events[0].Type)
		assert.Equal(t, "node-1", events[0].NodeID)
		assert.Equal(t, flow.TraceExit, events[1].Type)
		assert.Equal(t, "node-1", events[1].NodeID)
		assert.Equal(t, flow.TraceEnter, events[2].Type)
		assert.Equal(t, "node-2", events[2].NodeID)
		assert.Equal(t, flow.TraceExit, events[3].Type)
		assert.Equal(t, "node-2", events[3].NodeID)
	})
}
