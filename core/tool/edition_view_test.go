package tool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// sameLanguageBlock is a block read from an en-US to en-US bilingual file: its
// target is filed under the source language.
func sameLanguageBlock() *model.Block {
	b := model.NewBlock("b1", "colour source")
	b.SourceLocale = "en-US"
	b.SetTargetVariant(model.Variant("en-US"), &model.Target{Runs: []model.Run{model.TextR("colour target")}, Status: model.TargetStatusTranslated})
	return b
}

// The view reads the source as the block's authoritative edition, so a block
// holding a target under its source language still hands a tool its source.
func TestView_ReadsTheAuthoritativeEdition(t *testing.T) {
	b := sameLanguageBlock()
	v := tool.NewBlockView(b)

	assert.Equal(t, "colour source", model.RunsText(v.SourceRuns()))
	var units []string
	for u := range v.SourceUnits("") {
		units = append(units, model.RunsText(u.SourceRuns()))
	}
	assert.Equal(t, []string{"colour source"}, units)
	assert.Equal(t, "colour target", v.TargetText("en-US"))
}

// SetSourceStatus stamps the authoritative edition and leaves its content, its
// origin and the content the reader produced as they were.
func TestView_SetSourceStatusKeepsTheContent(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetSourceOrigin(&model.Origin{Kind: model.OriginHuman, Tool: "editor"})
	v := tool.NewBlockView(b)

	v.SetSourceStatus(model.SourceStatusEstablished)

	assert.Equal(t, model.SourceStatusEstablished, v.SourceStatus())
	assert.Equal(t, model.SourceStatusEstablished, b.SourceStatus)
	assert.Equal(t, "Hello", b.SourceText())
	o, ok := b.SourceOrigin()
	require.True(t, ok)
	assert.Equal(t, model.Origin{Kind: model.OriginHuman, Tool: "editor"}, *o)
	runs, edited := b.SourceAsRead()
	assert.False(t, edited, "a status stamp is not an edit")
	assert.Equal(t, "Hello", model.RunsText(runs))
}

// SetSourceStatus changes the status and nothing else on the block: the
// source-origin annotation stays the value it was, whatever its type, and the
// block records no copy of its source as read, so content a reader sets
// afterwards is still the source as read.
func TestView_SetSourceStatusTouchesOnlyTheStatus(t *testing.T) {
	origin := &model.Origin{Kind: model.OriginHuman, Tool: "editor"}
	raw := &model.RawAnnotation{Kind: model.AnnoSourceOrigin, Body: []byte(`{"kind":`)}
	tests := []struct {
		name string
		anno model.Payload
	}{
		{"origin", origin},
		{"undecoded origin", raw},
		{"no origin", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := model.NewBlock("b1", "Hello")
			if tt.anno != nil {
				b.SetAnno(model.AnnoSourceOrigin, tt.anno)
			}

			tool.NewBlockView(b).SetSourceStatus(model.SourceStatusEstablished)

			assert.Equal(t, model.SourceStatusEstablished, b.SourceStatus)
			got, ok := b.Anno(model.AnnoSourceOrigin)
			if tt.anno == nil {
				assert.False(t, ok)
			} else {
				require.True(t, ok)
				assert.Same(t, tt.anno, got)
			}
			b.SetSourceRuns([]model.Run{model.TextR("Hello again")})
			runs, edited := b.SourceAsRead()
			assert.False(t, edited, "a status stamp records no source as read")
			assert.Equal(t, "Hello again", model.RunsText(runs))
		})
	}
}

// The immutability backstop sees an in-place change to every edition: a target
// filed under the source language is a target, and the source is the source.
func TestWithImmutabilityCheck_SameLanguageTarget(t *testing.T) {
	ctx := tool.WithImmutabilityCheck(context.Background(), true)
	tests := []struct {
		name   string
		mutate func(v tool.BlockView)
		want   string
	}{
		{"source", func(v tool.BlockView) { v.SourceRuns()[0].Text.Text = "changed" }, "changed source"},
		{"target", func(v tool.BlockView) { v.TargetRuns("en-US")[0].Text.Text = "changed" }, "changed target"},
		{"target status", func(v tool.BlockView) { v.Target("en-US").Status = model.TargetStatusEstablished }, "changed target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := &tool.BaseTool{ToolName: "sneaky"}
			bt.Annotate = func(v tool.BlockView) error {
				tt.mutate(v)
				return nil
			}
			_, err := bt.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: sameLanguageBlock()})
			require.ErrorContains(t, err, tt.want)
		})
	}

	untouched := &tool.BaseTool{ToolName: "reader"}
	untouched.Annotate = func(tool.BlockView) error { return nil }
	_, err := untouched.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: sameLanguageBlock()})
	require.NoError(t, err)
}
