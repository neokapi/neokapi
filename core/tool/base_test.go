package tool_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseToolPassThrough(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName:        "pass-through",
		ToolDescription: "passes all parts through unchanged",
	}

	assert.Equal(t, "pass-through", bt.Name())
	assert.Equal(t, "passes all parts through unchanged", bt.Description())

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	var result []*model.Part
	for p := range out {
		result = append(result, p)
	}

	require.Len(t, result, len(parts))
	for i, p := range result {
		assert.Equal(t, parts[i].Type, p.Type)
		assert.Equal(t, parts[i].Resource, p.Resource)
	}
}

func TestBaseToolDispatch(t *testing.T) {
	var handledTypes []model.PartType

	bt := &tool.BaseTool{
		ToolName: "tracker",
		Annotate: func(tool.BlockView) error {
			handledTypes = append(handledTypes, model.PartBlock)
			return nil
		},
		HandleDataFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleMediaFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleLayerStartFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
		HandleLayerEndFn: func(part *model.Part) (*model.Part, error) {
			handledTypes = append(handledTypes, part.Type)
			return part, nil
		},
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartMedia, Resource: &model.Media{ID: "m1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	expected := []model.PartType{
		model.PartLayerStart,
		model.PartBlock,
		model.PartData,
		model.PartMedia,
		model.PartBlock,
		model.PartLayerEnd,
	}
	assert.Equal(t, expected, handledTypes)
}

func TestBaseToolContextCancellation(t *testing.T) {
	bt := &tool.BaseTool{ToolName: "cancellable"}

	ctx, cancel := context.WithCancel(t.Context())
	in := make(chan *model.Part) // unbuffered, will block
	out := make(chan *model.Part, 10)

	done := make(chan error, 1)
	go func() {
		done <- bt.Process(ctx, in, out)
	}()

	cancel()
	err := <-done
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBaseToolErrorPropagation(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName: "error-tool",
		Annotate: func(tool.BlockView) error {
			return assert.AnError
		},
	}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	require.Error(t, err)
}

func TestBaseToolModifyBlock(t *testing.T) {
	bt := &tool.BaseTool{
		ToolName: "uppercase",
		Translate: func(v tool.TargetView) error {
			if v.Translatable() {
				v.SetTargetText(model.LocaleFrench, "TRANSLATED")
			}
			return nil
		},
	}

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewBlock("tu1", "Hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	ctx := t.Context()
	err := bt.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "TRANSLATED", resultBlock.TargetText(model.LocaleFrench))
}

func TestBaseToolConfig(t *testing.T) {
	bt := &tool.BaseTool{ToolName: "test"}
	assert.Nil(t, bt.Config())

	// nil config is ok
	err := bt.SetConfig(nil)
	require.NoError(t, err)
}

// TestImmutabilityGuard exercises the dev/test backstop in the typed-handler
// dispatch. Capability is enforced primarily by the parameter type — an
// Annotate handler simply has no source/target setters — so the backstop's
// remaining job is to catch in-place edits through the live run slices the
// view hands back. These cases mutate via those aliased slices to trip it.
func TestImmutabilityGuard(t *testing.T) {
	mkBlock := func() *model.Block {
		b := model.NewBlock("b1", "Hello world")
		b.SourceLocale = "en"
		return b
	}
	run := func(bt *tool.BaseTool, b *model.Block) error {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		return bt.Process(context.Background(), in, out)
	}
	oneSpanOverlay := []model.Span{{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}}}

	t.Run("annotate handler mutating source via aliased slice is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-src"}
		bt.Annotate = func(v tool.BlockView) error {
			v.SourceRuns()[0].Text.Text = "changed" // in-place edit through the live slice
			return nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "changed source")
	})
	t.Run("transform producer's plan rewrites source via the applier", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "src-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			changed := "changed"
			return tool.EditPlan{ReplaceAll: &changed}, nil
		}
		b := mkBlock()
		require.NoError(t, run(bt, b))
		assert.Equal(t, "changed", b.SourceText())
	})
	t.Run("transform producer mutating source in place is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "sneaky-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			v.SourceRuns()[0].Text.Text = "changed" // in-place edit through the live slice
			return tool.EditPlan{}, nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "read-only producer")
	})
	t.Run("transform producer mutating target in place is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "sneaky-tgt-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			v.TargetRuns("fr")[0].Text.Text = "changed"
			return tool.EditPlan{}, nil
		}
		b := mkBlock()
		b.SetTargetText("fr", "Bonjour")
		require.ErrorContains(t, run(bt, b), "changed target")
	})
	t.Run("transform plan with secrets and no vault sink is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "leaky-redactor"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			changed := "[REDACTED]"
			return tool.EditPlan{
				ReplaceAll: &changed,
				Secrets:    []tool.Secret{{Token: "rdx1", Original: "Hello world"}},
			}, nil
		}
		b := mkBlock()
		require.ErrorContains(t, run(bt, b), "no VaultSecrets sink")
		// Fail-closed: the rewrite must not have landed.
		assert.Equal(t, "Hello world", b.SourceText())
	})
	t.Run("transform plan vaults secrets before the rewrite", func(t *testing.T) {
		var vaulted []tool.Secret
		bt := &tool.BaseTool{ToolName: "redactor"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			changed := "[REDACTED]"
			return tool.EditPlan{
				ReplaceAll: &changed,
				Secrets:    []tool.Secret{{Token: "rdx1", Original: "Hello world"}},
			}, nil
		}
		bt.VaultSecrets = func(v tool.BlockView, secrets []tool.Secret) error {
			vaulted = secrets
			return nil
		}
		b := mkBlock()
		require.NoError(t, run(bt, b))
		assert.Equal(t, "[REDACTED]", b.SourceText())
		require.Len(t, vaulted, 1)
		assert.Equal(t, "Hello world", vaulted[0].Original)
	})
	t.Run("transform plan changing text without edits is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "unmapped-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			return tool.EditPlan{NewRuns: []model.Run{{Text: &model.TextRun{Text: "different"}}}}, nil
		}
		require.ErrorContains(t, run(bt, mkBlock()), "without a mapping")
	})
	t.Run("annotate handler mutating target via aliased slice is rejected", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "bad-tgt"}
		bt.Annotate = func(v tool.BlockView) error {
			v.TargetRuns("fr")[0].Text.Text = "changed" // in-place edit through the live slice
			return nil
		}
		b := mkBlock()
		b.SetTargetText("fr", "Bonjour")
		require.ErrorContains(t, run(bt, b), "changed target")
	})
	t.Run("translate handler may write target", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "translator"}
		bt.Translate = func(v tool.TargetView) error {
			v.SetTargetText("fr", "Bonjour")
			return nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("annotate handler writing only overlays/properties is allowed", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "analyzer"}
		bt.Annotate = func(v tool.BlockView) error {
			v.SetProperty("word-count", "2")
			v.SetSegmentation(nil, oneSpanOverlay)
			return nil
		}
		require.NoError(t, run(bt, mkBlock()))
	})
	t.Run("structured plan with inconsistent edits drops the overlay span", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "late-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			// Shrink "Hello world" (11 runes) to "hi" but claim only a 2-rune
			// deletion: the edits do not describe the rewrite.
			return tool.EditPlan{
				NewRuns: []model.Run{{Text: &model.TextRun{Text: "hi"}}},
				Edits:   []model.RunEdit{{Start: 0, End: 2, NewLen: 0}},
			}, nil
		}
		b := mkBlock()
		// A term span over the trailing "ld" (runes 9..11): outside the claimed
		// edit, but its shifted range (7..9) cannot fit "hi" — the remap drops it
		// rather than mis-anchor it, and the bounds invariant holds.
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRangeFor(b.Source, 9, 11)})
		require.NoError(t, run(bt, b))
		assert.Equal(t, "hi", b.SourceText())
		assert.Nil(t, b.OverlayOf(model.OverlayTerm), "the unmappable span (and its emptied overlay) is dropped")
	})
	t.Run("structured plan rebases overlays across the rewrite", func(t *testing.T) {
		// The plan deletes the leading "Hello " (runes 0..6); the applier rebases
		// the term overlay over "world", so the surviving span anchors in-bounds.
		bt := &tool.BaseTool{ToolName: "rebase-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			return tool.EditPlan{
				NewRuns: []model.Run{{Text: &model.TextRun{Text: "world"}}},
				Edits:   []model.RunEdit{{Start: 0, End: 6, NewLen: 0}},
			}, nil
		}
		b := mkBlock()
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRangeFor(b.Source, 6, 11)})
		require.NoError(t, run(bt, b))
		// The span was rebased onto the new runs and now covers "world" (0..5).
		sp := b.OverlaySpan(model.OverlayTerm, "t1")
		require.NotNil(t, sp)
		s, e := sp.Range.TextSpan(b.Source)
		assert.Equal(t, 0, s)
		assert.Equal(t, 5, e)
	})
	t.Run("opaque ReplaceAll drops source overlays", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "opaque-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			rewritten := "a fully rewritten sentence"
			return tool.EditPlan{ReplaceAll: &rewritten}, nil
		}
		b := mkBlock()
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRangeFor(b.Source, 0, 5)})
		require.NoError(t, run(bt, b))
		assert.Equal(t, "a fully rewritten sentence", b.SourceText())
		assert.Nil(t, b.OverlayOf(model.OverlayTerm))
	})
	t.Run("structure-only plan re-anchors overlays onto the new runs", func(t *testing.T) {
		// Splitting one text run into two changes run indices but not the text;
		// the plan carries no edits and the applier re-anchors the span.
		bt := &tool.BaseTool{ToolName: "split-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			return tool.EditPlan{NewRuns: []model.Run{
				{Text: &model.TextRun{Text: "Hello "}},
				{Text: &model.TextRun{Text: "world"}},
			}}, nil
		}
		b := mkBlock()
		b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "t1", Range: model.RunRangeFor(b.Source, 6, 11)})
		require.NoError(t, run(bt, b))
		sp := b.OverlaySpan(model.OverlayTerm, "t1")
		require.NotNil(t, sp)
		s, e := sp.Range.TextSpan(b.Source)
		assert.Equal(t, 6, s)
		assert.Equal(t, 11, e)
	})
	t.Run("plan target replacement preserves variant metadata", func(t *testing.T) {
		bt := &tool.BaseTool{ToolName: "tgt-xform"}
		bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
			var plan tool.EditPlan
			plan.SetTarget("fr", []model.Run{{Text: &model.TextRun{Text: "BONJOUR"}}})
			return plan, nil
		}
		b := mkBlock()
		b.SetTarget("fr", model.NewTarget([]model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}, model.TargetStatusTranslated))
		require.NoError(t, run(bt, b))
		assert.Equal(t, "BONJOUR", b.TargetText("fr"))
		assert.Equal(t, model.TargetStatusTranslated, b.Target("fr").Status)
	})
}
